package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"jerboa/internal/db"
	"jerboa/internal/models"
)

type ChatHandler struct {
	queries *db.Queries
	hub     *Hub
}

func NewChatHandler(queries *db.Queries, hub *Hub) *ChatHandler {
	return &ChatHandler{queries: queries, hub: hub}
}

func (h *ChatHandler) List(w http.ResponseWriter, r *http.Request) {
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

	messages, err := h.queries.ListChatMessages(r.Context(), band.ID, 50)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if messages == nil {
		messages = []models.ChatMessage{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func (h *ChatHandler) Send(w http.ResponseWriter, r *http.Request) {
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
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Body == "" {
		http.Error(w, `{"error":"body is required"}`, http.StatusBadRequest)
		return
	}

	msg, err := h.queries.CreateChatMessage(r.Context(), band.ID, user.ID, req.Body)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	h.hub.Broadcast("band:"+band.ID.String(), WSMessage{
		Type:    "chat.message",
		Payload: msg,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(msg)
}
