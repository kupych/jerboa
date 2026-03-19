package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"jerboa/internal/db"
)

type WSMessage struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

type wsClient struct {
	conn     *websocket.Conn
	channels map[string]bool
	send     chan WSMessage
}

type Hub struct {
	mu       sync.RWMutex
	clients  map[*wsClient]bool
	queries  *db.Queries
}

func NewHub(queries *db.Queries) *Hub {
	return &Hub{
		clients: make(map[*wsClient]bool),
		queries: queries,
	}
}

func (h *Hub) Broadcast(channel string, msg WSMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		if client.channels[channel] {
			select {
			case client.send <- msg:
			default:
				// Client too slow, skip
			}
		}
	}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	user := UserFrom(r.Context())
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		slog.Error("websocket accept", "error", err)
		return
	}

	client := &wsClient{
		conn:     conn,
		channels: make(map[string]bool),
		send:     make(chan WSMessage, 64),
	}

	h.mu.Lock()
	h.clients[client] = true
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, client)
		h.mu.Unlock()
		conn.Close(websocket.StatusNormalClosure, "")
	}()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Writer goroutine
	go func() {
		for {
			select {
			case msg := <-client.send:
				ctx2, cancel2 := context.WithTimeout(ctx, 5*time.Second)
				err := wsjson.Write(ctx2, conn, msg)
				cancel2()
				if err != nil {
					cancel()
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Reader loop — handle subscribe/unsubscribe
	for {
		var msg struct {
			Type    string `json:"type"`
			Channel string `json:"channel"`
		}
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			return
		}

		switch msg.Type {
		case "subscribe":
			if h.canAccess(ctx, user.ID, msg.Channel) {
				client.channels[msg.Channel] = true
				wsjson.Write(ctx, conn, WSMessage{
					Type:    "subscribed",
					Payload: map[string]string{"channel": msg.Channel},
				})
			}
		case "unsubscribe":
			delete(client.channels, msg.Channel)
		case "ping":
			wsjson.Write(ctx, conn, WSMessage{Type: "pong"})
		}
	}
}

func (h *Hub) canAccess(ctx context.Context, userID interface{}, channel string) bool {
	// Simple channel access check
	// Channels are formatted as "band:<id>" or "track:<id>"
	// For now, allow any authenticated user to subscribe
	// TODO: verify band/track membership
	return true
}

