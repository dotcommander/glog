package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/dotcommander/glog/internal/domain/entities"
	"github.com/dotcommander/glog/internal/domain/ports"
)

// hostColumns is the SELECT column list for the hosts table.
const hostColumns = `id, name, api_key, COALESCE(tags, '[]'), status, created_at, last_seen,
       last_log_id, log_count, error_count, error_rate,
       description, hostname, ip, user_agent, COALESCE(metadata, '{}')`

// HostRepository handles database operations for hosts.
type HostRepository struct {
	db *Database
}

// NewHostRepository creates a new HostRepository.
func NewHostRepository(db *Database) *HostRepository {
	return &HostRepository{db: db}
}

// scanHost scans a single row into a Host struct.
func scanHost(scanner interface{ Scan(...any) error }, host *entities.Host) error {
	return scanner.Scan(
		&host.ID, &host.Name, &host.APIKey, &host.Tags, &host.Status,
		&host.CreatedAt, &host.LastSeen, &host.LastLogID, &host.LogCount,
		&host.ErrorCount, &host.ErrorRate, &host.Description, &host.Hostname,
		&host.IP, &host.UserAgent, &host.Metadata,
	)
}

// Create inserts a new host.
func (r *HostRepository) Create(host *entities.Host) error {
	// Validate host
	if host.Name == "" {
		return fmt.Errorf("host name cannot be empty")
	}
	if host.APIKey == "" {
		return fmt.Errorf("API key cannot be empty")
	}

	query := `
		INSERT INTO hosts (
			name, api_key, tags, status, created_at, last_seen,
			last_log_id, log_count, error_count, error_rate,
			description, hostname, ip, user_agent, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.Conn().Exec(
		query,
		host.Name,
		host.APIKey,
		host.Tags,
		host.Status,
		host.CreatedAt,
		host.LastSeen,
		host.LastLogID,
		host.LogCount,
		host.ErrorCount,
		host.ErrorRate,
		host.Description,
		host.Hostname,
		host.IP,
		host.UserAgent,
		host.Metadata,
	)
	if err != nil {
		return fmt.Errorf("failed to insert host: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}

	host.ID = id
	return nil
}

// FindByID retrieves a host by ID.
func (r *HostRepository) FindByID(id int64) (*entities.Host, error) {
	query := `SELECT ` + hostColumns + ` FROM hosts WHERE id = ?`

	var host entities.Host
	if err := scanHost(r.db.Conn().QueryRow(query, id), &host); err == sql.ErrNoRows {
		return nil, ports.ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("failed to query host: %w", err)
	}

	return &host, nil
}

// FindByAPIKey retrieves a host by API key.
func (r *HostRepository) FindByAPIKey(ctx context.Context, apiKey string) (*entities.Host, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key cannot be empty")
	}

	query := `SELECT ` + hostColumns + ` FROM hosts WHERE api_key = ?`

	// Truncate key for logging (security)
	keyPreview := apiKey
	if len(apiKey) > 20 {
		keyPreview = apiKey[:20] + "..."
	}
	slog.Debug("FindByAPIKey: looking up key", "key_preview", keyPreview)

	config := DefaultRetryConfig()

	return WithRetry(ctx, config, func() (*entities.Host, error) {
		var host entities.Host
		err := scanHost(r.db.Conn().QueryRow(query, apiKey), &host)

		if err == nil {
			slog.Debug("FindByAPIKey: host found", "host_id", host.ID)
			return &host, nil
		}

		if err == sql.ErrNoRows {
			slog.Debug("FindByAPIKey: no rows found")
			return nil, ports.ErrNotFound
		}

		slog.Debug("FindByAPIKey: query error", "error", err)
		return nil, fmt.Errorf("failed to query host: %w", err)
	})
}

// FindByName retrieves a host by name.
func (r *HostRepository) FindByName(name string) (*entities.Host, error) {
	if name == "" {
		return nil, fmt.Errorf("host name cannot be empty")
	}

	query := `SELECT ` + hostColumns + ` FROM hosts WHERE name = ?`

	var host entities.Host
	if err := scanHost(r.db.Conn().QueryRow(query, name), &host); err == sql.ErrNoRows {
		return nil, ports.ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("failed to query host: %w", err)
	}

	return &host, nil
}

// List retrieves all hosts.
func (r *HostRepository) List() ([]*entities.Host, error) {
	query := `SELECT ` + hostColumns + ` FROM hosts ORDER BY created_at DESC`

	rows, err := r.db.Conn().Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query hosts: %w", err)
	}
	defer rows.Close()

	var hosts []*entities.Host
	for rows.Next() {
		var host entities.Host
		if err := scanHost(rows, &host); err != nil {
			return nil, fmt.Errorf("failed to scan host: %w", err)
		}
		hosts = append(hosts, &host)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating hosts: %w", err)
	}

	return hosts, nil
}

// UpdateLastSeen updates the last_seen timestamp for a host.
func (r *HostRepository) UpdateLastSeen(ctx context.Context, hostID int64) error {
	if hostID <= 0 {
		return fmt.Errorf("invalid host ID: %d", hostID)
	}

	query := `UPDATE hosts SET last_seen = CURRENT_TIMESTAMP WHERE id = ?`
	config := DefaultRetryConfig()

	return WithRetryNoResult(ctx, config, func() error {
		result, err := r.db.Conn().Exec(query, hostID)
		if err != nil {
			return fmt.Errorf("failed to update last_seen: %w", err)
		}

		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get rows affected: %w", err)
		}

		if rows == 0 {
			return ports.ErrNotFound
		}

		return nil
	})
}

// UpdateLastLogID updates the last_log_id for a host.
func (r *HostRepository) UpdateLastLogID(hostID int64, logID int64) error {
	query := `UPDATE hosts SET last_log_id = ? WHERE id = ?`

	_, err := r.db.Conn().Exec(query, logID, hostID)
	if err != nil {
		return fmt.Errorf("failed to update last_log_id: %w", err)
	}

	return nil
}

// Count returns the total number of hosts.
func (r *HostRepository) Count() (int64, error) {
	var count int64
	err := r.db.Conn().QueryRow("SELECT COUNT(*) FROM hosts").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count hosts: %w", err)
	}
	return count, nil
}

// CountOnline returns the number of online hosts.
func (r *HostRepository) CountOnline() (int64, error) {
	var count int64
	query := `
		SELECT COUNT(*)
		FROM hosts
		WHERE last_seen > datetime('now', '-5 minutes')
	`
	err := r.db.Conn().QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count online hosts: %w", err)
	}
	return count, nil
}

// IncrementLogCounters atomically increments log_count and optionally error_count for a host.
func (r *HostRepository) IncrementLogCounters(hostID int64, totalNew int64, errorNew int64) error {
	query := `
		UPDATE hosts SET
			log_count = log_count + ?,
			error_count = error_count + ?,
			error_rate = CASE
				WHEN (log_count + ?) > 0
				THEN CAST((error_count + ?) AS REAL) / CAST((log_count + ?) AS REAL) * 100
				ELSE 0
			END
		WHERE id = ?
	`
	_, err := r.db.Conn().Exec(query, totalNew, errorNew, totalNew, errorNew, totalNew, hostID)
	if err != nil {
		return fmt.Errorf("failed to increment log counters: %w", err)
	}
	return nil
}

// Delete removes a host by ID.
func (r *HostRepository) Delete(id int64) error {
	result, err := r.db.Conn().Exec("DELETE FROM hosts WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete host: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ports.ErrNotFound
	}

	return nil
}
