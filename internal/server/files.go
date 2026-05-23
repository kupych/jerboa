package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"

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

// InitiateMultipart starts an S3 multipart upload. The client receives a
// file_id it can use to request presigned part URLs and complete the upload.
// The file row is inserted with status='pending' and becomes visible in the
// file list only after CompleteMultipart succeeds.
func (h *FileHandler) InitiateMultipart(w http.ResponseWriter, r *http.Request) {
	if h.s3 == nil {
		http.Error(w, `{"error":"object storage not configured"}`, http.StatusNotImplemented)
		return
	}

	user := UserFrom(r.Context())
	band, ok := h.getBand(w, r, user)
	if !ok {
		return
	}

	var body struct {
		Name        string `json:"name"`
		ContentType string `json:"content_type"`
		FileSize    int64  `json:"file_size"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		http.Error(w, `{"error":"name, content_type and file_size required"}`, http.StatusBadRequest)
		return
	}
	if body.ContentType == "" {
		body.ContentType = "application/octet-stream"
	}

	ext := filepath.Ext(body.Name)
	key := fmt.Sprintf("band-files/%s/%s%s", band.ID, uuid.New().String(), ext)

	uploadID, err := h.s3.InitiateMultipart(r.Context(), key, body.ContentType)
	if err != nil {
		http.Error(w, `{"error":"failed to initiate upload"}`, http.StatusInternalServerError)
		return
	}

	bf := &models.BandFile{
		BandID:      band.ID,
		Name:        body.Name,
		StorageKey:  key,
		UploadID:    uploadID,
		FileSize:    body.FileSize,
		ContentType: body.ContentType,
		UploadedBy:  user.ID,
	}
	if err := h.queries.CreatePendingBandFile(r.Context(), bf); err != nil {
		h.s3.AbortMultipart(r.Context(), key, uploadID)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"file_id": bf.ID.String()})
}

// PresignPart returns a presigned PUT URL for a single chunk of a multipart upload.
// The browser PUTs the chunk body directly to the URL; it must save the ETag
// response header and include it in the CompleteMultipart call.
func (h *FileHandler) PresignPart(w http.ResponseWriter, r *http.Request) {
	if h.s3 == nil {
		http.Error(w, `{"error":"object storage not configured"}`, http.StatusNotImplemented)
		return
	}

	user := UserFrom(r.Context())
	band, ok := h.getBand(w, r, user)
	if !ok {
		return
	}

	fileID, err := uuid.Parse(r.URL.Query().Get("file_id"))
	if err != nil {
		http.Error(w, `{"error":"invalid file_id"}`, http.StatusBadRequest)
		return
	}
	partNumber, err := strconv.Atoi(r.URL.Query().Get("part"))
	if err != nil || partNumber < 1 || partNumber > 10000 {
		http.Error(w, `{"error":"part must be 1–10000"}`, http.StatusBadRequest)
		return
	}

	bf, err := h.queries.GetBandFile(r.Context(), fileID, band.ID)
	if err != nil || bf == nil || bf.Status != "pending" {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	presigned, err := h.s3.PresignPart(r.Context(), bf.StorageKey, bf.UploadID, partNumber, time.Hour)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": presigned})
}

// CompleteMultipart finalises the S3 multipart upload and marks the DB row complete.
func (h *FileHandler) CompleteMultipart(w http.ResponseWriter, r *http.Request) {
	if h.s3 == nil {
		http.Error(w, `{"error":"object storage not configured"}`, http.StatusNotImplemented)
		return
	}

	user := UserFrom(r.Context())
	band, ok := h.getBand(w, r, user)
	if !ok {
		return
	}

	var body struct {
		FileID string `json:"file_id"`
		Parts  []struct {
			PartNumber int    `json:"part_number"`
			ETag       string `json:"etag"`
		} `json:"parts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.FileID == "" || len(body.Parts) == 0 {
		http.Error(w, `{"error":"file_id and parts required"}`, http.StatusBadRequest)
		return
	}

	fileID, err := uuid.Parse(body.FileID)
	if err != nil {
		http.Error(w, `{"error":"invalid file_id"}`, http.StatusBadRequest)
		return
	}

	bf, err := h.queries.GetBandFile(r.Context(), fileID, band.ID)
	if err != nil || bf == nil || bf.Status != "pending" {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	parts := make([]minio.CompletePart, len(body.Parts))
	for i, p := range body.Parts {
		parts[i] = minio.CompletePart{PartNumber: p.PartNumber, ETag: p.ETag}
	}

	if err := h.s3.CompleteMultipart(r.Context(), bf.StorageKey, bf.UploadID, parts); err != nil {
		http.Error(w, `{"error":"failed to complete upload"}`, http.StatusInternalServerError)
		return
	}

	if err := h.queries.CompleteBandFile(r.Context(), fileID, bf.FileSize); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	bf.Status = "complete"
	bf.UploadID = ""
	bf.Uploader = &models.User{ID: user.ID, DisplayName: user.DisplayName, Email: user.Email}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(bf)
}

// ListParts returns the parts S3 already has for a pending multipart upload.
// The client uses this on resume to know which chunks it can skip.
func (h *FileHandler) ListParts(w http.ResponseWriter, r *http.Request) {
	if h.s3 == nil {
		http.Error(w, `{"error":"object storage not configured"}`, http.StatusNotImplemented)
		return
	}

	user := UserFrom(r.Context())
	band, ok := h.getBand(w, r, user)
	if !ok {
		return
	}

	fileID, err := uuid.Parse(r.URL.Query().Get("file_id"))
	if err != nil {
		http.Error(w, `{"error":"invalid file_id"}`, http.StatusBadRequest)
		return
	}

	bf, err := h.queries.GetBandFile(r.Context(), fileID, band.ID)
	if err != nil || bf == nil || bf.Status != "pending" {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	parts, err := h.s3.ListParts(r.Context(), bf.StorageKey, bf.UploadID)
	if err != nil {
		// Upload ID no longer known to S3 (expired or aborted externally).
		http.Error(w, `{"error":"upload not found"}`, http.StatusNotFound)
		return
	}

	type partJSON struct {
		PartNumber int    `json:"part_number"`
		ETag       string `json:"etag"`
	}
	out := make([]partJSON, len(parts))
	for i, p := range parts {
		out[i] = partJSON{PartNumber: p.PartNumber, ETag: p.ETag}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"parts": out})
}

// AbortMultipart cancels an in-progress multipart upload and removes the DB row.
func (h *FileHandler) AbortMultipart(w http.ResponseWriter, r *http.Request) {
	if h.s3 == nil {
		http.Error(w, `{"error":"object storage not configured"}`, http.StatusNotImplemented)
		return
	}

	user := UserFrom(r.Context())
	band, ok := h.getBand(w, r, user)
	if !ok {
		return
	}

	fileID, err := uuid.Parse(r.URL.Query().Get("file_id"))
	if err != nil {
		http.Error(w, `{"error":"invalid file_id"}`, http.StatusBadRequest)
		return
	}

	bf, err := h.queries.GetBandFile(r.Context(), fileID, band.ID)
	if err != nil || bf == nil || bf.Status != "pending" {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	_ = h.s3.AbortMultipart(r.Context(), bf.StorageKey, bf.UploadID)

	if key, err := h.queries.DeleteBandFile(r.Context(), fileID, band.ID); err == nil && key != "" {
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
