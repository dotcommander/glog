package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/dotcommander/glog/internal/constants"
	"github.com/dotcommander/glog/internal/infrastructure/sse"
)

// SSEStreamHandler handles GET /api/v1/events - SSE streaming endpoint.
func SSEStreamHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set SSE headers
		sse.SetHeaders(w)

		// Create client channel
		client := make(chan sse.Event, constants.SSEChannelSize)

		// Register client
		h.hub.AddClient(client)

		// Ensure cleanup on disconnect
		defer h.hub.RemoveClient(client)

		// Send initial connection event
		connectEvent := sse.Event{
			Type: "connected",
			Data: map[string]interface{}{
				"status":    "connected",
				"timestamp": time.Now().Format(time.RFC3339),
			},
		}
		if err := sse.WriteEvent(w, connectEvent); err != nil {
			slog.Error("failed to send SSE connection event", "error", err)
			return
		}

		// Flush to ensure headers are sent
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		// Handle client disconnection
		ctx := r.Context()
		done := ctx.Done()

		// Send events to client
		keepAlive := time.NewTimer(constants.SSEKeepAlive)
		defer keepAlive.Stop()

		for {
			select {
			case <-done:
				// Client disconnected
				return

			case event, ok := <-client:
				if !ok {
					// Channel closed
					return
				}

				// Send event to client
				if err := sse.WriteEvent(w, event); err != nil {
					slog.Error("failed to send SSE event", "event_type", event.Type, "error", err)
					return
				}

				// Flush to send immediately
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}

			case <-keepAlive.C:
				// Send keep-alive comment
				_, _ = fmt.Fprintf(w, ": keep-alive\n\n")
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				keepAlive.Reset(constants.SSEKeepAlive)
			}
		}
	}
}
