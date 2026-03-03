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

		// Get logs from database
		logs, total, err := h.logRepo.FindAll(filters)
		if err != nil {
			http.Error(w, "Failed to fetch logs: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Build export response - Host is always eager-loaded by FindAll via JOIN
		var exportLogs []LogExportResponse
		for _, log := range logs {
			exportLog := LogExportResponse{
				ID:        log.ID,
				HostID:    log.HostID,
				Level:     string(log.Level),
				Message:   log.Message,
				Fields:    log.Fields,
				Timestamp: log.Timestamp.Format(time.RFC3339),
				CreatedAt: log.CreatedAt.Format(time.RFC3339),
				HostName:  log.Host.Name,
				HostTags:  log.Host.Tags,
			}

			// Add HTTP context if present
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

			// Add derived metadata if present
			if log.DerivedLevel != nil {
				exportLog.DerivedLevel = *log.DerivedLevel
			}
			if log.DerivedSource != nil {
				exportLog.DerivedSource = *log.DerivedSource
			}
			if log.DerivedCategory != nil {
				exportLog.DerivedCategory = *log.DerivedCategory
			}

			exportLogs = append(exportLogs, exportLog)
		}

		// Set response headers
		w.Header().Set("Content-Type", getContentType(format))
		w.Header().Set("X-Total-Count", strconv.Itoa(total))
		w.Header().Set("X-Export-Format", string(format))
		w.Header().Set("X-Export-Timestamp", time.Now().Format(time.RFC3339))

		// Write export
		if err := writeExport(w, exportLogs, format); err != nil {
			// If we already wrote to response, just log error
			slog.Error("failed to write export", "format", format, "error", err)
		}
	}
}

// parseLogFilters extracts filters from query parameters.
func parseLogFilters(r *http.Request) ports.LogFilters {
	filters := ports.LogFilters{}

	// Parse limit with validation
	if limit := r.URL.Query().Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 {
			filters.Limit = l
		}
	}
	if filters.Limit <= 0 {
		filters.Limit = constants.DefaultLimit
	}
	if filters.Limit > constants.MaxLimit {
		filters.Limit = constants.MaxLimit
	}

	// Parse offset with validation
	if offset := r.URL.Query().Get("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil && o >= 0 {
			filters.Offset = o
		}
	}

	// Parse level filter
	if level := r.URL.Query().Get("level"); level != "" {
		filters.Level = level
	}

	// Parse search query (support both "q" and "search" params)
	if search := r.URL.Query().Get("q"); search != "" {
		filters.Search = search
	} else if search := r.URL.Query().Get("search"); search != "" {
		filters.Search = search
	}

	// Parse date range
	if fromDate := r.URL.Query().Get("from"); fromDate != "" {
		filters.FromDate = fromDate
	}
	if toDate := r.URL.Query().Get("to"); toDate != "" {
		filters.ToDate = toDate
	}

	// Parse host filter
	if hostID := r.URL.Query().Get("host_id"); hostID != "" {
		if id, err := strconv.ParseInt(hostID, 10, 64); err == nil {
			filters.HostID = &id
		}
	}

	// Parse order
	if orderBy := r.URL.Query().Get("order_by"); orderBy != "" {
		filters.OrderBy = orderBy
	}

	return filters
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

// writeExport writes the logs in the specified format.
func writeExport(w http.ResponseWriter, logs []LogExportResponse, format ExportFormat) error {
	switch format {
	case JSONFormat:
		return writeJSONExport(w, logs)
	case CSVFormat:
		return writeCSVExport(w, logs)
	case NDJSONFormat:
		return writeNDJSONExport(w, logs)
	default:
		return fmt.Errorf("unsupported export format: %s", format)
	}
}

// writeJSONExport writes logs as a JSON array.
func writeJSONExport(w http.ResponseWriter, logs []LogExportResponse) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if len(logs) == 0 {
		// Return empty array instead of null
		_, err := w.Write([]byte("[]"))
		return err
	}
	return encoder.Encode(logs)
}

// writeCSVExport writes logs as CSV.
func writeCSVExport(w http.ResponseWriter, logs []LogExportResponse) error {
	writer := csv.NewWriter(w)

	// Write header
	header := []string{
		"id", "host_id", "level", "message", "timestamp", "created_at",
		"http_method", "http_url", "http_status", "http_duration_ms",
		"derived_level", "derived_source", "derived_category", "host_name", "host_tags",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write data rows
	for _, log := range logs {
		row := []string{
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
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	writer.Flush()
	return writer.Error()
}

// writeNDJSONExport writes logs as newline-delimited JSON.
func writeNDJSONExport(w http.ResponseWriter, logs []LogExportResponse) error {
	encoder := json.NewEncoder(w)
	for _, log := range logs {
		if err := encoder.Encode(log); err != nil {
			return err
		}
	}
	return nil
}
