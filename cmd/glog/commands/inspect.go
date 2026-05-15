package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/dotcommander/glog/internal/infrastructure/persistence/sqlite"
	"github.com/spf13/cobra"
)

type inspectDBReport struct {
	Path            string               `json:"path"`
	Size            int64                `json:"size"`
	SQLiteVersion   string               `json:"sqlite_version"`
	WALMode         bool                 `json:"wal_mode"`
	CacheSize       int                  `json:"cache_size"`
	SynchronousMode string               `json:"synchronous_mode"`
	Durability      string               `json:"durability"`
	Tables          int                  `json:"tables"`
	Indexes         []inspectIndex       `json:"indexes"`
	Counts          map[string]int64     `json:"counts"`
	OldestLog       string               `json:"oldest_log,omitempty"`
	NewestLog       string               `json:"newest_log,omitempty"`
	TopHosts        []inspectHostCount   `json:"top_hosts"`
	TopFingerprints []inspectFingerprint `json:"top_fingerprints,omitempty"`
	GeneratedAt     string               `json:"generated_at"`
}

type inspectIndex struct {
	Name  string `json:"name"`
	Table string `json:"table"`
	SQL   string `json:"sql,omitempty"`
}

type inspectHostCount struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	LogCount int64  `json:"log_count"`
}

type inspectFingerprint struct {
	Fingerprint string `json:"fingerprint"`
	Count       int64  `json:"count"`
}

// InspectCmd creates the inspect command group.
func InspectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect GLog data files offline",
	}
	cmd.AddCommand(inspectDBCmd())
	return cmd
}

func inspectDBCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Inspect a SQLite GLog database",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, _ := cmd.Flags().GetString("db")
			jsonOut, _ := cmd.Flags().GetBool("json")
			db, err := sqlite.New(dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			report, err := buildInspectDBReport(db)
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			printInspectDBReport(report)
			return nil
		},
	}
	cmd.Flags().String("db", "glog.db", "Database file path")
	cmd.Flags().Bool("json", false, "Output JSON")
	return cmd
}

func buildInspectDBReport(db *sqlite.Database) (*inspectDBReport, error) {
	stats, err := db.GetStats()
	if err != nil {
		return nil, err
	}
	version, _ := db.GetVersion()
	report := &inspectDBReport{
		Path:            stats.Path,
		Size:            stats.Size,
		SQLiteVersion:   version,
		WALMode:         stats.WALMode,
		CacheSize:       stats.CacheSize,
		SynchronousMode: stats.SynchronousMode,
		Durability:      stats.DurabilityProfile,
		Tables:          stats.Tables,
		Indexes:         []inspectIndex{},
		Counts:          map[string]int64{},
		TopHosts:        []inspectHostCount{},
		TopFingerprints: []inspectFingerprint{},
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	for _, table := range []string{"hosts", "logs"} {
		if tableExists(db, table) {
			var count int64
			if err := db.Conn().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err == nil {
				report.Counts[table] = count
			}
		}
	}
	if indexes := inspectIndexes(db); indexes != nil {
		report.Indexes = indexes
	}
	report.OldestLog, report.NewestLog = inspectLogRange(db)
	if hosts := inspectTopHosts(db); hosts != nil {
		report.TopHosts = hosts
	}
	if columnExists(db, "logs", "fingerprint") {
		if fingerprints := inspectTopFingerprints(db); fingerprints != nil {
			report.TopFingerprints = fingerprints
		}
	}
	return report, nil
}

func printInspectDBReport(report *inspectDBReport) {
	fmt.Printf("GLog database: %s\n", report.Path)
	fmt.Printf("size=%d sqlite=%s wal=%t synchronous=%s durability=%s\n\n",
		report.Size, report.SQLiteVersion, report.WALMode, report.SynchronousMode, report.Durability)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TABLE\tROWS")
	for name, count := range report.Counts {
		fmt.Fprintf(w, "%s\t%d\n", name, count)
	}
	w.Flush()
	if report.OldestLog != "" || report.NewestLog != "" {
		fmt.Printf("\nlogs: %s .. %s\n", report.OldestLog, report.NewestLog)
	}
	fmt.Printf("indexes: %d\n", len(report.Indexes))
}

func tableExists(db *sqlite.Database, table string) bool {
	var count int
	_ = db.Conn().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = ?`, table).Scan(&count)
	return count > 0
}

func columnExists(db *sqlite.Database, table, column string) bool {
	rows, err := db.Conn().Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err == nil && name == column {
			return true
		}
	}
	return false
}

func inspectIndexes(db *sqlite.Database) []inspectIndex {
	rows, err := db.Conn().Query(`SELECT name, tbl_name, COALESCE(sql, '') FROM sqlite_master WHERE type='index' ORDER BY tbl_name, name`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var indexes []inspectIndex
	for rows.Next() {
		var idx inspectIndex
		if err := rows.Scan(&idx.Name, &idx.Table, &idx.SQL); err == nil {
			indexes = append(indexes, idx)
		}
	}
	return indexes
}

func inspectLogRange(db *sqlite.Database) (string, string) {
	if !tableExists(db, "logs") {
		return "", ""
	}
	var oldest, newest string
	_ = db.Conn().QueryRow(`SELECT COALESCE(MIN(timestamp), ''), COALESCE(MAX(timestamp), '') FROM logs`).Scan(&oldest, &newest)
	return oldest, newest
}

func inspectTopHosts(db *sqlite.Database) []inspectHostCount {
	if !tableExists(db, "hosts") {
		return nil
	}
	rows, err := db.Conn().Query(`SELECT id, name, log_count FROM hosts ORDER BY log_count DESC, name ASC LIMIT 10`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var hosts []inspectHostCount
	for rows.Next() {
		var host inspectHostCount
		if err := rows.Scan(&host.ID, &host.Name, &host.LogCount); err == nil {
			hosts = append(hosts, host)
		}
	}
	return hosts
}

func inspectTopFingerprints(db *sqlite.Database) []inspectFingerprint {
	rows, err := db.Conn().Query(`
		SELECT fingerprint, COUNT(*)
		FROM logs
		WHERE fingerprint IS NOT NULL AND fingerprint != ''
		GROUP BY fingerprint
		ORDER BY COUNT(*) DESC, fingerprint ASC
		LIMIT 10
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var fingerprints []inspectFingerprint
	for rows.Next() {
		var fp inspectFingerprint
		if err := rows.Scan(&fp.Fingerprint, &fp.Count); err == nil {
			fingerprints = append(fingerprints, fp)
		}
	}
	return fingerprints
}
