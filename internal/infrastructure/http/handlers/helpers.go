package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/dotcommander/glog/internal/constants"
)

// writeJSON encodes v as JSON and writes it to w with the given HTTP status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func decodeJSONRequest(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, constants.MaxRequestBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}
