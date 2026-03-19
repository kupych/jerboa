package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Addr           string
	BaseURL        string
	Secret         string
	DatabaseURL    string
	StoragePath    string
	MigrationsPath string

	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string

	FFmpegPath    string
	FFprobePath   string
	MaxUploadMB   int64
	WaveformPeaks int

	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	SMTPFrom string
}

func loadDotenv() {
	f, err := os.Open(".env")
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		// Don't override existing env vars
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

func Load() (*Config, error) {
	loadDotenv()

	c := &Config{
		Addr:           env("JERBOA_ADDR", ":8080"),
		BaseURL:        env("JERBOA_BASE_URL", "http://localhost:5173"),
		Secret:         env("JERBOA_SECRET", ""),
		DatabaseURL:    env("JERBOA_DB_URL", "postgres://jerboa:jerboa@localhost:5432/jerboa?sslmode=disable"),
		StoragePath:    env("JERBOA_STORAGE_PATH", "./data/uploads"),
		MigrationsPath: env("JERBOA_MIGRATIONS_PATH", "./migrations"),

		OIDCIssuer:       env("JERBOA_OIDC_ISSUER", ""),
		OIDCClientID:     env("JERBOA_OIDC_CLIENT_ID", ""),
		OIDCClientSecret: env("JERBOA_OIDC_CLIENT_SECRET", ""),

		FFmpegPath:    env("JERBOA_FFMPEG_PATH", "ffmpeg"),
		FFprobePath:   env("JERBOA_FFPROBE_PATH", "ffprobe"),
		MaxUploadMB:   envInt("JERBOA_MAX_UPLOAD_MB", 500),
		WaveformPeaks: int(envInt("JERBOA_WAVEFORM_PEAKS", 1800)),

		SMTPHost: env("JERBOA_SMTP_HOST", ""),
		SMTPPort: env("JERBOA_SMTP_PORT", "587"),
		SMTPUser: env("JERBOA_SMTP_USER", ""),
		SMTPPass: env("JERBOA_SMTP_PASS", ""),
		SMTPFrom: env("JERBOA_SMTP_FROM", ""),
	}

	if c.Secret == "" {
		return nil, fmt.Errorf("JERBOA_SECRET is required")
	}

	return c, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}
