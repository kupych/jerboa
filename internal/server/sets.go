package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"jerboa/internal/db"
	"jerboa/internal/models"
)

type SetHandler struct {
	queries *db.Queries
}

func NewSetHandler(queries *db.Queries) *SetHandler {
	return &SetHandler{queries: queries}
}

func (h *SetHandler) verifyBandAccess(r *http.Request) (*models.Band, error) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")

	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		return nil, err
	}

	isMember, _, err := h.queries.IsBandMember(r.Context(), band.ID, user.ID)
	if err != nil || !isMember {
		return nil, err
	}

	return band, nil
}

func (h *SetHandler) List(w http.ResponseWriter, r *http.Request) {
	band, err := h.verifyBandAccess(r)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	sets, err := h.queries.ListSets(r.Context(), band.ID)
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

func (h *SetHandler) Create(w http.ResponseWriter, r *http.Request) {
	band, err := h.verifyBandAccess(r)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		Name       string `json:"name"`
		SetType    string `json:"set_type"`
		RecordedAt string `json:"recorded_at"`
		Notes      string `json:"notes"`
		Items      []struct {
			SongID     *uuid.UUID `json:"song_id"`
			CustomName string     `json:"custom_name"`
			Notes      string     `json:"notes"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}

	if req.SetType == "" {
		req.SetType = "rehearsal"
	}

	var recordedAt *time.Time
	if req.RecordedAt != "" {
		t, err := time.Parse("2006-01-02", req.RecordedAt)
		if err != nil {
			http.Error(w, `{"error":"invalid recorded_at date"}`, http.StatusBadRequest)
			return
		}
		recordedAt = &t
	}

	set := &models.Set{
		BandID:     band.ID,
		Name:       req.Name,
		SetType:    req.SetType,
		RecordedAt: recordedAt,
		Notes:      req.Notes,
	}

	if err := h.queries.CreateSet(r.Context(), set); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	if len(req.Items) > 0 {
		items := make([]models.SetItem, len(req.Items))
		for i, it := range req.Items {
			items[i] = models.SetItem{
				SongID:     it.SongID,
				CustomName: it.CustomName,
				Notes:      it.Notes,
			}
		}
		created, err := h.queries.ReplaceSetItems(r.Context(), set.ID, items)
		if err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		set.Items = created
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(set)
}

func (h *SetHandler) Get(w http.ResponseWriter, r *http.Request) {
	band, err := h.verifyBandAccess(r)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	setID, err := uuid.Parse(chi.URLParam(r, "setID"))
	if err != nil {
		http.Error(w, `{"error":"invalid set id"}`, http.StatusBadRequest)
		return
	}

	set, err := h.queries.GetSet(r.Context(), setID)
	if err != nil || set == nil || set.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	items, err := h.queries.ListSetItems(r.Context(), set.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if items == nil {
		items = []models.SetItem{}
	}
	set.Items = items

	tracks, err := h.queries.ListSetTracks(r.Context(), set.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if tracks == nil {
		tracks = []models.Track{}
	}
	set.Tracks = tracks

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(set)
}

func (h *SetHandler) Perform(w http.ResponseWriter, r *http.Request) {
	band, err := h.verifyBandAccess(r)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	setID, err := uuid.Parse(chi.URLParam(r, "setID"))
	if err != nil {
		http.Error(w, `{"error":"invalid set id"}`, http.StatusBadRequest)
		return
	}

	set, err := h.queries.GetSet(r.Context(), setID)
	if err != nil || set == nil || set.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	items, err := h.queries.ListPerformItems(r.Context(), set.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if items == nil {
		items = []models.PerformItem{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"set_name": set.Name,
		"set_type": set.SetType,
		"items":    items,
	})
}

func (h *SetHandler) Update(w http.ResponseWriter, r *http.Request) {
	band, err := h.verifyBandAccess(r)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	setID, err := uuid.Parse(chi.URLParam(r, "setID"))
	if err != nil {
		http.Error(w, `{"error":"invalid set id"}`, http.StatusBadRequest)
		return
	}

	set, err := h.queries.GetSet(r.Context(), setID)
	if err != nil || set == nil || set.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		Name       *string `json:"name"`
		SetType    *string `json:"set_type"`
		RecordedAt *string `json:"recorded_at"`
		Notes      *string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	name := set.Name
	if req.Name != nil && *req.Name != "" {
		name = *req.Name
	}
	setType := set.SetType
	if req.SetType != nil && *req.SetType != "" {
		setType = *req.SetType
	}
	notes := set.Notes
	if req.Notes != nil {
		notes = *req.Notes
	}
	recordedAt := set.RecordedAt
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
	}

	if err := h.queries.UpdateSet(r.Context(), setID, name, setType, recordedAt, notes); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	set.Name = name
	set.SetType = setType
	set.RecordedAt = recordedAt
	set.Notes = notes

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(set)
}

func (h *SetHandler) Delete(w http.ResponseWriter, r *http.Request) {
	band, err := h.verifyBandAccess(r)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	setID, err := uuid.Parse(chi.URLParam(r, "setID"))
	if err != nil {
		http.Error(w, `{"error":"invalid set id"}`, http.StatusBadRequest)
		return
	}

	set, err := h.queries.GetSet(r.Context(), setID)
	if err != nil || set == nil || set.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	if err := h.queries.DeleteSet(r.Context(), setID); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *SetHandler) ReplaceItems(w http.ResponseWriter, r *http.Request) {
	band, err := h.verifyBandAccess(r)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	setID, err := uuid.Parse(chi.URLParam(r, "setID"))
	if err != nil {
		http.Error(w, `{"error":"invalid set id"}`, http.StatusBadRequest)
		return
	}

	set, err := h.queries.GetSet(r.Context(), setID)
	if err != nil || set == nil || set.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		Items []struct {
			SongID     *uuid.UUID `json:"song_id"`
			CustomName string     `json:"custom_name"`
			StartMS    *int64     `json:"start_ms"`
			EndMS      *int64     `json:"end_ms"`
			Notes      string     `json:"notes"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	items := make([]models.SetItem, len(req.Items))
	for i, it := range req.Items {
		items[i] = models.SetItem{
			SongID:     it.SongID,
			CustomName: it.CustomName,
			StartMS:    it.StartMS,
			EndMS:      it.EndMS,
			Notes:      it.Notes,
		}
	}

	created, err := h.queries.ReplaceSetItems(r.Context(), setID, items)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if created == nil {
		created = []models.SetItem{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(created)
}

func (h *SetHandler) AssignTrack(w http.ResponseWriter, r *http.Request) {
	band, err := h.verifyBandAccess(r)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	trackID, err := uuid.Parse(chi.URLParam(r, "trackID"))
	if err != nil {
		http.Error(w, `{"error":"invalid track id"}`, http.StatusBadRequest)
		return
	}

	track, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil || track == nil || track.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		SetID *uuid.UUID `json:"set_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	if err := h.queries.UpdateTrackSet(r.Context(), trackID, req.SetID); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
