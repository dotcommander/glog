package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/dotcommander/glog/internal/constants"
	"github.com/dotcommander/glog/internal/domain/entities"
	"github.com/dotcommander/glog/internal/domain/ports"
)

// allowedOrderBy is the set of valid column names for ORDER BY in log queries.
var allowedOrderBy = map[string]bool{
	"timestamp":  true,
	"created_at": true,
	"id":         true,
	"level":      true,
}

const logInsertSQL = `
	INSERT INTO logs (
		host_id, level, message, fields, timestamp, created_at,
		http_method, http_url, http_status, http_response_time_ms,
		derived_level, derived_source, derived_category, fingerprint
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`

// LogRepository handles database operations for logs.
type LogRepository struct {
	db *Database
}

// NewLogRepository creates a new LogRepository.
func NewLogRepository(db *Database) *LogRepository {
	return &LogRepository{db: db}
}

// logInsertArgs returns the ordered bind values for a log INSERT.
func logInsertArgs(log *entities.Log) []any {
	return []any{
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
		log.Fingerprint,
	}
}

// LogFilters is an alias for ports.LogFilters for backward compatibility.
type LogFilters = ports.LogFilters

func updateSummaryBuckets(exec sqlExecer, logs []*entities.Log) error {
	for _, log := range logs {
		bucketStart := log.Timestamp.UTC().Truncate(time.Hour)
		bucketStartText := bucketStart.Format(time.RFC3339)
		logTimestampText := log.Timestamp.UTC().Format(time.RFC3339)
		if _, err := exec.Exec(`
			INSERT INTO log_buckets (bucket_start, bucket_width, host_id, level, count)
			VALUES (?, '1h', ?, ?, 1)
			ON CONFLICT(bucket_start, bucket_width, host_id, level)
			DO UPDATE SET count = count + 1
		`, bucketStartText, log.HostID, string(log.Level)); err != nil {
			return fmt.Errorf("failed to update log bucket: %w", err)
		}

		if log.Fingerprint == "" {
			continue
		}
		if _, err := exec.Exec(`
			INSERT INTO fingerprint_buckets (
				bucket_start, bucket_width, fingerprint, level, count, first_seen, last_seen
			) VALUES (?, '1h', ?, ?, 1, ?, ?)
			ON CONFLICT(bucket_start, bucket_width, fingerprint, level)
			DO UPDATE SET
				count = count + 1,
				first_seen = CASE WHEN excluded.first_seen < first_seen THEN excluded.first_seen ELSE first_seen END,
				last_seen = CASE WHEN excluded.last_seen > last_seen THEN excluded.last_seen ELSE last_seen END
		`, bucketStartText, log.Fingerprint, string(log.Level), logTimestampText, logTimestampText); err != nil {
			return fmt.Errorf("failed to update fingerprint bucket: %w", err)
		}
	}
	return nil
}

func parseSQLiteTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}
	if idx := strings.Index(value, " m=+"); idx >= 0 {
		value = value[:idx]
	}
	if idx := strings.Index(value, " m=-"); idx >= 0 {
		value = value[:idx]
	}

	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999 -0700 MST",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported timestamp format %q", value)
}

// Create inserts a new log entry.
func (r *LogRepository) Create(log *entities.Log) error {
	if log.HostID == 0 {
		return fmt.Errorf("host_id is required")
	}
	if log.Message == "" {
		return fmt.Errorf("message is required")
	}

	tx, err := r.db.Conn().Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(logInsertSQL, logInsertArgs(log)...)
	if err != nil {
		return fmt.Errorf("failed to insert log: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}

	log.ID = id
	if err := updateSummaryBuckets(tx, []*entities.Log{log}); err != nil {
		return err
	}
	if err := updateHostLogCounters(tx, []*entities.Log{log}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
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
	stmt, err := tx.Prepare(logInsertSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	// Execute bulk insert
	ids := make([]int64, len(logs))
	for i, log := range logs {
		result, err := stmt.Exec(logInsertArgs(log)...)
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

	if err := updateSummaryBuckets(tx, logs); err != nil {
		return nil, err
	}
	if err := updateHostLogCounters(tx, logs); err != nil {
		return nil, err
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
		       l.derived_level, l.derived_source, l.derived_category, l.fingerprint,
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

// buildWhereClause constructs a SQL WHERE clause and argument slice from the given filters.
// Returns an empty string and nil args when no filters are active.
func buildWhereClause(filters LogFilters) (string, []interface{}) {
	var whereClauses []string
	var args []interface{}

	if filters.HostID != nil {
		whereClauses = append(whereClauses, "l.host_id = ?")
		args = append(args, *filters.HostID)
	}

	if filters.Level != "" {
		whereClauses = append(whereClauses, "l.level = ?")
		args = append(args, filters.Level)
	}

	if filters.Search != "" {
		searchTerm := "%" + filters.Search + "%"
		whereClauses = append(whereClauses, "(l.message LIKE ? OR l.fields LIKE ?)")
		args = append(args, searchTerm, searchTerm)
	}

	if filters.FromDate != "" {
		whereClauses = append(whereClauses, "l.timestamp >= ?")
		args = append(args, filters.FromDate)
	}

	if filters.ToDate != "" {
		whereClauses = append(whereClauses, "l.timestamp <= ?")
		args = append(args, filters.ToDate)
	}

	if len(whereClauses) == 0 {
		return "", nil
	}

	return "WHERE " + strings.Join(whereClauses, " AND "), args
}

// FindAll retrieves logs with filters and eager-loads host data.
func (r *LogRepository) FindAll(filters LogFilters) ([]*entities.Log, int, error) {
	whereStr, args := buildWhereClause(filters)

	// Get total count (no JOIN needed — all WHERE filters reference logs columns only)
	total, err := r.Count(filters)
	if err != nil {
		return nil, 0, err
	}

	// Main query with pagination
	orderBy := filters.OrderBy
	if orderBy == "" {
		orderBy = "timestamp"
	}

	if !allowedOrderBy[orderBy] {
		orderBy = "timestamp"
	}

	limit, offset := constants.NormalizePagination(filters.Limit, filters.Offset)

	query := fmt.Sprintf(`
		SELECT l.id, l.host_id, l.level, l.message, l.fields, l.timestamp, l.created_at,
		       l.http_method, l.http_url, l.http_status, l.http_response_time_ms,
		       l.derived_level, l.derived_source, l.derived_category, l.fingerprint,
		       h.id, h.name, h.tags, h.status, h.last_seen
		FROM logs l
		JOIN hosts h ON l.host_id = h.id
		%s
		ORDER BY l.%s DESC
		LIMIT ? OFFSET ?
	`, whereStr, orderBy)

	queryArgs := make([]interface{}, 0, len(args)+2)
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, limit, offset)

	rows, err := r.db.Conn().Query(query, queryArgs...)
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

// Count returns the number of logs matching filters.
func (r *LogRepository) Count(filters LogFilters) (int, error) {
	whereStr, args := buildWhereClause(filters)
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM logs l
		%s
	`, whereStr)

	var total int
	if err := r.db.Conn().QueryRow(countQuery, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to count logs: %w", err)
	}
	return total, nil
}

// StreamAll streams logs with filters and eager-loaded host data.
func (r *LogRepository) StreamAll(ctx context.Context, filters LogFilters, fn func(*entities.Log) error) error {
	whereStr, args := buildWhereClause(filters)
	orderBy := filters.OrderBy
	if orderBy == "" || !allowedOrderBy[orderBy] {
		orderBy = "timestamp"
	}

	queryArgs := make([]interface{}, 0, len(args)+2)
	queryArgs = append(queryArgs, args...)
	limitClause := ""
	if filters.Limit > 0 {
		limit, offset := constants.NormalizePagination(filters.Limit, filters.Offset)
		limitClause = "LIMIT ? OFFSET ?"
		queryArgs = append(queryArgs, limit, offset)
	}

	query := fmt.Sprintf(`
		SELECT l.id, l.host_id, l.level, l.message, l.fields, l.timestamp, l.created_at,
		       l.http_method, l.http_url, l.http_status, l.http_response_time_ms,
		       l.derived_level, l.derived_source, l.derived_category, l.fingerprint,
		       h.id, h.name, h.tags, h.status, h.last_seen
		FROM logs l
		JOIN hosts h ON l.host_id = h.id
		%s
		ORDER BY l.%s DESC
		%s
	`, whereStr, orderBy, limitClause)

	rows, err := r.db.Conn().QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return fmt.Errorf("failed to query logs: %w", err)
	}
	defer rows.Close()

	stride := filters.Stride
	if stride < 1 {
		stride = 1
	}
	seen := 0
	for rows.Next() {
		var log entities.Log
		if err := r.scanLogWithHost(rows, &log); err != nil {
			return fmt.Errorf("failed to scan log: %w", err)
		}
		if seen%stride == 0 {
			if err := fn(&log); err != nil {
				return err
			}
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating logs: %w", err)
	}
	return nil
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
	var fingerprint sql.NullString
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
		&fingerprint,
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
	if fingerprint.Valid {
		log.Fingerprint = fingerprint.String
	}

	return nil
}

// FindGrouped returns logs grouped by fingerprint with counts and time range.
func (r *LogRepository) FindGrouped(filters LogFilters) ([]ports.GroupedLog, int, error) {
	whereStr, args := buildWhereClause(filters)

	fpFilter := "l.fingerprint IS NOT NULL AND l.fingerprint != ''"
	if whereStr == "" {
		whereStr = "WHERE " + fpFilter
	} else {
		whereStr += " AND " + fpFilter
	}

	countQuery := fmt.Sprintf(`SELECT COUNT(DISTINCT l.fingerprint) FROM logs l %s`, whereStr)
	var total int
	if err := r.db.Conn().QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count grouped logs: %w", err)
	}

	limit, offset := constants.NormalizePagination(filters.Limit, filters.Offset)

	// Use MAX on a severity rank to return the highest severity level name
	// deterministically, instead of relying on SQLite's arbitrary non-aggregated
	// column behavior. l.message and l.host_id are representative (same
	// fingerprint = same normalized pattern).
	query := fmt.Sprintf(`
		SELECT l.fingerprint, COUNT(*) as cnt,
		       CASE MAX(CASE l.level
		           WHEN 'fatal' THEN 6 WHEN 'error' THEN 5 WHEN 'warn' THEN 4
		           WHEN 'info' THEN 3 WHEN 'debug' THEN 2 WHEN 'trace' THEN 1 ELSE 0 END)
		           WHEN 6 THEN 'fatal' WHEN 5 THEN 'error' WHEN 4 THEN 'warn'
		           WHEN 3 THEN 'info' WHEN 2 THEN 'debug' WHEN 1 THEN 'trace'
		           ELSE 'info' END as level,
		       l.message,
		       MIN(l.timestamp) as first_seen, MAX(l.timestamp) as last_seen,
		       l.host_id, h.name
		FROM logs l
		JOIN hosts h ON l.host_id = h.id
		%s
		GROUP BY l.fingerprint
		ORDER BY last_seen DESC
		LIMIT ? OFFSET ?
	`, whereStr)

	queryArgs := make([]interface{}, 0, len(args)+2)
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, limit, offset)

	rows, err := r.db.Conn().Query(query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query grouped logs: %w", err)
	}
	defer rows.Close()

	var groups []ports.GroupedLog
	for rows.Next() {
		var g ports.GroupedLog
		var firstSeen, lastSeen string
		if err := rows.Scan(
			&g.Fingerprint, &g.Count, &g.Level, &g.Message,
			&firstSeen, &lastSeen, &g.HostID, &g.HostName,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan grouped log: %w", err)
		}
		g.FirstSeen, err = parseSQLiteTime(firstSeen)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to parse grouped first_seen: %w", err)
		}
		g.LastSeen, err = parseSQLiteTime(lastSeen)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to parse grouped last_seen: %w", err)
		}
		groups = append(groups, g)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating grouped logs: %w", err)
	}

	return groups, total, nil
}

// FindLogBuckets returns hourly log count summaries.
func (r *LogRepository) FindLogBuckets(filters LogFilters) ([]ports.LogBucket, error) {
	var where []string
	var args []any
	where = append(where, "bucket_width = '1h'")
	fromClause := "log_buckets lb"
	if filters.HostID != nil {
		where = append(where, "host_id = ?")
		args = append(args, *filters.HostID)
	} else if !filters.IncludeInternal {
		fromClause = "log_buckets lb JOIN hosts h ON h.id = lb.host_id"
		where = append(where, "h.name != ?")
		args = append(args, constants.InternalHost)
	}
	if filters.Level != "" {
		where = append(where, "level = ?")
		args = append(args, filters.Level)
	}
	if filters.FromDate != "" {
		where = append(where, "bucket_start >= ?")
		args = append(args, filters.FromDate)
	}
	if filters.ToDate != "" {
		where = append(where, "bucket_start <= ?")
		args = append(args, filters.ToDate)
	}

	query := `
		SELECT bucket_start, bucket_width, host_id, level, count
		FROM ` + fromClause + `
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY bucket_start ASC, host_id ASC, level ASC
	`
	rows, err := r.db.Conn().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query log buckets: %w", err)
	}
	defer rows.Close()

	var buckets []ports.LogBucket
	for rows.Next() {
		var bucket ports.LogBucket
		if err := rows.Scan(&bucket.BucketStart, &bucket.BucketWidth, &bucket.HostID, &bucket.Level, &bucket.Count); err != nil {
			return nil, fmt.Errorf("failed to scan log bucket: %w", err)
		}
		buckets = append(buckets, bucket)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating log buckets: %w", err)
	}
	return buckets, nil
}

// FindFingerprintBuckets returns hourly fingerprint summaries.
func (r *LogRepository) FindFingerprintBuckets(filters LogFilters) ([]ports.FingerprintBucket, error) {
	var where []string
	var args []any
	where = append(where, "bucket_width = '1h'")
	if filters.Level != "" {
		where = append(where, "level = ?")
		args = append(args, filters.Level)
	}
	if filters.FromDate != "" {
		where = append(where, "bucket_start >= ?")
		args = append(args, filters.FromDate)
	}
	if filters.ToDate != "" {
		where = append(where, "bucket_start <= ?")
		args = append(args, filters.ToDate)
	}

	query := `
		SELECT bucket_start, bucket_width, fingerprint, level, count, first_seen, last_seen
		FROM fingerprint_buckets
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY bucket_start ASC, fingerprint ASC, level ASC
	`
	rows, err := r.db.Conn().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query fingerprint buckets: %w", err)
	}
	defer rows.Close()

	var buckets []ports.FingerprintBucket
	for rows.Next() {
		var bucket ports.FingerprintBucket
		if err := rows.Scan(
			&bucket.BucketStart, &bucket.BucketWidth, &bucket.Fingerprint,
			&bucket.Level, &bucket.Count, &bucket.FirstSeen, &bucket.LastSeen,
		); err != nil {
			return nil, fmt.Errorf("failed to scan fingerprint bucket: %w", err)
		}
		buckets = append(buckets, bucket)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating fingerprint buckets: %w", err)
	}
	return buckets, nil
}
