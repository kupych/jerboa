package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"jerboa/internal/db"
	"jerboa/internal/models"
	"jerboa/internal/storage"
)

type FeedbackHandler struct {
	queries *db.Queries
	store   *storage.Store
}

func NewFeedbackHandler(queries *db.Queries, store *storage.Store) *FeedbackHandler {
	return &FeedbackHandler{queries: queries, store: store}
}

func (h *FeedbackHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024) // 10MB max
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, `{"error":"request too large"}`, http.StatusRequestEntityTooLarge)
		return
	}

	body := r.FormValue("body")
	if body == "" {
		http.Error(w, `{"error":"body is required"}`, http.StatusBadRequest)
		return
	}

	fb := &models.Feedback{
		UserID:  user.ID,
		Body:    body,
		PageURL: r.FormValue("page_url"),
	}

	// Handle optional image
	file, header, err := r.FormFile("image")
	if err == nil {
		defer file.Close()
		ext := filepath.Ext(header.Filename)
		if ext == "" {
			ext = ".png"
		}
		dir := filepath.Join(h.store.Root(), "feedback")
		if err := os.MkdirAll(dir, 0750); err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		name := uuid.New().String() + ext
		path := filepath.Join(dir, name)
		f, err := os.Create(path)
		if err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		defer f.Close()
		if _, err := f.ReadFrom(file); err != nil {
			os.Remove(path)
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		fb.ImagePath = path
	}

	if err := h.queries.CreateFeedback(r.Context(), fb); err != nil {
		if fb.ImagePath != "" {
			os.Remove(fb.ImagePath)
		}
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	fb.HasImage = fb.ImagePath != ""
	fb.User = user

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(fb)
}

func (h *FeedbackHandler) List(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	if !user.IsAdmin {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	items, err := h.queries.ListFeedback(r.Context())
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if items == nil {
		items = []models.Feedback{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func (h *FeedbackHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	if !user.IsAdmin {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "feedbackID"))
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	if req.Status != "open" && req.Status != "closed" {
		http.Error(w, `{"error":"status must be open or closed"}`, http.StatusBadRequest)
		return
	}

	if err := h.queries.UpdateFeedbackStatus(r.Context(), id, req.Status); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *FeedbackHandler) ServeImage(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	if !user.IsAdmin {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "feedbackID"))
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}

	imgPath, err := h.queries.GetFeedbackImagePath(r.Context(), id)
	if err != nil || imgPath == "" {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	http.ServeFile(w, r, imgPath)
}
