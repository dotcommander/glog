package middleware

import (
	"context"
	"net/http"

	"github.com/dotcommander/glog/internal/domain/entities"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	// ContextKeyHost is the context key for the authenticated host.
	ContextKeyHost contextKey = "host"
)

// GetHostFromContext retrieves the authenticated host from the request context.
func GetHostFromContext(ctx context.Context) (*entities.Host, bool) {
	host, ok := ctx.Value(ContextKeyHost).(*entities.Host)
	return host, ok
}

// CORS creates CORS middleware.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
