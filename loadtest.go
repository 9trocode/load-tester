package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// parseJSON is a helper function to parse JSON from request body
func parseJSON(r *http.Request, v interface{}) error {
	decoder := json.NewDecoder(r.Body)
	return decoder.Decode(v)
}

type TestManager struct {
	db          *sql.DB
	activeTests map[string]*TestContext // UUID -> TestContext
	mu          sync.RWMutex
	// Rate limiting: track last test start time per IP (simple approach)
	// For production, consider using a proper rate limiter library
	lastTestStarts map[string]time.Time
	rateLimitMu    sync.Mutex
	// Track active tests per IP for abuse prevention
	testsPerIP   map[string]map[string]bool // IP -> Set of test UUIDs
	testsPerIPMu sync.Mutex
}

type TestContext struct {
	TestRun    *TestRun
	Context    context.Context
	Cancel     context.CancelFunc
	Metrics    *MetricsCollector
	IsRunning  *atomic.Bool
	AuthConfig *AuthConfig
	Method     string
	Body       string
	Headers    map[string]string
}

type AuthConfig struct {
	Type        string            `json:"type"`         // "jwt", "basic", "header"
	Token       string            `json:"token"`        // For JWT
	Username    string            `json:"username"`     // For Basic Auth
	Password    string            `json:"password"`     // For Basic Auth
	HeaderName  string            `json:"header_name"`  // For custom header
	HeaderValue string            `json:"header_value"` // For custom header
	Headers     map[string]string `json:"headers"`      // For multiple custom headers
}

type MetricsCollector struct {
	TotalRequests int64
	SuccessCount  int64
	ErrorCount    int64
	Latencies     []float64
	TimeSeries    []TimeSeriesPoint
	mu            sync.RWMutex
	StartTime     time.Time
}

type TimeSeriesPoint struct {
	Timestamp   time.Time `json:"timestamp"`
	Requests    int64     `json:"requests"`
	RPS         float64   `json:"rps"`
	AvgLatency  float64   `json:"avg_latency"`
	SuccessRate float64   `json:"success_rate"`
}

func NewTestManager(db *sql.DB) *TestManager {
	tm := &TestManager{
		db:             db,
		activeTests:    make(map[string]*TestContext),
		lastTestStarts: make(map[string]time.Time),
		testsPerIP:     make(map[string]map[string]bool),
	}

	// Start periodic cleanup goroutine for rate limit map
	go tm.cleanupRateLimitMap()

	return tm
}

// cleanupRateLimitMap periodically removes old entries from the rate limit map
func (tm *TestManager) cleanupRateLimitMap() {
	ticker := time.NewTicker(10 * time.Minute) // Run cleanup every 10 minutes
	defer ticker.Stop()

	for range ticker.C {
		tm.rateLimitMu.Lock()
		now := time.Now()
		for ip, lastStart := range tm.lastTestStarts {
			// Remove entries older than 1 hour
			if now.Sub(lastStart) > time.Hour {
				delete(tm.lastTestStarts, ip)
			}
		}
		tm.rateLimitMu.Unlock()
	}
}

// Shutdown gracefully stops all active tests
func (tm *TestManager) Shutdown() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	slog.Info("Shutting down active tests", "count", len(tm.activeTests))

	for testUUID, testCtx := range tm.activeTests {
		slog.Info("Cancelling test", "test_uuid", testUUID)
		testCtx.Cancel()
	}

	slog.Info("All active tests cancelled")
}

const (
	MaxUsers           = 1000  // Maximum concurrent users per test
	MaxDuration        = 300   // Maximum duration in seconds (5 minutes)
	MaxRampUpSec       = 300   // Maximum ramp-up time in seconds
	MinUsers           = 1     // Minimum users
	MinDuration        = 1     // Minimum duration in seconds
	MinRampUpSec       = 0     // Minimum ramp-up time in seconds (0 = start all users immediately)
	MaxConcurrentTests = 50    // Maximum concurrent active tests (prevents resource exhaustion)
	MaxTestsPerIP      = 3     // Maximum concurrent tests per IP address (prevents abuse)
	MaxLatencySamples  = 10000 // Maximum latency samples to keep in memory per test
	RateLimitSeconds   = 5     // Minimum seconds between test starts per IP
)

func (tm *TestManager) HandleStartTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Host                  string            `json:"host"`
		MaskHost              bool              `json:"mask_host"`
		Users                 int               `json:"users"`
		RampUpSec             int               `json:"ramp_up_sec"`
		Duration              int               `json:"duration"`
		Auth                  *AuthConfig       `json:"auth,omitempty"`
		Method                string            `json:"method,omitempty"`                  // HTTP method (GET, POST, PUT, DELETE, etc.)
		Body                  string            `json:"body,omitempty"`                    // Request body payload
		Headers               map[string]string `json:"headers,omitempty"`                 // Custom headers
		MaxConcurrentRequests int               `json:"max_concurrent_requests,omitempty"` // Max concurrent requests per user (default: 10)
		ErrorThreshold        float64           `json:"error_threshold,omitempty"`         // Error rate % to trigger circuit breaker (default: 0 = disabled)
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate host
	if req.Host == "" {
		http.Error(w, "Host is required", http.StatusBadRequest)
		return
	}

	// Validate host for security (SSRF prevention)
	if err := validateHost(req.Host); err != nil {
		http.Error(w, fmt.Sprintf("Invalid host: %v", err), http.StatusBadRequest)
		return
	}

	// Validate and enforce limits
	if req.Users < MinUsers || req.Users > MaxUsers {
		http.Error(w, fmt.Sprintf("Users must be between %d and %d", MinUsers, MaxUsers), http.StatusBadRequest)
		return
	}

	if req.RampUpSec < MinRampUpSec || req.RampUpSec > MaxRampUpSec {
		http.Error(w, fmt.Sprintf("Ramp-up time must be between %d and %d seconds", MinRampUpSec, MaxRampUpSec), http.StatusBadRequest)
		return
	}

	if req.Duration < MinDuration || req.Duration > MaxDuration {
		http.Error(w, fmt.Sprintf("Duration must be between %d and %d seconds", MinDuration, MaxDuration), http.StatusBadRequest)
		return
	}

	// Additional safety check: ramp-up should not exceed duration
	if req.RampUpSec > req.Duration {
		http.Error(w, "Ramp-up time cannot exceed test duration", http.StatusBadRequest)
		return
	}

	// Validate HTTP method (default to GET if not specified)
	if req.Method == "" {
		req.Method = "GET"
	}
	req.Method = strings.ToUpper(req.Method)
	validMethods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}
	methodValid := false
	for _, m := range validMethods {
		if req.Method == m {
			methodValid = true
			break
		}
	}
	if !methodValid {
		http.Error(w, fmt.Sprintf("Invalid HTTP method. Allowed: %v", validMethods), http.StatusBadRequest)
		return
	}

	// Validate body is only present for appropriate methods
	if req.Body != "" && (req.Method == "GET" || req.Method == "HEAD") {
		http.Error(w, "Request body not allowed for GET or HEAD methods", http.StatusBadRequest)
		return
	}

	// Check concurrent test limit
	tm.mu.RLock()
	activeTestCount := len(tm.activeTests)
	tm.mu.RUnlock()

	if activeTestCount >= MaxConcurrentTests {
		http.Error(w, fmt.Sprintf("Maximum concurrent tests limit reached (%d). Please wait for a test to complete.", MaxConcurrentTests), http.StatusServiceUnavailable)
		return
	}

	// Simple rate limiting: prevent starting tests too frequently from same IP
	clientIP := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		// Take first IP if multiple (comma-separated)
		clientIP = strings.Split(forwarded, ",")[0]
		clientIP = strings.TrimSpace(clientIP)
	}

	// Check concurrent tests per IP limit
	tm.testsPerIPMu.Lock()
	if testsForIP, exists := tm.testsPerIP[clientIP]; exists {
		if len(testsForIP) >= MaxTestsPerIP {
			tm.testsPerIPMu.Unlock()
			slog.Warn("IP exceeded concurrent test limit",
				"client_ip", clientIP,
				"active_tests", len(testsForIP),
				"max_allowed", MaxTestsPerIP)
			http.Error(w, fmt.Sprintf("Maximum concurrent tests per IP limit reached (%d). Please wait for a test to complete.", MaxTestsPerIP), http.StatusTooManyRequests)
			return
		}
	}
	tm.testsPerIPMu.Unlock()

	tm.rateLimitMu.Lock()
	lastStart, exists := tm.lastTestStarts[clientIP]
	now := time.Now()
	if exists && now.Sub(lastStart) < RateLimitSeconds*time.Second {
		tm.rateLimitMu.Unlock()
		http.Error(w, fmt.Sprintf("Rate limit: Please wait %d seconds between test starts.", RateLimitSeconds), http.StatusTooManyRequests)
		return
	}
	tm.lastTestStarts[clientIP] = now
	tm.rateLimitMu.Unlock()

	// Set defaults for optional fields
	maxConcurrentRequests := req.MaxConcurrentRequests
	if maxConcurrentRequests <= 0 {
		maxConcurrentRequests = 10 // Default: 10 requests per second per user
	}
	if maxConcurrentRequests > 100 {
		maxConcurrentRequests = 100 // Cap at 100 to prevent abuse
	}

	errorThreshold := req.ErrorThreshold
	if errorThreshold < 0 {
		errorThreshold = 0 // Disabled by default
	}
	if errorThreshold > 100 {
		errorThreshold = 100 // Cap at 100%
	}

	// Generate UUID for this test
	testUUID := uuid.New().String()

	// Create test run
	testRun := &TestRun{
		UUID:                  testUUID,
		Host:                  req.Host,
		MaskHost:		req.MaskHost,
		TotalUsers:            req.Users,
		RampUpSec:             req.RampUpSec,
		Duration:              req.Duration,
		Status:                "running",
		StartedAt:             time.Now(),
		Method:                req.Method,
		Body:                  req.Body,
		Headers:               req.Headers,
		MaxConcurrentRequests: maxConcurrentRequests,
		ErrorThreshold:        errorThreshold,
	}

	testRunID, err := SaveTestRun(tm.db, testRun)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to save test run: %v", err), http.StatusInternalServerError)
		return
	}

	testRun.ID = testRunID

	// Create test context
	ctx, cancel := context.WithCancel(context.Background())
	metrics := &MetricsCollector{
		StartTime:  time.Now(),
		Latencies:  make([]float64, 0),
		TimeSeries: make([]TimeSeriesPoint, 0),
	}
	isRunning := &atomic.Bool{}
	isRunning.Store(true)

	// Start time-series collection
	go metrics.collectTimeSeries(ctx)

	testCtx := &TestContext{
		TestRun:    testRun,
		Context:    ctx,
		Cancel:     cancel,
		Metrics:    metrics,
		IsRunning:  isRunning,
		AuthConfig: req.Auth,
		Method:     req.Method,
		Body:       req.Body,
		Headers:    req.Headers,
	}

	tm.mu.Lock()
	tm.activeTests[testUUID] = testCtx
	tm.mu.Unlock()

	// Track test for this IP
	tm.testsPerIPMu.Lock()
	if tm.testsPerIP[clientIP] == nil {
		tm.testsPerIP[clientIP] = make(map[string]bool)
	}
	tm.testsPerIP[clientIP][testUUID] = true
	tm.testsPerIPMu.Unlock()

	slog.Info("Test started",
		"test_uuid", testUUID,
		"client_ip", clientIP,
		"ip_active_tests", len(tm.testsPerIP[clientIP]))

	// Start load test
	go tm.runLoadTest(testCtx, clientIP)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"test_id":   testRunID,
		"test_uuid": testUUID,
		"status":    "started",
	})
}

func (tm *TestManager) runLoadTest(testCtx *TestContext, clientIP string) {
	defer func() {
		// Calculate final metrics before cleanup
		tm.calculateAndSaveMetrics(testCtx)

		testCtx.IsRunning.Store(false)
		testUUID := testCtx.TestRun.UUID

		// Log if test was stopped by circuit breaker
		if testCtx.TestRun.StoppedByCircuit {
			slog.Warn("Test stopped by circuit breaker",
				"test_uuid", testUUID,
				"error_threshold", testCtx.TestRun.ErrorThreshold,
				"error_count", testCtx.Metrics.ErrorCount,
				"total_requests", testCtx.Metrics.TotalRequests)
		}

		// Remove from active tests
		tm.mu.Lock()
		delete(tm.activeTests, testUUID)
		tm.mu.Unlock()

		// Remove from IP tracking
		tm.testsPerIPMu.Lock()
		if testsForIP, exists := tm.testsPerIP[clientIP]; exists {
			delete(testsForIP, testUUID)
			// Clean up empty IP entries
			if len(testsForIP) == 0 {
				delete(tm.testsPerIP, clientIP)
			}
		}
		tm.testsPerIPMu.Unlock()

		slog.Info("Test completed and cleaned up",
			"test_uuid", testUUID,
			"client_ip", clientIP)
	}()

	ctx := testCtx.Context
	testRun := testCtx.TestRun
	metrics := testCtx.Metrics
	authConfig := testCtx.AuthConfig
	duration := time.Duration(testRun.Duration) * time.Second

	// Calculate ramp-up rate
	usersPerSecond := float64(testRun.TotalUsers) / float64(testRun.RampUpSec)

	var wg sync.WaitGroup
	stopChan := make(chan struct{})
	rampUpStart := time.Now()

	// Start users gradually during ramp-up phase
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond) // Check every 100ms
		defer ticker.Stop()

		usersStarted := 0
		for usersStarted < testRun.TotalUsers {
			select {
			case <-ctx.Done():
				return
			case <-stopChan:
				return
			case <-ticker.C:
				elapsed := time.Since(rampUpStart).Seconds()
				if elapsed >= float64(testRun.RampUpSec) {
					// Ramp-up complete, start remaining users immediately
					for usersStarted < testRun.TotalUsers {
						select {
						case <-ctx.Done():
							return
						default:
							wg.Add(1)
							go tm.runUser(ctx, testRun.ID, testRun.Host, metrics, &wg, stopChan, authConfig, testRun.Method, testRun.Body, testRun.Headers, testRun.MaxConcurrentRequests)
							usersStarted++
						}
					}
					return
				}

				// Calculate target users at this point
				targetUsers := int(elapsed * usersPerSecond)
				if targetUsers > usersStarted {
					usersToAdd := targetUsers - usersStarted
					for i := 0; i < usersToAdd && usersStarted < testRun.TotalUsers; i++ {
						select {
						case <-ctx.Done():
							return
						default:
							wg.Add(1)
							go tm.runUser(ctx, testRun.ID, testRun.Host, metrics, &wg, stopChan, authConfig, testRun.Method, testRun.Body, testRun.Headers, testRun.MaxConcurrentRequests)
							usersStarted++
						}
					}
				}
			}
		}
	}()

	// Circuit breaker monitoring goroutine
	circuitBreakerTicker := time.NewTicker(2 * time.Second) // Check every 2 seconds
	defer circuitBreakerTicker.Stop()

	go func() {
		if testRun.ErrorThreshold <= 0 {
			return // Circuit breaker disabled
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-circuitBreakerTicker.C:
				metrics.mu.RLock()
				totalReqs := metrics.TotalRequests
				errorCount := metrics.ErrorCount
				metrics.mu.RUnlock()

				// Only check after we have at least 10 requests
				if totalReqs >= 10 {
					errorRate := (float64(errorCount) / float64(totalReqs)) * 100

					if errorRate >= testRun.ErrorThreshold {
						slog.Warn("Circuit breaker triggered",
							"test_uuid", testRun.UUID,
							"error_rate", errorRate,
							"threshold", testRun.ErrorThreshold,
							"total_requests", totalReqs,
							"errors", errorCount)

						testRun.StoppedByCircuit = true
						testCtx.Cancel() // Stop the test
						return
					}
				}
			}
		}
	}()

	// Wait for duration or cancellation
	select {
	case <-ctx.Done():
		// Stop all users
		close(stopChan)
		wg.Wait()
		return
	case <-time.After(duration):
		// Test duration completed
		close(stopChan)
		wg.Wait()
	}
}

func (tm *TestManager) runUser(ctx context.Context, testRunID int64, host string, metrics *MetricsCollector, wg *sync.WaitGroup, stopChan <-chan struct{}, authConfig *AuthConfig, method string, body string, headers map[string]string, maxConcurrentRequests int) {
	defer wg.Done()

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Normalize host to a valid URL
	targetURL := normalizeHost(host)

	// Calculate ticker interval based on max concurrent requests per second
	// maxConcurrentRequests requests per second = 1000ms / maxConcurrentRequests per interval
	tickerInterval := time.Duration(1000/maxConcurrentRequests) * time.Millisecond
	ticker := time.NewTicker(tickerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-stopChan:
			return
		case <-ticker.C:
			start := time.Now()

			// Create request with custom method, body, and context
			var bodyReader io.Reader
			if body != "" {
				bodyReader = strings.NewReader(body)
			}

			requestMethod := method
			if requestMethod == "" {
				requestMethod = "GET"
			}

			req, err := http.NewRequestWithContext(ctx, requestMethod, targetURL, bodyReader)
			if err != nil {
				metrics.Record(time.Since(start).Seconds()*1000, false, 0)
				continue
			}

			// Apply custom headers
			for key, value := range headers {
				req.Header.Set(key, value)
			}

			// Set Content-Type for POST/PUT/PATCH if body exists and not already set
			if body != "" && (requestMethod == "POST" || requestMethod == "PUT" || requestMethod == "PATCH") {
				if req.Header.Get("Content-Type") == "" {
					req.Header.Set("Content-Type", "application/json")
				}
			}

			// Apply authentication
			applyAuth(req, authConfig)

			resp, err := client.Do(req)
			completedAt := time.Now()
			latency := completedAt.Sub(start).Seconds() * 1000 // Convert to milliseconds

			success := err == nil && resp != nil && resp.StatusCode < 400
			statusCode := 0
			if resp != nil {
				statusCode = resp.StatusCode
				if _, err := io.Copy(io.Discard, resp.Body); err != nil {
					slog.Warn("Error reading response body", "error", err, "url", targetURL)
				}
				if err := resp.Body.Close(); err != nil {
					slog.Warn("Error closing response body", "error", err, "url", targetURL)
				}
			}

			metrics.Record(latency, success, statusCode)

			metric := &RequestMetric{
				TestRunID:  testRunID,
				Timestamp:  completedAt,
				Latency:    latency,
				Success:    success,
				StatusCode: statusCode,
			}
			if err := SaveRequestMetric(tm.db, metric); err != nil {
				slog.Error("Failed to save request metric", "error", err, "test_id", testRunID)
			}
		}
	}
}

// applyAuth applies authentication to the HTTP request based on auth config
func applyAuth(req *http.Request, authConfig *AuthConfig) {
	if authConfig == nil {
		return
	}

	switch authConfig.Type {
	case "jwt":
		if authConfig.Token != "" {
			req.Header.Set("Authorization", "Bearer "+authConfig.Token)
		}
	case "basic":
		if authConfig.Username != "" && authConfig.Password != "" {
			auth := authConfig.Username + ":" + authConfig.Password
			encoded := base64.StdEncoding.EncodeToString([]byte(auth))
			req.Header.Set("Authorization", "Basic "+encoded)
		}
	case "header":
		if authConfig.HeaderName != "" && authConfig.HeaderValue != "" {
			req.Header.Set(authConfig.HeaderName, authConfig.HeaderValue)
		}
		// Also apply any additional headers
		if authConfig.Headers != nil {
			for key, value := range authConfig.Headers {
				req.Header.Set(key, value)
			}
		}
	}
}

// normalizeHost converts various host formats to a valid HTTP URL
// validateHost validates the host input to prevent SSRF and other security issues
func validateHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("host cannot be empty")
	}

	// Try parsing as URL first
	var urlToCheck string
	if strings.Contains(host, "://") {
		urlToCheck = host
	} else {
		// Add scheme for parsing
		urlToCheck = "http://" + host
	}

	parsedURL, err := url.Parse(urlToCheck)
	if err != nil {
		return fmt.Errorf("invalid host format: %v", err)
	}

	// Block dangerous schemes
	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "" && scheme != "http" && scheme != "https" {
		return fmt.Errorf("only HTTP and HTTPS schemes are allowed, got: %s", scheme)
	}

	// Extract hostname for validation
	hostname := parsedURL.Hostname()
	if hostname == "" {
		// For cases like "192.168.1.1:8080" without scheme
		parts := strings.Split(host, ":")
		hostname = parts[0]
	}

	// Block localhost and loopback
	if hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" {
		return fmt.Errorf("localhost and loopback addresses are not allowed")
	}

	// Block private IP ranges
	if isLocalIP(hostname) {
		return fmt.Errorf("private IP addresses are not allowed")
	}

	// Block metadata services (cloud providers)
	metadataHosts := []string{
		"169.254.169.254", // AWS, Azure, GCP metadata
		"metadata.google.internal",
		"169.254.169.123", // Oracle Cloud
		"100.100.100.200", // Alibaba Cloud
	}
	for _, meta := range metadataHosts {
		if hostname == meta {
			return fmt.Errorf("metadata service addresses are not allowed")
		}
	}

	return nil
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)

	// If it already starts with http:// or https://, return as is
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		return host
	}

	// Check if it contains a port (format: host:port or ip:port)
	if strings.Contains(host, ":") {
		// Check if it's already a URL with port (unlikely but handle it)
		if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
			return host
		}
		// Assume HTTP for IP:port or hostname:port
		return "http://" + host
	}

	// For plain hostnames or IPs without port, default to HTTPS
	// But try HTTP first for local/internal IPs
	if isLocalIP(host) {
		return "http://" + host
	}

	// Default to HTTPS for external hostnames
	return "https://" + host
}

// isLocalIP checks if the host is a local/internal IP address
func isLocalIP(host string) bool {
	// Remove port if present
	host = strings.Split(host, ":")[0]

	// Check for localhost
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}

	// Check for private IP ranges
	if strings.HasPrefix(host, "192.168.") ||
		strings.HasPrefix(host, "10.") ||
		strings.HasPrefix(host, "172.16.") ||
		strings.HasPrefix(host, "172.17.") ||
		strings.HasPrefix(host, "172.18.") ||
		strings.HasPrefix(host, "172.19.") ||
		strings.HasPrefix(host, "172.20.") ||
		strings.HasPrefix(host, "172.21.") ||
		strings.HasPrefix(host, "172.22.") ||
		strings.HasPrefix(host, "172.23.") ||
		strings.HasPrefix(host, "172.24.") ||
		strings.HasPrefix(host, "172.25.") ||
		strings.HasPrefix(host, "172.26.") ||
		strings.HasPrefix(host, "172.27.") ||
		strings.HasPrefix(host, "172.28.") ||
		strings.HasPrefix(host, "172.29.") ||
		strings.HasPrefix(host, "172.30.") ||
		strings.HasPrefix(host, "172.31.") {
		return true
	}

	return false
}

func (tm *TestManager) calculateAndSaveMetrics(testCtx *TestContext) {
	metrics := testCtx.Metrics
	testRun := testCtx.TestRun

	metrics.mu.RLock()
	totalRequests := metrics.TotalRequests
	successCount := metrics.SuccessCount
	errorCount := metrics.ErrorCount
	latencies := make([]float64, len(metrics.Latencies))
	copy(latencies, metrics.Latencies)
	duration := time.Since(metrics.StartTime).Seconds()
	metrics.mu.RUnlock()

	var avgLatency, minLatency, maxLatency float64
	if len(latencies) > 0 {
		var sum float64
		minLatency = latencies[0]
		maxLatency = latencies[0]
		for _, lat := range latencies {
			sum += lat
			if lat < minLatency {
				minLatency = lat
			}
			if lat > maxLatency {
				maxLatency = lat
			}
		}
		avgLatency = sum / float64(len(latencies))
	}

	rps := float64(totalRequests) / duration

	now := time.Now()
	testRun.Status = "completed"
	testRun.CompletedAt = &now
	testRun.TotalRequests = totalRequests
	testRun.SuccessCount = successCount
	testRun.ErrorCount = errorCount
	testRun.AvgLatency = avgLatency
	testRun.MinLatency = minLatency
	testRun.MaxLatency = maxLatency
	testRun.RPS = rps

	if err := UpdateTestRun(tm.db, testCtx.TestRun); err != nil {
		slog.Error("Failed to update test run", "error", err, "test_id", testCtx.TestRun.ID)
	}
}

func (tm *TestManager) HandleGetStatus(w http.ResponseWriter, r *http.Request) {
	testUUID := r.URL.Path[len("/api/status/"):]

	tm.mu.RLock()
	testCtx, exists := tm.activeTests[testUUID]
	tm.mu.RUnlock()

	if exists {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"is_running": testCtx.IsRunning.Load(),
			"test_run":   testCtx.TestRun,
		})
		return
	}

	// If not in active tests, check database
	testRun, err := GetTestRunByUUID(tm.db, testUUID)
	if err != nil {
		http.Error(w, "Test not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"is_running": false,
		"test_run":   testRun,
	})
}

func (tm *TestManager) HandleStopTest(w http.ResponseWriter, r *http.Request) {
	testUUID := r.URL.Path[len("/api/stop/"):]

	tm.mu.RLock()
	testCtx, exists := tm.activeTests[testUUID]
	tm.mu.RUnlock()

	if !exists {
		http.Error(w, "Test not found or already stopped", http.StatusNotFound)
		return
	}

	// Calculate current metrics
	tm.calculateAndSaveMetrics(testCtx)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(testCtx.TestRun)
}

func (tm *TestManager) HandleGetMetrics(w http.ResponseWriter, r *http.Request) {
	testUUID := r.URL.Path[len("/api/metrics/"):]

	tm.mu.RLock()
	testCtx, exists := tm.activeTests[testUUID]
	tm.mu.RUnlock()

	if !exists {
		testRun, err := GetTestRunByUUID(tm.db, testUUID)
		if err != nil {
			http.Error(w, "Test not found", http.StatusNotFound)
			return
		}
		// Return completed test metrics in the same format as live metrics
		// Calculate error rate
		errorRate := 0.0
		if testRun.TotalRequests > 0 {
			errorRate = (float64(testRun.ErrorCount) / float64(testRun.TotalRequests)) * 100
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"total_requests": testRun.TotalRequests,
			"success_count":  testRun.SuccessCount,
			"error_count":    testRun.ErrorCount,
			"avg_latency":    testRun.AvgLatency,
			"min_latency":    testRun.MinLatency,
			"max_latency":    testRun.MaxLatency,
			"p50_latency":    0.0, // Not stored for completed tests
			"p95_latency":    0.0, // Not stored for completed tests
			"p99_latency":    0.0, // Not stored for completed tests
			"error_rate":     errorRate,
			"avg_rps":        testRun.RPS,
			"rps":            testRun.RPS,
			"duration":       float64(testRun.Duration),
			"is_running":     false,
		})
		return
	}

	metrics := testCtx.Metrics
	metrics.mu.RLock()
	latencies := make([]float64, len(metrics.Latencies))
	copy(latencies, metrics.Latencies)
	duration := time.Since(metrics.StartTime).Seconds()
	metrics.mu.RUnlock()

	var avgLatency, minLatency, maxLatency, p50Latency, p95Latency, p99Latency float64
	if len(latencies) > 0 {
		// Sort latencies for percentile calculation
		sortedLatencies := make([]float64, len(latencies))
		copy(sortedLatencies, latencies)
		sort.Float64s(sortedLatencies)

		var sum float64
		minLatency = sortedLatencies[0]
		maxLatency = sortedLatencies[len(sortedLatencies)-1]
		for _, lat := range sortedLatencies {
			sum += lat
		}
		avgLatency = sum / float64(len(sortedLatencies))

		// Calculate percentiles
		if len(sortedLatencies) > 0 {
			p50Index := int(float64(len(sortedLatencies)) * 0.50)
			p95Index := int(float64(len(sortedLatencies)) * 0.95)
			p99Index := int(float64(len(sortedLatencies)) * 0.99)

			if p50Index < len(sortedLatencies) {
				p50Latency = sortedLatencies[p50Index]
			}
			if p95Index < len(sortedLatencies) {
				p95Latency = sortedLatencies[p95Index]
			}
			if p99Index < len(sortedLatencies) {
				p99Latency = sortedLatencies[p99Index]
			}
		}
	}

	totalRequests := atomic.LoadInt64(&metrics.TotalRequests)
	successCount := atomic.LoadInt64(&metrics.SuccessCount)
	errorCount := atomic.LoadInt64(&metrics.ErrorCount)
	rps := float64(totalRequests) / duration
	errorRate := float64(0)
	if totalRequests > 0 {
		errorRate = (float64(errorCount) / float64(totalRequests)) * 100
	}

	// Calculate average RPS from time series
	var avgRPS float64
	metrics.mu.RLock()
	if len(metrics.TimeSeries) > 0 {
		var rpsSum float64
		for _, point := range metrics.TimeSeries {
			rpsSum += point.RPS
		}
		avgRPS = rpsSum / float64(len(metrics.TimeSeries))
	}
	metrics.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_requests":     totalRequests,
		"success_count":      successCount,
		"error_count":        errorCount,
		"avg_latency":        avgLatency,
		"min_latency":        minLatency,
		"max_latency":        maxLatency,
		"p50_latency":        p50Latency,
		"p95_latency":        p95Latency,
		"p99_latency":        p99Latency,
		"error_rate":         errorRate,
		"avg_rps":            avgRPS,
		"rps":                rps,
		"duration":           duration,
		"is_running":         testCtx.IsRunning.Load(),
		"stopped_by_circuit": testCtx.TestRun.StoppedByCircuit,
	})
}

func (tm *TestManager) HandleGetHistory(w http.ResponseWriter, r *http.Request) {
	testRuns, err := GetTopTestRuns(tm.db, 10)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get history: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(testRuns)
}

// HandleGetRunningTests returns all currently running tests
func (tm *TestManager) HandleGetRunningTests(w http.ResponseWriter, r *http.Request) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	runningTests := make([]map[string]interface{}, 0, len(tm.activeTests))
	for testUUID, testCtx := range tm.activeTests {
		runningTests = append(runningTests, map[string]interface{}{
			"test_id":     testCtx.TestRun.ID,
			"test_uuid":   testUUID,
			"host":        testCtx.TestRun.Host,
			"mask_host":   testCtx.TestRun.MaskHost,
			"total_users": testCtx.TestRun.TotalUsers,
			"duration":    testCtx.TestRun.Duration,
			"started_at":  testCtx.TestRun.StartedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"running_tests": runningTests,
		"count":         len(runningTests),
	})
}

func (tm *TestManager) HandleGetHistoricalMetrics(w http.ResponseWriter, r *http.Request) {
	testUUID := r.URL.Path[len("/api/historical-metrics/"):]

	testRun, err := GetTestRunByUUID(tm.db, testUUID)
	if err != nil {
		http.Error(w, "Test not found", http.StatusNotFound)
		return
	}

	// Get request metrics for this test
	metrics, err := GetRequestMetrics(tm.db, testRun.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get metrics: %v", err), http.StatusInternalServerError)
		return
	}

	// Calculate percentiles if we have data
	var p50Latency, p95Latency, p99Latency float64
	if len(metrics) > 0 {
		latencies := make([]float64, len(metrics))
		for i, m := range metrics {
			latencies[i] = m.Latency
		}
		sort.Float64s(latencies)

		p50Index := int(float64(len(latencies)) * 0.50)
		p95Index := int(float64(len(latencies)) * 0.95)
		p99Index := int(float64(len(latencies)) * 0.99)

		if p50Index < len(latencies) {
			p50Latency = latencies[p50Index]
		}
		if p95Index < len(latencies) {
			p95Latency = latencies[p95Index]
		}
		if p99Index < len(latencies) {
			p99Latency = latencies[p99Index]
		}
	}

	errorRate := float64(0)
	if testRun.TotalRequests > 0 {
		errorRate = (float64(testRun.ErrorCount) / float64(testRun.TotalRequests)) * 100
	}

	// Build time series data
	timeSeries := buildTimeSeriesFromMetrics(metrics, testRun.StartedAt)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":             testRun.ID,
		"host":           testRun.Host,
		"total_requests": testRun.TotalRequests,
		"success_count":  testRun.SuccessCount,
		"error_count":    testRun.ErrorCount,
		"avg_latency":    testRun.AvgLatency,
		"min_latency":    testRun.MinLatency,
		"max_latency":    testRun.MaxLatency,
		"p50_latency":    p50Latency,
		"p95_latency":    p95Latency,
		"p99_latency":    p99Latency,
		"error_rate":     errorRate,
		"rps":            testRun.RPS,
		"duration":       testRun.Duration,
		"started_at":     testRun.StartedAt,
		"completed_at":   testRun.CompletedAt,
		"time_series":    timeSeries,
	})
}

func buildTimeSeriesFromMetrics(metrics []*RequestMetric, startTime time.Time) []map[string]interface{} {
	points := buildTimeSeriesPoints(metrics, startTime)
	timeSeries := make([]map[string]interface{}, 0, len(points))
	for _, point := range points {
		timeSeries = append(timeSeries, map[string]interface{}{
			"timestamp":    point.Timestamp,
			"requests":     point.Requests,
			"rps":          point.RPS,
			"avg_latency":  point.AvgLatency,
			"success_rate": point.SuccessRate,
		})
	}
	return timeSeries
}

func buildTimeSeriesPoints(metrics []*RequestMetric, startTime time.Time) []TimeSeriesPoint {
	if len(metrics) == 0 {
		return []TimeSeriesPoint{}
	}

	type bucket struct {
		latencies    []float64
		successCount int
		totalCount   int
	}

	buckets := make(map[int]*bucket)
	maxOffset := 0

	for _, m := range metrics {
		secondOffset := int(m.Timestamp.Sub(startTime).Seconds())
		if secondOffset < 0 {
			secondOffset = 0
		}
		if secondOffset > maxOffset {
			maxOffset = secondOffset
		}

		b, exists := buckets[secondOffset]
		if !exists {
			b = &bucket{
				latencies: make([]float64, 0),
			}
			buckets[secondOffset] = b
		}

		b.latencies = append(b.latencies, m.Latency)
		b.totalCount++
		if m.Success {
			b.successCount++
		}
	}

	points := make([]TimeSeriesPoint, 0, len(buckets))

	for second := 0; second <= maxOffset; second++ {
		bucket, exists := buckets[second]
		if !exists {
			continue
		}

		var avgLatency float64
		if len(bucket.latencies) > 0 {
			sum := 0.0
			for _, lat := range bucket.latencies {
				sum += lat
			}
			avgLatency = sum / float64(len(bucket.latencies))
		}

		successRate := 0.0
		if bucket.totalCount > 0 {
			successRate = (float64(bucket.successCount) / float64(bucket.totalCount)) * 100
		}

		points = append(points, TimeSeriesPoint{
			Timestamp:   startTime.Add(time.Duration(second) * time.Second),
			Requests:    int64(bucket.totalCount),
			RPS:         float64(bucket.totalCount),
			AvgLatency:  avgLatency,
			SuccessRate: successRate,
		})
	}

	return points
}

func (tm *TestManager) HandleGetTimeSeries(w http.ResponseWriter, r *http.Request) {
	testUUID := r.URL.Path[len("/api/timeseries/"):]

	tm.mu.RLock()
	testCtx, exists := tm.activeTests[testUUID]
	tm.mu.RUnlock()

	if !exists {
		http.Error(w, "Test not found", http.StatusNotFound)
		return
	}

	testCtx.Metrics.mu.RLock()
	timeSeries := make([]TimeSeriesPoint, len(testCtx.Metrics.TimeSeries))
	copy(timeSeries, testCtx.Metrics.TimeSeries)
	testCtx.Metrics.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(timeSeries)
}

// HandleGetIPStats returns debug information about active tests per IP
func (tm *TestManager) HandleGetIPStats(w http.ResponseWriter, r *http.Request) {
	tm.testsPerIPMu.Lock()
	defer tm.testsPerIPMu.Unlock()

	type IPStats struct {
		IP         string   `json:"ip"`
		TestCount  int      `json:"test_count"`
		TestUUIDs  []string `json:"test_uuids"`
		AtLimit    bool     `json:"at_limit"`
		MaxAllowed int      `json:"max_allowed"`
	}

	stats := make([]IPStats, 0, len(tm.testsPerIP))
	for ip, tests := range tm.testsPerIP {
		uuids := make([]string, 0, len(tests))
		for uuid := range tests {
			uuids = append(uuids, uuid)
		}
		stats = append(stats, IPStats{
			IP:         ip,
			TestCount:  len(tests),
			TestUUIDs:  uuids,
			AtLimit:    len(tests) >= MaxTestsPerIP,
			MaxAllowed: MaxTestsPerIP,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_ips":        len(stats),
		"max_tests_per_ip": MaxTestsPerIP,
		"ip_stats":         stats,
	})
}

func (tm *TestManager) HandleGenerateReport(w http.ResponseWriter, r *http.Request) {
	testUUID := r.URL.Path[len("/api/report/"):]

	testRun, err := GetTestRunByUUID(tm.db, testUUID)
	if err != nil {
		http.Error(w, "Test not found", http.StatusNotFound)
		return
	}

	// Get time series if test is active
	var timeSeries []TimeSeriesPoint
	tm.mu.RLock()
	testCtx, exists := tm.activeTests[testUUID]
	tm.mu.RUnlock()

	if exists {
		testCtx.Metrics.mu.RLock()
		timeSeries = make([]TimeSeriesPoint, len(testCtx.Metrics.TimeSeries))
		copy(timeSeries, testCtx.Metrics.TimeSeries)
		testCtx.Metrics.mu.RUnlock()
	} else {
		historicalMetrics, err := GetRequestMetrics(tm.db, testRun.ID)
		if err == nil {
			timeSeries = buildTimeSeriesPoints(historicalMetrics, testRun.StartedAt)
		} else {
			log.Printf("failed to load historical time series for test %s: %v", testUUID, err)
		}
	}

	// Generate PDF
	pdfBytes, err := GeneratePDFReport(testRun, timeSeries)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate PDF: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=loadtest_report_%s.pdf", testUUID))
	w.Write(pdfBytes)
}

func (mc *MetricsCollector) Record(latency float64, success bool, statusCode int) {
	atomic.AddInt64(&mc.TotalRequests, 1)
	if success {
		atomic.AddInt64(&mc.SuccessCount, 1)
	} else {
		atomic.AddInt64(&mc.ErrorCount, 1)
	}

	mc.mu.Lock()
	mc.Latencies = append(mc.Latencies, latency)
	// Keep only last MaxLatencySamples latencies to avoid memory issues
	if len(mc.Latencies) > MaxLatencySamples {
		mc.Latencies = mc.Latencies[len(mc.Latencies)-MaxLatencySamples:]
	}
	mc.mu.Unlock()
}

func (mc *MetricsCollector) collectTimeSeries(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastRequestCount := int64(0)
	lastTimestamp := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentRequests := atomic.LoadInt64(&mc.TotalRequests)
			currentSuccess := atomic.LoadInt64(&mc.SuccessCount)
			now := time.Now()

			// Calculate RPS (requests in last second)
			elapsed := now.Sub(lastTimestamp).Seconds()
			if elapsed > 0 {
				rps := float64(currentRequests-lastRequestCount) / elapsed

				// Calculate average latency from recent latencies
				mc.mu.RLock()
				var avgLatency float64
				if len(mc.Latencies) > 0 {
					// Get last 100 latencies for recent average
					recentLatencies := mc.Latencies
					if len(recentLatencies) > 100 {
						recentLatencies = recentLatencies[len(recentLatencies)-100:]
					}
					var sum float64
					for _, lat := range recentLatencies {
						sum += lat
					}
					avgLatency = sum / float64(len(recentLatencies))
				}
				mc.mu.RUnlock()

				successRate := float64(0)
				if currentRequests > 0 {
					successRate = (float64(currentSuccess) / float64(currentRequests)) * 100
				}

				point := TimeSeriesPoint{
					Timestamp:   now,
					Requests:    currentRequests,
					RPS:         rps,
					AvgLatency:  avgLatency,
					SuccessRate: successRate,
				}

				mc.mu.Lock()
				mc.TimeSeries = append(mc.TimeSeries, point)
				// Keep only last 3600 points (1 hour at 1 point/second)
				if len(mc.TimeSeries) > 3600 {
					mc.TimeSeries = mc.TimeSeries[len(mc.TimeSeries)-3600:]
				}
				mc.mu.Unlock()

				lastRequestCount = currentRequests
				lastTimestamp = now
			}
		}
	}
}
