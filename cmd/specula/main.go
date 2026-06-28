package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/grunclepug/specula/internal/config"
	"github.com/grunclepug/specula/internal/db"
	shttp "github.com/grunclepug/specula/internal/http"
	"github.com/grunclepug/specula/internal/ping"
	"github.com/grunclepug/specula/internal/provider"
)

// Version is injected at compilation time using ldflags from the Makefile.
var Version = "dev"

func main() {
	// 1. Define command-line flags
	influxFlag := flag.Bool("influx", false, "Enable InfluxDB remote metrics shipping")
	targetsFlag := flag.String("targets", "", "Comma-separated list of targets to monitor (seeds SQLite DB if empty)")
	addrFlag := flag.String("addr", "", "HTTP server bind address (overrides SPECULA_ADDR env)")
	dbPathFlag := flag.String("db", "", "SQLite database file path (overrides SPECULA_DB_PATH env)")
	jsonLogFlag := flag.Bool("json-log", false, "Enable structured JSON logging (overrides SPECULA_JSON_LOG env)")
	versionFlag := flag.Bool("version", false, "Print application version and exit")

	flag.Parse()

	if *versionFlag {
		fmt.Printf("Specula Ping Monitor v%s\n", Version)
		os.Exit(0)
	}

	// 2. Load configuration from environment and optional .env file
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[Main] Configuration loading failed: %v", err)
	}

	// Override config defaults with flags if specified
	if *targetsFlag != "" {
		var targets []string
		for _, t := range strings.Split(*targetsFlag, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				targets = append(targets, t)
			}
		}
		cfg.Targets = targets
	}
	if *addrFlag != "" {
		cfg.Addr = *addrFlag
	}
	if *dbPathFlag != "" {
		cfg.DBPath = *dbPathFlag
	}

	// 3. Configure Structured Logging
	var handler slog.Handler
	if *jsonLogFlag || cfg.JSONLog {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	}
	slog.SetDefault(slog.New(handler))

	slog.Info("Initializing Specula Ping Monitor...", "version", Version)

	// 4. Initialize SQLite Database
	database, err := db.New(cfg.DBPath)
	if err != nil {
		slog.Error("Database initialization failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		slog.Info("Closing SQLite database...")
		_ = database.Close()
	}()

	// Seed default targets into the database if the database is currently empty
	if len(cfg.Targets) > 0 {
		if err := database.SeedDefaultHosts(cfg.Targets); err != nil {
			slog.Warn("Failed to seed default hosts", "error", err)
		}
	}

	// Check if we have targets present
	existingHosts, err := database.GetHosts()
	if err != nil {
		slog.Error("Failed to query database targets", "error", err)
		os.Exit(1)
	}
	if len(existingHosts) == 0 {
		slog.Warn("No targets currently saved in SQLite database. Add targets using the dashboard UI.")
	}

	// 5. Select storage provider and inject dependencies
	var prov provider.MetricProvider
	if *influxFlag {
		slog.Info("Storage Backend: InfluxDB Line Protocol")
		if cfg.InfluxURL == "" || cfg.InfluxToken == "" || cfg.InfluxOrg == "" || cfg.InfluxBucket == "" {
			slog.Error("InfluxDB is enabled but credentials are missing in configuration. Ensure INFLUX_URL, INFLUX_TOKEN, INFLUX_ORG, and INFLUX_BUCKET are configured.")
			os.Exit(1)
		}
		influxProv, err := provider.NewInfluxProvider(database, cfg.InfluxURL, cfg.InfluxToken, cfg.InfluxOrg, cfg.InfluxBucket)
		if err != nil {
			slog.Error("Failed to initialize InfluxDB provider", "error", err)
			os.Exit(1)
		}
		prov = influxProv
	} else {
		slog.Info("Storage Backend: In-Memory JSON API")
		prov = provider.NewJSONAPIProvider(database)
	}

	// Create background monitor orchestration
	monitor := ping.NewMonitor(database, prov, cfg.PingInterval, cfg.PingTimeout, cfg.PingCount)

	// Setup context for background routines shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start background status checks
	go monitor.Start(ctx)

	// Setup HTTP server handlers and router
	handlerHTTP, err := shttp.NewHandler(database, prov, Version)
	if err != nil {
		slog.Error("Failed to initialize HTTP handler", "error", err)
		os.Exit(1)
	}
	router := shttp.NewRouter(handlerHTTP)

	server := &http.Server{
		Addr:    cfg.Addr,
		Handler: router,
	}

	// Listen for termination signals to initiate graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("HTTP server listening", "addr", cfg.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	sig := <-sigChan
	slog.Info("Received shutdown signal", "signal", sig)

	// Stop background monitor ticker loop
	cancel()

	// Shutdown HTTP Server gracefully with 5 seconds timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP Server Shutdown Error", "error", err)
	}

	slog.Info("Specula Ping Monitor shutdown complete.")
}
