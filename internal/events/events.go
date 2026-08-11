package events

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/callmidavid/apidiff/pkg/types"
)

type Hub struct {
	mu        sync.RWMutex
	clients   map[chan []byte]bool
	broadcast chan []byte
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
	}
}

func (h *Hub) SSEHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

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
