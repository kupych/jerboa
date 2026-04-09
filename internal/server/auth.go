package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"jerboa/internal/auth"
	"jerboa/internal/db"
	"jerboa/internal/email"
)

type AuthHandler struct {
	provider *auth.Provider
	queries  *db.Queries
	mailer   *email.Mailer
	baseURL  string
	secure   bool
	devAuth  bool
}

func NewAuthHandler(provider *auth.Provider, queries *db.Queries, mailer *email.Mailer, baseURL string, devAuth bool) *AuthHandler {
	secure := len(baseURL) > 8 && baseURL[:8] == "https://"
	if devAuth && !isPrivateBaseURL(baseURL) {
		slog.Warn("JERBOA_DEV_AUTH set but BaseURL is not a private address — dev login will be refused", "base_url", baseURL)
	} else if devAuth {
		slog.Warn("JERBOA_DEV_AUTH is enabled — unauthenticated dev login active", "base_url", baseURL)
	}
	return &AuthHandler{
		provider: provider,
		queries:  queries,
		mailer:   mailer,
		baseURL:  baseURL,
		secure:   secure,
		devAuth:  devAuth,
	}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	// Dev bypass: only when explicitly enabled, no OIDC provider is configured,
	// AND BaseURL points at a private/loopback address. The private-address
	// check makes the flag self-defeating in production — flipping DEV_AUTH
	// alone is ignored, and pointing BaseURL at localhost would break
	// OIDC redirects/emails long before an attacker could use it.
	if h.provider == nil {
		if !h.devAuth || !isPrivateBaseURL(h.baseURL) {
			http.Error(w, `{"error":"auth not configured"}`, http.StatusServiceUnavailable)
			return
		}
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
		Secure:   h.secure,
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
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
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

// InviteLogin handles GET /auth/invite/{token} — authenticates via invite token.
// Works for first-time and returning users. The invite link is their permanent login.
func (h *AuthHandler) InviteLogin(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		http.Redirect(w, r, h.baseURL, http.StatusFound)
		return
	}

	invite, inviteEmail, err := h.queries.GetInviteByToken(r.Context(), token)
	if err != nil {
		slog.Error("invite login: lookup", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if invite == nil || inviteEmail == "" {
		http.Redirect(w, r, h.baseURL+"/no-access", http.StatusFound)
		return
	}

	inviteEmail = strings.ToLower(strings.TrimSpace(inviteEmail))

	// Upsert user (creates account if new, no-ops if existing)
	user, err := h.queries.UpsertUser(r.Context(), inviteEmail, "", "")
	if err != nil {
		slog.Error("invite login: upsert user", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Ensure band membership
	if err := h.queries.EnsureBandMember(r.Context(), invite.BandID, user.ID); err != nil {
		slog.Error("invite login: ensure member", "error", err)
	}

	// Mark invite used (no-ops if already used)
	if err := h.queries.MarkInviteUsed(r.Context(), token, user.ID); err != nil {
		slog.Error("invite login: mark used", "error", err)
	}

	// Create session
	sessionToken, err := h.queries.CreateSession(r.Context(), user.ID, 30*24*time.Hour)
	if err != nil {
		slog.Error("invite login: create session", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionToken,
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
	})

	slog.Info("invite login", "email", inviteEmail, "band_id", invite.BandID)
	http.Redirect(w, r, h.baseURL, http.StatusFound)
}

// isGoogleEmail returns true for domains that support Google OIDC login.
func isGoogleEmail(email string) bool {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return false
	}
	domain := strings.ToLower(parts[1])
	return domain == "gmail.com" || domain == "googlemail.com"
}

// SendMagicLink handles POST /auth/magic-link — sends a login email, or
// redirects to OIDC if the email is a Google account.
func (h *AuthHandler) SendMagicLink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		http.Error(w, `{"error":"email is required"}`, http.StatusBadRequest)
		return
	}

	reqEmail := strings.ToLower(strings.TrimSpace(req.Email))

	// If Google email and OIDC is configured, redirect to OIDC flow
	if h.provider != nil && isGoogleEmail(reqEmail) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"method": "oidc"})
		return
	}

	// Check user exists
	exists, err := h.queries.UserExists(r.Context(), reqEmail)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	// Always return success to prevent email enumeration
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"method": "magic-link"})

	if !exists {
		return
	}

	// Create magic link token (15 min TTL)
	token, err := h.queries.CreateMagicLink(r.Context(), reqEmail, 15*time.Minute)
	if err != nil {
		slog.Error("magic link: create token", "error", err)
		return
	}

	loginURL := h.baseURL + "/auth/magic-link/verify?token=" + token
	go h.mailer.SendMagicLoginEmail(reqEmail, loginURL)
}

// VerifyMagicLink handles GET /auth/magic-link/verify?token=xxx
func (h *AuthHandler) VerifyMagicLink(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Redirect(w, r, h.baseURL, http.StatusFound)
		return
	}

	linkEmail, err := h.queries.GetMagicLink(r.Context(), token)
	if err != nil {
		slog.Error("magic link: lookup", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if linkEmail == "" {
		http.Redirect(w, r, h.baseURL+"/no-access", http.StatusFound)
		return
	}

	// Mark as used
	h.queries.MarkMagicLinkUsed(r.Context(), token)

	// Get or create user
	user, err := h.queries.UpsertUser(r.Context(), linkEmail, "", "")
	if err != nil {
		slog.Error("magic link: upsert user", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	sessionToken, err := h.queries.CreateSession(r.Context(), user.ID, 30*24*time.Hour)
	if err != nil {
		slog.Error("magic link: create session", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionToken,
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
	})

	slog.Info("magic link login", "email", linkEmail)
	http.Redirect(w, r, h.baseURL+"/auth/open", http.StatusFound)
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
