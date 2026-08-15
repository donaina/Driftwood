package events

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/donaina/driftwood/pkg/types"
)

type Hub struct {
	mu              sync.RWMutex
	clients         map[chan []byte]bool
	broadcast       chan []byte
	droppedAlerts   int64 // counter for dropped alerts
}

func NewHub() *Hub {
	h := &Hub{
		clients:   make(map[chan []byte]bool),
		broadcast: make(chan []byte, 100),
	}
	go h.run()
	return h
}

func (h *Hub) run() {
	for msg := range h.broadcast {
		h.mu.RLock()
		for clientCh := range h.clients {
			select {
			case clientCh <- msg:
			default:
				// Client buffer full, skip
			}
		}
		h.mu.RUnlock()
	}
}

func (h *Hub) Publish(eventType string, data interface{}) {
	event := types.EventMessage{
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return
	}

	select {
	case h.broadcast <- payload:
	default:
		// Broadcast channel full - log drop
		if eventType == "alert" {
			atomic.AddInt64(&h.droppedAlerts, 1)
		}
	}
}

// DroppedAlerts returns the count of dropped alerts
func (h *Hub) DroppedAlerts() int64 {
	return atomic.LoadInt64(&h.droppedAlerts)
}

func (h *Hub) SSEHandler(w http.ResponseWriter, r *http.Request) {
	// CORS: only localhost (same as API)
	origin := r.Header.Get("Origin")
	if isAllowedOrigin(origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	messageChan := make(chan []byte, 50)

	h.mu.Lock()
	h.clients[messageChan] = true
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, messageChan)
		close(messageChan)
		h.mu.Unlock()
	}()

	// Send initial ping
	fmt.Fprintf(w, "event: ping\ndata: {\"connected\": true}\n\n")
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-messageChan:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", string(msg))
			flusher.Flush()
		}
	}
}

// isAllowedOrigin returns true if the origin is localhost (security: no wildcard CORS)
func isAllowedOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	allowed := []string{
		"http://localhost:8787",
		"http://127.0.0.1:8787",
		"http://[::1]:8787",
	}
	for _, a := range allowed {
		if origin == a {
			return true
		}
	}
	return false
}