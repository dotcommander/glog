package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/dotcommander/glog/internal/domain/entities"
	"github.com/dotcommander/glog/internal/domain/ports"
	"github.com/go-chi/chi/v5"
)

// LogResponse represents a log in API responses.
type LogResponse struct {
	ID        int64          `json:"id"`
	HostID    int64          `json:"host_id"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields,omitempty"`
	Timestamp string         `json:"timestamp"`
	CreatedAt string         `json:"created_at"`
	Host      *HostResponse  `json:"host,omitempty"`
}

// ListLogsHandler handles GET /api/v1/logs - Query logs.
func ListLogsHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filters, ok := parseValidatedLogFilters(w, r)
		if !ok {
			return
		}

		// Query logs
		logs, total, err := h.logRepo.FindAll(filters)
		if err != nil {
			http.Error(w, "Failed to fetch logs: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Convert to response format
		resp := make([]LogResponse, len(logs))
		for i, log := range logs {
			resp[i] = logToResponse(log)
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"logs":   resp,
			"total":  total,
			"limit":  filters.Limit,
			"offset": filters.Offset,
		})
	}
}

// GetLogHandler handles GET /api/v1/logs/{id} - Get a single log.
func GetLogHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse log ID from URL
		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid log ID", http.StatusBadRequest)
			return
		}

		log, err := h.logRepo.FindByID(id)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				http.Error(w, "Log not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Failed to fetch log: "+err.Error(), http.StatusInternalServerError)
			return
		}

		resp := logToResponse(log)
		writeJSON(w, http.StatusOK, resp)
	}
}

// logToResponse converts a Log entity to a LogResponse.
func logToResponse(log *entities.Log) LogResponse {
	resp := LogResponse{
		ID:        log.ID,
		HostID:    log.HostID,
		Level:     log.GetDisplayLevel(),
		Message:   log.Message,
		Fields:    log.Fields,
		Timestamp: log.Timestamp.Format(time.RFC3339),
		CreatedAt: log.CreatedAt.Format(time.RFC3339),
	}

	// Include host if available
	if log.Host != nil {
		resp.Host = &HostResponse{
			ID:     log.Host.ID,
			Name:   log.Host.Name,
			Tags:   log.Host.Tags,
			Status: string(log.Host.Status),
		}
	}

	return resp
}

// ListGroupedLogsHandler handles GET /api/v1/logs/groups - Query grouped logs by fingerprint.
func ListGroupedLogsHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filters, ok := parseValidatedLogFilters(w, r)
		if !ok {
			return
		}

		groups, total, err := h.logRepo.FindGrouped(filters)
		if err != nil {
			http.Error(w, "Failed to fetch grouped logs: "+err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"groups": groups,
			"total":  total,
			"limit":  filters.Limit,
			"offset": filters.Offset,
		})
	}
}
