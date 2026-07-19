package metrics

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/dotcommander/glog/internal/constants"
	"github.com/dotcommander/glog/internal/domain/entities"
	"github.com/dotcommander/glog/internal/domain/ports"
)

// Recorder writes low-cardinality internal server metrics through normal repositories.
type Recorder struct {
	logRepo ports.LogRepository
	hostID  int64
}

// NewRecorder creates or loads the reserved internal host.
func NewRecorder(hostRepo ports.HostRepository, logRepo ports.LogRepository) (*Recorder, error) {
	host, err := hostRepo.FindByName(constants.InternalHost)
	if err != nil {
		if err != ports.ErrNotFound {
			return nil, err
		}
		host, err = entities.NewHost(constants.InternalHost, []string{"internal"})
		if err != nil {
			return nil, err
		}
		host.Description = "GLog internal telemetry"
		host.Status = entities.HostStatusOnline
		if err := hostRepo.Create(host); err != nil {
			return nil, err
		}
	}
	return &Recorder{logRepo: logRepo, hostID: host.ID}, nil
}

// Record writes one internal telemetry log. Errors are logged but not returned.
func (r *Recorder) Record(event string, fields map[string]any) {
	if r == nil || r.hostID == 0 || event == "" {
		return
	}
	now := time.Now()
	log := &entities.Log{
		HostID:    r.hostID,
		Level:     entities.LogLevelInfo,
		Message:   "glog." + event,
		Fields:    entities.JSONMap(fields),
		Timestamp: now,
		CreatedAt: now,
	}
	if log.Fields == nil {
		log.Fields = entities.JSONMap{}
	}
	log.Fields["event"] = event
	if err := r.logRepo.Create(log); err != nil {
		slog.Debug("failed to record internal metric", "event", event, "error", err)
	}
}

// RecordError writes an internal error telemetry log.
func (r *Recorder) RecordError(event string, err error) {
	fields := map[string]any{"error": fmt.Sprint(err)}
	r.Record(event, fields)
}
