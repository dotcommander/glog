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
	"github.com/spf13/cobra"
)

// ServeCmd creates the serve command.
func ServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the GLog server",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, _ := cmd.Flags().GetString("db")
			addr, _ := cmd.Flags().GetString("addr")
			webDir, _ := cmd.Flags().GetString("web")
			durability, _ := cmd.Flags().GetString("durability")
			if !constants.IsValidDurabilityProfile(durability) {
				return fmt.Errorf("invalid durability profile %q: must be strict, balanced, or fast", durability)
			}

			// Parse address to extract host and port
			host := constants.DefaultHost
			port := constants.DefaultPort
			if addr != "" {
				h, portStr, err := net.SplitHostPort(addr)
				if err != nil {
					return fmt.Errorf("invalid address %q: %w", addr, err)
				}
				if h != "" {
					host = h
				}
				p, err := strconv.Atoi(portStr)
				if err != nil {
					return fmt.Errorf("invalid port in address %q: %w", addr, err)
				}
				port = p
			}

			// Initialize database
			db, err := sqlite.NewWithOptions(dbPath, sqlite.Options{DurabilityProfile: durability})
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
				recorder.Record("server.starting", map[string]any{"addr": addr, "db": dbPath, "durability": durability})
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
				WebDir:       webDir,
			}
			server := httpinfra.NewServer(deps, config)
			slog.Info("Starting GLog server", "host", host, "port", port)
			slog.Info("Database", "path", dbPath)
			if webDir != "" {
				slog.Info("Frontend", "path", webDir)
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
				ctx, cancel := context.WithTimeout(context.Background(), constants.ShutdownTimeout)
				defer cancel()
				if err := server.Shutdown(ctx); err != nil {
					return err
				}
				return <-errCh
			}
		},
	}

	cmd.Flags().String("addr", ":6016", "Server address to listen on")
	cmd.Flags().String("db", "glog.db", "Database file path")
	cmd.Flags().String("web", "", "Path to SvelteKit build output (e.g., web/build)")
	cmd.Flags().String("durability", constants.DurabilityBalanced, "SQLite durability profile (strict, balanced, fast)")

	return cmd
}

// MigrateCmd creates the migrate command.
func MigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, _ := cmd.Flags().GetString("db")
			slog.Info("Running migrations", "database", dbPath)

			db, err := sqlite.New(dbPath)
			if err != nil {
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer db.Close()

			if err := db.MigrateFS(sqlite.EmbeddedMigrations, "migrations"); err != nil {
				return fmt.Errorf("failed to run migrations: %w", err)
			}

			slog.Info("Migrations completed successfully")
			return nil
		},
	}

	cmd.Flags().String("db", "glog.db", "Database file path")

	return cmd
}
