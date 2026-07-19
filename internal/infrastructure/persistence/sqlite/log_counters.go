package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/dotcommander/glog/internal/domain/entities"
)

type sqlExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func updateHostLogCounters(exec sqlExecer, logs []*entities.Log) error {
	type counterDelta struct {
		total  int64
		errors int64
	}

	deltas := make(map[int64]counterDelta)
	hostIDs := make([]int64, 0, len(logs))
	for _, log := range logs {
		delta, ok := deltas[log.HostID]
		if !ok {
			hostIDs = append(hostIDs, log.HostID)
		}
		delta.total++
		if log.Level == entities.LogLevelError || log.Level == entities.LogLevelFatal {
			delta.errors++
		}
		deltas[log.HostID] = delta
	}

	for _, hostID := range hostIDs {
		delta := deltas[hostID]
		if err := incrementHostLogCounters(exec, hostID, delta.total, delta.errors); err != nil {
			return err
		}
	}
	return nil
}

// incrementHostLogCounters updates lifetime accepted-log counters in the same
// transaction as the corresponding log inserts.
func incrementHostLogCounters(exec sqlExecer, hostID int64, totalNew int64, errorNew int64) error {
	query := `
		UPDATE hosts SET
			log_count = log_count + ?,
			error_count = error_count + ?,
			error_rate = CASE
				WHEN (log_count + ?) > 0
				THEN CAST((error_count + ?) AS REAL) / CAST((log_count + ?) AS REAL)
				ELSE 0
			END
		WHERE id = ?
	`
	_, err := exec.Exec(query, totalNew, errorNew, totalNew, errorNew, totalNew, hostID)
	if err != nil {
		return fmt.Errorf("failed to increment log counters: %w", err)
	}
	return nil
}
