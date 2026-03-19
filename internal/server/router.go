package server

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"jerboa/internal/auth"
	"jerboa/internal/audio"
	"jerboa/internal/config"
	"jerboa/internal/db"
	"jerboa/internal/storage"
)

func NewRouter(cfg *config.Config, queries *db.Queries, authProvider *auth.Provider, store *storage.Store, processor *audio.Processor, webFS fs.FS) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(CORSMiddleware(cfg.BaseURL))

	hub := NewHub(queries)
	authH := NewAuthHandler(authProvider, queries, cfg.BaseURL)
	bandH := NewBandHandler(queries, cfg.BaseURL)
	trackH := NewTrackHandler(queries, store, processor, hub, cfg.MaxUploadMB)
	commentH := NewCommentHandler(queries, hub)

	// Auth routes (no auth middleware)
	r.Get("/auth/login", authH.Login)
	r.Get("/auth/callback", authH.Callback)

	// Authenticated routes
	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(queries))

		r.Post("/auth/logout", authH.Logout)
		r.Get("/auth/me", authH.Me)

		// Bands
		r.Get("/api/bands", bandH.List)
		r.Post("/api/bands", bandH.Create)
		r.Get("/api/bands/{slug}", bandH.Get)
		r.Post("/api/bands/{slug}/invite", bandH.Invite)
		r.Post("/api/invite/{token}", bandH.AcceptInvite)

		// Tracks
		r.Get("/api/bands/{slug}/tracks", trackH.List)
		r.Post("/api/bands/{slug}/tracks", trackH.Upload)
		r.Get("/api/bands/{slug}/tracks/{trackID}", trackH.Get)
		r.Get("/api/bands/{slug}/tracks/{trackID}/stream", trackH.Stream)
		r.Delete("/api/bands/{slug}/tracks/{trackID}", trackH.Delete)

		// Comments
		r.Get("/api/tracks/{trackID}/comments", commentH.List)
		r.Post("/api/tracks/{trackID}/comments", commentH.Create)
		r.Put("/api/comments/{commentID}", commentH.Update)
		r.Delete("/api/comments/{commentID}", commentH.Delete)

		// WebSocket
		r.Handle("/ws", hub)
	})

	// Serve frontend (SPA fallback)
	if webFS != nil {
		fileServer := http.FileServer(http.FS(webFS))
		r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			// Try to serve the file directly
			path := strings.TrimPrefix(r.URL.Path, "/")
			if path == "" {
				path = "index.html"
			}

			if f, err := webFS.Open(path); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}

			// SPA fallback: serve index.html for unmatched routes
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
		})
	}

	return r
}
