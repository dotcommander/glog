package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/dotcommander/glog/internal/constants"
	"github.com/dotcommander/glog/internal/domain/ports"
)

// LogBucketsHandler handles GET /api/v1/stats/logs.
func LogBucketsHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filters, ok := parseStatsFilters(w, r)
		if !ok {
			return
		}
		buckets, err := h.logRepo.FindLogBuckets(filters)
		if err != nil {
			http.Error(w, "Failed to fetch log stats: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if buckets == nil {
			buckets = []ports.LogBucket{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"buckets": buckets, "total": len(buckets)})
	}
}

// FingerprintBucketsHandler handles GET /api/v1/stats/fingerprints.
func FingerprintBucketsHandler(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filters, ok := parseStatsFilters(w, r)
		if !ok {
			return
		}
		buckets, err := h.logRepo.FindFingerprintBuckets(filters)
		if err != nil {
			http.Error(w, "Failed to fetch fingerprint stats: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if buckets == nil {
			buckets = []ports.FingerprintBucket{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"buckets": buckets, "total": len(buckets)})
	}
}

func parseStatsFilters(w http.ResponseWriter, r *http.Request) (ports.LogFilters, bool) {
	if bucket := r.URL.Query().Get("bucket"); bucket != "" && bucket != "1h" {
		http.Error(w, "Only bucket=1h is supported", http.StatusBadRequest)
		return ports.LogFilters{}, false
	}

	filters := ports.LogFilters{}
	filters.IncludeInternal = r.URL.Query().Get("include_internal") == "true"
	if level := r.URL.Query().Get("level"); level != "" {
		if !constants.IsValidLogLevel(level) {
			http.Error(w, "Invalid level. Must be one of: trace, debug, info, warn, error, fatal", http.StatusBadRequest)
			return ports.LogFilters{}, false
		}
		filters.Level = level
	}
	if hostID := r.URL.Query().Get("host_id"); hostID != "" {
		id, err := parseInt64Param(hostID)
		if err != nil {
			http.Error(w, "Invalid host_id", http.StatusBadRequest)
			return ports.LogFilters{}, false
		}
		filters.HostID = &id
	}
	if from := r.URL.Query().Get("from"); from != "" {
		ts, err := parseStatsTime(from)
		if err != nil {
			http.Error(w, "Invalid from timestamp", http.StatusBadRequest)
			return ports.LogFilters{}, false
		}
		filters.FromDate = ts.Format(time.RFC3339)
	}
	if to := r.URL.Query().Get("to"); to != "" {
		ts, err := parseStatsTime(to)
		if err != nil {
			http.Error(w, "Invalid to timestamp", http.StatusBadRequest)
			return ports.LogFilters{}, false
		}
		filters.ToDate = ts.Format(time.RFC3339)
	}
	return filters, true
}

func parseInt64Param(value string) (int64, error) {
	return strconv.ParseInt(value, 10, 64)
}

func parseStatsTime(value string) (time.Time, error) {
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts, nil
	}
	if ts, err := time.Parse("2006-01-02", value); err == nil {
		return ts, nil
	}
	return time.Time{}, fmt.Errorf("invalid timestamp %q", value)
}
