package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
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

	var s3Client *storage.S3Client
	if cfg.S3Endpoint != "" && cfg.S3Bucket != "" {
		s3Client, err = storage.NewS3Client(cfg.S3Endpoint, cfg.S3Bucket, cfg.S3Region, cfg.S3AccessKey, cfg.S3SecretKey)
		if err != nil {
			return fmt.Errorf("s3: %w", err)
		}
		slog.Info("S3 storage configured", "endpoint", cfg.S3Endpoint, "bucket", cfg.S3Bucket)
	} else {
		slog.Warn("S3 not configured — file uploads will return 501")
	}

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

	// Background job: transcode any WebM/WAV files that don't yet have an Opus sibling.
	// Fixes recordings made before the sync transcode was in place.
	go transcodeOrphans(cfg.StoragePath, processor)

	router := server.NewRouter(cfg, queries, authProvider, store, s3Client, processor, webFS)

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

// transcodeOrphans walks the storage directory and (re-)creates Opus siblings for any
// audio file that needs one. WebM siblings are always regenerated because previous
// versions used -c:a copy which produces Ogg/Opus that decodeAudioData rejects.
func transcodeOrphans(storageRoot string, processor *audio.Processor) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	sem := make(chan struct{}, 2)
	walked := 0
	converted := 0

	_ = filepath.Walk(storageRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".webm" && ext != ".wav" && ext != ".flac" {
			return nil
		}
		opus := path[:len(path)-len(ext)] + ".opus"

		// Always regenerate .opus siblings of WebM files — old ones were created
		// with -c:a copy and produce malformed output that decodeAudioData rejects.
		if ext == ".webm" {
			os.Remove(opus)
		} else if _, err := os.Stat(opus); err == nil {
			return nil // non-webm sibling already exists, skip
		}

		walked++
		sem <- struct{}{}
		go func() {
			defer func() { <-sem }()
			if err := processor.TranscodeToOpus(ctx, path, opus); err != nil {
				slog.Warn("orphan transcode failed", "file", path, "error", err)
			} else {
				slog.Info("orphan transcoded", "file", path)
				converted++
			}
		}()
		return nil
	})

	for i := 0; i < cap(sem); i++ {
		sem <- struct{}{}
	}
	if walked > 0 {
		slog.Info("orphan transcode complete", "found", walked, "converted", converted)
	}
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
