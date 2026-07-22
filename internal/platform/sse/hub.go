package sse

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"courtscore/internal/platform/authkit"
	"courtscore/internal/platform/httpx"
)

// Event is an SSE event sent to connected clients.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

// Hub manages SSE client connections per user.
type Hub struct {
	mu      sync.RWMutex
	clients map[string][]chan Event // userID → channels
}

func NewHub() *Hub {
	return &Hub{clients: make(map[string][]chan Event)}
}

// Subscribe registers a channel for the given userID and returns an unsubscribe func.
func (h *Hub) Subscribe(userID string) (chan Event, func()) {
	ch := make(chan Event, 8)
	h.mu.Lock()
	h.clients[userID] = append(h.clients[userID], ch)
	h.mu.Unlock()

	unsub := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		list := h.clients[userID]
		for i, c := range list {
			if c == ch {
				h.clients[userID] = append(list[:i], list[i+1:]...)
				break
			}
		}
		if len(h.clients[userID]) == 0 {
			delete(h.clients, userID)
		}
		close(ch)
	}
	return ch, unsub
}

// Publish sends an event to all connections of a specific user.
func (h *Hub) Publish(userID string, event Event) {
	h.mu.RLock()
	list := h.clients[userID]
	h.mu.RUnlock()
	for _, ch := range list {
		select {
		case ch <- event:
		default:
			slog.Warn("sse: dropped event (client buffer full)", "user", userID, "type", event.Type)
		}
	}
}

// Handler returns an HTTP handler for the SSE endpoint. It requires an
// authenticated user in the request context (set by authkit.Middleware).
func (h *Hub) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := authkit.UserIDFromCtx(r.Context())
		if !ok {
			httpx.Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			httpx.Error(w, http.StatusInternalServerError, "streaming not supported")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		ch, unsub := h.Subscribe(userID)
		defer unsub()

		writeEvent(w, flusher, Event{Type: "connected", Data: map[string]string{"user_id": userID}})

		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				writeEvent(w, flusher, Event{Type: "heartbeat"})
			case ev, open := <-ch:
				if !open {
					return
				}
				writeEvent(w, flusher, ev)
			}
		}
	}
}

func writeEvent(w http.ResponseWriter, f http.Flusher, ev Event) {
	data, _ := json.Marshal(ev.Data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
	f.Flush()
}
