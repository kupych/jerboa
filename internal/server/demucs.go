package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"jerboa/internal/audio"
	"jerboa/internal/db"
	"jerboa/internal/models"
	"jerboa/internal/replicate"
	"jerboa/internal/storage"
)

// validStems is the canonical set of stem names demucs models produce.
// The 6-stem variants add "guitar" and "piano".
var validStems = map[string]bool{
	"vocals": true, "drums": true, "bass": true, "other": true,
	"guitar": true, "piano": true,
}

type DemucsHandler struct {
	queries   *db.Queries
	store     *storage.Store
	processor *audio.Processor
	hub       *Hub
	mix       *MixBuilder
	rep       *replicate.Client
	model     string
	sem       chan struct{} // size 1 — one job at a time
}

func NewDemucsHandler(queries *db.Queries, store *storage.Store, processor *audio.Processor, hub *Hub, mix *MixBuilder, token, model string) *DemucsHandler {
	var rep *replicate.Client
	if token != "" {
		rep = replicate.New(token)
	}
	return &DemucsHandler{
		queries:   queries,
		store:     store,
		processor: processor,
		hub:       hub,
		mix:       mix,
		rep:       rep,
		model:     model,
		sem:       make(chan struct{}, 1),
	}
}

func (h *DemucsHandler) Separate(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")
	trackID, err := uuid.Parse(chi.URLParam(r, "trackID"))
	if err != nil {
		http.Error(w, `{"error":"invalid track id"}`, http.StatusBadRequest)
		return
	}

	if h.rep == nil {
		http.Error(w, `{"error":"replicate not configured"}`, http.StatusServiceUnavailable)
		return
	}

	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	_, role := CheckBandAccess(h.queries, r.Context(), band.ID, user.ID, user.IsAdmin)
	if role != "admin" && !user.IsAdmin {
		http.Error(w, `{"error":"admin only"}`, http.StatusForbidden)
		return
	}

	parent, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil || parent == nil || parent.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		Stems []string `json:"stems"`
		Mode  string   `json:"mode"` // "overdub" or "tracks"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	if len(req.Stems) == 0 {
		http.Error(w, `{"error":"stems is required"}`, http.StatusBadRequest)
		return
	}
	for _, s := range req.Stems {
		if !validStems[strings.ToLower(s)] {
			http.Error(w, fmt.Sprintf(`{"error":"unknown stem %q"}`, s), http.StatusBadRequest)
			return
		}
	}
	if req.Mode != "overdub" && req.Mode != "tracks" {
		req.Mode = "overdub"
	}

	select {
	case h.sem <- struct{}{}:
	default:
		http.Error(w, `{"error":"another separation job is running, try again shortly"}`, http.StatusTooManyRequests)
		return
	}

	go func() {
		defer func() { <-h.sem }()
		h.runJob(parent, band, user, req.Stems, req.Mode)
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "separating"})
}

func (h *DemucsHandler) runJob(parent *models.Track, band *models.Band, user *models.User, stems []string, mode string) {
	// Demucs jobs on CPU can be slow; give the whole job a generous ceiling.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	slog.Info("demucs: start", "track_id", parent.ID, "stems", stems, "mode", mode)

	src, err := h.store.Open(parent.FilePath)
	if err != nil {
		slog.Error("demucs: open source", "error", err)
		h.broadcastFailure(band.ID, parent.ID, "source file missing")
		return
	}
	uploadURL, err := h.rep.UploadFile(ctx, filepath.Base(parent.FilePath), "audio/*", src)
	src.Close()
	if err != nil {
		slog.Error("demucs: upload", "error", err)
		h.broadcastFailure(band.ID, parent.ID, "upload failed")
		return
	}

	pred, err := h.rep.RunModel(ctx, h.model, map[string]any{"audio": uploadURL})
	if err != nil {
		slog.Error("demucs: run model", "error", err)
		h.broadcastFailure(band.ID, parent.ID, "separation failed")
		return
	}

	stemURLs := extractStemURLs(pred.Output)
	if len(stemURLs) == 0 {
		slog.Error("demucs: no stem URLs in output", "output", pred.Output)
		h.broadcastFailure(band.ID, parent.ID, "no stems in output")
		return
	}

	tmpDir, err := os.MkdirTemp("", "demucs-*")
	if err != nil {
		slog.Error("demucs: tmp dir", "error", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	for _, name := range stems {
		name = strings.ToLower(name)
		url, ok := stemURLs[name]
		if !ok {
			slog.Warn("demucs: requested stem not in output", "stem", name)
			continue
		}
		if err := h.importStem(ctx, parent, band, user, name, url, tmpDir, mode); err != nil {
			slog.Error("demucs: import stem", "stem", name, "error", err)
		}
	}

	if mode == "overdub" {
		h.mix.Schedule(parent.ID)
	}
	h.hub.Broadcast("band:"+band.ID.String(), WSMessage{
		Type:    "activity.update",
		Payload: map[string]string{"band_id": band.ID.String()},
	})
	slog.Info("demucs: complete", "track_id", parent.ID)
}

func (h *DemucsHandler) importStem(ctx context.Context, parent *models.Track, band *models.Band, user *models.User, stem, url, tmpDir, mode string) error {
	// Output names are typically wav; let the URL/extension survive.
	tmpPath := filepath.Join(tmpDir, stem+extFromURL(url))
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	if err := h.rep.Download(ctx, url, f); err != nil {
		f.Close()
		return err
	}
	f.Close()

	storedPath, fileSize, err := h.store.Import(band.ID, filepath.Base(tmpPath), tmpPath)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}

	track := &models.Track{
		BandID:     band.ID,
		Title:      parent.Title + " — " + stem,
		UploadedBy: user.ID,
		FilePath:   storedPath,
		FileSize:   fileSize,
		Status:     "processing",
	}
	if mode == "overdub" {
		pid := parent.ID
		track.OverdubOf = &pid
	}

	if err := h.queries.CreateTrack(ctx, track); err != nil {
		h.store.Delete(storedPath)
		return fmt.Errorf("create track: %w", err)
	}

	// CreateTrack doesn't insert song_id directly; set it after for "tracks" mode
	// so the stem appears under the same song as its source. For overdub mode the
	// stem is a child of the parent, so song_id isn't needed on the child row.
	if mode == "tracks" && parent.SongID != nil {
		if err := h.queries.SetTrackSong(ctx, track.ID, parent.SongID); err != nil {
			slog.Warn("demucs: set song", "error", err)
		}
	}

	h.processStem(ctx, track.ID, storedPath, band.ID)
	return nil
}

// processStem mirrors TrackHandler.processTrack but lives here to avoid a circular wire.
func (h *DemucsHandler) processStem(ctx context.Context, trackID uuid.UUID, filePath string, bandID uuid.UUID) {
	meta, err := h.processor.Probe(ctx, filePath)
	if err != nil {
		slog.Error("demucs: probe", "id", trackID, "error", err)
		h.queries.UpdateTrackError(ctx, trackID)
		return
	}
	peaks, err := h.processor.GeneratePeaks(ctx, filePath)
	if err != nil {
		slog.Error("demucs: peaks", "id", trackID, "error", err)
		h.queries.UpdateTrackError(ctx, trackID)
		return
	}
	if h.processor.NeedsTranscode(meta.Format) {
		opus := opusSibling(filePath)
		if err := h.processor.TranscodeToOpus(ctx, filePath, opus); err != nil {
			slog.Warn("demucs: transcode", "id", trackID, "error", err)
		}
	}
	if err := h.queries.UpdateTrackProcessed(ctx, trackID, peaks, meta.DurationMS, meta.Format, meta.SampleRate); err != nil {
		slog.Error("demucs: update", "id", trackID, "error", err)
		return
	}
	if lufs, err := h.processor.MeasureLoudness(ctx, filePath); err == nil {
		h.queries.UpdateTrackLoudness(ctx, trackID, lufs)
	}
	h.hub.Broadcast("band:"+bandID.String(), WSMessage{
		Type:    "track.ready",
		Payload: map[string]any{"track_id": trackID},
	})
}

func (h *DemucsHandler) broadcastFailure(bandID, trackID uuid.UUID, msg string) {
	h.hub.Broadcast("band:"+bandID.String(), WSMessage{
		Type: "demucs.failed",
		Payload: map[string]any{
			"track_id": trackID,
			"error":    msg,
		},
	})
}

// extractStemURLs walks a Replicate prediction output and pulls out URLs
// keyed by stem name. Handles the common shapes: a flat dict, a nested dict,
// or a list of {name,url} objects.
func extractStemURLs(out any) map[string]string {
	result := map[string]string{}
	switch v := out.(type) {
	case map[string]any:
		for k, val := range v {
			lk := strings.ToLower(k)
			if !validStems[lk] {
				continue
			}
			if s, ok := val.(string); ok && strings.HasPrefix(s, "http") {
				result[lk] = s
			}
		}
	case []any:
		for _, item := range v {
			if obj, ok := item.(map[string]any); ok {
				name, _ := obj["name"].(string)
				url, _ := obj["url"].(string)
				if validStems[strings.ToLower(name)] && strings.HasPrefix(url, "http") {
					result[strings.ToLower(name)] = url
				}
			}
		}
	}
	return result
}

func extFromURL(url string) string {
	// Strip query string then take the last extension.
	if i := strings.IndexByte(url, '?'); i >= 0 {
		url = url[:i]
	}
	if i := strings.LastIndexByte(url, '.'); i >= 0 && len(url)-i <= 6 {
		return url[i:]
	}
	return ".wav"
}
