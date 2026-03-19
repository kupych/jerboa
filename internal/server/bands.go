package server

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"jerboa/internal/db"
	"jerboa/internal/models"
)

type BandHandler struct {
	queries *db.Queries
	baseURL string
}

func NewBandHandler(queries *db.Queries, baseURL string) *BandHandler {
	return &BandHandler{queries: queries, baseURL: baseURL}
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"invite_url": h.baseURL + "/#/invite/" + invite.Token,
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

