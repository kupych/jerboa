package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"jerboa/internal/auth"
	"jerboa/internal/db"
)

type AuthHandler struct {
	provider *auth.Provider
	queries  *db.Queries
	baseURL  string
	secure   bool
}

func NewAuthHandler(provider *auth.Provider, queries *db.Queries, baseURL string) *AuthHandler {
	secure := len(baseURL) > 8 && baseURL[:8] == "https://"
	return &AuthHandler{
		provider: provider,
		queries:  queries,
		baseURL:  baseURL,
		secure:   secure,
	}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	// Dev bypass: auto-login when no OIDC provider configured
	if h.provider == nil {
		h.devLogin(w, r)
		return
	}

	b := make([]byte, 16)
	rand.Read(b)
	state := hex.EncodeToString(b)

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, h.provider.AuthURL(state), http.StatusFound)
}

func (h *AuthHandler) devLogin(w http.ResponseWriter, r *http.Request) {
	user, err := h.queries.UpsertUser(r.Context(), "dev@jerboa.dad", "Dev User", "")
	if err != nil {
		http.Error(w, "failed to create dev user", http.StatusInternalServerError)
		return
	}

	token, err := h.queries.CreateSession(r.Context(), user.ID, 30*24*time.Hour)
	if err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, h.baseURL, http.StatusFound)
}

func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}

	// Clear state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	info, err := h.provider.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "authentication failed", http.StatusInternalServerError)
		return
	}

	// Check if user already exists — if not, require a pending invite
	exists, err := h.queries.UserExists(r.Context(), info.Email)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !exists {
		hasInvite, err := h.queries.HasPendingInvite(r.Context(), info.Email)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !hasInvite {
			http.Redirect(w, r, h.baseURL+"/no-access", http.StatusFound)
			return
		}
	}

	user, err := h.queries.UpsertUser(r.Context(), info.Email, info.Name, info.Picture)
	if err != nil {
		http.Error(w, "failed to create user", http.StatusInternalServerError)
		return
	}

	token, err := h.queries.CreateSession(r.Context(), user.ID, 7*24*time.Hour)
	if err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		MaxAge:   7 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, h.baseURL, http.StatusFound)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("session"); err == nil {
		h.queries.DeleteSession(r.Context(), cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func (h *AuthHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())

	var req struct {
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DisplayName == "" {
		http.Error(w, `{"error":"display_name is required"}`, http.StatusBadRequest)
		return
	}

	updated, err := h.queries.UpdateUserProfile(r.Context(), user.ID, req.DisplayName)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}
