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
	"jerboa/internal/email"
	"jerboa/internal/storage"
)

func NewRouter(cfg *config.Config, queries *db.Queries, authProvider *auth.Provider, store *storage.Store, s3 *storage.S3Client, processor *audio.Processor, webFS fs.FS) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(CORSMiddleware(cfg.BaseURL))

	hub := NewHub(queries)
	mailer := email.NewMailer(cfg)
	authH := NewAuthHandler(authProvider, queries, mailer, cfg.BaseURL)
	bandH := NewBandHandler(queries, cfg.BaseURL, mailer)
	trackH := NewTrackHandler(queries, store, processor, hub, cfg.MaxUploadMB)
	commentH := NewCommentHandler(queries, hub)
	songH := NewSongHandler(queries)
	importH := NewImportHandler(queries, store, processor, hub, cfg.YTDLPCookies)
	setH := NewSetHandler(queries)
	chatH := NewChatHandler(queries, hub)
	feedbackH := NewFeedbackHandler(queries, store)
	adminH := NewAdminHandler(queries)
	overdubH := NewOverdubHandler(queries, store, processor, hub)
	activityH := NewActivityHandler(queries)
	fileH := NewFileHandler(queries, s3, cfg.MaxFileMB)
	syncH := NewSyncHandler(queries, store, processor, hub, cfg.MaxUploadMB, cfg.BinDir)

	// Auth routes (no auth middleware)
	r.Get("/auth/login", authH.Login)
	r.Get("/auth/callback", authH.Callback)
	r.Get("/auth/invite/{token}", authH.InviteLogin)
	r.Post("/auth/magic-link", authH.SendMagicLink)
	r.Get("/auth/magic-link/verify", authH.VerifyMagicLink)

	// Authenticated routes
	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(queries))

		r.Post("/auth/logout", authH.Logout)
		r.Get("/auth/me", authH.Me)
		r.Patch("/auth/me", authH.UpdateProfile)

		// Bands
		r.Get("/api/bands", bandH.List)
		r.Post("/api/bands", bandH.Create)
		r.Get("/api/bands/{slug}", bandH.Get)
		r.Patch("/api/bands/{slug}", bandH.UpdateBand)
		r.Post("/api/bands/{slug}/invite", bandH.Invite)
		r.Post("/api/invite/{token}", bandH.AcceptInvite)
		r.Post("/api/bands/{slug}/members", bandH.AddMember)
		r.Delete("/api/bands/{slug}/members/{userID}", bandH.RemoveMember)
		r.Patch("/api/bands/{slug}/members/{userID}", bandH.UpdateMemberRole)

		// Songs
		r.Get("/api/bands/{slug}/songs", songH.List)
		r.Post("/api/bands/{slug}/songs", songH.Create)
		r.Get("/api/bands/{slug}/songs/{songID}", songH.Get)
		r.Patch("/api/bands/{slug}/songs/{songID}", songH.Update)
		r.Delete("/api/bands/{slug}/songs/{songID}", songH.Delete)
		r.Get("/api/bands/{slug}/songs/{songID}/sets", songH.ListSets)
		r.Get("/api/bands/{slug}/songs/{songID}/takes", songH.ListSetTakes)
		r.Patch("/api/bands/{slug}/tracks/{trackID}/song", songH.AssignTrack)

		// Tracks
		r.Get("/api/bands/{slug}/tracks", trackH.List)
		r.Post("/api/bands/{slug}/tracks", trackH.Upload)
		r.Post("/api/bands/{slug}/tracks/import", importH.ImportURL)
		r.Post("/api/bands/{slug}/tracks/{trackID}/retry", importH.RetryImport)
		r.Get("/api/bands/{slug}/tracks/{trackID}", trackH.Get)
		r.Get("/api/bands/{slug}/tracks/{trackID}/stream", trackH.Stream)
		r.Patch("/api/bands/{slug}/tracks/{trackID}", trackH.UpdateMeta)
		r.Patch("/api/bands/{slug}/tracks/{trackID}/tags", trackH.UpdateTags)
		r.Get("/api/bands/{slug}/tracks/{trackID}/personnel", trackH.ListPersonnel)
		r.Post("/api/bands/{slug}/tracks/{trackID}/personnel", trackH.AddPersonnel)
		r.Delete("/api/bands/{slug}/tracks/{trackID}/personnel/{userID}", trackH.RemovePersonnel)
		r.Delete("/api/bands/{slug}/tracks/{trackID}", trackH.Delete)

		// Overdubs
		r.Get("/api/bands/{slug}/tracks/{trackID}/overdubs", overdubH.List)
		r.Post("/api/bands/{slug}/tracks/{trackID}/overdubs", overdubH.Upload)
		r.Patch("/api/bands/{slug}/tracks/{trackID}/overdubs/{overdubID}/offset", overdubH.UpdateOffset)
		r.Post("/api/bands/{slug}/tracks/{trackID}/overdubs/vote", overdubH.Vote)
		r.Post("/api/bands/{slug}/tracks/{trackID}/overdubs/bounce", overdubH.Bounce)
		r.Post("/api/bands/{slug}/tracks/{trackID}/overdubs/bounce-mix", overdubH.BounceMix)
		r.Post("/api/bands/{slug}/tracks/{trackID}/overdubs/scrub", overdubH.Scrub)
		r.Delete("/api/bands/{slug}/tracks/{trackID}/bounce-versions", overdubH.PurgeBounceVersions)
		r.Post("/api/bands/{slug}/tracks/{trackID}/restore", overdubH.RestoreOriginal)

		// Sets
		r.Get("/api/bands/{slug}/sets", setH.List)
		r.Post("/api/bands/{slug}/sets", setH.Create)
		r.Get("/api/bands/{slug}/sets/{setID}", setH.Get)
		r.Get("/api/bands/{slug}/sets/{setID}/perform", setH.Perform)
		r.Patch("/api/bands/{slug}/sets/{setID}", setH.Update)
		r.Delete("/api/bands/{slug}/sets/{setID}", setH.Delete)
		r.Put("/api/bands/{slug}/sets/{setID}/items", setH.ReplaceItems)
		r.Patch("/api/bands/{slug}/tracks/{trackID}/set", setH.AssignTrack)

		// Chat
		r.Get("/api/bands/{slug}/chat", chatH.List)
		r.Post("/api/bands/{slug}/chat", chatH.Send)

		// Activity / Unread
		r.Get("/api/activity/unread", activityH.Unread)
		r.Get("/api/activity/feed", activityH.Feed)
		r.Get("/api/bands/{slug}/activity", activityH.BandFeed)
		r.Post("/api/bands/{slug}/seen", activityH.MarkSeen)

		// Comments
		r.Get("/api/tracks/{trackID}/comments", commentH.List)
		r.Post("/api/tracks/{trackID}/comments", commentH.Create)
		r.Put("/api/comments/{commentID}", commentH.Update)
		r.Delete("/api/comments/{commentID}", commentH.Delete)

		// Feedback
		r.Post("/api/feedback", feedbackH.Create)
		r.Get("/api/feedback", feedbackH.List)
		r.Patch("/api/feedback/{feedbackID}", feedbackH.UpdateStatus)
		r.Get("/api/feedback/{feedbackID}/image", feedbackH.ServeImage)

		// Reaper sync
		r.Get("/api/bands/{slug}/sync/state/{sessionName}", syncH.State)
		r.Post("/api/bands/{slug}/sync/file", syncH.UploadFile)
		r.Post("/api/bands/{slug}/sync/rpp", syncH.UploadRPP)
		r.Get("/api/bands/{slug}/sync/binary", syncH.DownloadBinary)
		r.Get("/api/bands/{slug}/sync/rpp/{trackID}", syncH.ServeRPP)

		// Band files
		r.Get("/api/bands/{slug}/files", fileH.List)
		r.Post("/api/bands/{slug}/files", fileH.Upload)
		r.Get("/api/bands/{slug}/files/{fileID}/download", fileH.Download)
		r.Delete("/api/bands/{slug}/files/{fileID}", fileH.Delete)

		// Admin
		r.Get("/api/admin/overview", adminH.Overview)
		r.Delete("/api/admin/users/{userID}", adminH.DeleteUser)
		r.Delete("/api/admin/invites/{inviteID}", adminH.DeleteInvite)

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
