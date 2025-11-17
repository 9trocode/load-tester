let currentTestId = null;
let metricsInterval = null;
let timeSeriesInterval = null;
let throughputChart = null;
let latencyChart = null;
let successRateChart = null;
let showAdvancedMetrics = false;
let expandedHistoryItems = new Set();
let testStartTime = null;
let testDurationSeconds = null;
let testUsers = null;
let collapsedHistoryItems = new Set(); // Track collapsed state (all start collapsed)

// URL and localStorage helpers
function getTestUUIDFromURL() {
  const path = window.location.pathname;
  // Check if path is /test/{uuid}
  const match = path.match(/^\/test\/([a-f0-9-]+)$/i);
  return match ? match[1] : null;
}

function setTestUUIDInURL(testUUID) {
  const url = `/test/${testUUID}`;
  window.history.pushState({}, "", url);
}

function removeTestUUIDFromURL() {
  window.history.pushState({}, "", "/");
}

// Helper to check if we're on a test-specific page
function isOnTestPage() {
  return getTestUUIDFromURL() !== null;
}

function saveTestUUIDToStorage(testUUID) {
  if (testUUID) {
    localStorage.setItem("currentTestUUID", testUUID);
  } else {
    localStorage.removeItem("currentTestUUID");
  }
}

function getTestUUIDFromStorage() {
  return localStorage.getItem("currentTestUUID");
}

// Check for running tests and resume if found
async function checkAndResumeTest() {
  console.log("[Resume] Starting test resumption check...");

  // Priority 1: URL path (/test/{uuid})
  let testUUID = getTestUUIDFromURL();
  console.log("[Resume] URL test_uuid:", testUUID);

  if (testUUID) {
    console.log("[Resume] Found test UUID in URL:", testUUID);
    await resumeTest(testUUID);
    return true;
  }

  // Priority 2: localStorage
  testUUID = getTestUUIDFromStorage();
  console.log("[Resume] localStorage test_uuid:", testUUID);

  if (testUUID) {
    console.log("[Resume] Found test UUID in localStorage:", testUUID);
    // Verify it's still running
    const isRunning = await checkIfTestRunning(testUUID);
    console.log("[Resume] Test still running?", isRunning);

    if (isRunning) {
      await resumeTest(testUUID);
      return true;
    } else {
      // Clean up if not running
      console.log("[Resume] Cleaning up localStorage (test not running)");
      saveTestUUIDToStorage(null);
    }
  }

  // Priority 3: Check for any running tests
  console.log("[Resume] Checking server for running tests...");
  try {
    const response = await fetch("/api/running");
    const data = await response.json();
    console.log("[Resume] Server running tests:", data);

    if (data.running_tests && data.running_tests.length > 0) {
      // Resume the most recent running test
      const mostRecent = data.running_tests[0];
      console.log("[Resume] Found running test:", mostRecent.test_uuid);
      await resumeTest(mostRecent.test_uuid);
      return true;
    }
  } catch (error) {
    console.error("[Resume] Error checking for running tests:", error);
  }

  console.log("[Resume] No running tests found");
  return false;
}

// Check if a specific test is still running
async function checkIfTestRunning(testUUID) {
  try {
    const response = await fetch(`/api/status/${testUUID}`);
    const data = await response.json();
    return data.is_running === true;
  } catch (error) {
    console.error("Error checking test status:", error);
    return false;
  }
}

// Resume monitoring an existing test
async function resumeTest(testUUID) {
  console.log("[Resume] Attempting to resume test:", testUUID);

  try {
    const response = await fetch(`/api/status/${testUUID}`);
    console.log("[Resume] Status response:", response.ok, response.status);

    if (!response.ok) {
      throw new Error("Test not found");
    }

    const data = await response.json();
    console.log("[Resume] Status data:", data);

    if (!data.is_running) {
      console.log("[Resume] Test", testUUID, "is no longer running");
      saveTestUUIDToStorage(null);
      removeTestUUIDFromURL();
      return;
    }

    // Set up the test
    currentTestId = testUUID;
    const testRun = data.test_run;

    testStartTime = new Date(testRun.started_at).getTime();
    testDurationSeconds = testRun.duration;
    testUsers = testRun.total_users;

    console.log("[Resume] Test config:", {
      currentTestId,
      testStartTime,
      testDurationSeconds,
      testUsers,
    });

    // Update URL and storage
    setTestUUIDInURL(testUUID);
    saveTestUUIDToStorage(testUUID);

    // Show metrics section
    domCache.ctaSection.style.display = "none";
    domCache.metricsSection.style.display = "block";
    domCache.historySection.style.display = "none"; // Hide history on test page

    // Show resumed banner
    const resumedBanner = document.getElementById("resumedBanner");
    if (resumedBanner) {
      console.log("[Resume] Showing banner");
      resumedBanner.style.display = "flex";
      // Auto-hide after 5 seconds
      setTimeout(() => {
        resumedBanner.style.display = "none";
      }, 5000);
    } else {
      console.warn("[Resume] Banner element not found!");
    }

    // Set the host URL
    domCache.currentHostUrl.textContent = maskUrl(testRun.host);
    domCache.virtualUsers.textContent = testRun.total_users;
    domCache.testDuration.textContent = formatTime(testRun.duration);

    // Reset charts
    resetCharts();

    // Start polling
    console.log("[Resume] Starting polling...");
    startMetricsPolling();
    startTimeSeriesPolling();

    console.log("[Resume] ✅ Successfully resumed test", testUUID);
  } catch (error) {
    console.error("[Resume] ❌ Error resuming test:", error);
    saveTestUUIDToStorage(null);
    removeTestUUIDFromURL();
  }
}

// DOM cache for performance
const domCache = {
  ctaSection: null,
  metricsSection: null,
  historySection: null,
  historyList: null,
  virtualUsers: null,
  elapsedTime: null,
  remainingTime: null,
  testDuration: null,
  progressPercentage: null,
  progressBarFill: null,
  currentHostUrl: null,
  totalRequests: null,
  successRate: null,
  rps: null,
  avgLatency: null,
  minLatency: null,
  maxLatency: null,
  p50Latency: null,
  p95Latency: null,
  p99Latency: null,
  errorRate: null,
  totalErrors: null,
  avgRPS: null,
};

// Initialize DOM cache
function initDOMCache() {
  domCache.ctaSection = document.getElementById("ctaSection");
  domCache.metricsSection = document.getElementById("metricsSection");
  domCache.historySection = document.getElementById("historySection");
  domCache.historyList = document.getElementById("historyList");
  domCache.virtualUsers = document.getElementById("virtualUsers");
  domCache.elapsedTime = document.getElementById("elapsedTime");
  domCache.remainingTime = document.getElementById("remainingTime");
  domCache.testDuration = document.getElementById("testDuration");
  domCache.progressPercentage = document.getElementById("progressPercentage");
  domCache.progressBarFill = document.getElementById("progressBarFill");
  domCache.currentHostUrl = document.getElementById("currentHostUrl");
  domCache.totalRequests = document.getElementById("totalRequests");
  domCache.successRate = document.getElementById("successRate");
  domCache.rps = document.getElementById("rps");
  domCache.avgLatency = document.getElementById("avgLatency");
  domCache.minLatency = document.getElementById("minLatency");
  domCache.maxLatency = document.getElementById("maxLatency");
  domCache.p50Latency = document.getElementById("p50Latency");
  domCache.p95Latency = document.getElementById("p95Latency");
  domCache.p99Latency = document.getElementById("p99Latency");
  domCache.errorRate = document.getElementById("errorRate");
  domCache.totalErrors = document.getElementById("totalErrors");
  domCache.avgRPS = document.getElementById("avgRPS");

  // Validate critical elements exist
  const missingElements = [];
  Object.keys(domCache).forEach((key) => {
    if (!domCache[key]) {
      missingElements.push(key);
    }
  });

  if (missingElements.length > 0) {
    console.error("[DOM Cache] Missing elements:", missingElements);
  } else {
    console.log("[DOM Cache] ✅ All elements found");
  }
}

// Throttle function for performance
function throttle(func, delay) {
  let lastCall = 0;
  return function (...args) {
    const now = Date.now();
    if (now - lastCall >= delay) {
      lastCall = now;
      return func.apply(this, args);
    }
  };
}

// URL masking configuration
const URL_MASK_LEVEL = {
  FULL: "full", // Show everything (no masking)
  DOMAIN: "domain", // Show only protocol and domain
  PARTIAL: "partial", // Mask subdomains, show TLD
  MINIMAL: "minimal", // Show only TLD
  HIDDEN: "hidden", // Show nothing (just asterisks)
};

// Current masking level (change this to adjust privacy)
const CURRENT_MASK_LEVEL = URL_MASK_LEVEL.DOMAIN;

// URL masking function with configurable privacy levels
function maskUrl(url, maskLevel = CURRENT_MASK_LEVEL) {
  if (!url) return "-";

  url = url.trim();

  // Parse URL
  let urlObj;
  try {
    urlObj = new URL(url);
  } catch (e) {
    // Try adding protocol if missing
    try {
      urlObj = new URL("http://" + url);
    } catch (e2) {
      // If all parsing fails, return masked placeholder
      return maskLevel === URL_MASK_LEVEL.FULL ? url : "***";
    }
  }

  const protocol = urlObj.protocol;
  const hostname = urlObj.hostname;
  const port = urlObj.port ? `:${urlObj.port}` : "";
  const pathname = urlObj.pathname;
  const search = urlObj.search;

  // Apply masking based on level
  switch (maskLevel) {
    case URL_MASK_LEVEL.FULL:
      // Show everything
      return url;

    case URL_MASK_LEVEL.DOMAIN:
      // Show protocol and full domain only (default for publishing)
      return `${protocol}//${hostname}${port}`;

    case URL_MASK_LEVEL.PARTIAL:
      // Mask subdomains, show only main domain and TLD
      const parts = hostname.split(".");
      if (parts.length > 2) {
        // Has subdomains: api.example.com -> ***.example.com
        const mainDomain = parts.slice(-2).join(".");
        return `${protocol}//***${port ? ":" + port.replace(/:\d+/, ":***") : ""}/${mainDomain}`;
      } else {
        // No subdomains: example.com -> example.com
        return `${protocol}//${hostname}${port}`;
      }

    case URL_MASK_LEVEL.MINIMAL:
      // Show only top-level domain
      const domainParts = hostname.split(".");
      const tld = domainParts[domainParts.length - 1];
      return `${protocol}//***${port ? ":***" : ""}.${tld}`;

    case URL_MASK_LEVEL.HIDDEN:
      // Hide everything
      return "***://***";

    default:
      // Fallback to DOMAIN level
      return `${protocol}//${hostname}${port}`;
  }
}

// Additional masking utilities
function maskIP(ip) {
  if (!ip) return "-";
  // Mask IP addresses: 192.168.1.1 -> ***.***.***.***
  const parts = ip.split(".");
  if (parts.length === 4) {
    return parts.map(() => "***").join(".");
  }
  return "***";
}

function maskEmail(email) {
  if (!email) return "-";
  // Mask email: user@example.com -> u***@example.com
  const parts = email.split("@");
  if (parts.length === 2) {
    const username = parts[0];
    const domain = parts[1];
    const maskedUsername =
      username.length > 2
        ? username[0] + "***"
        : username.length > 0
          ? username[0] + "***"
          : "***";
    return `${maskedUsername}@${domain}`;
  }
  return "***@***";
}

// Generate test summary text
function generateTestSummary(test) {
  const successRate =
    test.total_requests > 0
      ? ((test.success_count / test.total_requests) * 100).toFixed(1)
      : 0;

  return `Tested ${maskUrl(test.host)} with ${test.total_users} virtual user${test.total_users !== 1 ? "s" : ""} for ${test.duration}s - ${successRate}% success rate, ${test.rps.toFixed(2)} RPS, ${test.avg_latency.toFixed(2)}ms avg latency`;
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
  document.getElementById("method").value = "GET";
  document.getElementById("body").value = "";
  document.getElementById("bodyField").style.display = "none";
  document.getElementById("enableAuth").checked = false;
  document.getElementById("authConfig").style.display = "none";
  document.getElementById("enableHeaders").checked = false;
  document.getElementById("customHeaders").value = "";
  document.getElementById("headersConfig").style.display = "none";
}

// Show/hide auth config based on checkbox
function toggleAuthConfig() {
  const enableAuth = document.getElementById("enableAuth").checked;
  const authConfig = document.getElementById("authConfig");
  authConfig.style.display = enableAuth ? "block" : "none";
}

function toggleHeadersConfig() {
  const enableHeaders = document.getElementById("enableHeaders").checked;
  const headersConfig = document.getElementById("headersConfig");
  console.log("[toggleHeadersConfig] enableHeaders:", enableHeaders);
  console.log("[toggleHeadersConfig] headersConfig element:", headersConfig);
  if (headersConfig) {
    headersConfig.style.display = enableHeaders ? "block" : "none";
    console.log(
      "[toggleHeadersConfig] Set display to:",
      headersConfig.style.display,
    );
  } else {
    console.error("[toggleHeadersConfig] headersConfig element not found!");
  }
}

function toggleBodyField() {
  const method = document.getElementById("method").value;
  const bodyField = document.getElementById("bodyField");
  console.log("[toggleBodyField] method:", method);
  console.log("[toggleBodyField] bodyField element:", bodyField);
  // Show body field for POST, PUT, PATCH
  const methodsWithBody = ["POST", "PUT", "PATCH"];
  const shouldShow = methodsWithBody.includes(method);
  console.log("[toggleBodyField] shouldShow:", shouldShow);
  if (bodyField) {
    bodyField.style.display = shouldShow ? "block" : "none";
    console.log("[toggleBodyField] Set display to:", bodyField.style.display);
  } else {
    console.error("[toggleBodyField] bodyField element not found!");
  }
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
function getChartOptions(yAxisLabel = "", yAxisUnit = "", maxValue = null) {
  const isDark = document.documentElement.getAttribute("data-theme") === "dark";
  const gridColor = isDark ? "rgba(255,255,255,0.05)" : "rgba(0,0,0,0.05)";
  const textColor = isDark ? "#94a3b8" : "#64748b";

  return {
    responsive: true,
    maintainAspectRatio: true,
    interaction: {
      mode: "index",
      intersect: false,
    },
    plugins: {
      legend: {
        display: true,
        position: "top",
        align: "end",
        labels: {
          color: textColor,
          font: {
            size: 12,
            weight: "500",
          },
          padding: 10,
          usePointStyle: true,
          pointStyle: "circle",
        },
      },
      tooltip: {
        enabled: true,
        backgroundColor: isDark
          ? "rgba(30, 41, 59, 0.95)"
          : "rgba(255, 255, 255, 0.95)",
        titleColor: textColor,
        bodyColor: textColor,
        borderColor: gridColor,
        borderWidth: 1,
        padding: 12,
        displayColors: true,
        callbacks: {
          label: function (context) {
            let label = context.dataset.label || "";
            if (label) {
              label += ": ";
            }
            if (context.parsed.y !== null) {
              label += context.parsed.y.toLocaleString("en-US", {
                minimumFractionDigits: 2,
                maximumFractionDigits: 2,
              });
              if (yAxisUnit) {
                label += " " + yAxisUnit;
              }
            }
            return label;
          },
        },
      },
    },
    scales: {
      y: {
        beginAtZero: true,
        max: maxValue,
        grid: {
          color: gridColor,
          borderWidth: 0,
        },
        ticks: {
          color: textColor,
          font: {
            size: 11,
          },
          callback: function (value) {
            if (yAxisUnit) {
              return value.toLocaleString() + " " + yAxisUnit;
            }
            return value.toLocaleString();
          },
        },
        title: {
          display: yAxisLabel ? true : false,
          text: yAxisLabel,
          color: textColor,
          font: {
            size: 12,
            weight: "600",
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
          maxRotation: 0,
          autoSkip: true,
          maxTicksLimit: 10,
        },
        title: {
          display: true,
          text: "Time (seconds)",
          color: textColor,
          font: {
            size: 12,
            weight: "600",
          },
        },
      },
    },
  };
}

function initializeCharts() {
  console.log("[Charts] Initializing charts...");

  try {
    // Throughput Chart
    const throughputCanvas = document.getElementById("throughputChart");
    if (!throughputCanvas) {
      console.error("[Charts] Throughput chart canvas not found!");
      return;
    }
    const throughputCtx = throughputCanvas.getContext("2d");
    throughputChart = new Chart(throughputCtx, {
      type: "line",
      data: {
        labels: [],
        datasets: [
          {
            label: "Requests/sec",
            data: [],
            borderColor: "rgb(59, 130, 246)",
            backgroundColor: "rgba(59, 130, 246, 0.1)",
            tension: 0.4,
            fill: true,
            borderWidth: 2,
            pointRadius: 0,
            pointHoverRadius: 6,
            pointHoverBackgroundColor: "rgb(59, 130, 246)",
            pointHoverBorderColor: "#fff",
            pointHoverBorderWidth: 2,
          },
        ],
      },
      options: getChartOptions("Throughput", "req/s"),
    });

    // Latency Chart
    const latencyCanvas = document.getElementById("latencyChart");
    if (!latencyCanvas) {
      console.error("[Charts] Latency chart canvas not found!");
      return;
    }
    const latencyCtx = latencyCanvas.getContext("2d");
    latencyChart = new Chart(latencyCtx, {
      type: "line",
      data: {
        labels: [],
        datasets: [
          {
            label: "Latency (ms)",
            data: [],
            borderColor: "rgb(16, 185, 129)",
            backgroundColor: "rgba(16, 185, 129, 0.1)",
            tension: 0.4,
            fill: true,
            borderWidth: 2,
            pointRadius: 0,
            pointHoverRadius: 6,
            pointHoverBackgroundColor: "rgb(16, 185, 129)",
            pointHoverBorderColor: "#fff",
            pointHoverBorderWidth: 2,
          },
        ],
      },
      options: getChartOptions("Response Time", "ms"),
    });

    // Success Rate Chart
    const successRateCanvas = document.getElementById("successRateChart");
    if (!successRateCanvas) {
      console.error("[Charts] Success rate chart canvas not found!");
      return;
    }
    const successRateCtx = successRateCanvas.getContext("2d");
    successRateChart = new Chart(successRateCtx, {
      type: "line",
      data: {
        labels: [],
        datasets: [
          {
            label: "Success Rate (%)",
            data: [],
            borderColor: "rgb(139, 92, 246)",
            backgroundColor: "rgba(139, 92, 246, 0.1)",
            tension: 0.4,
            fill: true,
            borderWidth: 2,
            pointRadius: 0,
            pointHoverRadius: 6,
            pointHoverBackgroundColor: "rgb(139, 92, 246)",
            pointHoverBorderColor: "#fff",
            pointHoverBorderWidth: 2,
          },
        ],
      },
      options: getChartOptions("Success Rate", "%", 100),
    });

    console.log("[Charts] ✅ All charts initialized successfully");
  } catch (error) {
    console.error("[Charts] Error initializing charts:", error);
  }
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
document
  .getElementById("enableHeaders")
  .addEventListener("change", toggleHeadersConfig);
document.getElementById("method").addEventListener("change", toggleBodyField);

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

  if (rampUp < 0 || rampUp > 300) {
    alert("Ramp-up time must be between 0 and 300 seconds");
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

  // Get HTTP method
  const method = document.getElementById("method").value || "GET";

  // Get request body
  const bodyField = document.getElementById("body").value.trim();
  let requestBodyPayload = null;
  if (
    bodyField &&
    (method === "POST" || method === "PUT" || method === "PATCH")
  ) {
    // Validate JSON if provided
    try {
      JSON.parse(bodyField);
      requestBodyPayload = bodyField;
    } catch (e) {
      alert("Invalid JSON in request body: " + e.message);
      return;
    }
  }

  // Get custom headers
  const enableHeaders = document.getElementById("enableHeaders").checked;
  let customHeaders = null;
  if (enableHeaders) {
    const headersField = document.getElementById("customHeaders").value.trim();
    if (headersField) {
      try {
        customHeaders = JSON.parse(headersField);
        if (typeof customHeaders !== "object" || Array.isArray(customHeaders)) {
          alert("Headers must be a JSON object");
          return;
        }
      } catch (e) {
        alert("Invalid JSON in custom headers: " + e.message);
        return;
      }
    }
  }

  const requestBody = { host, users, ramp_up_sec: rampUp, duration };
  if (auth) {
    requestBody.auth = auth;
  }
  if (method && method !== "GET") {
    requestBody.method = method;
  }
  if (requestBodyPayload) {
    requestBody.body = requestBodyPayload;
  }
  if (customHeaders) {
    requestBody.headers = customHeaders;
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
    currentTestId = data.test_uuid; // Use UUID instead of numeric ID

    // Save test UUID to URL and localStorage
    setTestUUIDInURL(currentTestId);
    saveTestUUIDToStorage(currentTestId);

    // Store test configuration
    testStartTime = Date.now();
    testDurationSeconds = duration;
    testUsers = users;

    // Store host for display
    document.getElementById("currentHostUrl").textContent = maskUrl(host);

    // Update overview fields using cache
    domCache.virtualUsers.textContent = users;
    domCache.testDuration.textContent = duration + "s";
    domCache.elapsedTime.textContent = "0s";
    domCache.remainingTime.textContent = duration + "s";
    domCache.progressPercentage.textContent = "0%";
    domCache.progressBarFill.style.width = "0%";

    closeTestModal();
    domCache.ctaSection.style.display = "none";
    domCache.metricsSection.style.display = "block";
    domCache.historySection.style.display = "none"; // Always hide history on test page

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
  console.log("[Polling] Starting metrics polling for test:", currentTestId);

  if (metricsInterval) {
    clearInterval(metricsInterval);
  }

  // Show last updated indicator
  const lastUpdatedEl = document.getElementById("lastUpdated");
  if (lastUpdatedEl) {
    lastUpdatedEl.style.display = "block";
  }

  metricsInterval = setInterval(async () => {
    if (!currentTestId) {
      console.warn("[Polling] No current test ID, stopping polling");
      return;
    }

    try {
      console.log("[Polling] Fetching metrics for:", currentTestId);
      const response = await fetch(`/api/metrics/${currentTestId}`);

      if (!response.ok) {
        console.error(
          "[Polling] Failed to fetch metrics:",
          response.status,
          response.statusText,
        );
        stopMetricsPolling();
        return;
      }

      const metrics = await response.json();
      console.log("[Polling] Received metrics:", {
        total_requests: metrics.total_requests,
        rps: metrics.rps,
        is_running: metrics.is_running,
      });

      updateMetrics(metrics);

      if (!metrics.is_running) {
        console.log("[Polling] Test completed, stopping polling");
        stopMetricsPolling();
        stopTimeSeriesPolling();

        // Hide last updated indicator
        const lastUpdatedEl = document.getElementById("lastUpdated");
        if (lastUpdatedEl) {
          lastUpdatedEl.style.display = "none";
        }

        if (domCache.ctaSection) domCache.ctaSection.style.display = "block";
        if (domCache.metricsSection)
          domCache.metricsSection.style.display = "none";

        // Only show history if not on a test-specific page
        if (domCache.historySection) {
          domCache.historySection.style.display = isOnTestPage()
            ? "none"
            : "block";
        }

        currentTestId = null;
        testStartTime = null;
        testDurationSeconds = null;
        testUsers = null;
        // Clean up URL and storage when test completes
        removeTestUUIDFromURL();
        saveTestUUIDToStorage(null);
        loadHistory();
      }
    } catch (error) {
      console.error("[Polling] Error fetching metrics:", error);
    }
  }, 1000);
}

function startTimeSeriesPolling() {
  console.log(
    "[Polling] Starting time series polling for test:",
    currentTestId,
  );

  if (timeSeriesInterval) {
    clearInterval(timeSeriesInterval);
  }

  timeSeriesInterval = setInterval(async () => {
    if (!currentTestId) return;

    try {
      const response = await fetch(`/api/timeseries/${currentTestId}`);
      if (!response.ok) {
        console.error(
          "[Polling] Failed to fetch time series:",
          response.status,
        );
        return;
      }

      const timeSeries = await response.json();
      console.log(
        "[Polling] Received time series data points:",
        timeSeries.length,
      );
      updateCharts(timeSeries);
    } catch (error) {
      console.error("[Polling] Error fetching time series:", error);
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

// Throttled update for better performance
const updateMetricsThrottled = throttle(function (metrics) {
  if (!metrics) {
    console.error("[Metrics] No metrics data received");
    return;
  }

  console.log("[Metrics] Updating with data:", {
    total_requests: metrics.total_requests,
    rps: metrics.rps,
    is_running: metrics.is_running,
  });

  // Update last updated timestamp
  const lastUpdatedEl = document.getElementById("lastUpdated");
  const lastUpdatedTimeEl = document.getElementById("lastUpdatedTime");
  if (lastUpdatedEl && lastUpdatedTimeEl) {
    lastUpdatedEl.style.display = "block";
    const now = new Date();
    lastUpdatedTimeEl.textContent = now.toLocaleTimeString();
  }

  // Update live overview (elapsed time, remaining time, progress)
  if (testStartTime && testDurationSeconds) {
    const elapsedSeconds = Math.floor((Date.now() - testStartTime) / 1000);
    const remainingSeconds = Math.max(0, testDurationSeconds - elapsedSeconds);
    const progressPercent = Math.min(
      100,
      (elapsedSeconds / testDurationSeconds) * 100,
    );

    if (domCache.elapsedTime) {
      domCache.elapsedTime.textContent = formatTime(elapsedSeconds);
    }
    if (domCache.remainingTime) {
      domCache.remainingTime.textContent = formatTime(remainingSeconds);
    }
    if (domCache.progressPercentage) {
      domCache.progressPercentage.textContent =
        progressPercent.toFixed(1) + "%";
    }
    if (domCache.progressBarFill) {
      domCache.progressBarFill.style.width = progressPercent + "%";
    }
  }

  // Batch DOM updates
  requestAnimationFrame(() => {
    try {
      // Basic metrics with null checks
      if (domCache.totalRequests) {
        domCache.totalRequests.textContent = (
          metrics.total_requests || 0
        ).toLocaleString();
      }

      const successRate =
        metrics.total_requests > 0
          ? ((metrics.success_count / metrics.total_requests) * 100).toFixed(1)
          : 0;
      if (domCache.successRate) {
        domCache.successRate.textContent = successRate + "%";
      }

      if (domCache.rps) {
        domCache.rps.textContent =
          (metrics.rps || 0).toLocaleString("en-US", {
            minimumFractionDigits: 2,
            maximumFractionDigits: 2,
          }) + " req/s";
      }
      if (domCache.avgLatency) {
        domCache.avgLatency.textContent =
          (metrics.avg_latency || 0).toLocaleString("en-US", {
            minimumFractionDigits: 2,
            maximumFractionDigits: 2,
          }) + " ms";
      }
      if (domCache.minLatency) {
        domCache.minLatency.textContent =
          (metrics.min_latency || 0).toLocaleString("en-US", {
            minimumFractionDigits: 2,
            maximumFractionDigits: 2,
          }) + " ms";
      }
      if (domCache.maxLatency) {
        domCache.maxLatency.textContent =
          (metrics.max_latency || 0).toLocaleString("en-US", {
            minimumFractionDigits: 2,
            maximumFractionDigits: 2,
          }) + " ms";
      }

      // Advanced metrics
      if (domCache.p50Latency) {
        domCache.p50Latency.textContent =
          (metrics.p50_latency || 0).toLocaleString("en-US", {
            minimumFractionDigits: 2,
            maximumFractionDigits: 2,
          }) + " ms";
      }
      if (domCache.p95Latency) {
        domCache.p95Latency.textContent =
          (metrics.p95_latency || 0).toLocaleString("en-US", {
            minimumFractionDigits: 2,
            maximumFractionDigits: 2,
          }) + " ms";
      }
      if (domCache.p99Latency) {
        domCache.p99Latency.textContent =
          (metrics.p99_latency || 0).toLocaleString("en-US", {
            minimumFractionDigits: 2,
            maximumFractionDigits: 2,
          }) + " ms";
      }
      if (domCache.errorRate) {
        domCache.errorRate.textContent =
          (metrics.error_rate || 0).toLocaleString("en-US", {
            minimumFractionDigits: 2,
            maximumFractionDigits: 2,
          }) + "%";
      }
      if (domCache.totalErrors) {
        domCache.totalErrors.textContent = (
          metrics.error_count || 0
        ).toLocaleString();
      }
      if (domCache.avgRPS) {
        domCache.avgRPS.textContent =
          (metrics.avg_rps || 0).toLocaleString("en-US", {
            minimumFractionDigits: 2,
            maximumFractionDigits: 2,
          }) + " req/s";
      }

      console.log("[Metrics] ✅ DOM updated successfully");
    } catch (error) {
      console.error("[Metrics] Error updating DOM:", error);
    }
  });
}, 100); // Throttle to max 10 updates per second

function updateMetrics(metrics) {
  updateMetricsThrottled(metrics);
}

function formatTime(seconds) {
  if (seconds < 60) {
    return seconds + "s";
  }
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  return minutes + "m " + remainingSeconds + "s";
}

// Throttled chart updates
const updateChartsThrottled = throttle(function (timeSeries) {
  if (!timeSeries || timeSeries.length === 0) {
    console.log("[Charts] No time series data to display");
    return;
  }

  console.log("[Charts] Updating with", timeSeries.length, "data points");

  const recentData = timeSeries.slice(-60);

  // Pre-allocate arrays for better performance
  const dataLength = recentData.length;
  const labels = new Array(dataLength);
  const rpsData = new Array(dataLength);
  const latencyData = new Array(dataLength);
  const successRateData = new Array(dataLength);

  // Single loop instead of multiple map calls
  for (let i = 0; i < dataLength; i++) {
    const point = recentData[i];
    labels[i] = new Date(point.timestamp).toLocaleTimeString();
    rpsData[i] = point.rps;
    latencyData[i] = point.avg_latency;
    successRateData[i] = point.success_rate;
  }

  // Batch chart updates
  requestAnimationFrame(() => {
    try {
      if (throughputChart) {
        throughputChart.data.labels = labels;
        throughputChart.data.datasets[0].data = rpsData;
        throughputChart.update("none");
      } else {
        console.warn("[Charts] Throughput chart not initialized");
      }

      if (latencyChart) {
        latencyChart.data.labels = labels;
        latencyChart.data.datasets[0].data = latencyData;
        latencyChart.update("none");
      } else {
        console.warn("[Charts] Latency chart not initialized");
      }

      if (successRateChart) {
        successRateChart.data.labels = labels;
        successRateChart.data.datasets[0].data = successRateData;
        successRateChart.update("none");
      } else {
        console.warn("[Charts] Success rate chart not initialized");
      }

      console.log("[Charts] ✅ Charts updated successfully");
    } catch (error) {
      console.error("[Charts] Error updating charts:", error);
    }
  });
}, 500); // Throttle to max 2 updates per second

function updateCharts(timeSeries) {
  updateChartsThrottled(timeSeries);
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
    // Hide history section on error
    domCache.historySection.style.display = "none";
    // Show CTA if no test is running
    if (!currentTestId) {
      domCache.ctaSection.style.display = "block";
    }
  }
}

function displayHistory(history) {
  if (history.length === 0) {
    // Hide history section completely when there are no tests at all
    domCache.historySection.style.display = "none";
    if (!currentTestId) {
      domCache.ctaSection.style.display = "block";
    }
    return;
  }

  // Show CTA if there's 1 or more history items and no test is running
  if (!currentTestId) {
    domCache.ctaSection.style.display = "block";
    // Only show history if not on a test-specific page
    domCache.historySection.style.display = isOnTestPage() ? "none" : "block";
  }

  // Use DocumentFragment for better performance
  const fragment = document.createDocumentFragment();
  const tempDiv = document.createElement("div");

  // Build HTML in single pass
  const htmlParts = new Array(history.length);
  for (let i = 0; i < history.length; i++) {
    const test = history[i];
    const successRate =
      test.total_requests > 0
        ? ((test.success_count / test.total_requests) * 100).toFixed(1)
        : 0;

    htmlParts[i] = `
        <div class="history-item" data-test-id="${test.id}" data-test-uuid="${test.uuid}">
            <div class="history-item-header">
                <div class="history-item-url">${escapeHtml(maskUrl(test.host))}</div>
                <div class="history-item-meta">
                    <span class="status-badge status-${test.status}">${test.status}</span>
                    <span class="history-item-time">${formatDate(test.started_at)}</span>
                </div>
            </div>
            <div class="history-item-summary">
                <p class="summary-text">${escapeHtml(generateTestSummary(test))}</p>
                <span class="expand-indicator">▼ Click to view details</span>
            </div>
            <div class="history-item-details" id="details-${test.id}" style="display: none;">
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
                        <span class="history-metric-value">${successRate}%</span>
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
                    <button class="btn btn-secondary btn-sm" data-action="advanced" data-test-id="${test.id}" data-test-uuid="${test.uuid}">
                        Advanced View
                    </button>
                    <button class="btn btn-secondary btn-sm" data-action="download" data-test-id="${test.id}" data-test-uuid="${test.uuid}">
                        Download Report
                    </button>
                </div>
                <div id="advanced-view-${test.id}" class="history-advanced-view" style="display: none;">
                    <div class="loading-state">Loading advanced metrics...</div>
                </div>
            </div>
        </div>
    `;
  }

  tempDiv.innerHTML = htmlParts.join("");
  while (tempDiv.firstChild) {
    fragment.appendChild(tempDiv.firstChild);
  }

  // Single DOM update
  domCache.historyList.innerHTML = "";
  domCache.historyList.appendChild(fragment);
}

function toggleHistoryDetails(testId) {
  const detailsDiv = document.getElementById(`details-${testId}`);
  const historyItem = document.querySelector(
    `[data-test-id="${testId}"].history-item`,
  );
  const expandIndicator = historyItem.querySelector(".expand-indicator");

  if (collapsedHistoryItems.has(testId)) {
    // Expand
    detailsDiv.style.display = "block";
    collapsedHistoryItems.delete(testId);
    expandIndicator.textContent = "▲ Click to hide details";
    historyItem.classList.add("expanded");
  } else {
    // Collapse
    detailsDiv.style.display = "none";
    collapsedHistoryItems.add(testId);
    expandIndicator.textContent = "▼ Click to view details";
    historyItem.classList.remove("expanded");
  }
}

async function toggleAdvancedView(testId, testUUID) {
  const advancedView = document.getElementById(`advanced-view-${testId}`);
  const button = document.querySelector(
    `[data-action="advanced"][data-test-id="${testId}"]`,
  );

  if (expandedHistoryItems.has(testId)) {
    // Collapse
    advancedView.style.display = "none";
    expandedHistoryItems.delete(testId);
    if (button) button.textContent = "Advanced View";
  } else {
    // Expand
    advancedView.style.display = "block";
    expandedHistoryItems.add(testId);
    if (button) button.textContent = "Hide Advanced";

    // Load advanced metrics if not already loaded
    if (!advancedView.dataset.loaded) {
      await loadAdvancedMetrics(testId, testUUID);
    }
  }
}

async function loadAdvancedMetrics(testId, testUUID) {
  const advancedView = document.getElementById(`advanced-view-${testId}`);

  try {
    const response = await fetch(`/api/historical-metrics/${testUUID}`);
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
            label: "Throughput",
            data: rpsData,
            borderColor: "#3b82f6",
            backgroundColor: "rgba(59, 130, 246, 0.1)",
            tension: 0.4,
            fill: true,
            borderWidth: 2.5,
            pointRadius: 0,
            pointHoverRadius: 4,
            pointHoverBackgroundColor: "#3b82f6",
            pointHoverBorderColor: "#fff",
            pointHoverBorderWidth: 2,
          },
        ],
      },
      options: getChartOptions("Requests per Second", "req/s"),
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
            label: "Response Time",
            data: latencyData,
            borderColor: "#10b981",
            backgroundColor: "rgba(16, 185, 129, 0.1)",
            tension: 0.4,
            fill: true,
            borderWidth: 2.5,
            pointRadius: 0,
            pointHoverRadius: 4,
            pointHoverBackgroundColor: "#10b981",
            pointHoverBorderColor: "#fff",
            pointHoverBorderWidth: 2,
          },
        ],
      },
      options: getChartOptions("Average Latency", "ms"),
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
            label: "Success Rate",
            data: successRateData,
            borderColor: "#f59e0b",
            backgroundColor: "rgba(245, 158, 11, 0.1)",
            tension: 0.4,
            fill: true,
            borderWidth: 2.5,
            pointRadius: 0,
            pointHoverRadius: 4,
            pointHoverBackgroundColor: "#f59e0b",
            pointHoverBorderColor: "#fff",
            pointHoverBorderWidth: 2,
          },
        ],
      },
      options: getChartOptions("Success Rate", "%", 100),
    });
  }
}

async function downloadReport(testId, testUUID) {
  try {
    const response = await fetch(`/api/report/${testUUID}`);
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

// Event delegation for history items
function setupEventDelegation() {
  domCache.historyList.addEventListener("click", (e) => {
    const historyItem = e.target.closest(".history-item");
    if (!historyItem) return;

    const testId = parseInt(historyItem.dataset.testId);
    const testUUID = historyItem.dataset.testUuid;

    // Handle button clicks
    if (e.target.tagName === "BUTTON") {
      const action = e.target.dataset.action;
      if (action === "advanced") {
        toggleAdvancedView(testId, testUUID);
      } else if (action === "download") {
        downloadReport(testId, testUUID);
      }
      return;
    }

    // Handle item click (expand/collapse)
    if (!e.target.closest(".history-item-actions")) {
      toggleHistoryDetails(testId);
    }
  });
}

// Real-time input validation
function setupInputValidation() {
  const usersInput = document.getElementById("users");
  const rampUpInput = document.getElementById("rampUp");
  const durationInput = document.getElementById("duration");

  function validateInput(input, min, max, fieldName) {
    const value = parseInt(input.value);

    if (isNaN(value) || value < min || value > max) {
      input.setCustomValidity(`${fieldName} must be between ${min} and ${max}`);
      input.style.borderColor = "#ef4444";
    } else {
      input.setCustomValidity("");
      input.style.borderColor = "";
    }
  }

  usersInput.addEventListener("input", () => {
    validateInput(usersInput, 1, 1000, "Users");
  });

  rampUpInput.addEventListener("input", () => {
    validateInput(rampUpInput, 0, 300, "Ramp-up time");

    // Also check if ramp-up exceeds duration
    const duration = parseInt(durationInput.value);
    const rampUp = parseInt(rampUpInput.value);
    if (!isNaN(duration) && !isNaN(rampUp) && rampUp > duration) {
      rampUpInput.setCustomValidity("Ramp-up time cannot exceed test duration");
      rampUpInput.style.borderColor = "#ef4444";
    }
  });

  durationInput.addEventListener("input", () => {
    validateInput(durationInput, 1, 300, "Duration");

    // Re-validate ramp-up when duration changes
    const duration = parseInt(durationInput.value);
    const rampUp = parseInt(rampUpInput.value);
    if (!isNaN(duration) && !isNaN(rampUp) && rampUp > duration) {
      rampUpInput.setCustomValidity("Ramp-up time cannot exceed test duration");
      rampUpInput.style.borderColor = "#ef4444";
    } else {
      rampUpInput.setCustomValidity("");
      rampUpInput.style.borderColor = "";
    }
  });
}

// Initialize charts and load history on page load
window.addEventListener("DOMContentLoaded", async () => {
  console.log("[Init] Page loaded, initializing...");

  // Initialize DOM cache first
  initDOMCache();
  console.log("[Init] DOM cache initialized");

  initTheme();
  initializeCharts();
  console.log("[Init] Theme and charts initialized");

  // Setup input validation
  setupInputValidation();
  console.log("[Init] Input validation initialized");

  // Setup event delegation
  setupEventDelegation();

  // Initialize form field visibility
  toggleBodyField();
  toggleHeadersConfig();

  // Check for running tests and resume if found
  console.log("[Init] Checking for running tests...");
  const resumed = await checkAndResumeTest();
  console.log("[Init] Resumed?", resumed);

  // Load history
  console.log("[Init] Loading history...");
  await loadHistory();

  // Show CTA if no test is running
  if (!resumed && !currentTestId) {
    console.log("[Init] Showing CTA");
    domCache.ctaSection.style.display = "block";
  } else {
    console.log("[Init] Test active, hiding CTA");
  }

  console.log("[Init] ✅ Initialization complete");

  // Update chart colors when theme changes
  const observer = new MutationObserver(() => {
    updateChartColors();
  });
  observer.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["data-theme"],
  });
});
