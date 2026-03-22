package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"jerboa/internal/audio"
	"jerboa/internal/db"
	"jerboa/internal/models"
	"jerboa/internal/storage"
)

type OverdubHandler struct {
	queries   *db.Queries
	store     *storage.Store
	processor *audio.Processor
	hub       *Hub
}

func NewOverdubHandler(queries *db.Queries, store *storage.Store, processor *audio.Processor, hub *Hub) *OverdubHandler {
	return &OverdubHandler{queries: queries, store: store, processor: processor, hub: hub}
}

// List returns overdubs for a parent track.
func (h *OverdubHandler) List(w http.ResponseWriter, r *http.Request) {
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

	isMember, _, err := h.queries.IsBandMember(r.Context(), band.ID, user.ID)
	if err != nil || !isMember {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	overdubs, err := h.queries.ListOverdubs(r.Context(), trackID, user.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if overdubs == nil {
		overdubs = []models.Track{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(overdubs)
}

// Upload handles uploading an overdub for a parent track.
func (h *OverdubHandler) Upload(w http.ResponseWriter, r *http.Request) {
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

	isMember, _, err := h.queries.IsBandMember(r.Context(), band.ID, user.ID)
	if err != nil || !isMember {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	parent, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil || parent == nil || parent.BandID != band.ID {
		http.Error(w, `{"error":"parent track not found"}`, http.StatusNotFound)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 200*1024*1024)
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

	// Parse offset
	var offsetMS int64
	if v := r.FormValue("offset_ms"); v != "" {
		fmt.Sscanf(v, "%d", &offsetMS)
	}

	title := r.FormValue("title")
	if title == "" {
		title = "overdub of " + parent.Title
	}

	filePath, fileSize, err := h.store.Save(band.ID, header.Filename, file)
	if err != nil {
		http.Error(w, `{"error":"failed to save file"}`, http.StatusInternalServerError)
		return
	}

	track := &models.Track{
		BandID:     band.ID,
		Title:      title,
		UploadedBy: user.ID,
		FilePath:   filePath,
		FileSize:   fileSize,
		Status:     "processing",
		OverdubOf:  &trackID,
		OffsetMS:   offsetMS,
	}

	if err := h.queries.CreateTrack(r.Context(), track); err != nil {
		h.store.Delete(filePath)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	track.Uploader = user

	go h.processOverdub(track.ID, filePath, band.ID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(track)
}

func (h *OverdubHandler) processOverdub(trackID uuid.UUID, filePath string, bandID uuid.UUID) {
	ctx := context.Background()

	meta, err := h.processor.Probe(ctx, filePath)
	if err != nil {
		slog.Error("probe overdub", "id", trackID, "error", err)
		h.queries.UpdateTrackError(ctx, trackID)
		return
	}

	peaks, err := h.processor.GeneratePeaks(ctx, filePath)
	if err != nil {
		slog.Error("generate overdub peaks", "id", trackID, "error", err)
		h.queries.UpdateTrackError(ctx, trackID)
		return
	}

	if err := h.queries.UpdateTrackProcessed(ctx, trackID, peaks, meta.DurationMS, meta.Format, meta.SampleRate); err != nil {
		slog.Error("update overdub", "id", trackID, "error", err)
		return
	}

	h.hub.Broadcast("band:"+bandID.String(), WSMessage{
		Type:    "track.ready",
		Payload: map[string]any{"track_id": trackID},
	})
}

// UpdateOffset adjusts the offset_ms of an overdub for fine-tuning sync.
func (h *OverdubHandler) UpdateOffset(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")
	overdubID, err := uuid.Parse(chi.URLParam(r, "overdubID"))
	if err != nil {
		http.Error(w, `{"error":"invalid overdub id"}`, http.StatusBadRequest)
		return
	}

	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	isMember, _, err := h.queries.IsBandMember(r.Context(), band.ID, user.ID)
	if err != nil || !isMember {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		OffsetMS int64 `json:"offset_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	if err := h.queries.UpdateOverdubOffset(r.Context(), overdubID, req.OffsetMS); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Vote casts or changes a vote for an overdub.
func (h *OverdubHandler) Vote(w http.ResponseWriter, r *http.Request) {
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

	isMember, _, err := h.queries.IsBandMember(r.Context(), band.ID, user.ID)
	if err != nil || !isMember {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		OverdubID *uuid.UUID `json:"overdub_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	if req.OverdubID == nil {
		// Unvote
		if err := h.queries.UnvoteOverdub(r.Context(), trackID, user.ID); err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
	} else {
		if err := h.queries.VoteOverdub(r.Context(), trackID, user.ID, *req.OverdubID); err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// Bounce mixes the parent track with an overdub into a new file.
func (h *OverdubHandler) Bounce(w http.ResponseWriter, r *http.Request) {
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

	_, role, err := h.queries.IsBandMember(r.Context(), band.ID, user.ID)
	if err != nil || role != "admin" {
		http.Error(w, `{"error":"admin only"}`, http.StatusForbidden)
		return
	}

	parent, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil || parent == nil || parent.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		OverdubID uuid.UUID `json:"overdub_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	overdub, err := h.queries.GetTrack(r.Context(), req.OverdubID)
	if err != nil || overdub == nil || overdub.OverdubOf == nil || *overdub.OverdubOf != trackID {
		http.Error(w, `{"error":"overdub not found"}`, http.StatusNotFound)
		return
	}

	// Bounce in background
	go h.doBounce(parent, overdub, band, user)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "bouncing"})
}

func (h *OverdubHandler) doBounce(parent, overdub *models.Track, band *models.Band, user *models.User) {
	ctx := context.Background()

	// Create temp output file
	tmpDir, err := os.MkdirTemp("", "bounce-*")
	if err != nil {
		slog.Error("bounce: create temp dir", "error", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	outPath := filepath.Join(tmpDir, "bounced.wav")
	delayMs := overdub.OffsetMS

	// ffmpeg: mix parent + overdub at offset
	// adelay delays the overdub by offset_ms, amix combines them
	filter := fmt.Sprintf("[1]adelay=%d|%d[ov];[0][ov]amix=inputs=2:duration=longest", delayMs, delayMs)

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", parent.FilePath,
		"-i", overdub.FilePath,
		"-filter_complex", filter,
		"-y", outPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		slog.Error("bounce: ffmpeg", "error", err, "output", string(output))
		return
	}

	// Import the bounced file into storage
	storedPath, fileSize, err := h.store.Import(band.ID, "bounced.wav", outPath)
	if err != nil {
		slog.Error("bounce: import", "error", err)
		return
	}

	// Create a new track for the bounced result
	bouncedTrack := &models.Track{
		BandID:     band.ID,
		Title:      parent.Title + " (bounced)",
		UploadedBy: user.ID,
		FilePath:   storedPath,
		FileSize:   fileSize,
		Status:     "processing",
		SongID:     parent.SongID,
		SetID:      parent.SetID,
	}

	if err := h.queries.CreateTrack(ctx, bouncedTrack); err != nil {
		h.store.Delete(storedPath)
		slog.Error("bounce: create track", "error", err)
		return
	}

	// Process the bounced audio
	meta, err := h.processor.Probe(ctx, storedPath)
	if err != nil {
		slog.Error("bounce: probe", "error", err)
		h.queries.UpdateTrackError(ctx, bouncedTrack.ID)
		return
	}

	peaks, err := h.processor.GeneratePeaks(ctx, storedPath)
	if err != nil {
		slog.Error("bounce: peaks", "error", err)
		h.queries.UpdateTrackError(ctx, bouncedTrack.ID)
		return
	}

	if err := h.queries.UpdateTrackProcessed(ctx, bouncedTrack.ID, peaks, meta.DurationMS, meta.Format, meta.SampleRate); err != nil {
		slog.Error("bounce: update", "error", err)
		return
	}

	h.hub.Broadcast("band:"+band.ID.String(), WSMessage{
		Type:    "track.ready",
		Payload: map[string]any{"track_id": bouncedTrack.ID},
	})
}

// Scrub deletes all non-winning overdubs for a parent track.
func (h *OverdubHandler) Scrub(w http.ResponseWriter, r *http.Request) {
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

	_, role, err := h.queries.IsBandMember(r.Context(), band.ID, user.ID)
	if err != nil || role != "admin" {
		http.Error(w, `{"error":"admin only"}`, http.StatusForbidden)
		return
	}

	var req struct {
		KeepID *uuid.UUID `json:"keep_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	// Get all overdubs for this track
	overdubs, err := h.queries.ListOverdubs(r.Context(), trackID, user.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	for _, od := range overdubs {
		if req.KeepID != nil && od.ID == *req.KeepID {
			continue
		}
		filePath, err := h.queries.DeleteTrack(r.Context(), od.ID)
		if err != nil {
			slog.Error("scrub: delete track", "id", od.ID, "error", err)
			continue
		}
		if filePath != "" {
			h.store.Delete(filePath)
		}
	}

	// Clean up votes
	h.queries.DeleteOverdubVotes(r.Context(), trackID)

	w.WriteHeader(http.StatusNoContent)
}
