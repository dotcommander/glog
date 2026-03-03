// Package sse provides Server-Sent Events functionality for real-time updates.
package sse

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dotcommander/glog/internal/constants"
)

// Event represents a Server-Sent Event.
type Event struct {
	Type string      `json:"event"`        // Event type (e.g., "log.created")
	Data interface{} `json:"data"`         // Event data
	ID   string      `json:"id,omitempty"` // Optional event ID
}

// Hub manages SSE client connections.
type Hub struct {
	mu      sync.RWMutex
	clients map[chan<- Event]struct{}
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[chan<- Event]struct{}),
	}
}

// AddClient adds a new client to the hub.
func (h *Hub) AddClient(client chan<- Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[client] = struct{}{}
}

// RemoveClient removes a client from the hub.
func (h *Hub) RemoveClient(client chan<- Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, client)
	close(client)
}

// Broadcast sends an event to all connected clients.
func (h *Hub) Broadcast(event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	timer := time.NewTimer(constants.SSEClientTimeout)
	defer timer.Stop()

	for client := range h.clients {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(constants.SSEClientTimeout)

		select {
		case client <- event:
			// Successfully sent
		case <-timer.C:
			// Client is slow, skip
			slog.Warn("SSE client slow, skipping event", "event_type", event.Type)
		}
	}
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Close disconnects all clients and closes the hub.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for client := range h.clients {
		close(client)
	}
	h.clients = make(map[chan<- Event]struct{})
}

// WriteEvent writes a Server-Sent Event to an http.ResponseWriter.
func WriteEvent(w http.ResponseWriter, event Event) error {
	// Write event type if present
	if event.Type != "" {
		_, _ = fmt.Fprintf(w, "event: %s\n", event.Type)
	}

	// Write event ID if present
	if event.ID != "" {
		_, _ = fmt.Fprintf(w, "id: %s\n", event.ID)
	}

	// Write event data
	data, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	// Split data into lines and prefix each with "data: "
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		_, _ = fmt.Fprintf(w, "data: %s\n", line)
	}

	// End event
	_, _ = fmt.Fprint(w, "\n")

	return nil
}

// SetHeaders sets the standard SSE response headers.
func SetHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
}
