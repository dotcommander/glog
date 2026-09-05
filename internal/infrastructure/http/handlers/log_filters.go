package handlers

import (
	"net/http"
	"strconv"

	"github.com/dotcommander/glog/internal/constants"
	"github.com/dotcommander/glog/internal/domain/ports"
)

const invalidLogLevelError = "Invalid level. Must be one of: trace, debug, info, warn, error, fatal"

// parseLogFilters extracts log query filters from query parameters.
func parseLogFilters(r *http.Request) ports.LogFilters {
	q := r.URL.Query()
	filters := ports.LogFilters{
		Level:    q.Get("level"),
		FromDate: q.Get("from"),
		ToDate:   q.Get("to"),
		OrderBy:  q.Get("order_by"),
	}

	if limit := q.Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 {
			filters.Limit = l
		}
	}
	if offset := q.Get("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil && o >= 0 {
			filters.Offset = o
		}
	}

	// Support both "q" and "search" params.
	if search := q.Get("q"); search != "" {
		filters.Search = search
	} else {
		filters.Search = q.Get("search")
	}

	if hostID := q.Get("host_id"); hostID != "" {
		if id, err := strconv.ParseInt(hostID, 10, 64); err == nil {
			filters.HostID = &id
		}
	}

	return filters
}

// parseValidatedLogFilters extracts, normalizes, and validates list-log filters.
func parseValidatedLogFilters(w http.ResponseWriter, r *http.Request) (ports.LogFilters, bool) {
	filters := parseLogFilters(r)
	filters.Limit, filters.Offset = constants.NormalizePagination(filters.Limit, filters.Offset)

	if filters.Level != "" && !constants.IsValidLogLevel(filters.Level) {
		http.Error(w, invalidLogLevelError, http.StatusBadRequest)
		return ports.LogFilters{}, false
	}

	return filters, true
}
