package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"jerboa/internal/audio"
	"jerboa/internal/db"
	"jerboa/internal/models"
	"jerboa/internal/storage"
)

type TrackHandler struct {
	queries   *db.Queries
	store     *storage.Store
	processor *audio.Processor
	hub       *Hub
	maxBytes  int64
}

func NewTrackHandler(queries *db.Queries, store *storage.Store, processor *audio.Processor, hub *Hub, maxUploadMB int64) *TrackHandler {
	return &TrackHandler{
		queries:   queries,
		store:     store,
		processor: processor,
		hub:       hub,
		maxBytes:  maxUploadMB * 1024 * 1024,
	}
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

	if err := h.queries.UpdateTrackProcessed(ctx, trackID, peaks, meta.DurationMS, meta.Format, meta.SampleRate); err != nil {
		slog.Error("update track", "id", trackID, "error", err)
		return
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

	f, err := h.store.Open(track.FilePath)
	if err != nil {
		http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
		return
	}
	defer f.Close()

	// Download mode
	if r.URL.Query().Get("dl") == "1" {
		ext := ".audio"
		if i := strings.LastIndex(track.FilePath, "."); i >= 0 {
			ext = track.FilePath[i:]
		}
		filename := track.Title + ext
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	}

	// Use file's real modtime so bounced tracks bust the browser cache
	modTime := track.CreatedAt
	if info, err := f.Stat(); err == nil {
		modTime = info.ModTime()
	}

	// http.ServeContent handles Range requests, Content-Type, caching
	http.ServeContent(w, r, track.FilePath, modTime, f)
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

	if err := h.queries.UpdateTrackTags(r.Context(), trackID, req.Tags); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	track.Tags = req.Tags
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(track)
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

	filePath, err := h.queries.DeleteTrack(r.Context(), trackID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	if filePath != "" {
		h.store.Delete(filePath)
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
