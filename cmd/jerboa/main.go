package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"jerboa/internal/auth"
	"jerboa/internal/audio"
	"jerboa/internal/config"
	"jerboa/internal/db"
	"jerboa/internal/server"
	"jerboa/internal/storage"
	"jerboa/web"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Handle subcommands
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		return runMigrate(ctx)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	slog.Info("connecting to database")
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Auto-migrate on startup
	slog.Info("running migrations")
	if err := db.Migrate(ctx, pool, cfg.MigrationsPath); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	queries := db.NewQueries(pool)

	store, err := storage.New(cfg.StoragePath)
	if err != nil {
		return err
	}

	processor := audio.NewProcessor(cfg.FFmpegPath, cfg.FFprobePath, cfg.WaveformPeaks)

	var authProvider *auth.Provider
	if cfg.OIDCIssuer != "" {
		callbackURL := cfg.BaseURL + "/auth/callback"
		authProvider, err = auth.NewProvider(ctx, cfg.OIDCIssuer, cfg.OIDCClientID, cfg.OIDCClientSecret, callbackURL)
		if err != nil {
			return fmt.Errorf("oidc: %w", err)
		}
		slog.Info("OIDC provider configured", "issuer", cfg.OIDCIssuer)
	} else {
		slog.Warn("no OIDC provider configured — auth endpoints will fail")
	}

	webFS := web.DistFS()
	if webFS != nil {
		slog.Info("serving embedded frontend")
	} else {
		slog.Info("no embedded frontend — use Vite dev server")
	}

	router := server.NewRouter(cfg, queries, authProvider, store, processor, webFS)

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 5 * time.Minute, // Long for audio streaming
		IdleTimeout:  2 * time.Minute,
	}

	go func() {
		<-ctx.Done()
		slog.Info("shutting down")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
	}()

	slog.Info("starting server", "addr", cfg.Addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func runMigrate(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	return db.Migrate(ctx, pool, cfg.MigrationsPath)
}
