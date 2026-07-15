package commands

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/dotcommander/glog/internal/constants"
	httpinfra "github.com/dotcommander/glog/internal/infrastructure/http"
	"github.com/dotcommander/glog/internal/infrastructure/http/handlers"
	"github.com/dotcommander/glog/internal/infrastructure/metrics"
	"github.com/dotcommander/glog/internal/infrastructure/persistence/sqlite"
	"github.com/dotcommander/glog/internal/infrastructure/sse"
)

type ServeCommand struct {
	Addr       string `default:":6016" help:"Server address to listen on."`
	DB         string `default:"glog.db" type:"path" help:"Database file path."`
	Web        string `type:"path" help:"Path to SvelteKit build output."`
	Durability string `default:"balanced" help:"SQLite durability profile: strict, balanced, or fast."`
}

func (c *ServeCommand) Run(ctx context.Context) error {
	if !constants.IsValidDurabilityProfile(c.Durability) {
		return fmt.Errorf("invalid durability profile %q: must be strict, balanced, or fast", c.Durability)
	}

	// Parse address to extract host and port
	host := constants.DefaultHost
	port := constants.DefaultPort
	if c.Addr != "" {
		h, portStr, err := net.SplitHostPort(c.Addr)
		if err != nil {
			return fmt.Errorf("invalid address %q: %w", c.Addr, err)
		}
		if h != "" {
			host = h
		}
		p, err := strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("invalid port in address %q: %w", c.Addr, err)
		}
		port = p
	}

	// Initialize database
	db, err := sqlite.NewWithOptions(c.DB, sqlite.Options{DurabilityProfile: c.Durability})
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer db.Close()

	// Run migrations from embedded SQL files
	if err := db.MigrateFS(sqlite.EmbeddedMigrations, "migrations"); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Create repositories
	hostRepo := sqlite.NewHostRepository(db)
	logRepo := sqlite.NewLogRepository(db)
	inboxRepo := sqlite.NewInboxRepository(db)

	// Create SSE hub
	hub := sse.NewHub()

	// Create handlers with dependency injection
	h := handlers.NewHandlers(hostRepo, logRepo, hub)
	h.SetInboxRepo(inboxRepo)
	recorder, err := metrics.NewRecorder(hostRepo, logRepo)
	if err != nil {
		slog.Warn("failed to initialize internal metrics", "error", err)
	} else {
		h.SetRecorder(recorder)
		recorder.Record("server.starting", map[string]any{"addr": c.Addr, "db": c.DB, "durability": c.Durability})
	}

	// Create router dependencies
	deps := &httpinfra.RouterDeps{
		DB:        db,
		HostRepo:  hostRepo,
		LogRepo:   logRepo,
		InboxRepo: inboxRepo,
		Hub:       hub,
		Handlers:  h,
	}

	// Create and start server
	config := &httpinfra.Config{
		Host:         host,
		Port:         port,
		ReadTimeout:  constants.ReadTimeout,
		WriteTimeout: constants.WriteTimeout,
		IdleTimeout:  constants.IdleTimeout,
		WebDir:       c.Web,
	}
	server := httpinfra.NewServer(deps, config)
	slog.Info("Starting GLog server", "host", host, "port", port)
	slog.Info("Database", "path", c.DB)
	if c.Web != "" {
		slog.Info("Frontend", "path", c.Web)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case err := <-errCh:
		return err
	case sig := <-sigCh:
		slog.Info("received shutdown signal", "signal", sig.String())
		if recorder != nil {
			recorder.Record("server.shutdown", map[string]any{"signal": sig.String()})
		}
		shutdownCtx, cancel := context.WithTimeout(ctx, constants.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-errCh
	}
}

type MigrateCommand struct {
	DB string `default:"glog.db" type:"path" help:"Database file path."`
}

func (c *MigrateCommand) Run() error {
	slog.Info("Running migrations", "database", c.DB)

	db, err := sqlite.New(c.DB)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	if err := db.MigrateFS(sqlite.EmbeddedMigrations, "migrations"); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	slog.Info("Migrations completed successfully")
	return nil
}
