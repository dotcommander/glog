package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/dotcommander/glog/internal/constants"
	"github.com/dotcommander/glog/internal/domain/entities"
	"github.com/dotcommander/glog/internal/domain/ports"
)

// LogRepository handles database operations for logs.
type LogRepository struct {
	db *Database
}

// NewLogRepository creates a new LogRepository.
func NewLogRepository(db *Database) *LogRepository {
	return &LogRepository{db: db}
}

// LogFilters is an alias for ports.LogFilters for backward compatibility.
type LogFilters = ports.LogFilters

// Create inserts a new log entry.
func (r *LogRepository) Create(log *entities.Log) error {
	if log.HostID == 0 {
		return fmt.Errorf("host_id is required")
	}
	if log.Message == "" {
		return fmt.Errorf("message is required")
	}

	query := `
		INSERT INTO logs (
			host_id, level, message, fields, timestamp, created_at,
			http_method, http_url, http_status, http_response_time_ms,
			derived_level, derived_source, derived_category
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.Conn().Exec(
		query,
		log.HostID,
		log.Level,
		log.Message,
		log.Fields,
		log.Timestamp,
		log.CreatedAt,
		log.Method,
		log.Path,
		log.StatusCode,
		log.Duration,
		log.DerivedLevel,
		log.DerivedSource,
		log.DerivedCategory,
	)
	if err != nil {
		return fmt.Errorf("failed to insert log: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}

	log.ID = id
	return nil
}

// BulkCreate inserts multiple logs in a transaction.
func (r *LogRepository) BulkCreate(logs []*entities.Log) ([]int64, error) {
	if len(logs) == 0 {
		return []int64{}, nil
	}

	// Limit batch size
	if len(logs) > constants.MaxBulkLogCount {
		return nil, fmt.Errorf("batch size exceeds maximum of %d", constants.MaxBulkLogCount)
	}

	// Start transaction
	tx, err := r.db.Conn().Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare statement
	stmt, err := tx.Prepare(`
		INSERT INTO logs (
			host_id, level, message, fields, timestamp, created_at,
			http_method, http_url, http_status, http_response_time_ms,
			derived_level, derived_source, derived_category
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	// Execute bulk insert
	ids := make([]int64, len(logs))
	for i, log := range logs {
		result, err := stmt.Exec(
			log.HostID,
			log.Level,
			log.Message,
			log.Fields,
			log.Timestamp,
			log.CreatedAt,
			log.Method,
			log.Path,
			log.StatusCode,
			log.Duration,
			log.DerivedLevel,
			log.DerivedSource,
			log.DerivedCategory,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to insert log %d: %w", i, err)
		}

		id, err := result.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("failed to get log ID %d: %w", i, err)
		}

		ids[i] = id
		log.ID = id
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return ids, nil
}

// FindByID retrieves a log by ID.
func (r *LogRepository) FindByID(id int64) (*entities.Log, error) {
	query := `
		SELECT l.id, l.host_id, l.level, l.message, l.fields, l.timestamp, l.created_at,
		       l.http_method, l.http_url, l.http_status, l.http_response_time_ms,
		       l.derived_level, l.derived_source, l.derived_category,
		       h.id, h.name, h.tags, h.status, h.last_seen
		FROM logs l
		JOIN hosts h ON l.host_id = h.id
		WHERE l.id = ?
	`

	var log entities.Log
	if err := r.scanLogWithHost(r.db.Conn().QueryRow(query, id), &log); err != nil {
		if err == sql.ErrNoRows {
			return nil, ports.ErrNotFound
		}
		return nil, fmt.Errorf("failed to query log: %w", err)
	}

	return &log, nil
}

// FindAll retrieves logs with filters and eager-loads host data.
func (r *LogRepository) FindAll(filters LogFilters) ([]*entities.Log, int, error) {
	// Build WHERE clause
	var whereClauses []string
	var args []interface{}
	var countArgs []interface{}

	if filters.HostID != nil {
		whereClauses = append(whereClauses, "l.host_id = ?")
		args = append(args, *filters.HostID)
		countArgs = append(countArgs, *filters.HostID)
	}

	if filters.Level != "" {
		whereClauses = append(whereClauses, "l.level = ?")
		args = append(args, filters.Level)
		countArgs = append(countArgs, filters.Level)
	}

	if filters.Search != "" {
		// Simple search on message and fields
		searchTerm := "%" + filters.Search + "%"
		whereClauses = append(whereClauses, "(l.message LIKE ? OR l.fields LIKE ?)")
		args = append(args, searchTerm, searchTerm)
		countArgs = append(countArgs, searchTerm, searchTerm)
	}

	if filters.FromDate != "" {
		whereClauses = append(whereClauses, "l.timestamp >= ?")
		args = append(args, filters.FromDate)
		countArgs = append(countArgs, filters.FromDate)
	}

	if filters.ToDate != "" {
		whereClauses = append(whereClauses, "l.timestamp <= ?")
		args = append(args, filters.ToDate)
		countArgs = append(countArgs, filters.ToDate)
	}

	// Build WHERE string
	whereStr := ""
	if len(whereClauses) > 0 {
		whereStr = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Get total count (no JOIN needed — all WHERE filters reference logs columns only)
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM logs l
		%s
	`, whereStr)

	var total int
	if err := r.db.Conn().QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count logs: %w", err)
	}

	// Main query with pagination
	orderBy := filters.OrderBy
	if orderBy == "" {
		orderBy = "timestamp"
	}

	allowedOrderBy := map[string]bool{
		"timestamp":  true,
		"created_at": true,
		"id":         true,
		"level":      true,
	}
	if !allowedOrderBy[orderBy] {
		orderBy = "timestamp"
	}

	limit := filters.Limit
	if limit <= 0 {
		limit = constants.DefaultLimit
	}
	if limit > constants.MaxLimit {
		limit = constants.MaxLimit
	}

	offset := filters.Offset
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`
		SELECT l.id, l.host_id, l.level, l.message, l.fields, l.timestamp, l.created_at,
		       l.http_method, l.http_url, l.http_status, l.http_response_time_ms,
		       l.derived_level, l.derived_source, l.derived_category,
		       h.id, h.name, h.tags, h.status, h.last_seen
		FROM logs l
		JOIN hosts h ON l.host_id = h.id
		%s
		ORDER BY l.%s DESC
		LIMIT ? OFFSET ?
	`, whereStr, orderBy)

	args = append(args, limit, offset)

	rows, err := r.db.Conn().Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query logs: %w", err)
	}
	defer rows.Close()

	var logs []*entities.Log
	for rows.Next() {
		var log entities.Log
		if err := r.scanLogWithHost(rows, &log); err != nil {
			return nil, 0, fmt.Errorf("failed to scan log: %w", err)
		}
		logs = append(logs, &log)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating logs: %w", err)
	}

	return logs, total, nil
}

// Delete removes a log by ID.
func (r *LogRepository) Delete(id int64) error {
	result, err := r.db.Conn().Exec("DELETE FROM logs WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete log: %w", err)
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

// CountByHost returns the number of logs for a host.
func (r *LogRepository) CountByHost(hostID int64) (int64, error) {
	var count int64
	err := r.db.Conn().QueryRow("SELECT COUNT(*) FROM logs WHERE host_id = ?", hostID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count logs: %w", err)
	}
	return count, nil
}

// CountByHostAndLevel returns the number of logs for a host with a specific level.
func (r *LogRepository) CountByHostAndLevel(hostID int64, level entities.LogLevel) (int64, error) {
	var count int64
	err := r.db.Conn().QueryRow("SELECT COUNT(*) FROM logs WHERE host_id = ? AND level = ?", hostID, level).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count logs by host and level: %w", err)
	}
	return count, nil
}

// CountByHostSince returns the number of logs for a host since a given time.
func (r *LogRepository) CountByHostSince(hostID int64, since time.Time) (int64, error) {
	var count int64
	err := r.db.Conn().QueryRow("SELECT COUNT(*) FROM logs WHERE host_id = ? AND timestamp >= ?", hostID, since).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count logs since: %w", err)
	}
	return count, nil
}

// GetLastLogTime returns the timestamp of the most recent log for a host.
func (r *LogRepository) GetLastLogTime(hostID int64) (*time.Time, error) {
	var timestamp time.Time
	err := r.db.Conn().QueryRow("SELECT timestamp FROM logs WHERE host_id = ? ORDER BY timestamp DESC LIMIT 1", hostID).Scan(&timestamp)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get last log time: %w", err)
	}
	return &timestamp, nil
}

// CountByLevel returns the number of logs with a specific level.
func (r *LogRepository) CountByLevel(level entities.LogLevel) (int64, error) {
	var count int64
	err := r.db.Conn().QueryRow("SELECT COUNT(*) FROM logs WHERE level = ?", level).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count logs: %w", err)
	}
	return count, nil
}

// DeleteOlderThan deletes logs older than the specified time.
func (r *LogRepository) DeleteOlderThan(cutoff time.Time) (int64, error) {
	result, err := r.db.Conn().Exec("DELETE FROM logs WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old logs: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rows, nil
}

// GetHostStats returns aggregated log statistics for a host in a single query.
func (r *LogRepository) GetHostStats(hostID int64) (*ports.HostLogStats, error) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	weekAgo := now.AddDate(0, 0, -7)

	query := `
		SELECT
			COUNT(*) AS total_logs,
			COUNT(CASE WHEN level = 'error' OR level = 'fatal' THEN 1 END) AS error_logs,
			COUNT(CASE WHEN level = 'warn' THEN 1 END) AS warning_logs,
			MAX(timestamp) AS last_log_time,
			COUNT(CASE WHEN timestamp >= ? THEN 1 END) AS logs_today,
			COUNT(CASE WHEN timestamp >= ? THEN 1 END) AS logs_this_week
		FROM logs
		WHERE host_id = ?
	`

	var stats ports.HostLogStats
	var lastLogTime sql.NullTime

	err := r.db.Conn().QueryRow(query, todayStart, weekAgo, hostID).Scan(
		&stats.TotalLogs,
		&stats.ErrorLogs,
		&stats.WarningLogs,
		&lastLogTime,
		&stats.LogsToday,
		&stats.LogsThisWeek,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get host stats: %w", err)
	}

	if lastLogTime.Valid {
		stats.LastLogTime = &lastLogTime.Time
	}

	return &stats, nil
}

// scanLogWithHost scans a log row with host data.
func (r *LogRepository) scanLogWithHost(scanner interface{ Scan(...interface{}) error }, log *entities.Log) error {
	// Initialize Host struct to avoid nil pointer dereference
	log.Host = &entities.Host{}

	var fieldsJSON string
	var derivedLevel, derivedSource, derivedCategory sql.NullString
	var tagsJSON string
	var method, path sql.NullString

	err := scanner.Scan(
		&log.ID,
		&log.HostID,
		&log.Level,
		&log.Message,
		&fieldsJSON,
		&log.Timestamp,
		&log.CreatedAt,
		&method,
		&path,
		&log.StatusCode,
		&log.Duration,
		&derivedLevel,
		&derivedSource,
		&derivedCategory,
		&log.Host.ID,
		&log.Host.Name,
		&tagsJSON,
		&log.Host.Status,
		&log.Host.LastSeen,
	)
	if err != nil {
		return err
	}

	// Set HTTP context fields
	if method.Valid && method.String != "" {
		log.Method = &method.String
	}
	if path.Valid && path.String != "" {
		log.Path = &path.String
	}

	// Parse JSON fields
	if err := log.Fields.Scan([]byte(fieldsJSON)); err != nil && fieldsJSON != "" {
		return fmt.Errorf("failed to parse fields JSON: %w", err)
	}

	if err := log.Host.Tags.Scan([]byte(tagsJSON)); err != nil && tagsJSON != "" {
		return fmt.Errorf("failed to parse tags JSON: %w", err)
	}

	// Set derived fields
	if derivedLevel.Valid {
		log.DerivedLevel = &derivedLevel.String
	}
	if derivedSource.Valid {
		log.DerivedSource = &derivedSource.String
	}
	if derivedCategory.Valid {
		log.DerivedCategory = &derivedCategory.String
	}

	return nil
}
