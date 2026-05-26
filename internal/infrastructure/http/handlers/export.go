package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dotcommander/glog/internal/constants"
	"github.com/dotcommander/glog/internal/domain/entities"
	"github.com/dotcommander/glog/internal/domain/ports"
)

// ExportFormat represents supported export formats.
type ExportFormat string

const (
	JSONFormat   ExportFormat = "json"
	CSVFormat    ExportFormat = "csv"
	NDJSONFormat ExportFormat = "ndjson"
)

// LogExportResponse represents a log in export format.
type LogExportResponse struct {
	ID              int64                  `json:"id,omitempty"`
	HostID          int64                  `json:"host_id"`
	Level           string                 `json:"level"`
	Message         string                 `json:"message"`
	Fields          map[string]interface{} `json:"fields,omitempty"`
	Timestamp       string                 `json:"timestamp"`
	CreatedAt       string                 `json:"created_at"`
	HTTPMethod      string                 `json:"http_method,omitempty"`
	HTTPURL         string                 `json:"http_url,omitempty"`
	HTTPStatus      int                    `json:"http_status,omitempty"`
	HTTPDuration    int64                  `json:"http_duration_ms,omitempty"`
	DerivedLevel    string                 `json:"derived_level,omitempty"`
	DerivedSource   string                 `json:"derived_source,omitempty"`
	DerivedCategory string                 `json:"derived_category,omitempty"`
	HostName        string                 `json:"host_name,omitempty"`
	HostTags        []string               `json:"host_tags,omitempty"`
}

// ExportLogsHandler handles GET /api/v1/export/{format} - Export logs in various formats.
func ExportLogsHandler(h *Handlers, formatStr string) http.HandlerFunc {
	format := ExportFormat(formatStr)

	return func(w http.ResponseWriter, r *http.Request) {
		// Parse query parameters (same as ListLogs)
		filters := parseLogFilters(r)
		filters.Limit = 0
		filters.Offset = 0
		if stride := r.URL.Query().Get("stride"); stride != "" {
			if n, err := strconv.Atoi(stride); err == nil && n > 0 {
				filters.Stride = n
			}
		}
		if filters.Level != "" && !constants.IsValidLogLevel(filters.Level) {
			http.Error(w, "Invalid level. Must be one of: trace, debug, info, warn, error, fatal", http.StatusBadRequest)
			return
		}

		total, err := h.logRepo.Count(filters)
		if err != nil {
			http.Error(w, "Failed to count logs: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Set response headers
		w.Header().Set("Content-Type", getContentType(format))
		w.Header().Set("X-Total-Count", strconv.Itoa(total))
		w.Header().Set("X-Export-Format", string(format))
		w.Header().Set("X-Export-Timestamp", time.Now().Format(time.RFC3339))

		// Write export
		start := time.Now()
		var exported int
		if err := writeStreamingExport(r, w, h, filters, format, &exported); err != nil {
			// If we already wrote to response, just log error
			slog.Error("failed to write export", "format", format, "error", err)
			if h.recorder != nil {
				h.recorder.RecordError("export.failed", err)
			}
			return
		}
		if h.recorder != nil {
			h.recorder.Record("export.completed", map[string]any{
				"format":      string(format),
				"count":       exported,
				"duration_ms": time.Since(start).Milliseconds(),
			})
		}
	}
}

// getContentType returns the appropriate Content-Type header for the format.
func getContentType(format ExportFormat) string {
	switch format {
	case JSONFormat:
		return "application/json; charset=utf-8"
	case CSVFormat:
		return "text/csv; charset=utf-8"
	case NDJSONFormat:
		return "application/x-ndjson; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

func writeStreamingExport(r *http.Request, w http.ResponseWriter, h *Handlers, filters ports.LogFilters, format ExportFormat, exported *int) error {
	switch format {
	case JSONFormat:
		return writeStreamingJSONExport(r, w, h, filters, exported)
	case CSVFormat:
		return writeStreamingCSVExport(r, w, h, filters, exported)
	case NDJSONFormat:
		return writeStreamingNDJSONExport(r, w, h, filters, exported)
	default:
		return fmt.Errorf("unsupported export format: %s", format)
	}
}

func logToExportResponse(log *entities.Log) LogExportResponse {
	exportLog := LogExportResponse{
		ID:        log.ID,
		HostID:    log.HostID,
		Level:     string(log.Level),
		Message:   log.Message,
		Fields:    log.Fields,
		Timestamp: log.Timestamp.Format(time.RFC3339),
		CreatedAt: log.CreatedAt.Format(time.RFC3339),
	}
	if log.Host != nil {
		exportLog.HostName = log.Host.Name
		exportLog.HostTags = log.Host.Tags
	}
	if log.Method != nil && *log.Method != "" {
		exportLog.HTTPMethod = *log.Method
	}
	if log.Path != nil && *log.Path != "" {
		exportLog.HTTPURL = *log.Path
	}
	if log.StatusCode > 0 {
		exportLog.HTTPStatus = log.StatusCode
	}
	if log.Duration > 0 {
		exportLog.HTTPDuration = log.Duration
	}
	if log.DerivedLevel != nil {
		exportLog.DerivedLevel = *log.DerivedLevel
	}
	if log.DerivedSource != nil {
		exportLog.DerivedSource = *log.DerivedSource
	}
	if log.DerivedCategory != nil {
		exportLog.DerivedCategory = *log.DerivedCategory
	}
	return exportLog
}

func writeStreamingJSONExport(r *http.Request, w http.ResponseWriter, h *Handlers, filters ports.LogFilters, exported *int) error {
	_, _ = w.Write([]byte("["))
	first := true
	encoder := json.NewEncoder(w)
	err := h.logRepo.StreamAll(r.Context(), filters, func(logEntry *entities.Log) error {
		if !first {
			if _, err := w.Write([]byte(",")); err != nil {
				return err
			}
		}
		first = false
		(*exported)++
		return encoder.Encode(logToExportResponse(logEntry))
	})
	if err != nil {
		return err
	}
	_, err = w.Write([]byte("]"))
	return err
}

func writeStreamingCSVExport(r *http.Request, w http.ResponseWriter, h *Handlers, filters ports.LogFilters, exported *int) error {
	writer := csv.NewWriter(w)
	if err := writer.Write(csvHeader()); err != nil {
		return err
	}
	err := h.logRepo.StreamAll(r.Context(), filters, func(logEntry *entities.Log) error {
		if err := writer.Write(csvRow(logToExportResponse(logEntry))); err != nil {
			return err
		}
		(*exported)++
		return nil
	})
	writer.Flush()
	if flushErr := writer.Error(); flushErr != nil {
		return flushErr
	}
	return err
}

func writeStreamingNDJSONExport(r *http.Request, w http.ResponseWriter, h *Handlers, filters ports.LogFilters, exported *int) error {
	encoder := json.NewEncoder(w)
	return h.logRepo.StreamAll(r.Context(), filters, func(logEntry *entities.Log) error {
		(*exported)++
		return encoder.Encode(logToExportResponse(logEntry))
	})
}

func csvHeader() []string {
	return []string{
		"id", "host_id", "level", "message", "timestamp", "created_at",
		"http_method", "http_url", "http_status", "http_duration_ms",
		"derived_level", "derived_source", "derived_category", "host_name", "host_tags",
	}
}

func csvRow(log LogExportResponse) []string {
	return []string{
		strconv.FormatInt(log.ID, 10),
		strconv.FormatInt(log.HostID, 10),
		log.Level,
		log.Message,
		log.Timestamp,
		log.CreatedAt,
		log.HTTPMethod,
		log.HTTPURL,
		strconv.Itoa(log.HTTPStatus),
		strconv.FormatInt(log.HTTPDuration, 10),
		log.DerivedLevel,
		log.DerivedSource,
		log.DerivedCategory,
		log.HostName,
		strings.Join(log.HostTags, ";"),
	}
}
