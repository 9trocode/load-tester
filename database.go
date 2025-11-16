package main

import (
	"database/sql"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type TestRun struct {
	ID            int64      `json:"id"`
	UUID          string     `json:"uuid"`
	Host          string     `json:"host"`
	TotalUsers    int        `json:"total_users"`
	RampUpSec     int        `json:"ramp_up_sec"`
	Duration      int        `json:"duration"`
	Status        string     `json:"status"`
	StartedAt     time.Time  `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	TotalRequests int64      `json:"total_requests"`
	SuccessCount  int64      `json:"success_count"`
	ErrorCount    int64      `json:"error_count"`
	AvgLatency    float64    `json:"avg_latency"`
	MinLatency    float64    `json:"min_latency"`
	MaxLatency    float64    `json:"max_latency"`
	RPS           float64    `json:"rps"`
}

type RequestMetric struct {
	TestRunID  int64
	Timestamp  time.Time
	Latency    float64
	Success    bool
	StatusCode int
}

func InitDB() (*sql.DB, error) {
	// Enable WAL mode for better concurrent read performance
	// SQLite connection string with WAL mode and connection pool settings
	db, err := sql.Open("sqlite3", "./loadtest.db?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=1")
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
		rps REAL DEFAULT 0
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
	result, err := db.Exec(
		`INSERT INTO test_runs (uuid, host, total_users, ramp_up_sec, duration, status, started_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		testRun.UUID, testRun.Host, testRun.TotalUsers, testRun.RampUpSec, testRun.Duration, testRun.Status, testRun.StartedAt,
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

	err := db.QueryRow(
		`SELECT id, uuid, host, total_users, ramp_up_sec, duration, status, started_at, completed_at,
		 total_requests, success_count, error_count, avg_latency, min_latency, max_latency, rps
		 FROM test_runs WHERE id = ?`,
		id,
	).Scan(
		&testRun.ID, &testRun.UUID, &testRun.Host, &testRun.TotalUsers, &testRun.RampUpSec, &testRun.Duration,
		&testRun.Status, &testRun.StartedAt, &completedAt,
		&testRun.TotalRequests, &testRun.SuccessCount, &testRun.ErrorCount,
		&testRun.AvgLatency, &testRun.MinLatency, &testRun.MaxLatency, &testRun.RPS,
	)
	if err != nil {
		return nil, err
	}

	if completedAt.Valid {
		testRun.CompletedAt = &completedAt.Time
	}

	return &testRun, nil
}

func GetTestRunByUUID(db *sql.DB, uuid string) (*TestRun, error) {
	var testRun TestRun
	var completedAt sql.NullTime

	err := db.QueryRow(
		`SELECT id, uuid, host, total_users, ramp_up_sec, duration, status, started_at, completed_at,
		 total_requests, success_count, error_count, avg_latency, min_latency, max_latency, rps
		 FROM test_runs WHERE uuid = ?`,
		uuid,
	).Scan(
		&testRun.ID, &testRun.UUID, &testRun.Host, &testRun.TotalUsers, &testRun.RampUpSec, &testRun.Duration,
		&testRun.Status, &testRun.StartedAt, &completedAt,
		&testRun.TotalRequests, &testRun.SuccessCount, &testRun.ErrorCount,
		&testRun.AvgLatency, &testRun.MinLatency, &testRun.MaxLatency, &testRun.RPS,
	)
	if err != nil {
		return nil, err
	}

	if completedAt.Valid {
		testRun.CompletedAt = &completedAt.Time
	}

	return &testRun, nil
}

func GetTopTestRuns(db *sql.DB, limit int) ([]*TestRun, error) {
	rows, err := db.Query(
		`SELECT id, uuid, host, total_users, ramp_up_sec, duration, status, started_at, completed_at,
		 total_requests, success_count, error_count, avg_latency, min_latency, max_latency, rps
		 FROM test_runs
		 WHERE status = 'completed'
		 ORDER BY started_at DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var testRuns []*TestRun
	for rows.Next() {
		var testRun TestRun
		var completedAt sql.NullTime

		err := rows.Scan(
			&testRun.ID, &testRun.UUID, &testRun.Host, &testRun.TotalUsers, &testRun.RampUpSec, &testRun.Duration,
			&testRun.Status, &testRun.StartedAt, &completedAt,
			&testRun.TotalRequests, &testRun.SuccessCount, &testRun.ErrorCount,
			&testRun.AvgLatency, &testRun.MinLatency, &testRun.MaxLatency, &testRun.RPS,
		)
		if err != nil {
			return nil, err
		}

		if completedAt.Valid {
			testRun.CompletedAt = &completedAt.Time
		}

		testRuns = append(testRuns, &testRun)
	}

	return testRuns, nil
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
