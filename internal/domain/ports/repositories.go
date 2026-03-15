// Package ports defines the interfaces (ports) for external dependencies.
// Following Clean Architecture, these interfaces allow the domain layer
// to remain independent of infrastructure implementations.
package ports

import (
	"context"
	"errors"
	"time"

	"github.com/dotcommander/glog/internal/domain/entities"
)

// ErrNotFound is returned when a requested entity does not exist.
var ErrNotFound = errors.New("not found")

// HostRepository defines operations for host persistence.
type HostRepository interface {
	// Create inserts a new host.
	Create(host *entities.Host) error

	// FindByID retrieves a host by ID.
	FindByID(id int64) (*entities.Host, error)

	// FindByAPIKey retrieves a host by API key.
	FindByAPIKey(ctx context.Context, apiKey string) (*entities.Host, error)

	// FindByName retrieves a host by name.
	FindByName(name string) (*entities.Host, error)

	// List retrieves all hosts.
	List() ([]*entities.Host, error)

	// UpdateLastSeen updates the last_seen timestamp for a host.
	UpdateLastSeen(ctx context.Context, hostID int64) error

	// UpdateLastLogID updates the last_log_id for a host.
	UpdateLastLogID(hostID int64, logID int64) error

	// Count returns the total number of hosts.
	Count() (int64, error)

	// CountOnline returns the number of online hosts.
	CountOnline() (int64, error)

	// Delete removes a host by ID.
	Delete(id int64) error

	// IncrementLogCounters atomically increments log_count and optionally error_count for a host.
	IncrementLogCounters(hostID int64, totalNew int64, errorNew int64) error
}

// LogFilters represents query filters for logs.
type LogFilters struct {
	HostID   *int64
	Level    string
	Search   string
	FromDate string
	ToDate   string
	Limit    int
	Offset   int
	OrderBy  string // "timestamp" or "created_at"
}

// HostLogStats contains aggregated log statistics for a host.
type HostLogStats struct {
	TotalLogs    int64
	ErrorLogs    int64
	WarningLogs  int64
	LastLogTime  *time.Time
	LogsToday    int64
	LogsThisWeek int64
}

// GroupedLog represents a group of logs sharing the same fingerprint.
type GroupedLog struct {
	Fingerprint string    `json:"fingerprint"`
	Count       int       `json:"count"`
	Level       string    `json:"level"`
	Message     string    `json:"message"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
	HostID      int64     `json:"host_id"`
	HostName    string    `json:"host_name"`
}

// LogRepository defines operations for log persistence.
type LogRepository interface {
	// Create inserts a new log entry.
	Create(log *entities.Log) error

	// BulkCreate inserts multiple logs in a transaction.
	BulkCreate(logs []*entities.Log) ([]int64, error)

	// FindByID retrieves a log by ID.
	FindByID(id int64) (*entities.Log, error)

	// FindAll retrieves logs with filters and eager-loads host data.
	FindAll(filters LogFilters) ([]*entities.Log, int, error)

	// Delete removes a log by ID.
	Delete(id int64) error

	// CountByHost returns the number of logs for a host.
	CountByHost(hostID int64) (int64, error)

	// CountByHostAndLevel returns the number of logs for a host with a specific level.
	CountByHostAndLevel(hostID int64, level entities.LogLevel) (int64, error)

	// CountByHostSince returns the number of logs for a host since a given time.
	CountByHostSince(hostID int64, since time.Time) (int64, error)

	// GetLastLogTime returns the timestamp of the most recent log for a host.
	GetLastLogTime(hostID int64) (*time.Time, error)

	// CountByLevel returns the number of logs with a specific level.
	CountByLevel(level entities.LogLevel) (int64, error)

	// DeleteOlderThan deletes logs older than the specified time.
	DeleteOlderThan(cutoff time.Time) (int64, error)

	// GetHostStats returns aggregated log statistics for a host in a single query.
	GetHostStats(hostID int64) (*HostLogStats, error)

	// FindGrouped returns logs grouped by fingerprint with counts and time range.
	FindGrouped(filters LogFilters) ([]GroupedLog, int, error)
}

