package server

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"jerboa/internal/db"
	"jerboa/internal/email"
	"jerboa/internal/models"
)

type BandHandler struct {
	queries *db.Queries
	baseURL string
	mailer  *email.Mailer
}

func NewBandHandler(queries *db.Queries, baseURL string, mailer *email.Mailer) *BandHandler {
	return &BandHandler{queries: queries, baseURL: baseURL, mailer: mailer}
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	slug := slugRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "-")
	return strings.Trim(slug, "-")
}

func (h *BandHandler) List(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	bands, err := h.queries.ListUserBands(r.Context(), user.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if bands == nil {
		bands = []models.BandWithRole{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bands)
}

func (h *BandHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())

	if !user.IsAdmin {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}

	slug := slugify(body.Name)
	if slug == "" {
		http.Error(w, `{"error":"invalid name"}`, http.StatusBadRequest)
		return
	}

	band, err := h.queries.CreateBand(r.Context(), body.Name, slug, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			http.Error(w, `{"error":"band name already taken"}`, http.StatusConflict)
			return
		}
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(band)
}

func (h *BandHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")

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

	members, _ := h.queries.GetBandMembers(r.Context(), band.ID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"band":    band,
		"members": members,
	})
}

func (h *BandHandler) Invite(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")

	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	_, role, err := h.queries.IsBandMember(r.Context(), band.ID, user.ID)
	if err != nil || role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		http.Error(w, `{"error":"email is required"}`, http.StatusBadRequest)
		return
	}

	invite, err := h.queries.CreateInvite(r.Context(), band.ID, user.ID, req.Email, 7*24*time.Hour)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	inviteURL := h.baseURL + "/auth/invite/" + invite.Token

	h.mailer.SendInvite(req.Email, band.Name, inviteURL)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"invite_url": inviteURL,
	})
}

func (h *BandHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	token := chi.URLParam(r, "token")

	band, err := h.queries.AcceptInviteReturningBand(r.Context(), token, user.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if band == nil {
		http.Error(w, `{"error":"invalid or expired invite"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(band)
}

func (h *BandHandler) UpdateBand(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")

	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	_, role, err := h.queries.IsBandMember(r.Context(), band.ID, user.ID)
	if err != nil || role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	var req struct {
		Name        string `json:"name"`
		ColorScheme string `json:"color_scheme"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	if req.ColorScheme != "" {
		validSchemes := map[string]bool{
			"teal": true, "violet": true, "rose": true, "amber": true,
			"lime": true, "cyan": true, "fuchsia": true, "orange": true,
		}
		if !validSchemes[req.ColorScheme] {
			http.Error(w, `{"error":"invalid color scheme"}`, http.StatusBadRequest)
			return
		}
		if err := h.queries.UpdateBandColorScheme(r.Context(), band.ID, req.ColorScheme); err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
	}

	if req.Name != "" {
		newSlug := slugify(req.Name)
		updated, err := h.queries.UpdateBandName(r.Context(), band.ID, req.Name, newSlug)
		if err != nil {
			if strings.Contains(err.Error(), "duplicate") {
				http.Error(w, `{"error":"band name already taken"}`, http.StatusConflict)
				return
			}
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(updated)
		return
	}

	// Re-fetch band to return updated state
	band, _ = h.queries.GetBandBySlug(r.Context(), slug)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(band)
}

func (h *BandHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")

	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	_, role, err := h.queries.IsBandMember(r.Context(), band.ID, user.ID)
	if err != nil || role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	memberID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		http.Error(w, `{"error":"invalid user_id"}`, http.StatusBadRequest)
		return
	}

	// Can't remove yourself
	if memberID == user.ID {
		http.Error(w, `{"error":"cannot remove yourself"}`, http.StatusBadRequest)
		return
	}

	if err := h.queries.RemoveBandMember(r.Context(), band.ID, memberID); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *BandHandler) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")

	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	_, role, err := h.queries.IsBandMember(r.Context(), band.ID, user.ID)
	if err != nil || role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	memberID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		http.Error(w, `{"error":"invalid user_id"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	if req.Role != "admin" && req.Role != "member" {
		http.Error(w, `{"error":"role must be admin or member"}`, http.StatusBadRequest)
		return
	}

	if err := h.queries.UpdateBandMemberRole(r.Context(), band.ID, memberID, req.Role); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *BandHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")

	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	_, role, err := h.queries.IsBandMember(r.Context(), band.ID, user.ID)
	if err != nil || role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		http.Error(w, `{"error":"email is required"}`, http.StatusBadRequest)
		return
	}

	target, err := h.queries.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if target == nil {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}

	if err := h.queries.AddBandMember(r.Context(), band.ID, target.ID, "member"); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "added"})
}

