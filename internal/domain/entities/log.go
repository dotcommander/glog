package entities

import (
	"strings"
	"time"
)

// LogLevel represents the severity level of a log entry.
type LogLevel string

const (
	LogLevelTrace LogLevel = "trace"
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
	LogLevelFatal LogLevel = "fatal"
)

// LogPriority orders log levels by severity.
var LogPriority = map[LogLevel]int{
	LogLevelTrace: 0,
	LogLevelDebug: 1,
	LogLevelInfo:  2,
	LogLevelWarn:  3,
	LogLevelError: 4,
	LogLevelFatal: 5,
}

// Compare returns -1 if l < other, 1 if l > other, 0 if equal.
func (l LogLevel) Compare(other LogLevel) int {
	lp, lok := LogPriority[l]
	op, ook := LogPriority[other]
	if !lok || !ook {
		return 0
	}
	if lp < op {
		return -1
	} else if lp > op {
		return 1
	}
	return 0
}

// Log represents a log entry with enhanced metadata.
type Log struct {
	ID        int64     `json:"id"`
	HostID    int64     `json:"host_id"`
	Level     LogLevel  `json:"level"`
	Message   string    `json:"message"`
	Fields    JSONMap   `json:"fields,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	CreatedAt time.Time `json:"created_at"`

	// HTTP request context (for HTTP logs)
	Method     *string `json:"method,omitempty"`
	Path       *string `json:"path,omitempty"`
	StatusCode int     `json:"status_code,omitempty"`
	Duration   int64   `json:"duration_ms,omitempty"`

	// Derived metadata (from pattern matching)
	DerivedLevel    *string `json:"derived_level,omitempty"`
	DerivedSource   *string `json:"derived_source,omitempty"`
	DerivedCategory *string `json:"derived_category,omitempty"`
	Fingerprint     string  `json:"fingerprint,omitempty"`

	// Eager-loaded host (for API responses)
	Host *Host `json:"host,omitempty"`

	// Search optimization (full-text search index)
	SearchText string `json:"-"` // Concatenated text for FTS
}

// EffectiveLevel returns the most specific level available.
func (l *Log) EffectiveLevel() LogLevel {
	if l.DerivedLevel != nil && *l.DerivedLevel != "" {
		return LogLevel(*l.DerivedLevel)
	}
	return l.Level
}

// IsError returns true if the log level is error or fatal.
func (l *Log) IsError() bool {
	level := l.EffectiveLevel()
	return level == LogLevelError || level == LogLevelFatal
}

// IsWarning returns true if the log level is warn or higher.
func (l *Log) IsWarning() bool {
	level := l.EffectiveLevel()
	return level == LogLevelWarn || level == LogLevelError || level == LogLevelFatal
}

// GetColor returns the color associated with this log's level.
func (l *Log) GetColor() string {
	switch l.EffectiveLevel() {
	case LogLevelFatal:
		return "#991b1b" // Dark red
	case LogLevelError:
		return "#ef4444" // Red
	case LogLevelWarn:
		return "#f59e0b" // Amber
	case LogLevelInfo:
		return "#3b82f6" // Blue
	case LogLevelDebug:
		return "#6b7280" // Gray
	case LogLevelTrace:
		return "#9ca3af" // Light gray
	default:
		return "#8b5cf6" // Purple (default)
	}
}

// GetDisplayLevel returns the level string for display.
func (l *Log) GetDisplayLevel() string {
	level := l.EffectiveLevel()
	return string(level)
}

// IsHTTP returns true if this is an HTTP request log.
func (l *Log) IsHTTP() bool {
	return (l.Method != nil && *l.Method != "") || l.StatusCode > 0
}

// IsSuccessful returns true if HTTP status indicates success.
func (l *Log) IsSuccessful() bool {
	if !l.IsHTTP() {
		return false
	}
	return l.StatusCode >= 200 && l.StatusCode < 300
}

// IsServerError returns true if HTTP status is 5xx.
func (l *Log) IsServerError() bool {
	if !l.IsHTTP() {
		return false
	}
	return l.StatusCode >= 500 && l.StatusCode < 600
}

// IsClientError returns true if HTTP status is 4xx.
func (l *Log) IsClientError() bool {
	if !l.IsHTTP() {
		return false
	}
	return l.StatusCode >= 400 && l.StatusCode < 500
}

// IsRedirect returns true if HTTP status is 3xx.
func (l *Log) IsRedirect() bool {
	if !l.IsHTTP() {
		return false
	}
	return l.StatusCode >= 300 && l.StatusCode < 400
}

// BuildSearchText constructs the search text for full-text search.
func (l *Log) BuildSearchText() string {
	var parts []string

	// Add message
	parts = append(parts, l.Message)

	// Add fields (string values only)
	for key, value := range l.Fields {
		if str, ok := value.(string); ok {
			parts = append(parts, key+":"+str)
		}
	}

	// Add derived metadata
	if l.DerivedLevel != nil && *l.DerivedLevel != "" {
		parts = append(parts, *l.DerivedLevel)
	}
	if l.DerivedSource != nil && *l.DerivedSource != "" {
		parts = append(parts, *l.DerivedSource)
	}
	if l.DerivedCategory != nil && *l.DerivedCategory != "" {
		parts = append(parts, *l.DerivedCategory)
	}

	// Add HTTP context
	if l.Method != nil && *l.Method != "" {
		parts = append(parts, *l.Method)
	}
	if l.Path != nil && *l.Path != "" {
		parts = append(parts, *l.Path)
	}

	return strings.Join(parts, " ")
}
