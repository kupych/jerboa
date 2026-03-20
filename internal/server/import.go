package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"jerboa/internal/audio"
	"jerboa/internal/db"
	"jerboa/internal/models"
	"jerboa/internal/storage"
)

type ImportHandler struct {
	queries     *db.Queries
	store       *storage.Store
	processor   *audio.Processor
	hub         *Hub
	cookiesFile string
}

func NewImportHandler(queries *db.Queries, store *storage.Store, processor *audio.Processor, hub *Hub, cookiesFile string) *ImportHandler {
	return &ImportHandler{
		queries:     queries,
		store:       store,
		processor:   processor,
		hub:         hub,
		cookiesFile: cookiesFile,
	}
}

func (h *ImportHandler) ImportURL(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")

	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	isMember, _, err := h.queries.IsBandMember(r.Context(), band.ID, user.ID)
	if err != nil || !isMember {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var req struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		http.Error(w, `{"error":"url is required"}`, http.StatusBadRequest)
		return
	}

	// Validate URL
	parsed, err := url.Parse(req.URL)
	if err != nil || (parsed.Host != "www.youtube.com" && parsed.Host != "youtube.com" && parsed.Host != "youtu.be" && parsed.Host != "m.youtube.com") {
		http.Error(w, `{"error":"only YouTube URLs are supported"}`, http.StatusBadRequest)
		return
	}

	// Check yt-dlp is available
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		http.Error(w, `{"error":"yt-dlp not installed on server"}`, http.StatusInternalServerError)
		return
	}

	// Get video title if no title provided
	title := req.Title
	if title == "" {
		args := []string{"--get-title", "--no-warnings"}
		if h.cookiesFile != "" {
			args = append(args, "--cookies", h.cookiesFile)
		}
		args = append(args, req.URL)
		out, err := exec.CommandContext(r.Context(), "yt-dlp", args...).Output()
		if err == nil {
			title = strings.TrimSpace(string(out))
		}
		if title == "" {
			title = "YouTube Import"
		}
	}

	// Create track record first (status: processing)
	track := &models.Track{
		BandID:     band.ID,
		Title:      title,
		UploadedBy: user.ID,
		FilePath:   "", // will be set after download
		SourceURL:  req.URL,
		Status:     "processing",
	}

	if err := h.queries.CreateTrack(r.Context(), track); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	track.Uploader = user

	// Download and process in background
	go h.downloadAndProcess(track.ID, req.URL, band.ID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(track)
}

func (h *ImportHandler) RetryImport(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	slug := chi.URLParam(r, "slug")
	trackID, err := uuid.Parse(chi.URLParam(r, "trackID"))
	if err != nil {
		http.Error(w, `{"error":"invalid track id"}`, http.StatusBadRequest)
		return
	}

	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	isMember, _, err := h.queries.IsBandMember(r.Context(), band.ID, user.ID)
	if err != nil || !isMember {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	track, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil || track == nil || track.BandID != band.ID {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	if track.SourceURL == "" {
		http.Error(w, `{"error":"not an imported track"}`, http.StatusBadRequest)
		return
	}

	if track.Status != "error" {
		http.Error(w, `{"error":"track is not in error state"}`, http.StatusBadRequest)
		return
	}

	if err := h.queries.ResetTrackStatus(r.Context(), trackID); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	go h.downloadAndProcess(trackID, track.SourceURL, band.ID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "processing"})
}

func (h *ImportHandler) downloadAndProcess(trackID uuid.UUID, sourceURL string, bandID uuid.UUID) {
	ctx := context.Background()

	// Download to temp file
	tmpDir := filepath.Join(h.store.Root(), "tmp")
	os.MkdirAll(tmpDir, 0750)
	outTemplate := filepath.Join(tmpDir, trackID.String()+".%(ext)s")

	args := []string{
		"-x",              // extract audio
		"--audio-quality", "0", // best quality
		"--no-playlist",   // single video only
		"--no-warnings",
		"-o", outTemplate,
	}
	if h.cookiesFile != "" {
		args = append(args, "--cookies", h.cookiesFile)
	}
	args = append(args, sourceURL)
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("yt-dlp download failed", "id", trackID, "error", err, "output", string(output))
		h.queries.UpdateTrackError(ctx, trackID)
		return
	}

	// Find the downloaded file
	downloaded := filepath.Join(tmpDir, trackID.String()+".opus")
	// yt-dlp might output different extension, try common ones
	for _, ext := range []string{".opus", ".m4a", ".mp3", ".ogg", ".wav", ".webm"} {
		candidate := filepath.Join(tmpDir, trackID.String()+ext)
		if fileExists(candidate) {
			downloaded = candidate
			break
		}
	}

	if !fileExists(downloaded) {
		slog.Error("downloaded file not found", "id", trackID, "dir", tmpDir)
		h.queries.UpdateTrackError(ctx, trackID)
		return
	}

	// Move to permanent storage
	filePath, fileSize, err := h.store.Import(bandID, filepath.Base(downloaded), downloaded)
	if err != nil {
		slog.Error("import file to store", "id", trackID, "error", err)
		h.queries.UpdateTrackError(ctx, trackID)
		return
	}

	// Update track with file path and size
	h.queries.UpdateTrackFile(ctx, trackID, filePath, fileSize)

	// Process audio (probe + waveform)
	meta, err := h.processor.Probe(ctx, filePath)
	if err != nil {
		slog.Error("probe imported track", "id", trackID, "error", err)
		h.queries.UpdateTrackError(ctx, trackID)
		return
	}

	peaks, err := h.processor.GeneratePeaks(ctx, filePath)
	if err != nil {
		slog.Error("generate peaks for import", "id", trackID, "error", err)
		h.queries.UpdateTrackError(ctx, trackID)
		return
	}

	if err := h.queries.UpdateTrackProcessed(ctx, trackID, peaks, meta.DurationMS, meta.Format, meta.SampleRate); err != nil {
		slog.Error("update imported track", "id", trackID, "error", err)
		return
	}

	h.hub.Broadcast("band:"+bandID.String(), WSMessage{
		Type: "track.ready",
		Payload: map[string]any{
			"track_id": trackID,
		},
	})
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
