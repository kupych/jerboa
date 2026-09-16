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

	YTDLPCookies string

	ReplicateToken       string
	ReplicateDemucsModel string

	S3Endpoint  string
	S3Bucket    string
	S3Region    string
	S3AccessKey string
	S3SecretKey string
	MaxFileMB   int64
	BinDir      string

	// Project mirror storage (Backblaze B2). Kept separate from the main S3
	// bucket on purpose: the mirror is the band's off-site backup, so it lives
	// with a different provider than the server and its uploads.
	MirrorS3Endpoint  string
	MirrorS3Bucket    string
	MirrorS3Region    string
	MirrorS3AccessKey string
	MirrorS3SecretKey string

	DevAuth bool // JERBOA_DEV_AUTH — allows unauthenticated dev login when OIDC is absent
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

		YTDLPCookies: env("JERBOA_YTDLP_COOKIES", ""),

		ReplicateToken:       env("JERBOA_REPLICATE_TOKEN", ""),
		ReplicateDemucsModel: env("JERBOA_REPLICATE_DEMUCS_MODEL", "ryan5453/demucs"),

		S3Endpoint:  env("JERBOA_S3_ENDPOINT", ""),
		S3Bucket:    env("JERBOA_S3_BUCKET", ""),
		S3Region:    env("JERBOA_S3_REGION", ""),
		S3AccessKey: env("JERBOA_S3_ACCESS_KEY", ""),
		S3SecretKey: env("JERBOA_S3_SECRET_KEY", ""),
		MaxFileMB:   envInt("JERBOA_MAX_FILE_MB", 2000),
		BinDir:      env("JERBOA_BIN_DIR", "./bin"),

		MirrorS3Endpoint:  env("JERBOA_MIRROR_S3_ENDPOINT", ""),
		MirrorS3Bucket:    env("JERBOA_MIRROR_S3_BUCKET", ""),
		MirrorS3Region:    env("JERBOA_MIRROR_S3_REGION", ""),
		MirrorS3AccessKey: env("JERBOA_MIRROR_S3_ACCESS_KEY", ""),
		MirrorS3SecretKey: env("JERBOA_MIRROR_S3_SECRET_KEY", ""),

		DevAuth: envBool("JERBOA_DEV_AUTH", false),
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

func envBool(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}
