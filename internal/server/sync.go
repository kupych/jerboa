package server

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"jerboa/internal/audio"
	"jerboa/internal/db"
	"jerboa/internal/models"
	"jerboa/internal/storage"
)

type SyncHandler struct {
	queries   *db.Queries
	store     *storage.Store
	processor *audio.Processor
	hub       *Hub
	maxBytes  int64
	binDir    string
}

func NewSyncHandler(queries *db.Queries, store *storage.Store, processor *audio.Processor, hub *Hub, maxUploadMB int64, binDir string) *SyncHandler {
	return &SyncHandler{
		queries:   queries,
		store:     store,
		processor: processor,
		hub:       hub,
		maxBytes:  maxUploadMB * 1024 * 1024,
		binDir:    binDir,
	}
}

// SyncConfig is embedded into the downloaded binary.
type SyncConfig struct {
	ServerURL string `json:"server_url"`
	BandSlug  string `json:"band_slug"`
	Token     string `json:"token"`
	SongID    string `json:"song_id,omitempty"`
}

// Sessions lists all Reaper sessions for a band.
// GET /api/bands/{slug}/sync/sessions
func (h *SyncHandler) Sessions(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	band, ok := h.getBand(w, r, user)
	if !ok {
		return
	}

	sessions, err := h.queries.ListSyncSessions(r.Context(), band.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if sessions == nil {
		sessions = []db.SyncSession{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"sessions": sessions})
}

// State returns filenames+hashes already on the server for a given session.
// GET /api/bands/{slug}/sync/state/{sessionName}
func (h *SyncHandler) State(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	band, ok := h.getBand(w, r, user)
	if !ok {
		return
	}

	sessionName := chi.URLParam(r, "sessionName")
	songID := r.URL.Query().Get("song_id")
	track, err := h.queries.GetOrCreateRppSession(r.Context(), band.ID, user.ID, sessionName, songID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	files, err := h.queries.GetSyncState(r.Context(), track.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if files == nil {
		files = []models.SyncFile{}
	}

	stems, err := h.queries.ListBakedStems(r.Context(), track.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if stems == nil {
		stems = []models.BakedStem{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"track_id":     track.ID,
		"files":        files,
		"baked_stems":  stems,
	})
}

// UploadFile uploads a single audio file as an overdub on the session track.
// POST /api/bands/{slug}/sync/file
func (h *SyncHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	band, ok := h.getBand(w, r, user)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.maxBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, `{"error":"file too large"}`, http.StatusRequestEntityTooLarge)
		return
	}

	trackID, err := uuid.Parse(r.FormValue("track_id"))
	if err != nil {
		http.Error(w, `{"error":"invalid track_id"}`, http.StatusBadRequest)
		return
	}

	filename := r.FormValue("filename")
	fileHash := r.FormValue("file_hash")
	var offsetMS int64
	fmt.Sscanf(r.FormValue("offset_ms"), "%d", &offsetMS)
	var gain float64 = 1.0
	if g, err := strconv.ParseFloat(r.FormValue("gain"), 64); err == nil && g > 0 {
		gain = g
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"file is required"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	filePath, fileSize, err := h.store.Save(band.ID, header.Filename, file)
	if err != nil {
		http.Error(w, `{"error":"failed to save file"}`, http.StatusInternalServerError)
		return
	}

	overdub := &models.Track{
		BandID:     band.ID,
		Title:      filename,
		UploadedBy: user.ID,
		FilePath:   filePath,
		FileSize:   fileSize,
		Status:     "processing",
		OverdubOf:  &trackID,
		OffsetMS:   offsetMS,
		Gain:       gain,
	}

	if err := h.queries.CreateTrack(r.Context(), overdub); err != nil {
		h.store.Delete(filePath)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	if err := h.queries.UpsertSyncFile(r.Context(), trackID, &overdub.ID, filename, fileHash); err != nil {
		slog.Warn("upsert sync file", "error", err)
	}

	go h.processTrack(overdub.ID, filePath, band.ID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"id": overdub.ID})
}

// UploadBaked uploads a per-Reaper-track bounce (post-FX/automation/levels)
// as an overdub on the session track. Replaces any prior baked stem for the
// same reaper_guid.
// POST /api/bands/{slug}/sync/baked
func (h *SyncHandler) UploadBaked(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	band, ok := h.getBand(w, r, user)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.maxBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, `{"error":"file too large"}`, http.StatusRequestEntityTooLarge)
		return
	}

	trackID, err := uuid.Parse(r.FormValue("track_id"))
	if err != nil {
		http.Error(w, `{"error":"invalid track_id"}`, http.StatusBadRequest)
		return
	}
	reaperGUID := r.FormValue("reaper_guid")
	reaperName := r.FormValue("reaper_name")
	renderHash := r.FormValue("render_hash")
	if reaperGUID == "" || renderHash == "" {
		http.Error(w, `{"error":"reaper_guid and render_hash required"}`, http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"file is required"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	filePath, fileSize, err := h.store.Save(band.ID, header.Filename, file)
	if err != nil {
		http.Error(w, `{"error":"failed to save file"}`, http.StatusInternalServerError)
		return
	}

	title := reaperName
	if title == "" {
		title = header.Filename
	}
	overdub := &models.Track{
		BandID:     band.ID,
		Title:      title,
		UploadedBy: user.ID,
		FilePath:   filePath,
		FileSize:   fileSize,
		Status:     "processing",
		OverdubOf:  &trackID,
		Kind:       "baked_stem",
	}
	if err := h.queries.CreateTrack(r.Context(), overdub); err != nil {
		h.store.Delete(filePath)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	prevOverdubID, err := h.queries.UpsertBakedStem(r.Context(), trackID, reaperGUID, reaperName, renderHash, overdub.ID)
	if err != nil {
		slog.Warn("upsert baked stem", "error", err)
	}
	if prevOverdubID != nil {
		if oldPath, err := h.queries.DeleteTrack(r.Context(), *prevOverdubID); err == nil && oldPath != "" {
			if shared, err := h.queries.IsFileShared(r.Context(), oldPath); err == nil && !shared {
				h.store.Delete(oldPath)
			}
		}
	}

	go h.processTrack(overdub.ID, filePath, band.ID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"id": overdub.ID})
}

// UploadRPP stores a new .rpp version for the session track.
// POST /api/bands/{slug}/sync/rpp
func (h *SyncHandler) UploadRPP(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	band, ok := h.getBand(w, r, user)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}

	trackID, err := uuid.Parse(r.FormValue("track_id"))
	if err != nil {
		http.Error(w, `{"error":"invalid track_id"}`, http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"file is required"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	filePath, _, err := h.store.Save(band.ID, header.Filename, file)
	if err != nil {
		http.Error(w, `{"error":"failed to save rpp"}`, http.StatusInternalServerError)
		return
	}

	version, err := h.queries.CreateRppVersion(r.Context(), trackID, filePath)
	if err != nil {
		h.store.Delete(filePath)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"version": version.Version})
}

// DownloadBinary serves a pre-built sync binary with config baked in.
// GET /api/bands/{slug}/sync/binary?platform=linux|windows
func (h *SyncHandler) DownloadBinary(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	band, ok := h.getBand(w, r, user)
	if !ok {
		return
	}

	platform := r.URL.Query().Get("platform")
	if platform != "linux" && platform != "windows" {
		platform = runtime.GOOS
		if platform != "linux" && platform != "windows" {
			platform = "linux"
		}
	}

	binName := fmt.Sprintf("jerboa-sync-%s-amd64", platform)
	if platform == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(h.binDir, binName)

	binData, err := os.ReadFile(binPath)
	if err != nil {
		http.Error(w, `{"error":"sync binary not built — run make build-sync on the server"}`, http.StatusNotFound)
		return
	}

	token, err := h.queries.RotateAPIToken(r.Context(), user.ID, band.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}

	cfg := SyncConfig{
		ServerURL: fmt.Sprintf("%s://%s", scheme, host),
		BandSlug:  band.Slug,
		Token:     token.Token,
		SongID:    r.URL.Query().Get("song_id"),
	}

	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	length := make([]byte, 8)
	binary.LittleEndian.PutUint64(length, uint64(len(cfgJSON)))

	downloadName := fmt.Sprintf("jerboa-sync-%s", band.Slug)
	if platform == "windows" {
		downloadName += ".exe"
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, downloadName))
	w.Header().Set("Cache-Control", "no-store")
	w.Write(binData)
	w.Write(cfgJSON)
	w.Write(length)
}

// ServeRPP downloads the latest .rpp for a session track.
// GET /api/bands/{slug}/sync/rpp/{trackID}
func (h *SyncHandler) ServeRPP(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	band, ok := h.getBand(w, r, user)
	if !ok {
		return
	}

	trackID, err := uuid.Parse(chi.URLParam(r, "trackID"))
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}

	// Verify the track belongs to this band to prevent cross-band enumeration.
	track, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil || track == nil || track.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	v, err := h.queries.GetLatestRppVersion(r.Context(), trackID)
	if err != nil || v == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	f, err := h.store.Open(v.StorageKey)
	if err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="session-v%d.rpp"`, v.Version))
	io.Copy(w, f)
}

func (h *SyncHandler) processTrack(trackID uuid.UUID, filePath string, bandID uuid.UUID) {
	ctx := context.Background()

	meta, err := h.processor.Probe(ctx, filePath)
	if err != nil {
		slog.Error("sync: probe track", "id", trackID, "error", err)
		h.queries.UpdateTrackError(ctx, trackID)
		return
	}

	peaks, err := h.processor.GeneratePeaks(ctx, filePath)
	if err != nil {
		slog.Error("sync: generate peaks", "id", trackID, "error", err)
		h.queries.UpdateTrackError(ctx, trackID)
		return
	}

	if err := h.queries.UpdateTrackProcessed(ctx, trackID, peaks, meta.DurationMS, meta.Format, meta.SampleRate); err != nil {
		slog.Error("sync: update track", "id", trackID, "error", err)
		return
	}

	if lufs, err := h.processor.MeasureLoudness(ctx, filePath); err == nil {
		if err := h.queries.UpdateTrackLoudness(ctx, trackID, lufs); err != nil {
			slog.Warn("sync: update track loudness", "id", trackID, "error", err)
		}
	} else {
		slog.Debug("sync: measure loudness", "id", trackID, "error", err)
	}

	if h.processor.NeedsTranscode(meta.Format) {
		if err := h.processor.TranscodeToOpus(ctx, filePath, opusSibling(filePath)); err != nil {
			slog.Error("sync: transcode", "id", trackID, "error", err)
		}
	}

	h.hub.Broadcast("band:"+bandID.String(), WSMessage{
		Type:    "track.ready",
		Payload: map[string]any{"track_id": trackID},
	})
}

func (h *SyncHandler) getBand(w http.ResponseWriter, r *http.Request, user *models.User) (*models.Band, bool) {
	slug := chi.URLParam(r, "slug")
	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return nil, false
	}
	ok, _ := CheckBandAccess(h.queries, r.Context(), band.ID, user.ID, user.IsAdmin)
	if !ok {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return nil, false
	}
	return band, true
}
