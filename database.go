package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const migrationsDir = "./migrations"

type TestRun struct {
	ID                    int64             `json:"id"`
	UUID                  string            `json:"uuid"`
	Host                  string            `json:"host"`
	MaskHost              bool              `json:"mask_host"`
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
	MaxConcurrentRequests int               `json:"max_concurrent_requests,omitempty"`
	ErrorThreshold        float64           `json:"error_threshold,omitempty"`
	StoppedByCircuit      bool              `json:"stopped_by_circuit,omitempty"`
}

type RequestMetric struct {
	TestRunID  int64
	Timestamp  time.Time
	Latency    float64
	Success    bool
	StatusCode int
}

func InitDB() (*sql.DB, error) {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/loadtest.db"
	}

	slog.Info("Database initialization", "db_path", dbPath)

	dbDir := "./data"
	if dbPath != "./data/loadtest.db" {
		lastSlash := strings.LastIndex(dbPath, "/")
		if lastSlash > 0 {
			dbDir = dbPath[:lastSlash]
		}
	}

	slog.Info("Creating database directory", "db_dir", dbDir)
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		slog.Error("Failed to create database directory", "db_dir", dbDir, "error", err)
		return nil, err
	}

	slog.Info("Database directory ready", "db_dir", dbDir)

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=1")
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, err
	}

	createTableSQL := `
	CREATE TABLE IF NOT EXISTS test_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uuid TEXT NOT NULL UNIQUE,
		host TEXT NOT NULL,
		mask_host INTEGER NOT NULL DEFAULT 1,
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

	if _, err := db.Exec(createTableSQL); err != nil {
		return nil, err
	}

	if err := applyMigrations(db); err != nil {
		return nil, err
	}

	return db, nil
}

func applyMigrations(db *sql.DB) error {
	if err := ensureMigrationTable(db); err != nil {
		return err
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Info("No migrations directory found; skipping migrations")
			return nil
		}
		return err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".sql") {
			continue
		}

		applied, err := isMigrationApplied(db, name)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		if err := executeMigration(db, name); err != nil {
			return err
		}
	}

	return nil
}

func ensureMigrationTable(db *sql.DB) error {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS schema_migrations (
		name TEXT PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

func isMigrationApplied(db *sql.DB, name string) (bool, error) {
	var count int
	if err := db.QueryRow("SELECT COUNT(1) FROM schema_migrations WHERE name = ?", name).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func executeMigration(db *sql.DB, name string) error {
	path := filepath.Join(migrationsDir, name)
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	sqlStmt := strings.TrimSpace(string(content))
	if sqlStmt == "" {
		slog.Info("Skipping empty migration file", "migration", name)
		return recordMigration(db, name)
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(sqlStmt); err != nil {
		errorLower := strings.ToLower(err.Error())
		if !strings.Contains(errorLower, "duplicate column") {
			tx.Rollback()
			return fmt.Errorf("migration %s failed: %w", name, err)
		}
		slog.Info("Migration already applied (column exists)", "migration", name, "error", err)
	}

	if err := recordMigration(tx, name); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

func recordMigration(exec sqlExec, name string) error {
	_, err := exec.Exec("INSERT INTO schema_migrations (name) VALUES (?)", name)
	return err
}

type sqlExec interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

func SaveTestRun(db *sql.DB, testRun *TestRun) (int64, error) {
	var headersJSON string
	if testRun.Headers != nil && len(testRun.Headers) > 0 {
		headersBytes, err := json.Marshal(testRun.Headers)
		if err != nil {
			return 0, err
		}
		headersJSON = string(headersBytes)
	}

	result, err := db.Exec(
		`INSERT INTO test_runs (uuid, host, mask_host, total_users, ramp_up_sec, duration, status, started_at, method, body, headers)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		testRun.UUID, testRun.Host, testRun.MaskHost, testRun.TotalUsers, testRun.RampUpSec, testRun.Duration, testRun.Status, testRun.StartedAt,
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
	var maskHost sql.NullBool

	err := db.QueryRow(
		`SELECT id, uuid, host, mask_host, total_users, ramp_up_sec, duration, status, started_at, completed_at,
		 total_requests, success_count, error_count, avg_latency, min_latency, max_latency, rps,
		 method, body, headers
		 FROM test_runs WHERE id = ?`,
		id,
	).Scan(
		&testRun.ID, &testRun.UUID, &testRun.Host, &maskHost, &testRun.TotalUsers, &testRun.RampUpSec, &testRun.Duration,
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
	if maskHost.Valid {
		testRun.MaskHost = maskHost.Bool
	} else {
		testRun.MaskHost = true
	}

	return &testRun, nil
}

func GetTestRunByUUID(db *sql.DB, uuid string) (*TestRun, error) {
	var testRun TestRun
	var completedAt sql.NullTime
	var method, body, headersJSON sql.NullString
	var maskHost sql.NullBool

	err := db.QueryRow(
		`SELECT id, uuid, host, mask_host, total_users, ramp_up_sec, duration, status, started_at, completed_at,
		 total_requests, success_count, error_count, avg_latency, min_latency, max_latency, rps,
		 method, body, headers
		 FROM test_runs WHERE uuid = ?`,
		uuid,
	).Scan(
		&testRun.ID, &testRun.UUID, &testRun.Host, &maskHost, &testRun.TotalUsers, &testRun.RampUpSec, &testRun.Duration,
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
	if maskHost.Valid {
		testRun.MaskHost = maskHost.Bool
	} else {
		testRun.MaskHost = true
	}

	return &testRun, nil
}

func GetTopTestRuns(db *sql.DB, limit int) ([]TestRun, error) {
	rows, err := db.Query(
		`SELECT id, uuid, host, mask_host, total_users, ramp_up_sec, duration, status, started_at, completed_at,
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
		var maskHost sql.NullBool

		err := rows.Scan(
			&testRun.ID, &testRun.UUID, &testRun.Host, &maskHost, &testRun.TotalUsers, &testRun.RampUpSec, &testRun.Duration,
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
		if maskHost.Valid {
			testRun.MaskHost = maskHost.Bool
		} else {
			testRun.MaskHost = true
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
