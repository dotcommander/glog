package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/dotcommander/glog/internal/constants"
	"github.com/dotcommander/glog/internal/domain/entities"
	"github.com/dotcommander/glog/internal/domain/ports"
	"github.com/dotcommander/glog/internal/infrastructure/http/middleware"
	"github.com/go-chi/chi/v5"
)

// CreateLogRequest represents the request body for log creation.
type CreateLogRequest struct {
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields,omitempty"`
	Timestamp string         `json:"timestamp,omitempty"` // ISO 8601 format
}

// CreateLogResponse represents the response for log creation.
type CreateLogResponse struct {
	ID      int64 `json:"id"`
	HostID  int64 `json:"host_id,omitempty"`
	Success bool  `json:"success"`
}

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

// BulkCreateLogsRequest represents the request body for bulk log creation.
type BulkCreateLogsRequest struct {
	Logs []CreateLogRequest `json:"logs"`
}

// BulkCreateLogsResponse represents the response for bulk log creation.
type BulkCreateLogsResponse struct {
	IDs     []int64 `json:"ids"`
	Count   int     `json:"count"`
	Success bool    `json:"success"`
}

// parseLogRequest validates a CreateLogRequest and constructs a Log entity.
func parseLogRequest(req CreateLogRequest, hostID int64, now time.Time) (*entities.Log, error) {
	if req.Message == "" {
		return nil, fmt.Errorf("message is required")
	}
	if len(req.Message) > constants.MaxMessageLength {
		return nil, fmt.Errorf("message too long (max %d characters)", constants.MaxMessageLength)
	}
	level := entities.LogLevelInfo
	if req.Level != "" {
		if !constants.IsValidLogLevel(req.Level) {
			return nil, fmt.Errorf("invalid level. Must be one of: trace, debug, info, warn, error, fatal")
		}
		level = entities.LogLevel(req.Level)
	}
	var timestamp time.Time
	if req.Timestamp != "" {
		var err error
		timestamp, err = time.Parse(time.RFC3339, req.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("invalid timestamp format. Use ISO 8601 (RFC3339)")
		}
	}
	if timestamp.IsZero() {
		timestamp = now
	}
	if timestamp.After(now.Add(constants.FutureTimestampLimit)) {
		return nil, fmt.Errorf("timestamp cannot be more than %v in the future", constants.FutureTimestampLimit)
	}
	return &entities.Log{
		HostID:    hostID,
		Level:     level,
		Message:   req.Message,
		Fields:    req.Fields,
		Timestamp: timestamp,
		CreatedAt: now,
	}, nil
}

// CreateLogHandler handles POST /api/v1/logs - Create a new log entry.
func CreateLogHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract host from context (set by auth middleware)
		host, ok := middleware.GetHostFromContext(r.Context())
		if !ok {
			http.Error(w, "Unauthorized - no host authentication", http.StatusUnauthorized)
			return
		}

		// Parse request body
		var req CreateLogRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		log, err := parseLogRequest(req, host.ID, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Apply smart metadata derivation
		h.patternMatcher.Analyze(log)

		// Save to database
		if err := h.logRepo.Create(log); err != nil {
			http.Error(w, "Failed to save log: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Update host counters
		var errorDelta int64
		if log.Level == entities.LogLevelError || log.Level == entities.LogLevelFatal {
			errorDelta = 1
		}
		if err := h.hostRepo.IncrementLogCounters(host.ID, 1, errorDelta); err != nil {
			slog.Warn("failed to update host counters", "error", err)
		}

		// Note: UpdateLastSeen is handled by auth middleware, no need to call again

		// Broadcast SSE event
		h.BroadcastLogCreated(logToResponse(log))

		// Return response
		resp := CreateLogResponse{
			ID:      log.ID,
			HostID:  host.ID,
			Success: true,
		}
		writeJSON(w, http.StatusCreated, resp)
	}
}

// CreateBulkLogsHandler handles POST /api/v1/logs/bulk - Create multiple log entries.
func CreateBulkLogsHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract host from context
		host, ok := middleware.GetHostFromContext(r.Context())
		if !ok {
			http.Error(w, "Unauthorized - no host authentication", http.StatusUnauthorized)
			return
		}

		// Parse request body
		var req BulkCreateLogsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Validate logs array
		if len(req.Logs) == 0 {
			http.Error(w, "Logs array cannot be empty", http.StatusBadRequest)
			return
		}

		if len(req.Logs) > constants.MaxBulkLogCount {
			http.Error(w, fmt.Sprintf("Too many logs. Maximum is %d per request", constants.MaxBulkLogCount), http.StatusBadRequest)
			return
		}

		// Convert to log entities
		now := time.Now()
		logs := make([]*entities.Log, len(req.Logs))
		for i, logReq := range req.Logs {
			log, err := parseLogRequest(logReq, host.ID, now)
			if err != nil {
				http.Error(w, "Log "+strconv.Itoa(i)+": "+err.Error(), http.StatusBadRequest)
				return
			}
			h.patternMatcher.Analyze(log)
			logs[i] = log
		}

		// Save to database
		ids, err := h.logRepo.BulkCreate(logs)
		if err != nil {
			http.Error(w, "Failed to save logs: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Update host counters
		var errorCount int64
		for _, l := range logs {
			if l.Level == entities.LogLevelError || l.Level == entities.LogLevelFatal {
				errorCount++
			}
		}
		if err := h.hostRepo.IncrementLogCounters(host.ID, int64(len(logs)), errorCount); err != nil {
			slog.Warn("failed to update host counters", "error", err)
		}

		// Note: UpdateLastSeen is handled by auth middleware, no need to call again

		// Return response
		resp := BulkCreateLogsResponse{
			IDs:     ids,
			Count:   len(ids),
			Success: true,
		}

		writeJSON(w, http.StatusCreated, resp)
	}
}

// ListLogsHandler handles GET /api/v1/logs - Query logs.
func ListLogsHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filters := parseLogFilters(r)

		filters.Limit, filters.Offset = constants.NormalizePagination(filters.Limit, filters.Offset)

		// Validate level if provided
		if filters.Level != "" && !constants.IsValidLogLevel(filters.Level) {
			http.Error(w, "Invalid level. Must be one of: trace, debug, info, warn, error, fatal", http.StatusBadRequest)
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
