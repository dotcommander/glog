package handlers

import (
	"net/http"
	"time"

	"github.com/dotcommander/glog/internal/infrastructure/sse"
)

// DBProbe abstracts database health operations without coupling to a specific driver.
type DBProbe interface {
	Ping() error
	Path() string
	Size() int64
}

// HealthHandler handles health check requests.
type HealthHandler struct {
	db  DBProbe
	hub *sse.Hub
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(db DBProbe, hub *sse.Hub) *HealthHandler {
	return &HealthHandler{
		db:  db,
		hub: hub,
	}
}

// ServeHTTP handles GET /health - Health check endpoint.
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	dbStatus := "ok"
	if err := h.db.Ping(); err != nil {
		dbStatus = "error"
	}

	sseClients := h.hub.ClientCount()

	response := map[string]interface{}{
		"status": "healthy",
		"database": map[string]interface{}{
			"status": dbStatus,
			"path":   h.db.Path(),
			"size":   h.db.Size(),
		},
		"sse": map[string]interface{}{
			"clients": sseClients,
		},
		"timestamp": time.Now().Format(time.RFC3339),
	}

	status := http.StatusOK
	if dbStatus == "error" {
		response["status"] = "degraded"
		status = http.StatusServiceUnavailable
	}

	writeJSON(w, status, response)
}
