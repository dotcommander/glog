package handlers

import (
	"net/http"
	"time"

	"github.com/dotcommander/glog/internal/infrastructure/persistence/sqlite"
	"github.com/dotcommander/glog/internal/infrastructure/sse"
)

// DBProbe abstracts database health operations without coupling to a specific driver.
type DBProbe interface {
	Ping() error
	Path() string
	Size() int64
}

type DBStatsProbe interface {
	GetStats() (*sqlite.Stats, error)
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
	var durability map[string]interface{}
	if statsProbe, ok := h.db.(DBStatsProbe); ok {
		if stats, err := statsProbe.GetStats(); err == nil {
			durability = map[string]interface{}{
				"profile":      stats.DurabilityProfile,
				"synchronous":  stats.SynchronousMode,
				"wal_enabled":  stats.WALMode,
				"sqlite_cache": stats.CacheSize,
			}
		}
	}

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
	if durability != nil {
		response["durability"] = durability
	}

	status := http.StatusOK
	if dbStatus == "error" {
		response["status"] = "degraded"
		status = http.StatusServiceUnavailable
	}

	writeJSON(w, status, response)
}
