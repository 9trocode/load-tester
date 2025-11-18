package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

var logger *slog.Logger

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const requestIDKey contextKey = "request_id"

func main() {
	// Initialize structured logging
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	logger.Info("Starting PipeOps Load Tester", "version", "1.0.0")

	// Initialize database
	db, err := InitDB()
	if err != nil {
		logger.Error("Failed to initialize database", "error", err)
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("Error closing database", "error", err)
		}
	}()

	// Create test manager
	testManager := NewTestManager(db)

	// Setup routes with request ID middleware
	http.HandleFunc("/", requestIDMiddleware(serveIndex))
	http.HandleFunc("/test/", requestIDMiddleware(serveIndex)) // Serve index.html for /test/{uuid}
	http.HandleFunc("/api/start", requestIDMiddleware(testManager.HandleStartTest))
	http.HandleFunc("/api/status/", requestIDMiddleware(testManager.HandleGetStatus))
	http.HandleFunc("/api/metrics/", requestIDMiddleware(testManager.HandleGetMetrics))
	http.HandleFunc("/api/timeseries/", requestIDMiddleware(testManager.HandleGetTimeSeries))
	http.HandleFunc("/api/history", requestIDMiddleware(testManager.HandleGetHistory))
	http.HandleFunc("/api/running", requestIDMiddleware(testManager.HandleGetRunningTests))
	http.HandleFunc("/api/historical-metrics/", requestIDMiddleware(testManager.HandleGetHistoricalMetrics))
	http.HandleFunc("/api/stop/", requestIDMiddleware(testManager.HandleStopTest))
	http.HandleFunc("/api/report/", requestIDMiddleware(testManager.HandleGenerateReport))
	http.HandleFunc("/api/ip-stats", requestIDMiddleware(testManager.HandleGetIPStats))

	// Serve static files with no-cache headers
	http.Handle("/static/", noCacheMiddleware(http.StripPrefix("/static/", http.FileServer(http.Dir("static")))))

	// Get port from environment variable or default to 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Add colon if not present
	if port[0] != ':' {
		port = ":" + port
	}

	// Create HTTP server
	server := &http.Server{
		Addr:         port,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Channel to listen for shutdown signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	// Start server in a goroutine
	go func() {
		logger.Info("Server starting", "port", port, "address", fmt.Sprintf("http://localhost%s", port))
		fmt.Printf("PipeOps Load Tester running on http://localhost%s\n", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server failed to start", "error", err)
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-shutdown
	logger.Info("Shutdown signal received, initiating graceful shutdown")

	// Stop all active tests
	testManager.Shutdown()

	// Create a deadline for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
	}

	logger.Info("Server exited gracefully")
}

// requestIDMiddleware adds a unique request ID to each request
func requestIDMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if request ID exists in header, otherwise generate one
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Add request ID to response header
		w.Header().Set("X-Request-ID", requestID)

		// Add request ID to context
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		r = r.WithContext(ctx)

		// Log request with ID
		slog.InfoContext(ctx, "Incoming request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"request_id", requestID,
		)

		// Call next handler
		next(w, r)
	}
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	// Add no-cache headers for index.html
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	http.ServeFile(w, r, "static/index.html")
}

// noCacheMiddleware adds no-cache headers to prevent browser caching
func noCacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}
