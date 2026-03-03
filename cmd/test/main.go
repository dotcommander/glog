package main

import (
	"fmt"
	"log"

	"github.com/dotcommander/glog/internal/infrastructure/http"
	"github.com/dotcommander/glog/internal/infrastructure/http/handlers"
	"github.com/dotcommander/glog/internal/infrastructure/persistence/sqlite"
	"github.com/dotcommander/glog/internal/infrastructure/sse"
)

func main() {
	// Open database
	dbPath := "./test.db"
	fmt.Printf("Opening database at %s\n", dbPath)
	db, err := sqlite.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Run migrations from embedded SQL files
	fmt.Println("Running database migrations...")
	if err := db.MigrateFS(sqlite.EmbeddedMigrations, "migrations"); err != nil {
		db.Close()
		log.Fatalf("Failed to run migrations: %v", err)
	}
	fmt.Println("Migrations completed")

	// Create repositories
	hostRepo := sqlite.NewHostRepository(db)
	logRepo := sqlite.NewLogRepository(db)

	// Create SSE hub
	hub := sse.NewHub()

	// Create handlers with dependency injection
	h := handlers.NewHandlers(hostRepo, logRepo, hub)

	// Create router dependencies
	deps := &http.RouterDeps{
		DB:       db,
		HostRepo: hostRepo,
		LogRepo:  logRepo,
		Hub:      hub,
		Handlers: h,
	}

	// Create HTTP server
	config := &http.Config{
		Host: "localhost",
		Port: 6016,
	}
	server := http.NewServer(deps, config)

	// Test endpoints
	fmt.Println("\nTesting endpoints:")
	fmt.Println("==================")

	// Note: Server needs to be running to test endpoints
	// These tests would need to be run in separate goroutines
	// For now, just start the server

	fmt.Println("\nGLog server running on localhost:6016")
	fmt.Println("Press Ctrl+C to stop")

	// Start server
	log.Fatal(server.Start())
}
