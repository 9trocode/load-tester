# Updates: Advanced History View

## What's New

### ✅ Advanced Results in Test History

Your load tester now includes an **Advanced History View** feature that provides detailed analytics and interactive charts for completed tests.

## Features

### 1. URL Protection (Already Working)
- URLs in test history are **automatically masked** to protect sensitive information
- Hides path segments and query parameters
- Preserves host and port information
- Example: `https://api.example.com/users/123?token=abc` → `https://api.example.com/users***?***`

### 2. Advanced Metrics View (NEW)
Each completed test in history now has an **"Advanced View"** button that reveals:

#### Percentile Latencies
- **P50 (Median)**: 50% of requests were faster than this
- **P95**: 95% of requests were faster than this (common SLA target)
- **P99**: 99% of requests were faster than this (identifies worst-case scenarios)

#### Error Analysis
- **Error Rate**: Percentage of failed requests
- Helps identify reliability issues

#### Interactive Charts
Three time-series charts showing test progression:
1. **Throughput Over Time**: Requests per second throughout the test
2. **Response Time Over Time**: Average latency trends
3. **Success Rate Over Time**: Request success percentage

## How to Use

### Step 1: Complete a Load Test
1. Click **"New Test"** button
2. Configure your test parameters
3. Click **"Start Test"**
4. Wait for completion

### Step 2: View Basic History
- Test appears in the **Test History** section
- Shows: Users, Requests, Success Rate, RPS, Latency, Duration

### Step 3: Expand Advanced View
1. Click **"Advanced View"** button on any completed test
2. View percentile metrics and charts
3. Click **"Hide Advanced"** to collapse

## Why This Matters

### Understanding Percentiles

**Average latency doesn't tell the whole story.**

Example scenario:
```
Average Latency: 50ms
P50: 45ms
P95: 120ms
P99: 500ms
```

**What this means:**
- Half your users get responses in ~45ms (fast!)
- 95% get responses under 120ms (acceptable)
- But 5% experience delays over 120ms
- And 1% suffer severe 500ms+ delays (poor experience!)

The average (50ms) hides the fact that some users have a terrible experience. The P99 metric reveals this critical insight.

### Real-World Application

**Setting SLAs:**
- "99% of requests complete in under 100ms" uses P99
- "95% of requests complete in under 50ms" uses P95

**Identifying Issues:**
- High P99 but low P50 → occasional spikes/outliers
- High P95 and P99 → systemic performance problems
- Charts help pinpoint when issues occurred

## Technical Details

### New API Endpoint
```
GET /api/historical-metrics/{test_id}
```

**Returns:**
- All basic test run data
- Calculated percentile latencies (P50, P95, P99)
- Error rate statistics
- Time-series data for charts

### How It Works

1. **Data Collection**: All request metrics saved to database during test
2. **On Expand**: Frontend fetches historical metrics
3. **Backend Processing**:
   - Retrieves all request latencies for the test
   - Sorts latencies and calculates percentiles
   - Reconstructs time-series by grouping requests per second
4. **Frontend Rendering**:
   - Displays metrics in a grid
   - Creates three Chart.js visualizations
   - Caches data to avoid re-fetching

### Performance

- **Lazy Loading**: Data only fetched when you click "Advanced View"
- **Caching**: Data cached after first load per test
- **Efficient**: Single database query retrieves all needed data

## Files Modified

- `loadtest.go` - Added `/api/historical-metrics/` endpoint
- `database.go` - Added `GetRequestMetrics()` function
- `app.js` - Added advanced view toggle and chart rendering
- `index.html` - Added "Advanced View" button to history items
- `style.css` - Styling for advanced metrics and charts
- `README.md` - Updated documentation

## Example Use Cases

### 1. Performance Regression Testing
- Run test before deployment → save P95/P99 values
- Run test after deployment → compare percentiles
- Catch performance degradation early

### 2. Capacity Planning
- Gradually increase user count across multiple tests
- Track how P99 degrades with load
- Find breaking point before it hits production

### 3. SLA Validation
- Set target: "P95 < 100ms"
- Run load test
- Verify: Does P95 meet target?
- Use charts to see if performance degrades over time

### 4. Infrastructure Comparison
- Test on Server A → record P95/P99
- Test on Server B → record P95/P99
- Compare: Which infrastructure performs better?

## Known Limitations

- Shows last 10 completed tests in history
- Charts show data grouped by second
- Requires test to be completed (not available for running tests)
- Advanced view data loaded on-demand (slight delay on first click)

## Future Enhancements

Potential additions:
- Export chart data as CSV
- Compare multiple test runs side-by-side
- Custom percentiles (P90, P99.9, etc.)
- Real-time percentile calculation during test
- Historical trend analysis across all tests
- Performance baseline tracking

## Getting Started

1. **Build and run:**
   ```bash
   go build -o load-tester .
   ./load-tester
   ```

2. **Access the app:**
   ```
   http://localhost:8080
   ```

3. **Run a test and explore the advanced view!**

## Questions?

**Q: Do I need to do anything special to enable this?**
A: No, it's automatically available for all completed tests.

**Q: What if I have old test data?**
A: Advanced view works for any test that has request metrics in the database.

**Q: Can I disable the advanced view?**
A: It's opt-in per test - just don't click "Advanced View" if you don't need it.

**Q: Does this slow down the main app?**
A: No, data is only fetched when you expand a specific test's advanced view.

## Summary

You now have professional-grade load testing analytics with:
- ✅ URL masking for security
- ✅ Percentile latency analysis
- ✅ Interactive time-series charts
- ✅ Error rate tracking
- ✅ Export to PDF reports

**Happy Load Testing! 🚀**
