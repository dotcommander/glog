package sqlite

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dotcommander/glog/internal/constants"
	_ "modernc.org/sqlite"
)

// Database wraps a SQLite database connection.
type Database struct {
	conn *sql.DB
	path string
}

// New creates a new Database connection.
func New(path string) (*Database, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database connection
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pooling for SQLite
	// SQLite works best with single writer to avoid lock contention
	conn.SetMaxOpenConns(constants.DBMaxOpenConns)
	conn.SetMaxIdleConns(constants.DBMaxIdleConns)
	conn.SetConnMaxLifetime(constants.DBConnMaxLifetime)

	// Enable WAL mode for better concurrency (readers don't block writers)
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Set busy timeout to handle transient locks (retry for 5 seconds)
	if _, err := conn.Exec(fmt.Sprintf("PRAGMA busy_timeout = %d", constants.DBBusyTimeout)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	// Increase cache size for better performance
	if _, err := conn.Exec(fmt.Sprintf("PRAGMA cache_size = %d", constants.DBCacheSize)); err != nil {
		// Non-fatal error, continue
		slog.Warn("failed to set cache size", "error", err)
	}

	// Enable foreign keys
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	db := &Database{
		conn: conn,
		path: path,
	}

	return db, nil
}

// Conn returns the underlying database connection.
func (db *Database) Conn() *sql.DB {
	return db.conn
}

// Path returns the database file path.
func (db *Database) Path() string {
	return db.path
}

// Size returns the database file size in bytes.
func (db *Database) Size() int64 {
	fi, err := os.Stat(db.path)
	if err != nil {
		return 0
	}
	return fi.Size()
}

// Close closes the database connection.
func (db *Database) Close() error {
	if db.conn == nil {
		return nil
	}

	// Checkpoint WAL before closing
	if _, err := db.conn.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		// Log warning but continue closing
		slog.Warn("WAL checkpoint failed", "error", err)
	}

	return db.conn.Close()
}

// Ping checks if the database connection is alive.
func (db *Database) Ping() error {
	return db.conn.Ping()
}

// MigrateFS runs database migrations from an fs.FS.
// The dir parameter is the subdirectory inside fsys that contains the *.sql files.
func (db *Database) MigrateFS(fsys fs.FS, dir string) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory %q: %w", dir, err)
	}

	// Collect and sort SQL file names for deterministic ordering.
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		path := dir + "/" + name
		content, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", name, err)
		}
		if err := db.applyMigrationContent(name, string(content)); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", name, err)
		}
	}

	return nil
}

// Migrate runs database migrations from the embedded SQL files.
// The migrationsPath parameter is ignored and kept only for backward compatibility.
func (db *Database) Migrate(_ string) error {
	return db.MigrateFS(EmbeddedMigrations, "migrations")
}

// applyMigrationContent executes the SQL content of a single migration file.
func (db *Database) applyMigrationContent(name, content string) error {
	slog.Info("applying migration", "file", name)

	// Split content into individual statements.
	// SQLite driver doesn't handle multiple statements well in one Exec.
	statements := splitSQLStatements(content)

	slog.Debug("migration parsed", "statements", len(statements))

	for i, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" || strings.HasPrefix(stmt, "--") {
			continue // Skip empty lines and comments
		}

		slog.Debug("executing statement", "index", i+1, "preview", truncate(stmt, 50))
		_, err := db.conn.Exec(stmt)
		if err != nil {
			// Check if it's a "table/index already exists" error
			if strings.Contains(err.Error(), "already exists") {
				slog.Debug("object already exists, skipping", "index", i+1)
				continue
			}
			return fmt.Errorf("failed to execute statement %q: %w", stmt, err)
		}
		slog.Debug("statement executed successfully", "index", i+1)
	}

	return nil
}

// truncate returns the first n runes of a string.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}

// splitSQLStatements splits SQL content into individual statements
func splitSQLStatements(content string) []string {
	var statements []string
	var current strings.Builder
	inString := false
	stringChar := byte(0)
	inDownMigration := false

	lines := strings.Split(content, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for down migration marker
		if strings.HasPrefix(trimmed, "-- +goose Down") {
			inDownMigration = true
			continue // Skip this line and all subsequent lines
		}

		// Skip comment lines and down migrations
		if inDownMigration || strings.HasPrefix(trimmed, "--") {
			continue
		}

		// Process line character by character to handle strings properly
		for i := 0; i < len(line); i++ {
			ch := line[i]

			// Handle string literals
			if !inString && (ch == '\'' || ch == '"') {
				inString = true
				stringChar = ch
			} else if inString && ch == stringChar {
				// Check for escaped quotes
				if i+1 >= len(line) || line[i+1] != stringChar {
					inString = false
				} else {
					i++ // Skip next quote
				}
			}

			current.WriteByte(ch)
		}

		// Add newline back
		current.WriteByte('\n')

		// Check for statement end (semicolon) when not in string
		if !inString && strings.HasSuffix(strings.TrimSpace(line), ";") {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
				current.Reset()
			}
		}
	}

	return statements
}

// Stats returns database statistics.
type Stats struct {
	Path            string
	Size            int64
	Tables          int
	TotalRows       int64
	IndexCount      int
	WALMode         bool
	CacheSize       int
	SynchronousMode string
}

// GetStats returns database statistics.
func (db *Database) GetStats() (*Stats, error) {
	stats := &Stats{
		Path: db.path,
	}

	// Get file size
	if fi, err := os.Stat(db.path); err == nil {
		stats.Size = fi.Size()
	}

	// Count tables
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`
	if err := db.conn.QueryRow(query).Scan(&stats.Tables); err != nil {
		return nil, fmt.Errorf("failed to count tables: %w", err)
	}

	// Get total rows (estimate)
	query = `
		SELECT SUM("rows")
		FROM sqlite_stat1
		WHERE tbl IN ('hosts', 'logs')
	`
	_ = db.conn.QueryRow(query).Scan(&stats.TotalRows) // Ignore error, may not exist

	// Count indexes
	query = `SELECT COUNT(*) FROM sqlite_master WHERE type='index'`
	if err := db.conn.QueryRow(query).Scan(&stats.IndexCount); err != nil {
		return nil, fmt.Errorf("failed to count indexes: %w", err)
	}

	// Get PRAGMA settings
	var journalMode string
	if err := db.conn.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err == nil {
		stats.WALMode = (journalMode == "wal")
	}

	if err := db.conn.QueryRow("PRAGMA cache_size").Scan(&stats.CacheSize); err != nil {
		return nil, fmt.Errorf("failed to get cache size: %w", err)
	}

	if err := db.conn.QueryRow("PRAGMA synchronous").Scan(&stats.SynchronousMode); err != nil {
		return nil, fmt.Errorf("failed to get synchronous mode: %w", err)
	}

	return stats, nil
}

// Vacuum runs VACUUM to reclaim space and defragment the database.
func (db *Database) Vacuum() error {
	_, err := db.conn.Exec("VACUUM")
	return err
}

// GetVersion returns the SQLite version.
func (db *Database) GetVersion() (string, error) {
	var version string
	err := db.conn.QueryRow("SELECT sqlite_version()").Scan(&version)
	return version, err
}
