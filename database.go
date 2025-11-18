package main

import (
	"database/sql"
	"encoding/json"
	"time"

	"os"

	_ "github.com/mattn/go-sqlite3"
)

type TestRun struct {
	ID                    int64             `json:"id"`
	UUID                  string            `json:"uuid"`
	Host                  string            `json:"host"`
	TotalUsers            int               `json:"total_users"`
	RampUpSec             int               `json:"ramp_up_sec"`
	Duration              int               `json:"duration"`
	Status                string            `json:"status"`
	StartedAt             time.Time         `json:"started_at"`
	CompletedAt           *time.Time        `json:"completed_at,omitempty"`
	TotalRequests         int64             `json:"total_requests"`
	SuccessCount          int64             `json:"success_count"`
	ErrorCount            int64             `json:"error_count"`
	AvgLatency            float64           `json:"avg_latency"`
	MinLatency            float64           `json:"min_latency"`
	MaxLatency            float64           `json:"max_latency"`
	RPS                   float64           `json:"rps"`
	Method                string            `json:"method,omitempty"`
	Body                  string            `json:"body,omitempty"`
	Headers               map[string]string `json:"headers,omitempty"`
	MaxConcurrentRequests int               `json:"max_concurrent_requests,omitempty"` // Max concurrent requests per user
	ErrorThreshold        float64           `json:"error_threshold,omitempty"`         // Error rate threshold to trigger circuit breaker (0-100)
	StoppedByCircuit      bool              `json:"stopped_by_circuit,omitempty"`      // Whether test was stopped by circuit breaker
}

type RequestMetric struct {
	TestRunID  int64
	Timestamp  time.Time
	Latency    float64
	Success    bool
	StatusCode int
}

func InitDB() (*sql.DB, error) {
	// Get database path from environment variable or use default
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/loadtest.db"
	}

	// Extract directory from database path
	dbDir := "./data"
	if dbPath != "./data/loadtest.db" {
		// If custom path is provided, extract directory
		lastSlash := -1
		for i := len(dbPath) - 1; i >= 0; i-- {
			if dbPath[i] == '/' {
				lastSlash = i
				break
			}
		}
		if lastSlash > 0 {
			dbDir = dbPath[:lastSlash]
		}
	}

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, err
	}

	// Enable WAL mode for better concurrent read performance
	// SQLite connection string with WAL mode and connection pool settings
	// Database path can be configured via DB_PATH environment variable
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=1")
	if err != nil {
		return nil, err
	}

	// Configure connection pool for better concurrency
	// SQLite has limitations, but we can still optimize
	db.SetMaxOpenConns(25)                 // SQLite recommends 1, but we use more for read-heavy workloads
	db.SetMaxIdleConns(5)                  // Keep some connections idle
	db.SetConnMaxLifetime(5 * time.Minute) // Recycle connections periodically

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, err
	}

	// Create tables
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS test_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uuid TEXT NOT NULL UNIQUE,
		host TEXT NOT NULL,
		total_users INTEGER NOT NULL,
		ramp_up_sec INTEGER NOT NULL,
		duration INTEGER NOT NULL,
		status TEXT NOT NULL,
		started_at DATETIME NOT NULL,
		completed_at DATETIME,
		total_requests INTEGER DEFAULT 0,
		success_count INTEGER DEFAULT 0,
		error_count INTEGER DEFAULT 0,
		avg_latency REAL DEFAULT 0,
		min_latency REAL DEFAULT 0,
		max_latency REAL DEFAULT 0,
		rps REAL DEFAULT 0,
		method TEXT DEFAULT 'GET',
		body TEXT,
		headers TEXT
	);

	CREATE TABLE IF NOT EXISTS request_metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		test_run_id INTEGER NOT NULL,
		timestamp DATETIME NOT NULL,
		latency REAL NOT NULL,
		success INTEGER NOT NULL,
		status_code INTEGER NOT NULL,
		FOREIGN KEY (test_run_id) REFERENCES test_runs(id)
	);

	CREATE INDEX IF NOT EXISTS idx_test_runs_started_at ON test_runs(started_at DESC);
	CREATE INDEX IF NOT EXISTS idx_test_runs_uuid ON test_runs(uuid);
	CREATE INDEX IF NOT EXISTS idx_request_metrics_test_run ON request_metrics(test_run_id);
	`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func SaveTestRun(db *sql.DB, testRun *TestRun) (int64, error) {
	// Serialize headers to JSON
	var headersJSON string
	if testRun.Headers != nil && len(testRun.Headers) > 0 {
		headersBytes, err := json.Marshal(testRun.Headers)
		if err != nil {
			return 0, err
		}
		headersJSON = string(headersBytes)
	}

	result, err := db.Exec(
		`INSERT INTO test_runs (uuid, host, total_users, ramp_up_sec, duration, status, started_at, method, body, headers)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		testRun.UUID, testRun.Host, testRun.TotalUsers, testRun.RampUpSec, testRun.Duration, testRun.Status, testRun.StartedAt,
		testRun.Method, testRun.Body, headersJSON,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func UpdateTestRun(db *sql.DB, testRun *TestRun) error {
	_, err := db.Exec(
		`UPDATE test_runs SET
		 status = ?, completed_at = ?, total_requests = ?, success_count = ?, error_count = ?,
		 avg_latency = ?, min_latency = ?, max_latency = ?, rps = ?
		 WHERE id = ?`,
		testRun.Status, testRun.CompletedAt, testRun.TotalRequests, testRun.SuccessCount, testRun.ErrorCount,
		testRun.AvgLatency, testRun.MinLatency, testRun.MaxLatency, testRun.RPS, testRun.ID,
	)
	return err
}

func GetTestRun(db *sql.DB, id int64) (*TestRun, error) {
	var testRun TestRun
	var completedAt sql.NullTime
	var method, body, headersJSON sql.NullString

	err := db.QueryRow(
		`SELECT id, uuid, host, total_users, ramp_up_sec, duration, status, started_at, completed_at,
		 total_requests, success_count, error_count, avg_latency, min_latency, max_latency, rps,
		 method, body, headers
		 FROM test_runs WHERE id = ?`,
		id,
	).Scan(
		&testRun.ID, &testRun.UUID, &testRun.Host, &testRun.TotalUsers, &testRun.RampUpSec, &testRun.Duration,
		&testRun.Status, &testRun.StartedAt, &completedAt,
		&testRun.TotalRequests, &testRun.SuccessCount, &testRun.ErrorCount,
		&testRun.AvgLatency, &testRun.MinLatency, &testRun.MaxLatency, &testRun.RPS,
		&method, &body, &headersJSON,
	)
	if err != nil {
		return nil, err
	}

	if completedAt.Valid {
		testRun.CompletedAt = &completedAt.Time
	}

	if method.Valid {
		testRun.Method = method.String
	}
	if body.Valid {
		testRun.Body = body.String
	}
	if headersJSON.Valid && headersJSON.String != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(headersJSON.String), &headers); err == nil {
			testRun.Headers = headers
		}
	}

	return &testRun, nil
}

func GetTestRunByUUID(db *sql.DB, uuid string) (*TestRun, error) {
	var testRun TestRun
	var completedAt sql.NullTime
	var method, body, headersJSON sql.NullString

	err := db.QueryRow(
		`SELECT id, uuid, host, total_users, ramp_up_sec, duration, status, started_at, completed_at,
		 total_requests, success_count, error_count, avg_latency, min_latency, max_latency, rps,
		 method, body, headers
		 FROM test_runs WHERE uuid = ?`,
		uuid,
	).Scan(
		&testRun.ID, &testRun.UUID, &testRun.Host, &testRun.TotalUsers, &testRun.RampUpSec, &testRun.Duration,
		&testRun.Status, &testRun.StartedAt, &completedAt,
		&testRun.TotalRequests, &testRun.SuccessCount, &testRun.ErrorCount,
		&testRun.AvgLatency, &testRun.MinLatency, &testRun.MaxLatency, &testRun.RPS,
		&method, &body, &headersJSON,
	)
	if err != nil {
		return nil, err
	}

	if completedAt.Valid {
		testRun.CompletedAt = &completedAt.Time
	}

	if method.Valid {
		testRun.Method = method.String
	}
	if body.Valid {
		testRun.Body = body.String
	}
	if headersJSON.Valid && headersJSON.String != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(headersJSON.String), &headers); err == nil {
			testRun.Headers = headers
		}
	}

	return &testRun, nil
}

func GetTopTestRuns(db *sql.DB, limit int) ([]TestRun, error) {
	rows, err := db.Query(
		`SELECT id, uuid, host, total_users, ramp_up_sec, duration, status, started_at, completed_at,
		 total_requests, success_count, error_count, avg_latency, min_latency, max_latency, rps,
		 method, body, headers
		 FROM test_runs
		 ORDER BY started_at DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var testRuns []TestRun
	for rows.Next() {
		var testRun TestRun
		var completedAt sql.NullTime
		var method, body, headersJSON sql.NullString

		err := rows.Scan(
			&testRun.ID, &testRun.UUID, &testRun.Host, &testRun.TotalUsers, &testRun.RampUpSec, &testRun.Duration,
			&testRun.Status, &testRun.StartedAt, &completedAt,
			&testRun.TotalRequests, &testRun.SuccessCount, &testRun.ErrorCount,
			&testRun.AvgLatency, &testRun.MinLatency, &testRun.MaxLatency, &testRun.RPS,
			&method, &body, &headersJSON,
		)
		if err != nil {
			return nil, err
		}

		if completedAt.Valid {
			testRun.CompletedAt = &completedAt.Time
		}

		if method.Valid {
			testRun.Method = method.String
		}
		if body.Valid {
			testRun.Body = body.String
		}
		if headersJSON.Valid && headersJSON.String != "" {
			var headers map[string]string
			if err := json.Unmarshal([]byte(headersJSON.String), &headers); err == nil {
				testRun.Headers = headers
			}
		}

		testRuns = append(testRuns, testRun)
	}

	return testRuns, rows.Err()
}

func SaveRequestMetric(db *sql.DB, metric *RequestMetric) error {
	success := 0
	if metric.Success {
		success = 1
	}
	_, err := db.Exec(
		`INSERT INTO request_metrics (test_run_id, timestamp, latency, success, status_code)
		 VALUES (?, ?, ?, ?, ?)`,
		metric.TestRunID, metric.Timestamp, metric.Latency, success, metric.StatusCode,
	)
	return err
}

func GetRequestMetrics(db *sql.DB, testRunID int64) ([]*RequestMetric, error) {
	rows, err := db.Query(
		`SELECT test_run_id, timestamp, latency, success, status_code
		 FROM request_metrics
		 WHERE test_run_id = ?
		 ORDER BY timestamp ASC`,
		testRunID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []*RequestMetric
	for rows.Next() {
		var metric RequestMetric
		var success int

		err := rows.Scan(
			&metric.TestRunID,
			&metric.Timestamp,
			&metric.Latency,
			&success,
			&metric.StatusCode,
		)
		if err != nil {
			return nil, err
		}

		metric.Success = success == 1
		metrics = append(metrics, &metric)
	}

	return metrics, nil
}
