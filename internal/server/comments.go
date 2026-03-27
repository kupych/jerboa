package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"jerboa/internal/db"
	"jerboa/internal/models"
)

type CommentHandler struct {
	queries *db.Queries
	hub     *Hub
}

func NewCommentHandler(queries *db.Queries, hub *Hub) *CommentHandler {
	return &CommentHandler{queries: queries, hub: hub}
}

func (h *CommentHandler) List(w http.ResponseWriter, r *http.Request) {
	trackID, err := uuid.Parse(chi.URLParam(r, "trackID"))
	if err != nil {
		http.Error(w, `{"error":"invalid track id"}`, http.StatusBadRequest)
		return
	}

	// Verify user has access to this track's band
	if err := h.verifyTrackAccess(r, trackID); err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	comments, err := h.queries.ListComments(r.Context(), trackID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if comments == nil {
		comments = []models.Comment{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comments)
}

func (h *CommentHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	trackID, err := uuid.Parse(chi.URLParam(r, "trackID"))
	if err != nil {
		http.Error(w, `{"error":"invalid track id"}`, http.StatusBadRequest)
		return
	}

	if err := h.verifyTrackAccess(r, trackID); err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	var body struct {
		Body        string     `json:"body"`
		TimestampMS *int64     `json:"timestamp_ms"`
		ParentID    *uuid.UUID `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Body == "" {
		http.Error(w, `{"error":"body is required"}`, http.StatusBadRequest)
		return
	}

	comment := &models.Comment{
		TrackID:     trackID,
		UserID:      user.ID,
		ParentID:    body.ParentID,
		Body:        body.Body,
		TimestampMS: body.TimestampMS,
	}

	if err := h.queries.CreateComment(r.Context(), comment); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	comment.User = user

	// Get the track's band for the broadcast channel
	track, _ := h.queries.GetTrack(r.Context(), trackID)
	if track != nil {
		h.hub.Broadcast("track:"+trackID.String(), WSMessage{
			Type:    "comment.new",
			Payload: comment,
		})
		h.hub.Broadcast("band:"+track.BandID.String(), WSMessage{
			Type:    "activity.update",
			Payload: map[string]string{"band_id": track.BandID.String()},
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(comment)
}

func (h *CommentHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	commentID, err := uuid.Parse(chi.URLParam(r, "commentID"))
	if err != nil {
		http.Error(w, `{"error":"invalid comment id"}`, http.StatusBadRequest)
		return
	}

	var body struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Body == "" {
		http.Error(w, `{"error":"body is required"}`, http.StatusBadRequest)
		return
	}

	if err := h.queries.UpdateComment(r.Context(), commentID, user.ID, body.Body); err != nil {
		http.Error(w, `{"error":"not found or forbidden"}`, http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *CommentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	commentID, err := uuid.Parse(chi.URLParam(r, "commentID"))
	if err != nil {
		http.Error(w, `{"error":"invalid comment id"}`, http.StatusBadRequest)
		return
	}

	if err := h.queries.DeleteComment(r.Context(), commentID, user.ID); err != nil {
		http.Error(w, `{"error":"not found or forbidden"}`, http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

var errForbidden = errors.New("forbidden")

func (h *CommentHandler) verifyTrackAccess(r *http.Request, trackID uuid.UUID) error {
	user := UserFrom(r.Context())
	track, err := h.queries.GetTrack(r.Context(), trackID)
	if err != nil {
		return err
	}
	if track == nil {
		return errForbidden
	}
	isMember, _ := CheckBandAccess(h.queries, r.Context(), track.BandID, user.ID, user.IsAdmin)
	if !isMember {
		return errForbidden
	}
	return nil
}
