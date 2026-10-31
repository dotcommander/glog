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
	ContextKeyHost         contextKey = "host"
	contextKeyLogCommitted contextKey = "log-committed"
)

// WithLogCommitMarker allows the ingestion route to distinguish a committed write
// from a rejected request without inferring transaction state from HTTP status.
func WithLogCommitMarker(ctx context.Context) (context.Context, *bool) {
	committed := new(bool)
	return context.WithValue(ctx, contextKeyLogCommitted, committed), committed
}

// MarkLogCommitted is called only after the log transaction commits.
func MarkLogCommitted(ctx context.Context) {
	if committed, ok := ctx.Value(contextKeyLogCommitted).(*bool); ok {
		*committed = true
	}
}

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
