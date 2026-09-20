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

// Hub fans published events out to connected SSE subscribers.
//
// An event carries the project it belongs to, and a subscriber carries the
// project it is watching, so one install's stream can serve several projects
// without each subscriber seeing the others' traffic. Without this, switching to
// one client's project still delivered another client's live alerts — isolation
// that would be cosmetic precisely where the dashboard is most alive.
type Hub struct {
	mu sync.RWMutex
	// clients maps a subscriber's channel to the project it is watching. An
	// empty scope means every project; see scopeMatches.
	clients       map[chan []byte]string
	broadcast     chan broadcastMessage
	droppedAlerts int64 // counter for dropped alerts
}

// broadcastMessage is one event on its way to the subscribers it belongs to.
// The project is carried alongside the encoded payload rather than inside it,
// because the payload is what the browser receives verbatim and the scope is
// this hub's business, not the client's.
type broadcastMessage struct {
	payload   []byte
	projectID string
}

func NewHub() *Hub {
	h := &Hub{
		clients:   make(map[chan []byte]string),
		broadcast: make(chan broadcastMessage, 100),
	}
	go h.run()
	return h
}

func (h *Hub) run() {
	for msg := range h.broadcast {
		h.mu.RLock()
		for clientCh, scope := range h.clients {
			if !scopeMatches(scope, msg.projectID) {
				continue
			}
			select {
			case clientCh <- msg.payload:
			default:
				// Client buffer full, skip
			}
		}
		h.mu.RUnlock()
	}
}

// scopeMatches decides whether a subscriber watching scope should receive an
// event belonging to projectID.
//
// Two empty values, and they are not the same empty. An event with no project is
// install-level — the project list changed, the mock mode changed — and goes to
// everyone, because there is no subscriber for whom it is another project's
// news. A subscriber with no scope asked for everything and gets everything,
// which is what a caller with no project in mind should get. The asymmetry is
// deliberate: "belongs to no project" is a statement about the event, while "no
// scope" is a request, and a request is answered generously.
func scopeMatches(scope, projectID string) bool {
	if projectID == "" || scope == "" {
		return true
	}
	return scope == projectID
}

// Publish sends an event to every subscriber watching its project.
//
// projectID is the project the event belongs to; an empty one means install-level
// and is delivered to all subscribers.
//
// The project is an explicit argument rather than something inferred from data,
// which was the tempting alternative and does not work: the event types this
// carries include alert_explained, whose only field is a traffic id;
// traffic_cleared, which carries nothing at all; and config_updated, which
// carries a config. Three of the ten call sites could not have been inferred,
// and a convention that most callers can follow is worse than an argument every
// caller must supply.
func (h *Hub) Publish(projectID, eventType string, data interface{}) {
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
	case h.broadcast <- broadcastMessage{payload: payload, projectID: projectID}:
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

// SSEHandler streams events for one project to the caller.
//
// projectID is resolved by the caller rather than read from the request here. The
// hub knows what a scope is and nothing about where a default comes from — that
// the absent case means "the active project" is a fact about the store, and this
// package does not have one. An empty projectID subscribes to every project.
//
// The scope is fixed for the life of the connection. A client that switches
// project reconnects rather than sending a new scope, because a subscription is
// not a query parameter on a poll: changing it mid-stream would leave events
// already in flight belonging to the project the client just left, with nothing
// in the frame to say so.
func (h *Hub) SSEHandler(w http.ResponseWriter, r *http.Request, projectID string) {
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
	h.clients[messageChan] = projectID
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
