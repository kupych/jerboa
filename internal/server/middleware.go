package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"jerboa/internal/db"
	"jerboa/internal/models"
)

type ctxKey string

const userCtxKey ctxKey = "user"

// opusSibling returns the path of the Opus transcode alongside the original file.
func opusSibling(filePath string) string {
	if i := strings.LastIndex(filePath, "."); i >= 0 {
		return filePath[:i] + ".opus"
	}
	return filePath + ".opus"
}

// safeDeleteFile deletes filePath and its Opus sibling only if no other track row
// references the same path — cloned overdubs share a file with their source track.
func safeDeleteFile(ctx context.Context, q *db.Queries, store interface{ Delete(string) error }, filePath string) {
	if shared, err := q.IsFileShared(ctx, filePath); err == nil && shared {
		return
	}
	store.Delete(filePath)
	store.Delete(opusSibling(filePath))
}

func UserFrom(ctx context.Context) *models.User {
	u, _ := ctx.Value(userCtxKey).(*models.User)
	return u
}

// CheckBandAccess checks membership or superadmin status.
// Returns (isMember bool, role string). Superadmins get "admin" role.
func CheckBandAccess(q *db.Queries, ctx context.Context, bandID, userID uuid.UUID, isAdmin bool) (bool, string) {
	if isAdmin {
		return true, "admin"
	}
	ok, role, err := q.IsBandMember(ctx, bandID, userID)
	if err != nil {
		return false, ""
	}
	return ok, role
}

func AuthMiddleware(q *db.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var user *models.User

			// Try Bearer token first
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				token := strings.TrimPrefix(auth, "Bearer ")
				u, err := q.GetUserByToken(r.Context(), token)
				if err == nil && u != nil {
					user = u
				}
			}

			// Fall back to session cookie
			if user == nil {
				cookie, err := r.Cookie("session")
				if err == nil {
					u, err := q.GetSession(r.Context(), cookie.Value)
					if err == nil && u != nil {
						user = u
					}
				}
			}

			if user == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), userCtxKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func CORSMiddleware(baseURL string) func(http.Handler) http.Handler {
	origin := baseURL
	// Strip path from baseURL to get just the origin
	if idx := strings.Index(baseURL, "://"); idx > 0 {
		rest := baseURL[idx+3:]
		if slash := strings.Index(rest, "/"); slash > 0 {
			origin = baseURL[:idx+3+slash]
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
