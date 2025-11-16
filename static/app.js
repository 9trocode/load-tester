let currentTestId = null;
let metricsInterval = null;
let timeSeriesInterval = null;
let throughputChart = null;
let latencyChart = null;
let successRateChart = null;
let showAdvancedMetrics = false;
let expandedHistoryItems = new Set();

// URL masking function - completely masks URLs showing only protocol and domain
function maskUrl(url) {
  if (!url) return "-";

  url = url.trim();

  // Try to parse as full URL first
  try {
    const urlObj = new URL(url);
    const hostname = urlObj.hostname;
    const protocol = urlObj.protocol;

    // Completely mask everything after hostname
    return `${protocol}//${hostname}`;
  } catch (e) {
    // If URL parsing fails, try to extract hostname manually
    // Handle cases like "192.168.1.1:8080" or "api.example.com"

    // Check if it contains :// (protocol)
    if (url.includes("://")) {
      const parts = url.split("://");
      if (parts.length === 2) {
        const hostPart = parts[1];
        // Extract just the hostname (before first / or ? or :)
        const hostOnly = hostPart.split("/")[0].split("?")[0].split(":")[0];
        return `${parts[0]}//${hostOnly}`;
      }
    }

    // No protocol - might be IP:port or hostname:port or just hostname
    // Extract just the hostname/IP (before any / or ? or :)
    const hostOnly = url.split("/")[0].split("?")[0].split(":")[0];
    return hostOnly;
  }
}

// Modal functions
function openTestModal() {
  document.getElementById("testModal").style.display = "flex";
  document.body.style.overflow = "hidden";
}

function closeTestModal() {
  document.getElementById("testModal").style.display = "none";
  document.body.style.overflow = "";
}

// Reset form
function resetForm() {
  document.getElementById("testForm").reset();
  document.getElementById("host").value = "";
  document.getElementById("users").value = "10";
  document.getElementById("rampUp").value = "5";
  document.getElementById("duration").value = "30";
  document.getElementById("enableAuth").checked = false;
  document.getElementById("authConfig").style.display = "none";
  document.getElementById("authType").value = "jwt";
  showAuthTypeConfig("jwt");
}

// Show/hide auth config based on checkbox
function toggleAuthConfig() {
  const enableAuth = document.getElementById("enableAuth");
  const authConfig = document.getElementById("authConfig");
  authConfig.style.display = enableAuth.checked ? "block" : "none";
}

// Show auth type specific config
function showAuthTypeConfig(type) {
  document.querySelectorAll(".auth-type-config").forEach((el) => {
    el.style.display = "none";
  });

  if (type === "jwt") {
    document.getElementById("jwtAuth").style.display = "block";
  } else if (type === "basic") {
    document.getElementById("basicAuth").style.display = "block";
  } else if (type === "header") {
    document.getElementById("headerAuth").style.display = "block";
  }
}

// Get auth config from form
function getAuthConfig() {
  const enableAuth = document.getElementById("enableAuth");
  if (!enableAuth.checked) {
    return null;
  }

  const authType = document.getElementById("authType").value;
  const auth = { type: authType };

  if (authType === "jwt") {
    const token = document.getElementById("jwtToken").value.trim();
    if (token) {
      auth.token = token;
      return auth;
    }
  } else if (authType === "basic") {
    const username = document.getElementById("basicUsername").value.trim();
    const password = document.getElementById("basicPassword").value.trim();
    if (username && password) {
      auth.username = username;
      auth.password = password;
      return auth;
    }
  } else if (authType === "header") {
    const headerName = document.getElementById("headerName").value.trim();
    const headerValue = document.getElementById("headerValue").value.trim();
    if (headerName && headerValue) {
      auth.header_name = headerName;
      auth.header_value = headerValue;
      return auth;
    }
  }

  return null;
}

// Toggle advanced metrics
function toggleAdvancedMetrics() {
  showAdvancedMetrics = !showAdvancedMetrics;
  const advancedMetrics = document.getElementById("advancedMetrics");
  const toggleBtn = document.getElementById("advancedMetricsToggle");

  if (showAdvancedMetrics) {
    advancedMetrics.style.display = "grid";
    toggleBtn.classList.add("active");
    toggleBtn.querySelector("span").textContent = "Basic";
  } else {
    advancedMetrics.style.display = "none";
    toggleBtn.classList.remove("active");
    toggleBtn.querySelector("span").textContent = "Advanced";
  }
}

// Chart.js configuration
function getChartOptions() {
  const isDark = document.documentElement.getAttribute("data-theme") === "dark";
  const gridColor = isDark ? "rgba(255,255,255,0.05)" : "rgba(0,0,0,0.05)";
  const textColor = isDark ? "#94a3b8" : "#64748b";

  return {
    responsive: true,
    maintainAspectRatio: true,
    plugins: {
      legend: {
        display: false,
      },
    },
    scales: {
      y: {
        beginAtZero: true,
        grid: {
          color: gridColor,
          borderWidth: 0,
        },
        ticks: {
          color: textColor,
          font: {
            size: 11,
          },
        },
      },
      x: {
        grid: {
          display: false,
        },
        ticks: {
          color: textColor,
          font: {
            size: 11,
          },
        },
      },
    },
  };
}

function initializeCharts() {
  const chartOptions = getChartOptions();

  // Throughput Chart
  const throughputCtx = document
    .getElementById("throughputChart")
    .getContext("2d");
  throughputChart = new Chart(throughputCtx, {
    type: "line",
    data: {
      labels: [],
      datasets: [
        {
          label: "Requests Per Second",
          data: [],
          borderColor: "rgb(59, 130, 246)",
          backgroundColor: "rgba(59, 130, 246, 0.05)",
          tension: 0.4,
          fill: true,
          borderWidth: 2,
          pointRadius: 0,
        },
      ],
    },
    options: chartOptions,
  });

  // Latency Chart
  const latencyCtx = document.getElementById("latencyChart").getContext("2d");
  latencyChart = new Chart(latencyCtx, {
    type: "line",
    data: {
      labels: [],
      datasets: [
        {
          label: "Average Latency (ms)",
          data: [],
          borderColor: "rgb(239, 68, 68)",
          backgroundColor: "rgba(239, 68, 68, 0.05)",
          tension: 0.4,
          fill: true,
          borderWidth: 2,
          pointRadius: 0,
        },
      ],
    },
    options: chartOptions,
  });

  // Success Rate Chart
  const successRateCtx = document
    .getElementById("successRateChart")
    .getContext("2d");
  successRateChart = new Chart(successRateCtx, {
    type: "line",
    data: {
      labels: [],
      datasets: [
        {
          label: "Success Rate (%)",
          data: [],
          borderColor: "rgb(16, 185, 129)",
          backgroundColor: "rgba(16, 185, 129, 0.05)",
          tension: 0.4,
          fill: true,
          borderWidth: 2,
          pointRadius: 0,
        },
      ],
    },
    options: {
      ...chartOptions,
      scales: {
        ...chartOptions.scales,
        y: {
          ...chartOptions.scales.y,
          max: 100,
        },
      },
    },
  });
}

// Event listeners
document
  .getElementById("openTestModalBtn")
  .addEventListener("click", openTestModal);
document
  .getElementById("closeModalBtn")
  .addEventListener("click", closeTestModal);
document.getElementById("resetFormBtn").addEventListener("click", resetForm);
document
  .getElementById("advancedMetricsToggle")
  .addEventListener("click", toggleAdvancedMetrics);
document
  .getElementById("enableAuth")
  .addEventListener("change", toggleAuthConfig);
document.getElementById("authType").addEventListener("change", (e) => {
  showAuthTypeConfig(e.target.value);
});

// Close modal on overlay click
document.getElementById("testModal").addEventListener("click", (e) => {
  if (e.target.id === "testModal") {
    closeTestModal();
  }
});

// Close modal on Escape key
document.addEventListener("keydown", (e) => {
  if (
    e.key === "Escape" &&
    document.getElementById("testModal").style.display === "flex"
  ) {
    closeTestModal();
  }
});

document.getElementById("testForm").addEventListener("submit", async (e) => {
  e.preventDefault();

  let host = document.getElementById("host").value.trim();
  if (!host) {
    alert("Please enter a target host");
    return;
  }

  const users = parseInt(document.getElementById("users").value);
  const rampUp = parseInt(document.getElementById("rampUp").value);
  const duration = parseInt(document.getElementById("duration").value);

  // Client-side validation
  if (users < 1 || users > 1000) {
    alert("Users must be between 1 and 1000");
    return;
  }

  if (rampUp < 1 || rampUp > 60) {
    alert("Ramp-up time must be between 1 and 60 seconds");
    return;
  }

  if (duration < 1 || duration > 300) {
    alert("Duration must be between 1 and 300 seconds (5 minutes)");
    return;
  }

  if (rampUp > duration) {
    alert("Ramp-up time cannot exceed test duration");
    return;
  }

  // Get auth config
  const auth = getAuthConfig();

  const requestBody = { host, users, ramp_up_sec: rampUp, duration };
  if (auth) {
    requestBody.auth = auth;
  }

  try {
    const response = await fetch("/api/start", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(requestBody),
    });

    if (!response.ok) {
      const errorText = await response.text();
      alert("Failed to start test: " + errorText);
      return;
    }

    const data = await response.json();
    currentTestId = data.test_id;

    // Store host for display
    document.getElementById("currentHostUrl").textContent = maskUrl(host);

    closeTestModal();
    document.getElementById("ctaSection").style.display = "none";
    document.getElementById("metricsSection").style.display = "block";
    document.getElementById("historySection").style.display = "none";

    // Reset charts
    resetCharts();
    startMetricsPolling();
    startTimeSeriesPolling();
  } catch (error) {
    alert("Error starting test: " + error.message);
  }
});

function resetCharts() {
  if (throughputChart) {
    throughputChart.data.labels = [];
    throughputChart.data.datasets[0].data = [];
    throughputChart.update();
  }
  if (latencyChart) {
    latencyChart.data.labels = [];
    latencyChart.data.datasets[0].data = [];
    latencyChart.update();
  }
  if (successRateChart) {
    successRateChart.data.labels = [];
    successRateChart.data.datasets[0].data = [];
    successRateChart.update();
  }
}

function startMetricsPolling() {
  if (metricsInterval) {
    clearInterval(metricsInterval);
  }

  metricsInterval = setInterval(async () => {
    if (!currentTestId) return;

    try {
      const response = await fetch(`/api/metrics/${currentTestId}`);
      if (!response.ok) {
        stopMetricsPolling();
        return;
      }

      const metrics = await response.json();
      updateMetrics(metrics);

      if (!metrics.is_running) {
        stopMetricsPolling();
        stopTimeSeriesPolling();
        document.getElementById("ctaSection").style.display = "block";
        document.getElementById("metricsSection").style.display = "none";
        document.getElementById("historySection").style.display = "block";
        currentTestId = null;
        loadHistory();
      }
    } catch (error) {
      console.error("Error fetching metrics:", error);
    }
  }, 1000);
}

function startTimeSeriesPolling() {
  if (timeSeriesInterval) {
    clearInterval(timeSeriesInterval);
  }

  timeSeriesInterval = setInterval(async () => {
    if (!currentTestId) return;

    try {
      const response = await fetch(`/api/timeseries/${currentTestId}`);
      if (!response.ok) {
        return;
      }

      const timeSeries = await response.json();
      updateCharts(timeSeries);
    } catch (error) {
      console.error("Error fetching time series:", error);
    }
  }, 2000);
}

function stopTimeSeriesPolling() {
  if (timeSeriesInterval) {
    clearInterval(timeSeriesInterval);
    timeSeriesInterval = null;
  }
}

function stopMetricsPolling() {
  if (metricsInterval) {
    clearInterval(metricsInterval);
    metricsInterval = null;
  }
}

function updateMetrics(metrics) {
  // Basic metrics
  document.getElementById("totalRequests").textContent =
    metrics.total_requests.toLocaleString();

  const successRate =
    metrics.total_requests > 0
      ? ((metrics.success_count / metrics.total_requests) * 100).toFixed(1)
      : 0;
  document.getElementById("successRate").textContent = successRate + "%";

  document.getElementById("rps").textContent = metrics.rps.toFixed(2);
  document.getElementById("avgLatency").textContent =
    metrics.avg_latency.toFixed(2) + " ms";
  document.getElementById("minLatency").textContent =
    metrics.min_latency.toFixed(2) + " ms";
  document.getElementById("maxLatency").textContent =
    metrics.max_latency.toFixed(2) + " ms";

  // Advanced metrics
  document.getElementById("p50Latency").textContent =
    (metrics.p50_latency || 0).toFixed(2) + " ms";
  document.getElementById("p95Latency").textContent =
    (metrics.p95_latency || 0).toFixed(2) + " ms";
  document.getElementById("p99Latency").textContent =
    (metrics.p99_latency || 0).toFixed(2) + " ms";
  document.getElementById("errorRate").textContent =
    (metrics.error_rate || 0).toFixed(2) + "%";
  document.getElementById("totalErrors").textContent = (
    metrics.error_count || 0
  ).toLocaleString();
  document.getElementById("avgRPS").textContent = (
    metrics.avg_rps || 0
  ).toFixed(2);
}

function updateCharts(timeSeries) {
  if (!timeSeries || timeSeries.length === 0) return;

  const recentData = timeSeries.slice(-60);

  const labels = recentData.map((point) => {
    const date = new Date(point.timestamp);
    return date.toLocaleTimeString();
  });

  const rpsData = recentData.map((point) => point.rps);
  const latencyData = recentData.map((point) => point.avg_latency);
  const successRateData = recentData.map((point) => point.success_rate);

  if (throughputChart) {
    throughputChart.data.labels = labels;
    throughputChart.data.datasets[0].data = rpsData;
    throughputChart.update("none");
  }

  if (latencyChart) {
    latencyChart.data.labels = labels;
    latencyChart.data.datasets[0].data = latencyData;
    latencyChart.update("none");
  }

  if (successRateChart) {
    successRateChart.data.labels = labels;
    successRateChart.data.datasets[0].data = successRateData;
    successRateChart.update("none");
  }
}

async function loadHistory() {
  try {
    const response = await fetch("/api/history");
    if (!response.ok) {
      throw new Error("Failed to load history");
    }

    const history = await response.json();
    displayHistory(history);
  } catch (error) {
    console.error("Error loading history:", error);
    // On error, show CTA if no test is running
    if (!currentTestId) {
      document.getElementById("ctaSection").style.display = "block";
      document.getElementById("historySection").style.display = "none";
    } else {
      document.getElementById("historyList").innerHTML =
        '<div class="empty-state">Error loading history</div>';
    }
  }
}

function displayHistory(history) {
  const historyList = document.getElementById("historyList");

  if (history.length === 0) {
    // Show CTA if no test is running, otherwise show empty state
    if (!currentTestId) {
      document.getElementById("ctaSection").style.display = "block";
      document.getElementById("historySection").style.display = "none";
    } else {
      historyList.innerHTML =
        '<div class="empty-state">No test history yet. Start your first test to see results here.</div>';
    }
    return;
  }

  // Show CTA if there's 1 or more history items and no test is running
  if (!currentTestId) {
    document.getElementById("ctaSection").style.display = "block";
    document.getElementById("historySection").style.display = "block";
  }

  historyList.innerHTML = history
    .map(
      (test) => `
        <div class="history-item" id="history-item-${test.id}">
            <div class="history-item-header">
                <div class="history-item-url">${escapeHtml(maskUrl(test.host))}</div>
                <div class="history-item-meta">
                    <span class="status-badge status-${test.status}">${test.status}</span>
                    <span class="history-item-time">${formatDate(test.started_at)}</span>
                </div>
            </div>
            <div class="history-item-metrics">
                <div class="history-metric">
                    <span class="history-metric-label">Users</span>
                    <span class="history-metric-value">${test.total_users}</span>
                </div>
                <div class="history-metric">
                    <span class="history-metric-label">Requests</span>
                    <span class="history-metric-value">${test.total_requests.toLocaleString()}</span>
                </div>
                <div class="history-metric">
                    <span class="history-metric-label">Success</span>
                    <span class="history-metric-value">${test.total_requests > 0 ? ((test.success_count / test.total_requests) * 100).toFixed(1) : 0}%</span>
                </div>
                <div class="history-metric">
                    <span class="history-metric-label">RPS</span>
                    <span class="history-metric-value">${test.rps.toFixed(2)}</span>
                </div>
                <div class="history-metric">
                    <span class="history-metric-label">Latency</span>
                    <span class="history-metric-value">${test.avg_latency.toFixed(2)}ms</span>
                </div>
                <div class="history-metric">
                    <span class="history-metric-label">Duration</span>
                    <span class="history-metric-value">${test.duration}s</span>
                </div>
            </div>
            <div class="history-item-actions">
                <button class="btn btn-secondary btn-sm" onclick="toggleAdvancedView(this, ${test.id})">
                    Advanced View
                </button>
                <button class="btn btn-secondary btn-sm" onclick="downloadReport(${test.id})">
                    Download Report
                </button>
            </div>
            <div id="advanced-view-${test.id}" class="history-advanced-view" style="display: none;">
                <div class="loading-state">Loading advanced metrics...</div>
            </div>
        </div>
    `,
    )
    .join("");
}

async function toggleAdvancedView(button, testId) {
  const advancedView = document.getElementById(`advanced-view-${testId}`);

  if (expandedHistoryItems.has(testId)) {
    // Collapse
    advancedView.style.display = "none";
    expandedHistoryItems.delete(testId);
    button.textContent = "Advanced View";
  } else {
    // Expand
    advancedView.style.display = "block";
    expandedHistoryItems.add(testId);
    button.textContent = "Hide Advanced";

    // Load advanced metrics if not already loaded
    if (!advancedView.dataset.loaded) {
      await loadAdvancedMetrics(testId);
    }
  }
}

async function loadAdvancedMetrics(testId) {
  const advancedView = document.getElementById(`advanced-view-${testId}`);

  try {
    const response = await fetch(`/api/historical-metrics/${testId}`);
    if (!response.ok) {
      throw new Error("Failed to load advanced metrics");
    }

    const data = await response.json();

    // Render advanced metrics
    advancedView.innerHTML = `
            <div class="advanced-metrics-grid">
                <div class="history-metric">
                    <span class="history-metric-label">P50 Latency</span>
                    <span class="history-metric-value">${data.p50_latency.toFixed(2)}ms</span>
                </div>
                <div class="history-metric">
                    <span class="history-metric-label">P95 Latency</span>
                    <span class="history-metric-value">${data.p95_latency.toFixed(2)}ms</span>
                </div>
                <div class="history-metric">
                    <span class="history-metric-label">P99 Latency</span>
                    <span class="history-metric-value">${data.p99_latency.toFixed(2)}ms</span>
                </div>
                <div class="history-metric">
                    <span class="history-metric-label">Error Rate</span>
                    <span class="history-metric-value">${data.error_rate.toFixed(2)}%</span>
                </div>
            </div>
            <div class="history-charts">
                <div class="history-chart-card">
                    <div class="chart-header">Throughput Over Time</div>
                    <div class="chart-container">
                        <canvas id="history-throughput-${testId}"></canvas>
                    </div>
                </div>
                <div class="history-chart-card">
                    <div class="chart-header">Response Time Over Time</div>
                    <div class="chart-container">
                        <canvas id="history-latency-${testId}"></canvas>
                    </div>
                </div>
                <div class="history-chart-card">
                    <div class="chart-header">Success Rate Over Time</div>
                    <div class="chart-container">
                        <canvas id="history-success-${testId}"></canvas>
                    </div>
                </div>
            </div>
        `;

    advancedView.dataset.loaded = "true";

    // Render charts
    renderHistoryCharts(testId, data.time_series);
  } catch (error) {
    console.error("Error loading advanced metrics:", error);
    advancedView.innerHTML =
      '<div class="empty-state">Failed to load advanced metrics</div>';
  }
}

function renderHistoryCharts(testId, timeSeries) {
  if (!timeSeries || timeSeries.length === 0) {
    return;
  }

  const chartOptions = getChartOptions();
  const labels = timeSeries.map((_, i) => `${i}s`);
  const rpsData = timeSeries.map((point) => point.rps || 0);
  const latencyData = timeSeries.map((point) => point.avg_latency || 0);
  const successRateData = timeSeries.map((point) => point.success_rate || 0);

  // Throughput chart
  const throughputCtx = document.getElementById(`history-throughput-${testId}`);
  if (throughputCtx) {
    new Chart(throughputCtx, {
      type: "line",
      data: {
        labels: labels,
        datasets: [
          {
            label: "RPS",
            data: rpsData,
            borderColor: "#3b82f6",
            backgroundColor: "rgba(59, 130, 246, 0.1)",
            tension: 0.4,
            fill: true,
            borderWidth: 2,
            pointRadius: 0,
          },
        ],
      },
      options: chartOptions,
    });
  }

  // Latency chart
  const latencyCtx = document.getElementById(`history-latency-${testId}`);
  if (latencyCtx) {
    new Chart(latencyCtx, {
      type: "line",
      data: {
        labels: labels,
        datasets: [
          {
            label: "Latency (ms)",
            data: latencyData,
            borderColor: "#10b981",
            backgroundColor: "rgba(16, 185, 129, 0.1)",
            tension: 0.4,
            fill: true,
            borderWidth: 2,
            pointRadius: 0,
          },
        ],
      },
      options: chartOptions,
    });
  }

  // Success rate chart
  const successCtx = document.getElementById(`history-success-${testId}`);
  if (successCtx) {
    new Chart(successCtx, {
      type: "line",
      data: {
        labels: labels,
        datasets: [
          {
            label: "Success Rate (%)",
            data: successRateData,
            borderColor: "#f59e0b",
            backgroundColor: "rgba(245, 158, 11, 0.1)",
            tension: 0.4,
            fill: true,
            borderWidth: 2,
            pointRadius: 0,
          },
        ],
      },
      options: {
        ...chartOptions,
        scales: {
          ...chartOptions.scales,
          y: {
            ...chartOptions.scales.y,
            max: 100,
          },
        },
      },
    });
  }
}

async function downloadReport(testId) {
  try {
    const response = await fetch(`/api/report/${testId}`);
    if (!response.ok) {
      throw new Error("Failed to generate PDF");
    }

    const blob = await response.blob();
    const url = window.URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `loadtest_report_${testId}.pdf`;
    document.body.appendChild(a);
    a.click();
    window.URL.revokeObjectURL(url);
    document.body.removeChild(a);
  } catch (error) {
    alert("Error downloading PDF: " + error.message);
  }
}

function formatDate(dateString) {
  const date = new Date(dateString);
  return date.toLocaleString();
}

function escapeHtml(text) {
  const div = document.createElement("div");
  div.textContent = text;
  return div.innerHTML;
}

// Theme toggle functionality
function initTheme() {
  const themeToggle = document.getElementById("themeToggle");
  const themeIcon = document.getElementById("themeIcon");
  const themeText = document.getElementById("themeText");
  const html = document.documentElement;

  // Check for saved theme preference or default to dark mode
  const savedTheme = localStorage.getItem("theme") || "dark";
  html.setAttribute("data-theme", savedTheme);
  updateThemeIcon(savedTheme === "dark", themeIcon, themeText);

  themeToggle.addEventListener("click", () => {
    const currentTheme = html.getAttribute("data-theme");
    const newTheme = currentTheme === "dark" ? "light" : "dark";
    html.setAttribute("data-theme", newTheme);
    localStorage.setItem("theme", newTheme);
    updateThemeIcon(newTheme === "dark", themeIcon, themeText);
  });
}

function updateThemeIcon(isDark, icon, text) {
  if (isDark) {
    icon.innerHTML =
      '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>';
    text.textContent = "Light Mode";
  } else {
    icon.innerHTML =
      '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>';
    text.textContent = "Dark Mode";
  }
}

// Update chart colors based on theme
function updateChartColors() {
  const isDark = document.documentElement.getAttribute("data-theme") === "dark";
  const gridColor = isDark ? "rgba(255,255,255,0.05)" : "rgba(0,0,0,0.05)";
  const textColor = isDark ? "#94a3b8" : "#64748b";

  if (throughputChart) {
    throughputChart.options.scales.x.grid.color = gridColor;
    throughputChart.options.scales.y.grid.color = gridColor;
    throughputChart.options.scales.x.ticks.color = textColor;
    throughputChart.options.scales.y.ticks.color = textColor;
    throughputChart.update("none");
  }

  if (latencyChart) {
    latencyChart.options.scales.x.grid.color = gridColor;
    latencyChart.options.scales.y.grid.color = gridColor;
    latencyChart.options.scales.x.ticks.color = textColor;
    latencyChart.options.scales.y.ticks.color = textColor;
    latencyChart.update("none");
  }

  if (successRateChart) {
    successRateChart.options.scales.x.grid.color = gridColor;
    successRateChart.options.scales.y.grid.color = gridColor;
    successRateChart.options.scales.x.ticks.color = textColor;
    successRateChart.options.scales.y.ticks.color = textColor;
    successRateChart.update("none");
  }
}

// Initialize charts and load history on page load
window.addEventListener("DOMContentLoaded", () => {
  initTheme();
  initializeCharts();

  // Load history and show CTA
  loadHistory().then(() => {
    // After loading, ensure CTA is shown if no test is running
    if (!currentTestId) {
      document.getElementById("ctaSection").style.display = "block";
    }
  });

  // Update chart colors when theme changes
  const observer = new MutationObserver(() => {
    updateChartColors();
  });
  observer.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["data-theme"],
  });
});
