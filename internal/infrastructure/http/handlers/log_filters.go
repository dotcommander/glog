package handlers

import (
	"net/http"
	"strconv"

	"github.com/dotcommander/glog/internal/domain/ports"
)

// parseLogFilters extracts log query filters from query parameters.
// Callers are responsible for normalizing limit/offset via constants.NormalizePagination.
func parseLogFilters(r *http.Request) ports.LogFilters {
	q := r.URL.Query()
	filters := ports.LogFilters{
		Level:   q.Get("level"),
		FromDate: q.Get("from"),
		ToDate:  q.Get("to"),
		OrderBy: q.Get("order_by"),
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
