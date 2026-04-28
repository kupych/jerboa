package server

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"jerboa/internal/db"
)

type PairHandler struct {
	queries *db.Queries
	baseURL string
}

func NewPairHandler(queries *db.Queries, baseURL string) *PairHandler {
	return &PairHandler{queries: queries, baseURL: baseURL}
}

// Pair codes use an unambiguous 32-char alphabet (no 0/O/1/I/L) so users can
// type them by sight without confusion.
const pairCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
const pairCodeTTL = 10 * time.Minute

func generatePairCode() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	chars := make([]byte, 8)
	for i := 0; i < 8; i++ {
		chars[i] = pairCodeAlphabet[int(b[i])%len(pairCodeAlphabet)]
	}
	return string(chars[:4]) + "-" + string(chars[4:]), nil
}

// Start mints a fresh pair code. Unauthenticated — the code is useless until
// an authenticated user authorizes it for one of their bands.
// POST /api/pair/start
func (h *PairHandler) Start(w http.ResponseWriter, r *http.Request) {
	code, err := generatePairCode()
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if err := h.queries.CreatePairCode(r.Context(), code, pairCodeTTL); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"code":               code,
		"verify_url":         fmt.Sprintf("%s/connect?code=%s", h.baseURL, code),
		"expires_in_seconds": int(pairCodeTTL.Seconds()),
	})
}

// Authorize binds a pending pair code to the calling user and a band of
// their choice. Authenticated.
// POST /api/pair/authorize
func (h *PairHandler) Authorize(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())

	var body struct {
		Code     string `json:"code"`
		BandSlug string `json:"band_slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" || body.BandSlug == "" {
		http.Error(w, `{"error":"code and band_slug required"}`, http.StatusBadRequest)
		return
	}

	band, err := h.queries.GetBandBySlug(r.Context(), body.BandSlug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"band not found"}`, http.StatusNotFound)
		return
	}
	ok, _ := CheckBandAccess(h.queries, r.Context(), band.ID, user.ID, user.IsAdmin)
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	if err := h.queries.AuthorizePairCode(r.Context(), body.Code, user.ID, band.ID); err != nil {
		http.Error(w, `{"error":"invalid or expired code"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// Poll returns the token + server URL once a code has been authorized.
// Single-shot: a successful poll consumes the code so subsequent calls 404.
// Unauthenticated — the code itself is the bearer.
// POST /api/pair/poll
func (h *PairHandler) Poll(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" {
		http.Error(w, `{"error":"code required"}`, http.StatusBadRequest)
		return
	}

	userID, bandID, err := h.queries.PollPairCode(r.Context(), body.Code)
	if err != nil {
		// Truly missing/expired/already-consumed.
		http.Error(w, `{"error":"invalid or expired code"}`, http.StatusNotFound)
		return
	}
	if userID == nil || bandID == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "pending"})
		return
	}

	bandSlug, err := h.queries.GetBandSlug(r.Context(), *bandID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	tok, err := h.queries.RotateAPIToken(r.Context(), *userID, *bandID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":     "authorized",
		"token":      tok.Token,
		"server_url": h.baseURL,
		"band_slug":  bandSlug,
	})
}
