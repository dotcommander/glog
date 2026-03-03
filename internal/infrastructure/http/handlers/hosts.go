package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dotcommander/glog/internal/constants"
	"github.com/dotcommander/glog/internal/domain/entities"
	"github.com/dotcommander/glog/internal/domain/ports"
	"github.com/go-chi/chi/v5"
)

// HostResponse represents a host in API responses.
type HostResponse struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Tags        []string `json:"tags"`
	Status      string   `json:"status"`
	CreatedAt   string   `json:"created_at"`
	LastSeen    string   `json:"last_seen"`
	LastLogID   *int64   `json:"last_log_id,omitempty"`
	LogCount    int64    `json:"log_count"`
	ErrorCount  int64    `json:"error_count"`
	ErrorRate   float64  `json:"error_rate"`
	Description string   `json:"description,omitempty"`
	Hostname    string   `json:"hostname,omitempty"`
	IP          string   `json:"ip,omitempty"`
	UserAgent   string   `json:"user_agent,omitempty"`
	Metadata    any      `json:"metadata,omitempty"`
	APIKey      string   `json:"api_key,omitempty"` // Only included on creation
}

// RegisterHostRequest represents the request body for host registration.
type RegisterHostRequest struct {
	Name        string   `json:"name"`
	Tags        []string `json:"tags,omitempty"`
	Description string   `json:"description,omitempty"`
	Hostname    string   `json:"hostname,omitempty"`
	IP          string   `json:"ip,omitempty"`
	UserAgent   string   `json:"user_agent,omitempty"`
	Metadata    any      `json:"metadata,omitempty"`
}

// hostToResponse converts a Host entity to a HostResponse.
func hostToResponse(host *entities.Host) HostResponse {
	resp := HostResponse{
		ID:          host.ID,
		Name:        host.Name,
		Tags:        host.Tags,
		Status:      string(host.Status),
		CreatedAt:   host.CreatedAt.Format(time.RFC3339),
		LastSeen:    host.LastSeen.Format(time.RFC3339),
		LastLogID:   host.LastLogID,
		LogCount:    host.LogCount,
		ErrorCount:  host.ErrorCount,
		ErrorRate:   host.ErrorRate,
		Description: host.Description,
		Hostname:    host.Hostname,
		IP:          host.IP,
		UserAgent:   host.UserAgent,
		APIKey:      host.APIKey, // Include API key (will be empty for GET requests)
	}

	// Only include metadata if it's not empty
	if len(host.Metadata) > 0 {
		resp.Metadata = host.Metadata
	}

	return resp
}

// HostStats represents host statistics.
type HostStats struct {
	TotalLogs    int64   `json:"total_logs"`
	ErrorLogs    int64   `json:"error_logs"`
	WarningLogs  int64   `json:"warning_logs"`
	ErrorRate    float64 `json:"error_rate"`
	LastLogTime  string  `json:"last_log_time,omitempty"`
	LogsToday    int64   `json:"logs_today"`
	LogsThisWeek int64   `json:"logs_this_week"`
}

// RegisterHostHandler handles POST /api/v1/hosts - Register a new host.
func RegisterHostHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse request body
		var req RegisterHostRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Validate required fields
		if req.Name == "" {
			http.Error(w, "Name is required", http.StatusBadRequest)
			return
		}

		if len(req.Name) > constants.MaxNameLength {
			http.Error(w, "Name must be at most 255 characters", http.StatusBadRequest)
			return
		}

		// Validate optional fields
		if req.Description != "" && len(req.Description) > constants.MaxDescriptionLength {
			http.Error(w, "Description must be at most 1000 characters", http.StatusBadRequest)
			return
		}

		// Create host entity
		host, err := entities.NewHost(req.Name, req.Tags)
		if err != nil {
			http.Error(w, "Failed to create host: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Set optional fields
		if req.Description != "" {
			host.Description = req.Description
		}
		if req.Hostname != "" {
			host.Hostname = req.Hostname
		}
		if req.IP != "" {
			host.IP = req.IP
		}
		if req.UserAgent != "" {
			host.UserAgent = req.UserAgent
		}
		if req.Metadata != nil {
			if metadata, ok := req.Metadata.(entities.JSONMap); ok {
				host.Metadata = metadata
			} else if mapData, ok := req.Metadata.(map[string]interface{}); ok {
				host.Metadata = entities.JSONMap{}
				for k, v := range mapData {
					host.Metadata[k] = v
				}
			}
		}

		// Save to database
		if err := h.hostRepo.Create(host); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint") {
				if strings.Contains(err.Error(), "name") {
					http.Error(w, "Host name already exists", http.StatusConflict)
					return
				}
				if strings.Contains(err.Error(), "api_key") {
					http.Error(w, "API key conflict (please try again)", http.StatusConflict)
					return
				}
			}
			http.Error(w, "Failed to save host: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Broadcast SSE event
		resp := hostToResponse(host)
		h.BroadcastHostRegistered(resp)

		// Return response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("failed to encode response", "error", err)
		}
	}
}

// ListHostsHandler handles GET /api/v1/hosts - List all hosts.
func ListHostsHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hosts, err := h.hostRepo.List()
		if err != nil {
			http.Error(w, "Failed to fetch hosts: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Convert to response format
		resp := make([]HostResponse, len(hosts))
		for i, host := range hosts {
			resp[i] = hostToResponse(host)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"hosts": resp,
			"total": len(resp),
		}); err != nil {
			slog.Error("failed to encode response", "error", err)
		}
	}
}

// GetHostHandler handles GET /api/v1/hosts/{id} - Get host details.
func GetHostHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse host ID from URL
		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid host ID", http.StatusBadRequest)
			return
		}

		host, err := h.hostRepo.FindByID(id)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				http.Error(w, "Host not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Failed to fetch host: "+err.Error(), http.StatusInternalServerError)
			return
		}

		resp := hostToResponse(host)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("failed to encode response", "error", err)
		}
	}
}

// GetHostStatsHandler handles GET /api/v1/hosts/{id}/stats - Get host statistics.
func GetHostStatsHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		hostID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid host ID", http.StatusBadRequest)
			return
		}

		// Verify host exists
		_, err = h.hostRepo.FindByID(hostID)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				http.Error(w, "Host not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Failed to fetch host: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Get all stats in a single query
		logStats, err := h.logRepo.GetHostStats(hostID)
		if err != nil {
			http.Error(w, "Failed to get host stats: "+err.Error(), http.StatusInternalServerError)
			return
		}

		var errorRate float64
		if logStats.TotalLogs > 0 {
			errorRate = float64(logStats.ErrorLogs) / float64(logStats.TotalLogs) * 100
		}

		stats := HostStats{
			TotalLogs:    logStats.TotalLogs,
			ErrorLogs:    logStats.ErrorLogs,
			WarningLogs:  logStats.WarningLogs,
			ErrorRate:    errorRate,
			LogsToday:    logStats.LogsToday,
			LogsThisWeek: logStats.LogsThisWeek,
		}

		if logStats.LastLogTime != nil {
			stats.LastLogTime = logStats.LastLogTime.Format(time.RFC3339)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(stats); err != nil {
			slog.Error("failed to encode response", "error", err)
		}
	}
}
