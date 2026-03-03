package commands

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"

	"github.com/dotcommander/glog/internal/constants"
	httpinfra "github.com/dotcommander/glog/internal/infrastructure/http"
	"github.com/dotcommander/glog/internal/infrastructure/http/handlers"
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
			db, err := sqlite.New(dbPath)
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

			// Create SSE hub
			hub := sse.NewHub()

			// Create handlers with dependency injection
			h := handlers.NewHandlers(hostRepo, logRepo, hub)

			// Create router dependencies
			deps := &httpinfra.RouterDeps{
				DB:       db,
				HostRepo: hostRepo,
				LogRepo:  logRepo,
				Hub:      hub,
				Handlers: h,
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

			return server.Start()
		},
	}

	cmd.Flags().String("addr", ":6016", "Server address to listen on")
	cmd.Flags().String("db", "glog.db", "Database file path")
	cmd.Flags().String("web", "", "Path to SvelteKit build output (e.g., web/build)")

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
