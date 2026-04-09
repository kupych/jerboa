package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"jerboa/internal/db"
	"jerboa/internal/models"
)

type SongHandler struct {
	queries *db.Queries
}

func NewSongHandler(queries *db.Queries) *SongHandler {
	return &SongHandler{queries: queries}
}

func (h *SongHandler) List(w http.ResponseWriter, r *http.Request) {
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

	songs, err := h.queries.ListSongs(r.Context(), band.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if songs == nil {
		songs = []models.Song{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(songs)
}

func (h *SongHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}

	song, err := h.queries.CreateSong(r.Context(), band.ID, req.Name)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(song)
}

func (h *SongHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")
	songID, err := uuid.Parse(chi.URLParam(r, "songID"))
	if err != nil {
		http.Error(w, `{"error":"invalid song id"}`, http.StatusBadRequest)
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

	song, err := h.queries.GetSong(r.Context(), songID)
	if err != nil || song.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(song)
}

func (h *SongHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")
	songID, err := uuid.Parse(chi.URLParam(r, "songID"))
	if err != nil {
		http.Error(w, `{"error":"invalid song id"}`, http.StatusBadRequest)
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

	song, err := h.queries.GetSong(r.Context(), songID)
	if err != nil || song.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		Name   *string `json:"name"`
		Lyrics *string `json:"lyrics"`
		Tabs   *string `json:"tabs"`
		Notes  *string `json:"notes"`
		BPM    *int    `json:"bpm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	name := song.Name
	if req.Name != nil && *req.Name != "" {
		name = *req.Name
	}
	lyrics := song.Lyrics
	if req.Lyrics != nil {
		lyrics = *req.Lyrics
	}
	tabs := song.Tabs
	if req.Tabs != nil {
		tabs = *req.Tabs
	}
	notes := song.Notes
	if req.Notes != nil {
		notes = *req.Notes
	}
	bpm := song.BPM
	if req.BPM != nil {
		bpm = *req.BPM
	}

	if err := h.queries.UpdateSong(r.Context(), songID, name, lyrics, tabs, notes, bpm); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	song.Name = name
	song.Lyrics = lyrics
	song.Tabs = tabs
	song.Notes = notes
	song.BPM = bpm
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(song)
}

func (h *SongHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")
	songID, err := uuid.Parse(chi.URLParam(r, "songID"))
	if err != nil {
		http.Error(w, `{"error":"invalid song id"}`, http.StatusBadRequest)
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

	// Ensure song actually belongs to this band before deletion.
	song, err := h.queries.GetSong(r.Context(), songID)
	if err != nil || song == nil || song.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	if err := h.queries.DeleteSong(r.Context(), songID); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *SongHandler) ListSetTakes(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")
	songID, err := uuid.Parse(chi.URLParam(r, "songID"))
	if err != nil {
		http.Error(w, `{"error":"invalid song id"}`, http.StatusBadRequest)
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

	takes, err := h.queries.ListSetTakesBySong(r.Context(), band.ID, songID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if takes == nil {
		takes = []models.SetTake{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(takes)
}

func (h *SongHandler) ListSets(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")
	songID, err := uuid.Parse(chi.URLParam(r, "songID"))
	if err != nil {
		http.Error(w, `{"error":"invalid song id"}`, http.StatusBadRequest)
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

	sets, err := h.queries.ListSetsBySong(r.Context(), band.ID, songID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if sets == nil {
		sets = []models.Set{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sets)
}

func (h *SongHandler) AssignTrack(w http.ResponseWriter, r *http.Request) {
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

	// Ensure track belongs to this band.
	track, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil || track == nil || track.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		SongID *uuid.UUID `json:"song_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	// If assigning to a song, ensure that song also belongs to this band.
	if req.SongID != nil {
		song, err := h.queries.GetSong(r.Context(), *req.SongID)
		if err != nil || song == nil || song.BandID != band.ID {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
	}

	if err := h.queries.SetTrackSong(r.Context(), trackID, req.SongID); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
