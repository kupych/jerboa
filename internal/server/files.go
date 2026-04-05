package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"jerboa/internal/db"
	"jerboa/internal/models"
	"jerboa/internal/storage"
)

type FileHandler struct {
	queries  *db.Queries
	s3       *storage.S3Client
	maxBytes int64
}

func NewFileHandler(queries *db.Queries, s3 *storage.S3Client, maxFileMB int64) *FileHandler {
	return &FileHandler{
		queries:  queries,
		s3:       s3,
		maxBytes: maxFileMB * 1024 * 1024,
	}
}

func (h *FileHandler) List(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	band, ok := h.getBand(w, r, user)
	if !ok {
		return
	}

	files, err := h.queries.ListBandFiles(r.Context(), band.ID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if files == nil {
		files = []models.BandFile{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func (h *FileHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if h.s3 == nil {
		http.Error(w, `{"error":"object storage not configured"}`, http.StatusNotImplemented)
		return
	}

	user := UserFrom(r.Context())
	band, ok := h.getBand(w, r, user)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.maxBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, `{"error":"file too large or bad request"}`, http.StatusRequestEntityTooLarge)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"file is required"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := filepath.Ext(header.Filename)
	storageKey := fmt.Sprintf("band-files/%s/%s%s", band.ID, uuid.New().String(), ext)

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	if err := h.s3.Put(r.Context(), storageKey, file, header.Size, contentType); err != nil {
		http.Error(w, `{"error":"upload failed"}`, http.StatusInternalServerError)
		return
	}

	bf := &models.BandFile{
		BandID:      band.ID,
		Name:        header.Filename,
		StorageKey:  storageKey,
		FileSize:    header.Size,
		ContentType: contentType,
		UploadedBy:  user.ID,
	}

	if err := h.queries.CreateBandFile(r.Context(), bf); err != nil {
		// Best-effort cleanup if DB write fails
		h.s3.Delete(r.Context(), storageKey)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	bf.Uploader = &models.User{
		ID:          user.ID,
		DisplayName: user.DisplayName,
		Email:       user.Email,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(bf)
}

func (h *FileHandler) Download(w http.ResponseWriter, r *http.Request) {
	if h.s3 == nil {
		http.Error(w, `{"error":"object storage not configured"}`, http.StatusNotImplemented)
		return
	}

	user := UserFrom(r.Context())
	band, ok := h.getBand(w, r, user)
	if !ok {
		return
	}

	fileID, err := uuid.Parse(chi.URLParam(r, "fileID"))
	if err != nil {
		http.Error(w, `{"error":"invalid file id"}`, http.StatusBadRequest)
		return
	}

	bf, err := h.queries.GetBandFile(r.Context(), fileID, band.ID)
	if err != nil || bf == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	presigned, err := h.s3.PresignedURL(r.Context(), bf.StorageKey, time.Hour)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, presigned, http.StatusTemporaryRedirect)
}

func (h *FileHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	band, ok := h.getBand(w, r, user)
	if !ok {
		return
	}

	fileID, err := uuid.Parse(chi.URLParam(r, "fileID"))
	if err != nil {
		http.Error(w, `{"error":"invalid file id"}`, http.StatusBadRequest)
		return
	}

	key, err := h.queries.DeleteBandFile(r.Context(), fileID, band.ID)
	if err != nil || key == "" {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	if h.s3 != nil {
		h.s3.Delete(r.Context(), key)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *FileHandler) getBand(w http.ResponseWriter, r *http.Request, user *models.User) (*models.Band, bool) {
	slug := chi.URLParam(r, "slug")
	band, err := h.queries.GetBandBySlug(r.Context(), slug)
	if err != nil || band == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return nil, false
	}
	ok, _ := CheckBandAccess(h.queries, r.Context(), band.ID, user.ID, user.IsAdmin)
	if !ok {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return nil, false
	}
	return band, true
}
