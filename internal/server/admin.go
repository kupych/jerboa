package server

import (
	"encoding/json"
	"net/http"
	"time"

	"jerboa/internal/db"
	"jerboa/internal/models"
)

type AdminHandler struct {
	queries *db.Queries
}

func NewAdminHandler(queries *db.Queries) *AdminHandler {
	return &AdminHandler{queries: queries}
}

func (h *AdminHandler) requireAdmin(w http.ResponseWriter, r *http.Request) *models.User {
	user := UserFrom(r.Context())
	if !user.IsAdmin {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return nil
	}
	return user
}

func (h *AdminHandler) Overview(w http.ResponseWriter, r *http.Request) {
	if h.requireAdmin(w, r) == nil {
		return
	}

	users, err := h.queries.ListAllUsers(r.Context())
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if users == nil {
		users = []models.AdminUser{}
	}

	bands, err := h.queries.ListAllBands(r.Context())
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if bands == nil {
		bands = []models.AdminBand{}
	}

	invites, err := h.queries.ListAllInvites(r.Context())
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if invites == nil {
		invites = []models.AdminInvite{}
	}

	// Compute invite statuses
	now := time.Now()
	for i := range invites {
		if invites[i].UsedAt != nil {
			invites[i].Status = "accepted"
		} else if invites[i].ExpiresAt.Before(now) {
			invites[i].Status = "expired"
		} else {
			invites[i].Status = "pending"
		}
	}

	feedback, err := h.queries.ListFeedback(r.Context())
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if feedback == nil {
		feedback = []models.Feedback{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"users":    users,
		"bands":    bands,
		"invites":  invites,
		"feedback": feedback,
	})
}
