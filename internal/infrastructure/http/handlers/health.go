package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/dotcommander/glog/internal/infrastructure/persistence/sqlite"
	"github.com/dotcommander/glog/internal/infrastructure/sse"
)

// HealthHandler handles health check requests.
type HealthHandler struct {
	db  *sqlite.Database
	hub *sse.Hub
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(db *sqlite.Database, hub *sse.Hub) *HealthHandler {
	return &HealthHandler{
		db:  db,
		hub: hub,
	}
}

// ServeHTTP handles GET /health - Health check endpoint.
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check database connection
	dbStatus := "ok"
	if err := h.db.Ping(); err != nil {
		dbStatus = "error"
	}

	// Get stats
	dbStats, _ := h.db.GetStats()
	sseClients := h.hub.ClientCount()

	// Build response
	response := map[string]interface{}{
		"status": "healthy",
		"database": map[string]interface{}{
			"status": dbStatus,
			"path":   h.db.Path(),
			"size":   dbStats.Size,
		},
		"sse": map[string]interface{}{
			"clients": sseClients,
		},
		"timestamp": time.Now().Format(time.RFC3339),
	}

	if dbStatus == "error" {
		response["status"] = "degraded"
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Error("failed to encode health response", "error", err)
	}
}
