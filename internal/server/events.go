package server

import (
	"encoding/json"
	"net/http"
	"time"

	"jerboa/internal/db"
	"jerboa/internal/models"
)

type EventsHandler struct {
	queries *db.Queries
}

func NewEventsHandler(queries *db.Queries) *EventsHandler {
	return &EventsHandler{queries: queries}
}

type incomingEvent struct {
	SessionID string          `json:"session_id"`
	Kind      string          `json:"kind"`
	Path      string          `json:"path"`
	Metadata  json.RawMessage `json:"metadata"`
	ClientTS  int64           `json:"client_ts"`
}

type incomingBatch struct {
	Events []incomingEvent `json:"events"`
}

const (
	maxEventsPerBatch = 200
	maxMetadataBytes  = 4096
	maxPathBytes      = 512
	maxKindBytes      = 64
	maxSessionIDBytes = 64
)

func (h *EventsHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB
	var batch incomingBatch
	if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	if len(batch.Events) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(batch.Events) > maxEventsPerBatch {
		batch.Events = batch.Events[:maxEventsPerBatch]
	}

	now := time.Now()
	inputs := make([]db.EventInput, 0, len(batch.Events))
	for _, e := range batch.Events {
		if len(e.SessionID) == 0 || len(e.SessionID) > maxSessionIDBytes {
			continue
		}
		if len(e.Kind) == 0 || len(e.Kind) > maxKindBytes {
			continue
		}
		if len(e.Path) > maxPathBytes {
			e.Path = e.Path[:maxPathBytes]
		}
		if len(e.Metadata) > maxMetadataBytes {
			// Drop oversized metadata — ingest the event without it rather than losing it.
			e.Metadata = nil
		}

		// Trust server time by default; only accept client_ts if it's within
		// a reasonable window to order events within a burst.
		ts := now
		if e.ClientTS > 0 {
			clientTime := time.UnixMilli(e.ClientTS)
			if delta := now.Sub(clientTime); delta < time.Hour && delta > -time.Minute {
				ts = clientTime
			}
		}

		input := db.EventInput{
			SessionID: e.SessionID,
			Kind:      e.Kind,
			Path:      e.Path,
			Metadata:  e.Metadata,
			CreatedAt: ts,
		}
		if user != nil {
			uid := user.ID
			input.UserID = &uid
		}
		inputs = append(inputs, input)
	}

	if err := h.queries.InsertEvents(r.Context(), inputs); err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *EventsHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	if !user.IsAdmin {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	sessions, err := h.queries.ListEventSessions(r.Context(), 200)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if sessions == nil {
		sessions = []models.EventSession{}
	}

	counts, err := h.queries.EventKindCounts(r.Context(), time.Now().Add(-30*24*time.Hour))
	if err != nil {
		counts = map[string]int{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"sessions":    sessions,
		"kind_counts": counts,
	})
}

func (h *EventsHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	if !user.IsAdmin {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	sessionID := r.URL.Query().Get("id")
	if sessionID == "" || len(sessionID) > maxSessionIDBytes {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}

	events, err := h.queries.ListEventsBySession(r.Context(), sessionID)
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if events == nil {
		events = []models.Event{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}
