package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dotcommander/glog/internal/constants"
	"github.com/dotcommander/glog/internal/domain/ports"
	"github.com/dotcommander/glog/internal/infrastructure/http/handlers"
	"github.com/dotcommander/glog/internal/infrastructure/http/middleware"
	"github.com/dotcommander/glog/internal/infrastructure/persistence/sqlite"
	"github.com/dotcommander/glog/internal/infrastructure/sse"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

const dashboardIndex = "index.html"

// RouterDeps holds dependencies for creating the router.
type RouterDeps struct {
	DB        *sqlite.Database
	HostRepo  ports.HostRepository
	LogRepo   ports.LogRepository
	InboxRepo ports.InboxRepository
	Hub       *sse.Hub
	Handlers  *handlers.Handlers
}

// apiKeyAuthMiddleware creates middleware that validates API keys.
// Uses the ports.HostRepository interface for flexibility.
func apiKeyAuthMiddleware(hostRepo ports.HostRepository, ingestion bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract API key from Authorization header
			// Format: "Bearer glog_<hex>"
			auth := r.Header.Get("Authorization")
			if auth == "" {
				http.Error(w, `{"error":"Missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			// Parse Authorization header
			parts := strings.SplitN(auth, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, `{"error":"Invalid authorization format. Expected 'Bearer <api_key>'"}`, http.StatusUnauthorized)
				return
			}

			apiKey := strings.TrimSpace(parts[1])
			if apiKey == "" {
				http.Error(w, `{"error":"API key cannot be empty"}`, http.StatusUnauthorized)
				return
			}

			// Validate API key format (basic check)
			if !strings.HasPrefix(apiKey, constants.APIKeyPrefix) && !strings.HasPrefix(apiKey, constants.APIKeyPrefixV) {
				http.Error(w, `{"error":"Invalid API key format"}`, http.StatusUnauthorized)
				return
			}

			// Load host from database
			host, err := hostRepo.FindByAPIKey(r.Context(), apiKey)
			if err != nil {
				slog.Debug("auth middleware: FindByAPIKey failed", "error", err)
				http.Error(w, `{"error":"Invalid API key"}`, http.StatusUnauthorized)
				return
			}

			slog.Debug("auth middleware: host found", "host_id", host.ID)

			ctx := context.WithValue(r.Context(), middleware.ContextKeyHost, host)
			if ingestion {
				// Successful ingestion updates freshness in its host-counter transaction;
				// rejected requests still count as authenticated activity.
				var committed *bool
				ctx, committed = middleware.WithLogCommitMarker(ctx)
				defer func() {
					if !*committed {
						if err := hostRepo.UpdateLastSeen(r.Context(), host.ID); err != nil {
							slog.Warn("auth middleware: failed to update last_seen", "host_id", host.ID, "error", err)
						}
					}
				}()
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Exports have no host-counter transaction.
			if err := hostRepo.UpdateLastSeen(r.Context(), host.ID); err != nil {
				slog.Warn("auth middleware: failed to update last_seen", "host_id", host.ID, "error", err)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// NewRouter creates and configures the HTTP router with dependency injection.
func NewRouter(deps *RouterDeps, config *Config) chi.Router {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(constants.RequestTimeout))

	// CORS middleware
	r.Use(middleware.CORS)

	// Create handlers with injected dependencies
	h := deps.Handlers

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Public routes
		r.Post("/hosts", handlers.RegisterHostHandler(h))
		r.Get("/hosts", handlers.ListHostsHandler(h))
		r.Get("/hosts/{id}", handlers.GetHostHandler(h))
		r.Get("/hosts/{id}/stats", handlers.GetHostStatsHandler(h))
		r.Get("/logs", handlers.ListLogsHandler(h))
		r.Get("/logs/groups", handlers.ListGroupedLogsHandler(h))
		r.Get("/logs/{id}", handlers.GetLogHandler(h))
		r.Get("/events", handlers.SSEStreamHandler(h))
		r.Get("/stats/logs", handlers.LogBucketsHandler(h))
		r.Get("/stats/fingerprints", handlers.FingerprintBucketsHandler(h))
		r.Get("/inbox", handlers.ListInboxItemsHandler(h))
		r.Post("/inbox", handlers.CreateInboxItemHandler(h))
		r.Get("/inbox/{id}", handlers.GetInboxItemHandler(h))
		r.Patch("/inbox/{id}", handlers.UpdateInboxItemHandler(h))
		r.Delete("/inbox/{id}", handlers.DeleteInboxItemHandler(h))
		r.Post("/inbox/{id}/done", handlers.CompleteInboxItemHandler(h))

		// Authenticated routes
		r.With(apiKeyAuthMiddleware(deps.HostRepo, true)).Post("/logs", handlers.CreateLogHandler(h))
		r.With(apiKeyAuthMiddleware(deps.HostRepo, true)).Post("/logs/bulk", handlers.CreateBulkLogsHandler(h))

		// Export routes (authenticated)
		r.With(apiKeyAuthMiddleware(deps.HostRepo, false)).Get("/export/json", handlers.ExportLogsHandler(h, "json"))
		r.With(apiKeyAuthMiddleware(deps.HostRepo, false)).Get("/export/csv", handlers.ExportLogsHandler(h, "csv"))
		r.With(apiKeyAuthMiddleware(deps.HostRepo, false)).Get("/export/ndjson", handlers.ExportLogsHandler(h, "ndjson"))
	})

	// Health check
	healthHandler := handlers.NewHealthHandler(deps.DB, deps.Hub)
	r.Get("/health", healthHandler.ServeHTTP)

	webFS := configuredWebFS(config)
	if webFS != nil {
		r.Get("/*", staticFileHandler(webFS))
	} else {
		// API-only mode: show API info at root
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok","message":"GLog API","version":"` + constants.APIVersion + `"}`))
		})

		// 404 handler for API-only mode
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": "Endpoint not found",
				"path":  r.URL.Path,
			})
		})
	}

	// Method not allowed handler
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":  "Method not allowed",
			"method": r.Method,
			"path":   r.URL.Path,
		})
	})

	return r
}

func configuredWebFS(config *Config) fs.FS {
	if config == nil {
		return nil
	}
	if config.WebDir != "" {
		if info, err := os.Stat(config.WebDir); err == nil && info.IsDir() {
			return os.DirFS(config.WebDir)
		}
	}
	return config.WebFS
}

func staticFileHandler(webFS fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(webFS))
	return func(w http.ResponseWriter, req *http.Request) {
		name := strings.TrimPrefix(req.URL.Path, "/")
		if name == "" {
			name = dashboardIndex
		}
		if info, err := fs.Stat(webFS, name); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, req)
			return
		}
		if _, err := fs.Stat(webFS, dashboardIndex); err != nil {
			http.NotFound(w, req)
			return
		}

		fallback := req.Clone(req.Context())
		urlCopy := *req.URL
		urlCopy.Path = "/"
		fallback.URL = &urlCopy
		fileServer.ServeHTTP(w, fallback)
	}
}

// Config holds HTTP server configuration.
type Config struct {
	Host         string
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	WebDir       string // Path to SvelteKit build output (optional)
	WebFS        fs.FS  // Embedded dashboard; WebDir overrides it when present.
}

// DefaultConfig returns the default server configuration.
func DefaultConfig() *Config {
	return &Config{
		Host:         constants.DefaultHost,
		Port:         constants.DefaultPort,
		ReadTimeout:  constants.ReadTimeout,
		WriteTimeout: constants.WriteTimeout,
		IdleTimeout:  constants.IdleTimeout,
	}
}

// Server represents the HTTP server.
type Server struct {
	server *http.Server
	router chi.Router
	deps   *RouterDeps
	config *Config
}

// NewServer creates a new HTTP server with dependency injection.
func NewServer(deps *RouterDeps, config *Config) *Server {
	if config == nil {
		config = DefaultConfig()
	}

	router := NewRouter(deps, config)

	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)

	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		IdleTimeout:  config.IdleTimeout,
	}

	return &Server{
		server: server,
		router: router,
		deps:   deps,
		config: config,
	}
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	slog.Info("starting GLog server",
		"addr", s.server.Addr,
		"health", fmt.Sprintf("http://%s/health", s.server.Addr),
		"api", fmt.Sprintf("http://%s/api/v1", s.server.Addr),
	)
	if s.config.WebDir != "" || s.config.WebFS != nil {
		slog.Info("dashboard enabled", "url", fmt.Sprintf("http://%s/", s.server.Addr))
	}

	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("shutting down server")

	// Close SSE hub
	if s.deps.Hub != nil {
		s.deps.Hub.Close()
	}

	// Shutdown HTTP server
	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown error: %w", err)
	}

	slog.Info("server stopped")
	return nil
}

// Router returns the router instance.
func (s *Server) Router() chi.Router {
	return s.router
}

// DB returns the database instance.
func (s *Server) DB() *sqlite.Database {
	return s.deps.DB
}

// Addr returns the server address.
func (s *Server) Addr() string {
	return s.server.Addr
}
