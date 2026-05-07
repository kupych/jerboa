package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	mix       *MixBuilder
}

func NewOverdubHandler(queries *db.Queries, store *storage.Store, processor *audio.Processor, hub *Hub, mix *MixBuilder) *OverdubHandler {
	return &OverdubHandler{queries: queries, store: store, processor: processor, hub: hub, mix: mix}
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

	isMember, _ := CheckBandAccess(h.queries, r.Context(), band.ID, user.ID, user.IsAdmin)
	if !isMember {
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

	isMember, _ := CheckBandAccess(h.queries, r.Context(), band.ID, user.ID, user.IsAdmin)
	if !isMember {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	parent, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil || parent == nil || parent.BandID != band.ID {
		http.Error(w, `{"error":"parent track not found"}`, http.StatusNotFound)
		return
	}

	up, err := streamMultipartToStorage(w, r, h.store, band.ID, 200*1024*1024)
	if err != nil {
		if up != nil && up.FilePath != "" {
			h.store.Delete(up.FilePath)
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, `{"error":"file too large"}`, http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, `{"error":"file is required"}`, http.StatusBadRequest)
		return
	}

	var offsetMS int64
	if v := up.Fields["offset_ms"]; v != "" {
		fmt.Sscanf(v, "%d", &offsetMS)
	}

	title := up.Fields["title"]
	if title == "" {
		title = "overdub of " + parent.Title
	}

	track := &models.Track{
		BandID:     band.ID,
		Title:      title,
		UploadedBy: user.ID,
		FilePath:   up.FilePath,
		FileSize:   up.FileSize,
		Status:     "processing",
		OverdubOf:  &trackID,
		OffsetMS:   offsetMS,
	}

	if err := h.queries.CreateTrack(r.Context(), track); err != nil {
		h.store.Delete(up.FilePath)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	track.Uploader = user

	go h.processOverdub(track.ID, up.FilePath, band.ID)

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
	h.hub.Broadcast("band:"+bandID.String(), WSMessage{
		Type:    "activity.update",
		Payload: map[string]string{"band_id": bandID.String()},
	})

	// Rebuild the parent's ephemeral session mix.
	if ov, err := h.queries.GetTrack(ctx, trackID); err == nil && ov != nil && ov.OverdubOf != nil {
		h.mix.Schedule(*ov.OverdubOf)
	}
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

	isMember, _ := CheckBandAccess(h.queries, r.Context(), band.ID, user.ID, user.IsAdmin)
	if !isMember {
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

	if od, err := h.queries.GetTrack(r.Context(), overdubID); err == nil && od != nil && od.OverdubOf != nil {
		h.mix.Schedule(*od.OverdubOf)
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

	isMember, _ := CheckBandAccess(h.queries, r.Context(), band.ID, user.ID, user.IsAdmin)
	if !isMember {
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

	_, role := CheckBandAccess(h.queries, r.Context(), band.ID, user.ID, user.IsAdmin)
	if role != "admin" {
		http.Error(w, `{"error":"admin only"}`, http.StatusForbidden)
		return
	}

	parent, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil || parent == nil || parent.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		OverdubID  uuid.UUID `json:"overdub_id"`
		ToNewTrack bool      `json:"to_new_track"`
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

	if req.ToNewTrack {
		go h.doBounceToNew(parent, overdub, band, user)
	} else {
		go h.doBounce(parent, overdub, band, user)
	}

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

	// ffmpeg: mix parent + overdub at offset
	// normalize=0 prevents amix from halving volumes and ducking on silence
	// Positive offset: delay the overdub. Negative offset: delay the parent. Zero: no delay.
	var filter string
	if overdub.OffsetMS > 0 {
		filter = fmt.Sprintf("[1]adelay=%d|%d[ov];[0][ov]amix=inputs=2:duration=longest:normalize=0", overdub.OffsetMS, overdub.OffsetMS)
	} else if overdub.OffsetMS < 0 {
		delay := -overdub.OffsetMS
		filter = fmt.Sprintf("[0]adelay=%d|%d[par];[par][1]amix=inputs=2:duration=longest:normalize=0", delay, delay)
	} else {
		filter = "[0][1]amix=inputs=2:duration=longest:normalize=0"
	}
	slog.Info("bounce: ffmpeg filter", "offset_ms", overdub.OffsetMS, "filter", filter)

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", parent.FilePath,
		"-i", overdub.FilePath,
		"-filter_complex", filter,
		"-ac", "2",
		"-ar", "48000",
		"-y", outPath,
	)
	output, err := cmd.CombinedOutput()
	slog.Info("bounce: ffmpeg done", "exit_err", err, "output", string(output))
	if err != nil {
		return
	}

	// Import the bounced file into storage
	storedPath, fileSize, err := h.store.Import(band.ID, "bounced.wav", outPath)
	if err != nil {
		slog.Error("bounce: import", "error", err)
		return
	}

	// Only create snapshot on FIRST bounce — preserves the true original
	existingOriginal := h.queries.GetPreBounceID(ctx, parent.ID)
	if existingOriginal == nil {
		snapshot := &models.Track{
			BandID:     band.ID,
			Title:      parent.Title + " (original)",
			UploadedBy: parent.UploadedBy,
			FilePath:   parent.FilePath,
			FileSize:   parent.FileSize,
			Status:     "ready",
		}

		if err := h.queries.CreateTrack(ctx, snapshot); err != nil {
			h.store.Delete(storedPath)
			slog.Error("bounce: create snapshot", "error", err)
			return
		}

		if err := h.queries.UpdateTrackProcessed(ctx, snapshot.ID, parent.WaveformData, parent.DurationMS, parent.Format, parent.SampleRate); err != nil {
			slog.Error("bounce: update snapshot", "error", err)
		}

		if err := h.queries.UpdateBouncedTo(ctx, snapshot.ID, &parent.ID); err != nil {
			slog.Error("bounce: update snapshot bounced_to", "error", err)
		}
	}

	// Replace the parent track's file with the bounced version
	if err := h.queries.UpdateTrackFile(ctx, parent.ID, storedPath, fileSize); err != nil {
		slog.Error("bounce: update parent file", "error", err)
		return
	}

	// Mark overdub as bounced
	if err := h.queries.UpdateBouncedTo(ctx, overdub.ID, &parent.ID); err != nil {
		slog.Error("bounce: update overdub bounced_to", "error", err)
	}

	// Reprocess the parent with the new file
	meta, err := h.processor.Probe(ctx, storedPath)
	if err != nil {
		slog.Error("bounce: probe", "error", err)
		h.queries.UpdateTrackError(ctx, parent.ID)
		return
	}

	peaks, err := h.processor.GeneratePeaks(ctx, storedPath)
	if err != nil {
		slog.Error("bounce: peaks", "error", err)
		h.queries.UpdateTrackError(ctx, parent.ID)
		return
	}

	if err := h.queries.UpdateTrackProcessed(ctx, parent.ID, peaks, meta.DurationMS, meta.Format, meta.SampleRate); err != nil {
		slog.Error("bounce: update parent", "error", err)
		return
	}

	slog.Info("bounce: complete", "parent_id", parent.ID)

	h.hub.Broadcast("band:"+band.ID.String(), WSMessage{
		Type:    "track.ready",
		Payload: map[string]any{"track_id": parent.ID},
	})
	h.hub.Broadcast("band:"+band.ID.String(), WSMessage{
		Type:    "activity.update",
		Payload: map[string]string{"band_id": band.ID.String()},
	})
	h.mix.Schedule(parent.ID)
}

func (h *OverdubHandler) doBounceToNew(parent, overdub *models.Track, band *models.Band, user *models.User) {
	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "bounce-*")
	if err != nil {
		slog.Error("bounce-new: create temp dir", "error", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	outPath := filepath.Join(tmpDir, "bounced.wav")

	var filter string
	if overdub.OffsetMS > 0 {
		filter = fmt.Sprintf("[1]adelay=%d|%d[ov];[0][ov]amix=inputs=2:duration=longest:normalize=0", overdub.OffsetMS, overdub.OffsetMS)
	} else if overdub.OffsetMS < 0 {
		delay := -overdub.OffsetMS
		filter = fmt.Sprintf("[0]adelay=%d|%d[par];[par][1]amix=inputs=2:duration=longest:normalize=0", delay, delay)
	} else {
		filter = "[0][1]amix=inputs=2:duration=longest:normalize=0"
	}

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", parent.FilePath,
		"-i", overdub.FilePath,
		"-filter_complex", filter,
		"-ac", "2",
		"-ar", "48000",
		"-y", outPath,
	)
	output, err := cmd.CombinedOutput()
	slog.Info("bounce-new: ffmpeg done", "exit_err", err, "output", string(output))
	if err != nil {
		return
	}

	storedPath, fileSize, err := h.store.Import(band.ID, "bounced.wav", outPath)
	if err != nil {
		slog.Error("bounce-new: import", "error", err)
		return
	}

	// Create as a new independent track
	newTrack := &models.Track{
		BandID:     band.ID,
		Title:      parent.Title + " (bounce)",
		UploadedBy: user.ID,
		FilePath:   storedPath,
		FileSize:   fileSize,
		Status:     "processing",
	}
	// Inherit song assignment from parent
	if parent.SongID != nil {
		newTrack.SongID = parent.SongID
	}

	if err := h.queries.CreateTrack(ctx, newTrack); err != nil {
		h.store.Delete(storedPath)
		slog.Error("bounce-new: create track", "error", err)
		return
	}

	// Mark overdub as bounced
	if err := h.queries.UpdateBouncedTo(ctx, overdub.ID, &newTrack.ID); err != nil {
		slog.Error("bounce-new: update overdub bounced_to", "error", err)
	}

	// Process the new track (waveform, metadata)
	meta, err := h.processor.Probe(ctx, storedPath)
	if err != nil {
		slog.Error("bounce-new: probe", "error", err)
		h.queries.UpdateTrackError(ctx, newTrack.ID)
		return
	}

	peaks, err := h.processor.GeneratePeaks(ctx, storedPath)
	if err != nil {
		slog.Error("bounce-new: peaks", "error", err)
		h.queries.UpdateTrackError(ctx, newTrack.ID)
		return
	}

	if err := h.queries.UpdateTrackProcessed(ctx, newTrack.ID, peaks, meta.DurationMS, meta.Format, meta.SampleRate); err != nil {
		slog.Error("bounce-new: update track", "error", err)
		return
	}

	slog.Info("bounce-new: complete", "parent_id", parent.ID, "new_id", newTrack.ID)

	h.hub.Broadcast("band:"+band.ID.String(), WSMessage{
		Type:    "track.ready",
		Payload: map[string]any{"track_id": newTrack.ID},
	})
	// Also notify the parent page so the overdub disappears from the list
	h.hub.Broadcast("band:"+band.ID.String(), WSMessage{
		Type:    "track.ready",
		Payload: map[string]any{"track_id": parent.ID},
	})
	h.hub.Broadcast("band:"+band.ID.String(), WSMessage{
		Type:    "activity.update",
		Payload: map[string]string{"band_id": band.ID.String()},
	})
}

// BounceMix bounces the parent track together with multiple selected overdubs.
func (h *OverdubHandler) BounceMix(w http.ResponseWriter, r *http.Request) {
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
	if role != "admin" {
		http.Error(w, `{"error":"admin only"}`, http.StatusForbidden)
		return
	}

	parent, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil || parent == nil || parent.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		OverdubIDs []uuid.UUID `json:"overdub_ids"`
		ToNewTrack bool        `json:"to_new_track"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	if len(req.OverdubIDs) == 0 {
		http.Error(w, `{"error":"no overdubs selected"}`, http.StatusBadRequest)
		return
	}

	// Fetch all selected overdubs and verify they belong to this parent
	var overdubs []*models.Track
	for _, oid := range req.OverdubIDs {
		od, err := h.queries.GetTrack(r.Context(), oid)
		if err != nil || od == nil || od.OverdubOf == nil || *od.OverdubOf != trackID {
			http.Error(w, fmt.Sprintf(`{"error":"overdub %s not found"}`, oid), http.StatusNotFound)
			return
		}
		overdubs = append(overdubs, od)
	}

	go h.doBounceMix(parent, overdubs, band, user, req.ToNewTrack)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "bouncing"})
}

func (h *OverdubHandler) doBounceMix(parent *models.Track, overdubs []*models.Track, band *models.Band, user *models.User, toNew bool) {
	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "bounce-mix-*")
	if err != nil {
		slog.Error("bounce-mix: create temp dir", "error", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	outPath := filepath.Join(tmpDir, "bounced.wav")

	// Build ffmpeg args: -i parent -i od1 -i od2 ... -filter_complex "..."
	// Normalize offsets: if any overdub has a negative offset, shift the parent forward.
	var minOffset int64
	for _, od := range overdubs {
		if od.OffsetMS < minOffset {
			minOffset = od.OffsetMS
		}
	}
	parentDelay := int64(0)
	if minOffset < 0 {
		parentDelay = -minOffset
	}

	args := []string{"-i", parent.FilePath}
	for _, od := range overdubs {
		args = append(args, "-i", od.FilePath)
	}

	// Build filter: apply adelay to any input that needs it, then amix all.
	// Input 0 = parent, inputs 1..N = overdubs.
	numInputs := 1 + len(overdubs)
	var filterParts []string
	var mixLabels []string

	// Parent
	if parentDelay > 0 {
		filterParts = append(filterParts, fmt.Sprintf("[0]adelay=%d|%d[p]", parentDelay, parentDelay))
		mixLabels = append(mixLabels, "[p]")
	} else {
		mixLabels = append(mixLabels, "[0]")
	}

	// Overdubs
	for i, od := range overdubs {
		ffIdx := i + 1
		delay := parentDelay + od.OffsetMS
		if delay > 0 {
			label := fmt.Sprintf("[d%d]", i)
			filterParts = append(filterParts, fmt.Sprintf("[%d]adelay=%d|%d%s", ffIdx, delay, delay, label))
			mixLabels = append(mixLabels, label)
		} else {
			mixLabels = append(mixLabels, fmt.Sprintf("[%d]", ffIdx))
		}
	}

	mixFilter := strings.Join(mixLabels, "") + fmt.Sprintf("amix=inputs=%d:duration=longest:normalize=0", numInputs)
	filterParts = append(filterParts, mixFilter)
	filter := strings.Join(filterParts, ";")

	slog.Info("bounce-mix: ffmpeg filter", "inputs", numInputs, "filter", filter)

	args = append(args, "-filter_complex", filter, "-ac", "2", "-ar", "48000", "-y", outPath)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	slog.Info("bounce-mix: ffmpeg done", "exit_err", err, "output", string(output))
	if err != nil {
		return
	}

	storedPath, fileSize, err := h.store.Import(band.ID, "bounced.wav", outPath)
	if err != nil {
		slog.Error("bounce-mix: import", "error", err)
		return
	}

	if toNew {
		// Create as a new independent track
		newTrack := &models.Track{
			BandID:     band.ID,
			Title:      parent.Title + " (mix)",
			UploadedBy: user.ID,
			FilePath:   storedPath,
			FileSize:   fileSize,
			Status:     "processing",
		}
		if parent.SongID != nil {
			newTrack.SongID = parent.SongID
		}
		if err := h.queries.CreateTrack(ctx, newTrack); err != nil {
			h.store.Delete(storedPath)
			slog.Error("bounce-mix: create track", "error", err)
			return
		}

		// Mark all selected overdubs as bounced
		for _, od := range overdubs {
			h.queries.UpdateBouncedTo(ctx, od.ID, &newTrack.ID)
		}

		meta, err := h.processor.Probe(ctx, storedPath)
		if err != nil {
			slog.Error("bounce-mix: probe", "error", err)
			h.queries.UpdateTrackError(ctx, newTrack.ID)
			return
		}
		peaks, err := h.processor.GeneratePeaks(ctx, storedPath)
		if err != nil {
			slog.Error("bounce-mix: peaks", "error", err)
			h.queries.UpdateTrackError(ctx, newTrack.ID)
			return
		}
		h.queries.UpdateTrackProcessed(ctx, newTrack.ID, peaks, meta.DurationMS, meta.Format, meta.SampleRate)
		slog.Info("bounce-mix: complete (new track)", "parent_id", parent.ID, "new_id", newTrack.ID)
		h.hub.Broadcast("band:"+band.ID.String(), WSMessage{Type: "track.ready", Payload: map[string]any{"track_id": newTrack.ID}})
	} else {
		// In-place: snapshot original, replace parent file, mark overdubs bounced
		existingOriginal := h.queries.GetPreBounceID(ctx, parent.ID)
		if existingOriginal == nil {
			snapshot := &models.Track{
				BandID:     band.ID,
				Title:      parent.Title + " (original)",
				UploadedBy: parent.UploadedBy,
				FilePath:   parent.FilePath,
				FileSize:   parent.FileSize,
				Status:     "ready",
			}
			if err := h.queries.CreateTrack(ctx, snapshot); err == nil {
				h.queries.UpdateTrackProcessed(ctx, snapshot.ID, parent.WaveformData, parent.DurationMS, parent.Format, parent.SampleRate)
				h.queries.UpdateBouncedTo(ctx, snapshot.ID, &parent.ID)
			}
		}

		h.queries.UpdateTrackFile(ctx, parent.ID, storedPath, fileSize)
		for _, od := range overdubs {
			h.queries.UpdateBouncedTo(ctx, od.ID, &parent.ID)
		}

		meta, err := h.processor.Probe(ctx, storedPath)
		if err != nil {
			slog.Error("bounce-mix: probe", "error", err)
			h.queries.UpdateTrackError(ctx, parent.ID)
			return
		}
		peaks, err := h.processor.GeneratePeaks(ctx, storedPath)
		if err != nil {
			slog.Error("bounce-mix: peaks", "error", err)
			h.queries.UpdateTrackError(ctx, parent.ID)
			return
		}
		h.queries.UpdateTrackProcessed(ctx, parent.ID, peaks, meta.DurationMS, meta.Format, meta.SampleRate)
		slog.Info("bounce-mix: complete (in-place)", "parent_id", parent.ID)
	}

	h.hub.Broadcast("band:"+band.ID.String(), WSMessage{Type: "track.ready", Payload: map[string]any{"track_id": parent.ID}})
	h.hub.Broadcast("band:"+band.ID.String(), WSMessage{Type: "activity.update", Payload: map[string]string{"band_id": band.ID.String()}})
	h.mix.Schedule(parent.ID)
}

// RestoreOriginal rolls back a track to its pre-bounce original state.
func (h *OverdubHandler) RestoreOriginal(w http.ResponseWriter, r *http.Request) {
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
	if role != "admin" {
		http.Error(w, `{"error":"admin only"}`, http.StatusForbidden)
		return
	}

	parent, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil || parent == nil || parent.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	originalID := h.queries.GetPreBounceID(r.Context(), trackID)
	if originalID == nil {
		http.Error(w, `{"error":"no original to restore"}`, http.StatusNotFound)
		return
	}

	original, err := h.queries.GetTrack(r.Context(), *originalID)
	if err != nil || original == nil {
		http.Error(w, `{"error":"original not found"}`, http.StatusNotFound)
		return
	}

	// Copy original file to new storage path (so parent owns its own copy)
	srcFile, err := h.store.Open(original.FilePath)
	if err != nil {
		http.Error(w, `{"error":"original file missing"}`, http.StatusInternalServerError)
		return
	}
	newPath, newSize, err := h.store.Save(band.ID, filepath.Base(original.FilePath), srcFile)
	srcFile.Close()
	if err != nil {
		http.Error(w, `{"error":"restore failed"}`, http.StatusInternalServerError)
		return
	}

	// Delete parent's current bounced file
	if parent.FilePath != "" {
		safeDeleteFile(r.Context(), h.queries, h.store, parent.FilePath)
	}

	// Restore parent to original state
	h.queries.UpdateTrackFile(r.Context(), parent.ID, newPath, newSize)
	h.queries.UpdateTrackProcessed(r.Context(), parent.ID, original.WaveformData, original.DurationMS, original.Format, original.SampleRate)

	// Delete all bounce snapshots (and their files)
	versions, _ := h.queries.ListBounceVersions(r.Context(), trackID)
	for _, v := range versions {
		filePath, err := h.queries.DeleteTrack(r.Context(), v.ID)
		if err != nil {
			slog.Error("restore: delete snapshot", "id", v.ID, "error", err)
			continue
		}
		if filePath != "" {
			safeDeleteFile(r.Context(), h.queries, h.store, filePath)
		}
	}

	// Un-bounce all overdubs so they reappear
	h.queries.ClearBouncedOverdubs(r.Context(), trackID)

	h.hub.Broadcast("band:"+band.ID.String(), WSMessage{
		Type:    "track.ready",
		Payload: map[string]any{"track_id": parent.ID},
	})
	h.mix.Schedule(trackID)

	w.WriteHeader(http.StatusNoContent)
}

// PurgeBounceVersions deletes all intermediate pre-bounce snapshots, keeping only the original.
func (h *OverdubHandler) PurgeBounceVersions(w http.ResponseWriter, r *http.Request) {
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
	if role != "admin" {
		http.Error(w, `{"error":"admin only"}`, http.StatusForbidden)
		return
	}

	versions, err := h.queries.ListBounceVersions(r.Context(), trackID)
	if err != nil || len(versions) <= 1 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Keep the first (oldest = true original), delete the rest
	for _, v := range versions[1:] {
		filePath, err := h.queries.DeleteTrack(r.Context(), v.ID)
		if err != nil {
			slog.Error("purge: delete track", "id", v.ID, "error", err)
			continue
		}
		if filePath != "" {
			safeDeleteFile(r.Context(), h.queries, h.store, filePath)
		}
	}

	w.WriteHeader(http.StatusNoContent)
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

	_, role := CheckBandAccess(h.queries, r.Context(), band.ID, user.ID, user.IsAdmin)
	if role != "admin" {
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
			safeDeleteFile(r.Context(), h.queries, h.store, filePath)
		}
	}

	// Clean up votes
	h.queries.DeleteOverdubVotes(r.Context(), trackID)

	h.mix.Schedule(trackID)
	w.WriteHeader(http.StatusNoContent)
}

// LinkOverdub clones an existing track row as an overdub of the parent track.
// The file is shared — not re-uploaded. The source track remains unchanged.
func (h *OverdubHandler) LinkOverdub(w http.ResponseWriter, r *http.Request) {
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

	parent, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil || parent == nil || parent.BandID != band.ID {
		http.Error(w, `{"error":"parent track not found"}`, http.StatusNotFound)
		return
	}

	var body struct {
		SourceTrackID string `json:"source_track_id"`
		OffsetMS      int64  `json:"offset_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	sourceID, err := uuid.Parse(body.SourceTrackID)
	if err != nil {
		http.Error(w, `{"error":"invalid source_track_id"}`, http.StatusBadRequest)
		return
	}

	source, err := h.queries.GetTrack(r.Context(), sourceID)
	if err != nil || source == nil || source.BandID != band.ID {
		http.Error(w, `{"error":"source track not found"}`, http.StatusNotFound)
		return
	}
	if source.OverdubOf != nil {
		http.Error(w, `{"error":"source track is already an overdub"}`, http.StatusBadRequest)
		return
	}
	if sourceID == trackID {
		http.Error(w, `{"error":"cannot link a track as its own overdub"}`, http.StatusBadRequest)
		return
	}

	cloned, err := h.queries.CloneTrackAsOverdub(r.Context(), sourceID, trackID, body.OffsetMS)
	if err != nil {
		slog.Error("link overdub: clone", "error", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	h.hub.Broadcast("band:"+band.ID.String(), WSMessage{
		Type:    "overdub.linked",
		Payload: map[string]any{"track_id": trackID, "overdub_id": cloned.ID},
	})
	h.mix.Schedule(trackID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(cloned)
}
