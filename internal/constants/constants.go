// Package constants provides shared constants for the GLog application.
package constants

import "time"

// API version and server defaults
const (
	APIVersion    = "1.0.0"
	DefaultPort   = 6016
	DefaultHost   = "0.0.0.0"
	APIKeyPrefix  = "glog_v1_"
	APIKeyPrefixV = "glog_" // Legacy prefix support
)

// HTTP timeouts
const (
	ReadTimeout      = 30 * time.Second
	WriteTimeout     = 30 * time.Second
	IdleTimeout      = 120 * time.Second
	RequestTimeout   = 60 * time.Second
	ShutdownTimeout  = 30 * time.Second
	SSEKeepAlive     = 30 * time.Second
	SSEClientTimeout = 100 * time.Millisecond
	SSEChannelSize   = 10
)

// Database constants
const (
	DBBusyTimeout     = 5000  // milliseconds
	DBCacheSize       = -2000 // ~2MB (negative = KiB)
	DBMaxOpenConns    = 1
	DBMaxIdleConns    = 1
	DBConnMaxLifetime = time.Hour
	MaxRetries        = 3
	RetryBaseDelay    = 10 * time.Millisecond
)

// Validation limits
const (
	MaxMessageLength     = 10000
	MaxNameLength        = 255
	MaxDescriptionLength = 1000
	MaxBulkLogCount      = 1000
	DefaultLimit         = 100
	MaxLimit             = 1000
	FutureTimestampLimit = 5 * time.Minute
)

// Log levels
const (
	LogLevelTrace = "trace"
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
	LogLevelFatal = "fatal"
)

// ValidLogLevels is the list of valid log level strings.
var ValidLogLevels = []string{
	LogLevelTrace,
	LogLevelDebug,
	LogLevelInfo,
	LogLevelWarn,
	LogLevelError,
	LogLevelFatal,
}

// IsValidLogLevel checks if a log level string is valid.
func IsValidLogLevel(level string) bool {
	for _, valid := range ValidLogLevels {
		if level == valid {
			return true
		}
	}
	return false
}

// NormalizePagination caps limit to [1, MaxLimit] and floors offset at 0.
func NormalizePagination(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
