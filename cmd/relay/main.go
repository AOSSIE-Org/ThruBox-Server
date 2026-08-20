package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AOSSIE-Org/ThruBox-Server/internal/config"
	"github.com/AOSSIE-Org/ThruBox-Server/internal/handler"
	"github.com/AOSSIE-Org/ThruBox-Server/internal/middleware"
	"github.com/AOSSIE-Org/ThruBox-Server/internal/store"
)

func main() {
	// Set up structured logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Load configuration
	cfg, err := config.Load("config.yaml")
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	slog.Info("configuration loaded",
		"port", cfg.Server.Port,
		"storage_driver", cfg.Storage.Driver,
		"storage_path", cfg.Storage.Path,
		"ttl_days", cfg.Messages.TTLDays,
		"rate_limit", cfg.Security.RateLimit,
		"allowed_origins", cfg.Security.AllowedOrigins,
	)

	// Initialize storage
	db, err := store.NewSQLiteStore(cfg.Storage.Path)
	if err != nil {
		slog.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	slog.Info("database initialized", "path", cfg.Storage.Path)

	// Start background purge goroutine (only if TTL is configured — skip in "forever" mode)
	purgeCtx, purgeCancel := context.WithCancel(context.Background())
	defer purgeCancel()
	if cfg.Messages.TTLDays > 0 {
		go purgeLoop(purgeCtx, db)
		slog.Info("auto-purge enabled", "ttl_days", cfg.Messages.TTLDays)
	} else {
		slog.Info("auto-purge disabled (ttl_days=0, messages stored forever until manually deleted)")
	}

	// Set up handlers
	msgHandler := &handler.MessageHandler{
		Store:          db,
		TTLDays:        cfg.Messages.TTLDays,
		MaxPayloadSize: cfg.Messages.MaxPayloadSize,
	}

	// Set up routes (Go 1.22+ pattern matching)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handler.HandleHealth)
	mux.HandleFunc("POST /api/messages", msgHandler.HandleCreate)
	mux.HandleFunc("GET /api/messages/{address}", msgHandler.HandleGetByAddress)
	mux.HandleFunc("DELETE /api/messages/{id}", msgHandler.HandleDelete)

	// Apply middleware chain: CORS → API Key → Rate Limiter → Router
	//
	// CORS is outermost on purpose. A browser preflight is an OPTIONS request
	// with no custom headers, so it carries no X-API-Key; if APIKeyAuth ran
	// first every preflight would 401 and the real request would never be
	// sent. CORS answers the preflight itself and lets everything else fall
	// through to authentication as normal.
	rateLimiter := middleware.NewRateLimiter(cfg.Security.RateLimit)
	defer rateLimiter.Stop()

	var h http.Handler = mux
	h = rateLimiter.Middleware(h)
	h = middleware.APIKeyAuth(cfg.Security.APIKey)(h)
	h = middleware.CORS(cfg.Security.AllowedOrigins, cfg.Security.APIKey != "")(h)

	if len(cfg.Security.AllowedOrigins) > 0 {
		slog.Info("CORS enabled", "allowed_origins", cfg.Security.AllowedOrigins)
	}

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.Addr(),
		Handler:      h,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		slog.Info("relay server starting", "addr", cfg.Addr())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("shutting down server", "signal", sig)

	// Give active connections 10 seconds to finish
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped gracefully")
}

// purgeLoop runs the expired message purge every hour.
func purgeLoop(ctx context.Context, s store.Store) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// Run once immediately on startup
	runPurge(ctx, s)

	for {
		select {
		case <-ticker.C:
			runPurge(ctx, s)
		case <-ctx.Done():
			slog.Info("purge loop stopped")
			return
		}
	}
}

// runPurge executes a single purge cycle.
func runPurge(ctx context.Context, s store.Store) {
	count, err := s.PurgeExpired(ctx)
	if err != nil {
		slog.Error("failed to purge expired messages", "error", err)
		return
	}
	if count > 0 {
		slog.Info("purged expired messages", "count", count)
	}
}
