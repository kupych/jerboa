package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"jerboa/internal/audio"
	"jerboa/internal/db"
	"jerboa/internal/models"
	"jerboa/internal/storage"
)

const maxConcurrentTranscodes = 2

type TrackHandler struct {
	queries     *db.Queries
	store       *storage.Store
	processor   *audio.Processor
	hub         *Hub
	mix         *MixBuilder
	maxBytes    int64
	transcoding sync.Map    // keyed by opus path, prevents duplicate background transcodes
	transcodeSem chan struct{} // limits concurrent ffmpeg processes
}

func NewTrackHandler(queries *db.Queries, store *storage.Store, processor *audio.Processor, hub *Hub, mix *MixBuilder, maxUploadMB int64) *TrackHandler {
	sem := make(chan struct{}, maxConcurrentTranscodes)
	for i := 0; i < maxConcurrentTranscodes; i++ {
		sem <- struct{}{}
	}
	return &TrackHandler{
		queries:      queries,
		store:        store,
		processor:    processor,
		hub:          hub,
		mix:          mix,
		maxBytes:     maxUploadMB * 1024 * 1024,
		transcodeSem: sem,
	}
}

func (h *TrackHandler) lazyTranscode(filePath string) {
	opusPath := opusSibling(filePath)
	if _, inProgress := h.transcoding.LoadOrStore(opusPath, true); inProgress {
		return
	}
	go func() {
		defer h.transcoding.Delete(opusPath)
		// Wait for a slot — drops the goroutine if none available within a second
		// so burst requests don't queue unbounded work
		select {
		case <-h.transcodeSem:
		case <-time.After(time.Second):
			h.transcoding.Delete(opusPath)
			return
		}
		defer func() { h.transcodeSem <- struct{}{} }()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := h.processor.TranscodeToOpus(ctx, filePath, opusPath); err != nil {
			slog.Error("lazy transcode", "file", filePath, "error", err)
		}
	}()
}

func (h *TrackHandler) List(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")

	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	isMember, _ := CheckBandAccess(h.queries, r.Context(), band.ID, user.ID, user.IsAdmin)
	if !isMember {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	tracks, err := h.queries.ListTracks(r.Context(), band.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if tracks == nil {
		tracks = []models.Track{}
	}
	if counts, err := h.queries.CountOverdubsForBand(r.Context(), band.ID); err == nil {
		for i := range tracks {
			if n, ok := counts[tracks[i].ID]; ok {
				tracks[i].OverdubCount = n
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tracks)
}

func (h *TrackHandler) Upload(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")

	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	isMember, _ := CheckBandAccess(h.queries, r.Context(), band.ID, user.ID, user.IsAdmin)
	if !isMember {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.maxBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, `{"error":"file too large"}`, http.StatusRequestEntityTooLarge)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"file is required"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	if !h.processor.IsSupported(header.Filename) {
		http.Error(w, `{"error":"unsupported audio format"}`, http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	if title == "" {
		title = strings.TrimSuffix(header.Filename, "."+fileExt(header.Filename))
	}

	filePath, fileSize, err := h.store.Save(band.ID, header.Filename, file)
	if err != nil {
		http.Error(w, `{"error":"failed to save file"}`, http.StatusInternalServerError)
		return
	}

	track := &models.Track{
		BandID:      band.ID,
		Title:       title,
		Description: r.FormValue("description"),
		UploadedBy:  user.ID,
		FilePath:    filePath,
		FileSize:    fileSize,
		Status:      "processing",
	}

	if err := h.queries.CreateTrack(r.Context(), track); err != nil {
		h.store.Delete(filePath)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	track.Uploader = user

	// Process audio in background
	go h.processTrack(track.ID, filePath, band.ID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(track)
}

func (h *TrackHandler) processTrack(trackID uuid.UUID, filePath string, bandID uuid.UUID) {
	ctx := context.Background()

	meta, err := h.processor.Probe(ctx, filePath)
	if err != nil {
		slog.Error("probe track", "id", trackID, "error", err)
		h.queries.UpdateTrackError(ctx, trackID)
		return
	}

	peaks, err := h.processor.GeneratePeaks(ctx, filePath)
	if err != nil {
		slog.Error("generate peaks", "id", trackID, "error", err)
		h.queries.UpdateTrackError(ctx, trackID)
		return
	}

	if h.processor.NeedsTranscode(meta.Format) {
		opusPath := opusSibling(filePath)
		if err := h.processor.TranscodeToOpus(ctx, filePath, opusPath); err != nil {
			slog.Error("transcode track", "id", trackID, "error", err)
		} else {
			// Re-probe the Opus sibling — MediaRecorder files often lack accurate duration
			// headers, so the original probe may return near-zero duration. The transcoded
			// Opus has proper headers that ffprobe can read accurately.
			if betterMeta, err := h.processor.Probe(ctx, opusPath); err == nil && betterMeta.DurationMS > meta.DurationMS {
				meta.DurationMS = betterMeta.DurationMS
			}
			if betterPeaks, err := h.processor.GeneratePeaks(ctx, opusPath); err == nil {
				peaks = betterPeaks
			}
		}
	}

	if err := h.queries.UpdateTrackProcessed(ctx, trackID, peaks, meta.DurationMS, meta.Format, meta.SampleRate); err != nil {
		slog.Error("update track", "id", trackID, "error", err)
		return
	}

	if lufs, err := h.processor.MeasureLoudness(ctx, filePath); err == nil {
		if err := h.queries.UpdateTrackLoudness(ctx, trackID, lufs); err != nil {
			slog.Warn("update track loudness", "id", trackID, "error", err)
		}
	} else {
		slog.Debug("measure loudness", "id", trackID, "error", err)
	}

	// Notify connected clients
	h.hub.Broadcast("band:"+bandID.String(), WSMessage{
		Type: "track.ready",
		Payload: map[string]any{
			"track_id": trackID,
		},
	})
	h.hub.Broadcast("band:"+bandID.String(), WSMessage{
		Type:    "activity.update",
		Payload: map[string]string{"band_id": bandID.String()},
	})
}

func (h *TrackHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")
	trackID, err := uuid.Parse(chi.URLParam(r, "trackID"))
	if err != nil {
		http.Error(w, `{"error":"invalid track id"}`, http.StatusBadRequest)
		return
	}

	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	isMember, _ := CheckBandAccess(h.queries, r.Context(), band.ID, user.ID, user.IsAdmin)
	if !isMember {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	track, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil || track == nil || track.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	// Check if this track has pre-bounce versions
	track.PreBounceID = h.queries.GetPreBounceID(r.Context(), trackID)
	track.BounceVersions = h.queries.CountBounceVersions(r.Context(), trackID)
	if track.OverdubOf == nil {
		if n, err := h.queries.CountOverdubs(r.Context(), trackID); err == nil {
			track.OverdubCount = n
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(track)
}

func (h *TrackHandler) Stream(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")
	trackID, err := uuid.Parse(chi.URLParam(r, "trackID"))
	if err != nil {
		http.Error(w, `{"error":"invalid track id"}`, http.StatusBadRequest)
		return
	}

	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	isMember, _ := CheckBandAccess(h.queries, r.Context(), band.ID, user.ID, user.IsAdmin)
	if !isMember {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	track, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil || track == nil || track.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	// Section download: extract a time range via ffmpeg and send as WAV
	startParam := r.URL.Query().Get("start_ms")
	endParam := r.URL.Query().Get("end_ms")
	if startParam != "" && endParam != "" && r.URL.Query().Get("dl") == "1" {
		startMs, err1 := strconv.ParseInt(startParam, 10, 64)
		endMs, err2 := strconv.ParseInt(endParam, 10, 64)
		if err1 != nil || err2 != nil || endMs <= startMs {
			http.Error(w, `{"error":"invalid start_ms/end_ms"}`, http.StatusBadRequest)
			return
		}
		startSec := float64(startMs) / 1000.0
		durSec := float64(endMs-startMs) / 1000.0

		dlTitle := r.URL.Query().Get("title")
		if dlTitle == "" {
			dlTitle = track.Title
		}
		filename := sanitizeHeaderValue(dlTitle) + ".wav"
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		w.Header().Set("Content-Type", "audio/wav")

		cmd := exec.CommandContext(r.Context(), "ffmpeg",
			"-ss", fmt.Sprintf("%.3f", startSec),
			"-t", fmt.Sprintf("%.3f", durSec),
			"-i", track.FilePath,
			"-ac", "2",
			"-ar", "48000",
			"-f", "wav",
			"pipe:1",
		)
		cmd.Stdout = w
		if err := cmd.Run(); err != nil {
			slog.Error("section download: ffmpeg", "error", err, "track", trackID)
		}
		return
	}

	isDownload := r.URL.Query().Get("dl") == "1"

	// For streaming, prefer the ephemeral session mix sibling when present
	// (parent + all overdubs baked together), else the plain Opus transcode.
	// Downloads always get the original file (WAV/FLAC/etc).
	servePath := track.FilePath
	useOpus := false
	mixHit := false
	if !isDownload && track.OverdubOf == nil {
		if mix := mixSibling(track.FilePath); fileExists(mix) {
			servePath = mix
			useOpus = true
			mixHit = true
		}
	}
	if !isDownload && !mixHit {
		opus := opusSibling(track.FilePath)
		if fileExists(opus) {
			servePath = opus
			useOpus = true
		} else if h.processor.NeedsTranscode(track.Format) {
			// Transcode synchronously so the client always gets a decodable file.
			// If a background transcode is already running, wait for it.
			if _, inProgress := h.transcoding.Load(opus); inProgress {
				for i := 0; i < 120; i++ {
					time.Sleep(500 * time.Millisecond)
					if fileExists(opus) {
						break
					}
				}
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				if err := h.processor.TranscodeToOpus(ctx, track.FilePath, opus); err != nil {
					slog.Warn("stream: transcode failed, serving original", "id", trackID, "error", err)
				}
			}
			if fileExists(opus) {
				servePath = opus
				useOpus = true
			}
		}
	}

	f, err := h.store.Open(servePath)
	if err != nil {
		http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
		return
	}
	defer f.Close()

	if isDownload {
		ext := ".audio"
		if i := strings.LastIndex(track.FilePath, "."); i >= 0 {
			ext = track.FilePath[i:]
		}
		filename := sanitizeHeaderValue(track.Title) + ext
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	} else if useOpus {
		w.Header().Set("Content-Type", "audio/ogg; codecs=opus")
	}

	// Use file's real modtime so bounced tracks bust the browser cache
	modTime := track.CreatedAt
	if info, err := f.Stat(); err == nil {
		modTime = info.ModTime()
	}

	// http.ServeContent handles Range requests, Content-Type, caching
	http.ServeContent(w, r, servePath, modTime, f)
}

func (h *TrackHandler) UpdateMeta(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")
	trackID, err := uuid.Parse(chi.URLParam(r, "trackID"))
	if err != nil {
		http.Error(w, `{"error":"invalid track id"}`, http.StatusBadRequest)
		return
	}

	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	isMember, _ := CheckBandAccess(h.queries, r.Context(), band.ID, user.ID, user.IsAdmin)
	if !isMember {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	track, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil || track == nil || track.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Notes       string  `json:"notes"`
		RecordedAt  *string `json:"recorded_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	if req.Title == "" {
		req.Title = track.Title
	}

	var recordedAt *time.Time
	if req.RecordedAt != nil {
		if *req.RecordedAt == "" {
			recordedAt = nil
		} else {
			t, err := time.Parse("2006-01-02", *req.RecordedAt)
			if err != nil {
				http.Error(w, `{"error":"invalid recorded_at date"}`, http.StatusBadRequest)
				return
			}
			recordedAt = &t
		}
	} else {
		recordedAt = track.RecordedAt
	}

	if err := h.queries.UpdateTrackMeta(r.Context(), trackID, req.Title, req.Description, req.Notes, recordedAt); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	track.Title = req.Title
	track.Description = req.Description
	track.Notes = req.Notes
	track.RecordedAt = recordedAt
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(track)
}

func (h *TrackHandler) UpdateTags(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")
	trackID, err := uuid.Parse(chi.URLParam(r, "trackID"))
	if err != nil {
		http.Error(w, `{"error":"invalid track id"}`, http.StatusBadRequest)
		return
	}

	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	isMember, _ := CheckBandAccess(h.queries, r.Context(), band.ID, user.ID, user.IsAdmin)
	if !isMember {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	track, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil || track == nil || track.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	if req.Tags == nil {
		req.Tags = []string{}
	}
	for i, tag := range req.Tags {
		req.Tags[i] = strings.ToLower(strings.TrimSpace(tag))
	}

	if err := h.queries.UpdateTrackTags(r.Context(), trackID, req.Tags); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	track.Tags = req.Tags
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(track)
}

func (h *TrackHandler) UpdateGain(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")
	trackID, err := uuid.Parse(chi.URLParam(r, "trackID"))
	if err != nil {
		http.Error(w, `{"error":"invalid track id"}`, http.StatusBadRequest)
		return
	}

	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	isMember, _ := CheckBandAccess(h.queries, r.Context(), band.ID, user.ID, user.IsAdmin)
	if !isMember {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	track, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil || track == nil || track.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		Gain float64 `json:"gain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	if req.Gain < 0 {
		http.Error(w, `{"error":"gain must be non-negative"}`, http.StatusBadRequest)
		return
	}

	if err := h.queries.UpdateTrackGain(r.Context(), trackID, req.Gain); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	// Gain change on a parent or overdub invalidates the session mix.
	if h.mix != nil {
		if track.OverdubOf != nil {
			h.mix.Schedule(*track.OverdubOf)
		} else {
			h.mix.Schedule(track.ID)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"gain": req.Gain})
}

func (h *TrackHandler) UpdateMuted(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")
	trackID, err := uuid.Parse(chi.URLParam(r, "trackID"))
	if err != nil {
		http.Error(w, `{"error":"invalid track id"}`, http.StatusBadRequest)
		return
	}

	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	isMember, _ := CheckBandAccess(h.queries, r.Context(), band.ID, user.ID, user.IsAdmin)
	if !isMember {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	track, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil || track == nil || track.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		Muted bool `json:"muted"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	if err := h.queries.UpdateTrackMuted(r.Context(), trackID, req.Muted); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	if h.mix != nil {
		if track.OverdubOf != nil {
			h.mix.Schedule(*track.OverdubOf)
		} else {
			h.mix.Schedule(track.ID)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"muted": req.Muted})
}

// BackfillLoudness measures loudness for all ready tracks that are missing it.
// Runs in the background; responds immediately with the count of tracks queued.
func (h *TrackHandler) BackfillLoudness(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	if !user.IsAdmin {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	tracks, err := h.queries.ListTracksWithoutLoudness(r.Context())
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	go func() {
		ctx := context.Background()
		for _, t := range tracks {
			filePath := t.FilePath
			// Prefer the Opus sibling for loudness measurement if it exists
			if opus := opusSibling(filePath); fileExists(opus) {
				filePath = opus
			}
			if lufs, err := h.processor.MeasureLoudness(ctx, filePath); err == nil {
				if err := h.queries.UpdateTrackLoudness(ctx, t.ID, lufs); err != nil {
					slog.Warn("backfill loudness: update", "id", t.ID, "error", err)
				}
			} else {
				slog.Debug("backfill loudness: measure", "id", t.ID, "error", err)
			}
		}
		slog.Info("backfill loudness: done", "count", len(tracks))
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"queued": len(tracks)})
}

func (h *TrackHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")
	trackID, err := uuid.Parse(chi.URLParam(r, "trackID"))
	if err != nil {
		http.Error(w, `{"error":"invalid track id"}`, http.StatusBadRequest)
		return
	}

	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	_, role := CheckBandAccess(h.queries, r.Context(), band.ID, user.ID, user.IsAdmin)

	track, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil || track == nil || track.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	// Only uploader or admin can delete
	if track.UploadedBy != user.ID && role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	parentOfOverdub := track.OverdubOf

	filePath, err := h.queries.DeleteTrack(r.Context(), trackID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	if filePath != "" {
		safeDeleteFile(r.Context(), h.queries, h.store, filePath)
	}

	if parentOfOverdub != nil && h.mix != nil {
		h.mix.Schedule(*parentOfOverdub)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *TrackHandler) ListPersonnel(w http.ResponseWriter, r *http.Request) {
	track, err := h.verifyTrackAccess(r)
	if err != nil || track == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	personnel, err := h.queries.ListTrackPersonnel(r.Context(), track.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if personnel == nil {
		personnel = []models.TrackPersonnel{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(personnel)
}

func (h *TrackHandler) AddPersonnel(w http.ResponseWriter, r *http.Request) {
	track, err := h.verifyTrackAccess(r)
	if err != nil || track == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		http.Error(w, `{"error":"invalid user_id"}`, http.StatusBadRequest)
		return
	}

	if err := h.queries.AddTrackPersonnel(r.Context(), track.ID, userID, req.Role); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *TrackHandler) RemovePersonnel(w http.ResponseWriter, r *http.Request) {
	track, err := h.verifyTrackAccess(r)
	if err != nil || track == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		http.Error(w, `{"error":"invalid user_id"}`, http.StatusBadRequest)
		return
	}

	if err := h.queries.RemoveTrackPersonnel(r.Context(), track.ID, userID); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// verifyTrackAccess checks band membership and returns the track if accessible.
func (h *TrackHandler) verifyTrackAccess(r *http.Request) (*models.Track, error) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")
	trackID, err := uuid.Parse(chi.URLParam(r, "trackID"))
	if err != nil {
		return nil, err
	}

	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		return nil, err
	}

	isMember, _ := CheckBandAccess(h.queries, r.Context(), band.ID, user.ID, user.IsAdmin)
	if !isMember {
		return nil, err
	}

	track, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil || track == nil || track.BandID != band.ID {
		return nil, err
	}

	return track, nil
}

func fileExt(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			return name[i+1:]
		}
	}
	return ""
}
