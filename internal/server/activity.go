package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"jerboa/internal/db"
	"jerboa/internal/models"
)

type ActivityHandler struct {
	queries *db.Queries
}

func NewActivityHandler(queries *db.Queries) *ActivityHandler {
	return &ActivityHandler{queries: queries}
}

func (h *ActivityHandler) Unread(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())

	counts, err := h.queries.GetUnreadCounts(r.Context(), user.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if counts == nil {
		counts = []models.UnreadCount{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(counts)
}

func (h *ActivityHandler) Feed(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())

	items, err := h.queries.GetActivityFeed(r.Context(), user.ID, 30)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if items == nil {
		items = []models.ActivityItem{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func (h *ActivityHandler) MarkSeen(w http.ResponseWriter, r *http.Request) {
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

	if err := h.queries.UpdateLastSeen(r.Context(), band.ID, user.ID); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
