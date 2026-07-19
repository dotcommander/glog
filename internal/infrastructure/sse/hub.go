// Package sse provides Server-Sent Events functionality for real-time updates.
package sse

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
)

// Event represents a Server-Sent Event.
type Event struct {
	Type string      `json:"event"`        // Event type (e.g., "log.created")
	Data interface{} `json:"data"`         // Event data
	ID   string      `json:"id,omitempty"` // Optional event ID
}

type clientState struct {
	mu     sync.RWMutex
	ch     chan<- Event
	closed bool
}

func (c *clientState) trySend(event Event) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return true
	}
	select {
	case c.ch <- event:
		return true
	default:
		return false
	}
}

func (c *clientState) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	close(c.ch)
	c.closed = true
}

// Hub manages SSE client connections.
type Hub struct {
	mu              sync.RWMutex
	clients         map[chan<- Event]*clientState
	warnSlowClients func(eventType string, dropped int)
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[chan<- Event]*clientState),
		warnSlowClients: func(eventType string, dropped int) {
			slog.Warn("SSE clients slow, skipping event", "event_type", eventType, "dropped_clients", dropped)
		},
	}
}

// AddClient adds a new client to the hub.
func (h *Hub) AddClient(client chan<- Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, exists := h.clients[client]; exists {
		return
	}
	h.clients[client] = &clientState{ch: client}
}

// RemoveClient removes a client from the hub.
func (h *Hub) RemoveClient(client chan<- Event) {
	h.mu.Lock()
	state, ok := h.clients[client]
	if !ok {
		h.mu.Unlock()
		return
	}
	delete(h.clients, client)
	h.mu.Unlock()
	state.close()
}

// Broadcast immediately offers an event to every connected client. Delivery is
// best effort: clients whose channel is not ready drop the event.
func (h *Hub) Broadcast(event Event) {
	h.mu.RLock()
	clients := make([]*clientState, 0, len(h.clients))
	for _, client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.RUnlock()

	dropped := 0
	for _, client := range clients {
		if !client.trySend(event) {
			dropped++
		}
	}
	if dropped > 0 {
		h.warnSlowClients(event.Type, dropped)
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
	clients := make([]*clientState, 0, len(h.clients))
	for _, client := range h.clients {
		clients = append(clients, client)
	}
	h.clients = make(map[chan<- Event]*clientState)
	h.mu.Unlock()

	for _, client := range clients {
		client.close()
	}
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
