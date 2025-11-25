// Command server runs the ICMP-Mon control plane.
//
// # Usage
//
//	server --database postgres://localhost/icmpmon --port 8080
//
// # Configuration
//
// The server can be configured via:
// - Command-line flags
// - Environment variables (ICMPMON_*)
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pilot-net/icmp-mon/control-plane/internal/api"
	"github.com/pilot-net/icmp-mon/control-plane/internal/service"
	"github.com/pilot-net/icmp-mon/control-plane/internal/store"
)

func main() {
	var (
		port     = flag.Int("port", 8080, "HTTP server port")
		dbURL    = flag.String("database", "", "Database URL (postgres://...)")
		debug    = flag.Bool("debug", false, "Enable debug logging")
		version  = flag.Bool("version", false, "Print version and exit")
	)
	flag.Parse()

	if *version {
		fmt.Println("icmpmon-server v0.1.0")
		os.Exit(0)
	}

	// Set up logging
	logLevel := slog.LevelInfo
	if *debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Get database URL from env if not provided
	if *dbURL == "" {
		*dbURL = os.Getenv("ICMPMON_DATABASE_URL")
	}
	if *dbURL == "" {
		*dbURL = "postgres://localhost:5432/icmpmon?sslmode=disable"
	}

	// Connect to database
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := store.NewStoreFromURL(ctx, *dbURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Verify database connection
	if err := db.Ping(ctx); err != nil {
		logger.Error("database ping failed", "error", err)
		os.Exit(1)
	}
	logger.Info("connected to database")

	// Create service and API
	svc := service.NewService(db, logger)
	apiServer := api.NewServer(svc, logger)

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		Handler:      apiServer,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server
	go func() {
		logger.Info("starting server", "port", *port)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
	}

	logger.Info("shutdown complete")
}
