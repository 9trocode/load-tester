# PipeOps Load Tester

A professional load testing and performance analysis tool built by PipeOps. Designed for comprehensive performance testing with real-time metrics, visual analytics, and detailed reporting.

## Features

- **Professional UI** - Clean, modern interface with PipeOps branding
- **Real-time Metrics** - Live updates of test performance with visual graphs
- **Interactive Charts** - Real-time graphs showing:
  - Throughput (Requests Per Second)
  - Average Response Time
  - Success Rate over time
- **Advanced Metrics** - Percentile latencies (P50, P95, P99) and detailed analytics
- **Advanced History View** - Expandable history items with graphs and percentile data
- **PDF Reports** - Generate comprehensive PDF reports for any test run
- **Test History** - View your recent test runs with full metrics
- **SQLite Database** - Portable, no external dependencies
- **Concurrent Testing** - Leverages Go's powerful concurrency
- **User Ramp-up** - Gradually increase load over time
- **Target Authentication** - Support for JWT, Basic Auth, and custom headers for target systems
- **URL Masking** - Automatically masks sensitive URL paths and query parameters in test history

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

### Railway (Recommended)

The easiest way to deploy is using Railway:

1. Push your code to GitHub
2. Go to [railway.app](https://railway.app)
3. Click "New Project" → "Deploy from GitHub repo"
4. Select your repository
5. Railway automatically detects `nixpacks.toml` and deploys with CGO enabled

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

- `POST /api/start` - Start a new load test
- `GET /api/status/{test_id}` - Get test status
- `GET /api/metrics/{test_id}` - Get real-time metrics with percentiles
- `GET /api/timeseries/{test_id}` - Get time-series data for graphs
- `GET /api/historical-metrics/{test_id}` - Get historical metrics with percentiles and time-series
- `POST /api/stop/{test_id}` - Stop a running test
- `GET /api/report/{test_id}` - Generate and download PDF report
- `GET /api/history` - Get recent test runs

## Database

The application uses SQLite and creates a `loadtest.db` file in the project directory. This file stores all test runs and metrics.

## PDF Reports

PDF reports include:

- Test configuration (host, users, ramp-up, duration)
- Performance metrics (requests, success rate, latency, RPS)
- Time-series summary table
- Professional formatting with PipeOps branding

Reports can be downloaded during or after a test run.

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

## Security & Privacy

- **URL Masking**: All URLs in test history are automatically masked to hide sensitive path and query parameter information
- **Target Authentication**: Configure JWT, Basic Auth, or custom headers to test authenticated endpoints
- **Local Storage**: All data stored locally in SQLite database

## License

Copyright © 2024 PipeOps. All rights reserved.

# load-tester
