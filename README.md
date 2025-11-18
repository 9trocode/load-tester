# PipeOps Load Tester

A professional load testing and performance analysis tool built by PipeOps. Designed for comprehensive performance testing with real-time metrics, visual analytics, and detailed reporting.

### Core Functionality

- **Professional UI** - Clean, modern interface with PipeOps branding and dark/light theme
- **Live Overview Card** - Real-time virtual users, elapsed/remaining time, progress bar
- **Real-time Metrics** - Live updates of test performance with visual graphs
- **Interactive Charts** - Real-time graphs showing:
  - Throughput (Requests Per Second)
  - Average Response Time
  - Success Rate over time
- **Advanced Metrics** - Percentile latencies (P50, P95, P99) and detailed analytics
- **Clickable History** - Expandable history items with templated summaries
- **Advanced History View** - Detailed metrics with graphs and percentile data
- **PDF Reports** - Generate comprehensive PDF reports with test summaries
- **SQLite Database** - Portable, no external dependencies
- **Concurrent Testing** - Leverages Go's powerful concurrency (max 5 concurrent tests)
- **User Ramp-up** - Gradually increase load over time
- **Target Authentication** - Support for JWT, Basic Auth, and custom headers
- **URL Masking** - Automatically masks sensitive URL paths and query parameters
- **Test Resumption** - Reconnect to running tests after refresh, browser close, or sharing URLs

### Security & Reliability

- **SSRF Protection** - Blocks localhost, private IPs, and cloud metadata services
- **Input Validation** - Comprehensive validation of all user inputs
- **Rate Limiting** - 5-second minimum between test starts per IP
- **Structured Logging** - JSON logs with contextual fields and request IDs
- **Request Tracing** - Unique request ID for every API call
- **Graceful Shutdown** - Handles SIGTERM/SIGINT, cancels active tests cleanly
- **Error Handling** - Proper error checking throughout with context

## Requirements

- Go 1.21 or higher
- SQLite3 (included via Go module)

## Installation

1. Clone the repository
2. Install dependencies:
   ```bash
   go mod download
   ```

## Building

The load tester uses SQLite, which requires CGO to be enabled:

**Option 1: Use the build script (recommended)**

```bash
./build.sh
```

**Option 2: Build manually**

```bash
CGO_ENABLED=1 go build -o load-tester .
```

**Note:** CGO must be enabled because the `go-sqlite3` driver requires it. If you see an error about `CGO_ENABLED=0`, make sure to build with `CGO_ENABLED=1`.

## Deployment

### Docker

**Quick Start (Debian-based - Recommended):**

```bash
docker build -t pipeops-load-tester .
docker run -d -p 8080:8080 pipeops-load-tester
```

**Alpine-based (Smaller Image):**

```bash
docker build -f Dockerfile.alpine -t pipeops-load-tester .
docker run -d -p 8080:8080 pipeops-load-tester
```

**With Docker Compose:**

```bash
docker-compose up -d
```

**Note:** The default `Dockerfile` uses Debian to avoid SQLite musl libc compatibility issues. For smaller images, use `Dockerfile.alpine`. See [DOCKER_BUILD.md](DOCKER_BUILD.md) for details on the build fix.

### Docker Troubleshooting

If you encounter database initialization errors in Docker:

```
ERROR: Failed to initialize database: unable to open database file: no such file or directory
```

**Quick Fix:**

```bash
# Run the automated fix script
./fix-permissions.sh

# Or manually fix permissions
docker exec -u root pipeops-load-tester chown -R pipeops:pipeops /home/pipeops/app/data
docker restart pipeops-load-tester
```

**For detailed troubleshooting**, see [docs/DOCKER_TROUBLESHOOTING.md](docs/DOCKER_TROUBLESHOOTING.md)

## Usage

1. Start the server:

   ```bash
   CGO_ENABLED=1 go run .
   ```

   Or build and run:

   ```bash
   ./build.sh
   ./load-tester
   ```

2. Open your browser and navigate to:

   ```
   http://localhost:8080
   ```

3. Configure your test:
   - **Host URL**: The endpoint you want to test (e.g., `https://example.com`)
   - **Number of Users**: Total concurrent users
   - **Ramp Up (seconds)**: Time to gradually reach full user count
   - **Duration (seconds)**: How long to run the test
   - **Authentication** (optional): Configure authentication for the target system

4. Click "Start Test" and monitor the real-time metrics and graphs

5. View test history and click "Advanced View" to see detailed metrics and graphs

6. Download PDF reports for completed tests

## Advanced History View

Each completed test in the history can be expanded to show:

- **Percentile Metrics**: P50, P95, P99 latency analysis
- **Error Rate**: Detailed error statistics
- **Interactive Charts**:
  - Throughput over time
  - Response time over time
  - Success rate over time

To view advanced metrics:

1. Complete a load test
2. Find the test in the history section
3. Click "Advanced View" button
4. Explore percentile data and time-series graphs

## API Endpoints

- `POST /api/start` - Start a new load test (returns UUID)
- `GET /test/{uuid}` - View live test metrics in browser
- `GET /api/status/{uuid}` - Get test status
- `GET /api/metrics/{uuid}` - Get real-time metrics with percentiles
- `GET /api/timeseries/{uuid}` - Get time-series data for graphs
- `GET /api/historical-metrics/{uuid}` - Get historical metrics with percentiles and time-series
- `GET /api/running` - Get all currently running tests (for auto-reconnection)
- `POST /api/stop/{uuid}` - Stop a running test
- `GET /api/report/{uuid}` - Generate and download PDF report
- `GET /api/history` - Get recent test runs

## Database

The application uses SQLite to store all test runs and metrics. By default, it creates a `loadtest.db` file in the `./data` directory.

### Configuration

You can customize the database location using the `DB_PATH` environment variable:

```bash
# Default location (if DB_PATH is not set)
./data/loadtest.db

# Custom location
export DB_PATH=/path/to/your/database.db
./load-tester
```

### Docker Deployment

When running in Docker, the database is stored at `/home/pipeops/app/data/loadtest.db` and persisted via volume mount:

```bash
# Using docker-compose (recommended)
docker-compose up -d

# Manual docker run with volume
docker run -d \
  -p 8080:8080 \
  -v load-tester-data:/home/pipeops/app/data \
  -e DB_PATH=/home/pipeops/app/data/loadtest.db \
  pipeops-load-tester
```

The volume mount ensures your test history persists across container restarts and upgrades.

## PDF Reports

PDF reports include:

- Test configuration (host, users, ramp-up, duration)
- Performance metrics (requests, success rate, latency, RPS)
- Time-series summary table
- Professional formatting with PipeOps branding

Reports can be downloaded during or after a test run.

## Live Overview Card

The Live Overview Card provides at-a-glance information about the running test:

- **Virtual Users**: Number of concurrent users currently active
- **Elapsed Time**: Time since the test started (e.g., "1m 23s")
- **Remaining Time**: Time until test completion
- **Test Duration**: Total planned duration of the test
- **Progress Bar**: Visual progress indicator with percentage

All values update in real-time as the test runs.

## Test History Summaries

Each completed test shows a concise summary:

```
Tested example.com with 10 virtual users for 30s -
98.5% success rate, 125.32 RPS, 45.23ms avg latency
```

Click any history item to expand and view:

- Full metrics breakdown
- Advanced percentile data
- Interactive time-series charts
- Download PDF report option

## Test Resumption with UUID URLs (Never Lose Your Tests!)

**Problem:** Accidentally refreshed the page during a test? Lost access to live metrics?

**Solution:** Each test gets a unique UUID-based URL!

### How It Works

When you start a test, you get a clean URL with a UUID:

```
http://localhost:8080/test/550e8400-e29b-41d4-a716-446655440000
```

This means you can:

- **Refresh the page** - URL persists, test reconnects automatically
- **Close and reopen browser** - Bookmark the URL and return anytime
- **Share the URL** - Colleagues can view the same live test metrics
- **No query parameters** - Clean, professional URLs

### Features

- **UUID in path** - Each test has a unique, permanent URL
- **Path-based routing** - `/test/{uuid}` format for clean URLs
- **localStorage backup** - Persists UUID even if you navigate away
- **Automatic detection** - Checks server for any running tests on page load
- **Visual feedback** - Shows "Reconnected to running test" banner with rotating icon
- **Multi-user viewing** - Multiple people can monitor the same test simultaneously
- **Smart cleanup** - Automatically returns to home when test completes

### Priority Order

The system reconnects in this order:

1. URL path (`/test/{uuid}`) - Extracted from browser address bar
2. localStorage (`currentTestUUID`) - Fallback if URL is lost
3. Server query (`/api/running`) - Finds any running tests
4. Show CTA - If no tests found

### Example Usage

```bash
# Start a test
curl -X POST http://localhost:8080/api/start \
  -H "Content-Type: application/json" \
  -d '{"host": "https://example.com", "users": 10, "duration": 60}'

# Response: {
#   "test_id": 1,
#   "test_uuid": "550e8400-e29b-41d4-a716-446655440000",
#   "status": "started"
# }

# Share this URL with your team:
# http://localhost:8080/test/550e8400-e29b-41d4-a716-446655440000

# Everyone can view live metrics in real-time!
```

See `UUID_IMPLEMENTATION.md` and `TEST_RESUMPTION_FEATURE.md` for complete documentation.

## Understanding Metrics

### Basic Metrics

- **Total Requests**: Number of HTTP requests made
- **Success Rate**: Percentage of successful responses (2xx, 3xx status codes)
- **RPS**: Requests per second
- **Avg Latency**: Mean response time across all requests
- **Min/Max Latency**: Fastest and slowest request times

### Advanced Metrics (Percentiles)

- **P50 (Median)**: 50% of requests were faster than this value
- **P95**: 95% of requests were faster than this (good for SLA targets)
- **P99**: 99% of requests were faster than this (identifies outliers)
- **Error Rate**: Percentage of failed requests

### Why Percentiles Matter

Average latency can be misleading. For example:

```
Average Latency: 50ms
P50: 45ms
P95: 120ms
P99: 500ms
```

This tells you that while most requests are fast (~45ms), 5% of users experience latency over 120ms, and 1% experience severe delays over 500ms. This is critical for understanding actual user experience.

## Security Features

### SSRF Protection

The load tester blocks the following to prevent Server-Side Request Forgery attacks:

- Localhost (127.0.0.1, ::1, localhost)
- Private IP ranges (10.x.x.x, 192.168.x.x, 172.16.x.x)
- Link-local addresses (169.254.x.x)
- Cloud metadata services:
  - 169.254.169.254 (AWS, Azure, GCP)
  - metadata.google.internal
  - 169.254.169.123 (Oracle Cloud)
  - 100.100.100.200 (Alibaba Cloud)
- Dangerous schemes (only HTTP/HTTPS allowed)

### Additional Security

- **URL Masking**: All URLs in test history are automatically masked to hide sensitive information
- **Target Authentication**: Configure JWT, Basic Auth, or custom headers to test authenticated endpoints
- **Rate Limiting**: Prevents abuse with per-IP rate limiting (5 seconds between tests)
- **Input Validation**: All user inputs are validated against defined limits
- **Request Tracing**: Every request has a unique ID for security auditing
- **Local Storage**: All data stored locally in SQLite database

## Monitoring & Observability

### Structured Logging

All logs are output in JSON format with contextual fields:

```json
{
  "time": "2024-01-15T10:30:45Z",
  "level": "INFO",
  "msg": "Incoming request",
  "method": "POST",
  "path": "/api/start",
  "remote_addr": "127.0.0.1:54321",
  "request_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

### Request Tracing

Every API request includes an `X-Request-ID` header for end-to-end tracing.

### Graceful Shutdown

Send SIGTERM or SIGINT to trigger graceful shutdown:

- Stops accepting new requests
- Cancels all active tests
- Closes database connections
- Exits within 30 seconds

## Resource Limits

| Parameter         | Minimum              | Maximum         | Default |
| ----------------- | -------------------- | --------------- | ------- |
| Users             | 1                    | 1,000           | 10      |
| Duration          | 1 sec                | 300 sec (5 min) | 30 sec  |
| Ramp-up           | 0 sec                | 300 sec         | 5 sec   |
| Concurrent Tests  | -                    | 50              | -       |
| Tests Per IP      | -                    | 3               | -       |
| Rate Limit (Time) | 5 sec between starts | -               | -       |

### Abuse Prevention

- **Time-Based Rate Limiting**: 5 seconds minimum between test starts from the same IP
- **Concurrent Tests Per IP**: Maximum 3 simultaneous tests per IP address
- **Global Concurrent Tests**: Maximum 50 tests running across all IPs
- **IP Tracking**: Tests tracked per IP and automatically cleaned up on completion
- **Monitoring**: Debug endpoint at `/api/ip-stats` shows active tests per IP

See `IP_RATE_LIMITING.md` for detailed documentation on abuse prevention mechanisms.

## Quick Start Guide

See `QUICK_START.md` for detailed instructions including:

- Installation and setup
- API usage examples
- Authentication configuration
- Troubleshooting guide
- Best practices

## Implementation Details

For complete implementation information, see:

- `UUID_IMPLEMENTATION.md` - UUID-based URL implementation (v2.0.0)
- `UPDATE_SUMMARY.md` - Latest features and changes
- `TEST_RESUMPTION_FEATURE.md` - Test resumption documentation
- `GO_CODE_REVIEW.md` - Code quality review
- `GO_REVIEW_ACTION_ITEMS.md` - Development roadmap

## License

Copyright © 2024 PipeOps. All rights reserved.
