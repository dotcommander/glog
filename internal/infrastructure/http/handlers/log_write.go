package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/dotcommander/glog/internal/constants"
	"github.com/dotcommander/glog/internal/domain/entities"
	"github.com/dotcommander/glog/internal/infrastructure/http/middleware"
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

// isErrorLevel reports whether level counts as an error for counter purposes.
func isErrorLevel(level entities.LogLevel) bool {
	return level == entities.LogLevelError || level == entities.LogLevelFatal
}

// CreateLogHandler handles POST /api/v1/logs - Create a new log entry.
func CreateLogHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, ok := middleware.GetHostFromContext(r.Context())
		if !ok {
			http.Error(w, "Unauthorized - no host authentication", http.StatusUnauthorized)
			return
		}

		var req CreateLogRequest
		if !decodeJSONRequest(w, r, &req) {
			return
		}

		log, err := parseLogRequest(req, host.ID, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		h.patternMatcher.Analyze(log)

		if err := h.logRepo.Create(log); err != nil {
			http.Error(w, "Failed to save log: "+err.Error(), http.StatusInternalServerError)
			return
		}

		var errorDelta int64
		if isErrorLevel(log.Level) {
			errorDelta = 1
		}
		if err := h.hostRepo.IncrementLogCounters(host.ID, 1, errorDelta); err != nil {
			slog.Warn("failed to update host counters", "error", err)
		}

		// UpdateLastSeen is handled by auth middleware.
		h.BroadcastLogCreated(logToResponse(log))
		if h.recorder != nil {
			h.recorder.Record("log.created", map[string]any{
				"host_id": log.HostID,
				"level":   string(log.Level),
			})
		}

		writeJSON(w, http.StatusCreated, CreateLogResponse{ID: log.ID, HostID: host.ID, Success: true})
	}
}

// CreateBulkLogsHandler handles POST /api/v1/logs/bulk - Create multiple log entries.
func CreateBulkLogsHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, ok := middleware.GetHostFromContext(r.Context())
		if !ok {
			http.Error(w, "Unauthorized - no host authentication", http.StatusUnauthorized)
			return
		}

		var req BulkCreateLogsRequest
		if !decodeJSONRequest(w, r, &req) {
			return
		}

		if len(req.Logs) == 0 {
			http.Error(w, "Logs array cannot be empty", http.StatusBadRequest)
			return
		}
		if len(req.Logs) > constants.MaxBulkLogCount {
			http.Error(w, fmt.Sprintf("Too many logs. Maximum is %d per request", constants.MaxBulkLogCount), http.StatusBadRequest)
			return
		}

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

		ids, err := h.logRepo.BulkCreate(logs)
		if err != nil {
			http.Error(w, "Failed to save logs: "+err.Error(), http.StatusInternalServerError)
			return
		}

		var errorCount int64
		for _, l := range logs {
			if isErrorLevel(l.Level) {
				errorCount++
			}
		}
		if err := h.hostRepo.IncrementLogCounters(host.ID, int64(len(logs)), errorCount); err != nil {
			slog.Warn("failed to update host counters", "error", err)
		}

		// UpdateLastSeen is handled by auth middleware.
		if h.recorder != nil {
			h.recorder.Record("log.bulk_created", map[string]any{
				"host_id":    host.ID,
				"batch_size": len(logs),
				"errors":     errorCount,
			})
		}

		writeJSON(w, http.StatusCreated, BulkCreateLogsResponse{IDs: ids, Count: len(ids), Success: true})
	}
}
