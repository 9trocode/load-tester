package main

import (
	"bytes"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jung-kurt/gofpdf"
)

type pdfColor struct {
	R, G, B int
}

var (
	colorPrimary     = pdfColor{37, 99, 235}
	colorText        = pdfColor{15, 23, 42}
	colorMuted       = pdfColor{71, 85, 105}
	colorBorder      = pdfColor{203, 213, 225}
	colorPanel       = pdfColor{248, 250, 252}
	colorSectionFill = pdfColor{226, 232, 240}
)

// String builder pool for memory optimization
var stringBuilderPool = sync.Pool{
	New: func() interface{} {
		return &strings.Builder{}
	},
}

// Get a string builder from pool
func getStringBuilder() *strings.Builder {
	sb := stringBuilderPool.Get().(*strings.Builder)
	sb.Reset()
	return sb
}

// Return string builder to pool
func putStringBuilder(sb *strings.Builder) {
	stringBuilderPool.Put(sb)
}

type metricCard struct {
	Label  string
	Value  string
	Helper string
}

type kvRow struct {
	Label string
	Value string
}

type timeSeriesSummary struct {
	HasData            bool
	SampleCount        int
	Start              time.Time
	End                time.Time
	Duration           time.Duration
	PeakRPS            float64
	AvgRPS             float64
	MedianRPS          float64
	AvgLatency         float64
	AvgSuccessRate     float64
	LatencyPercentiles map[string]float64
}

func GeneratePDFReport(testRun *TestRun, timeSeries []TimeSeriesPoint) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 20, 15)
	pdf.SetAutoPageBreak(true, 20)
	pdf.AddPage()

	renderTitle(pdf, testRun)
	renderOverview(pdf, testRun)

	summary := analyzeTimeSeries(timeSeries)
	renderMetricCards(pdf, testRun, summary)

	if summary.HasData {
		renderTimeSeriesInsights(pdf, summary)
		renderTimeSeriesTable(pdf, timeSeries)
	} else {
		renderNoTimeSeriesMessage(pdf)
	}

	renderFooter(pdf)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderTitle(pdf *gofpdf.Fpdf, testRun *TestRun) {
	pdf.SetFillColor(colorPrimary.R, colorPrimary.G, colorPrimary.B)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Arial", "B", 20)
	pdf.CellFormat(180, 12, "PipeOps Load Test Report", "", 1, "L", true, 0, "")
	pdf.SetFont("Arial", "", 11)
	pdf.CellFormat(180, 7, fmt.Sprintf("Generated on %s", time.Now().Format("02 Jan 2006 15:04:05 MST")), "", 1, "L", true, 0, "")
	pdf.Ln(6)

	pdf.SetTextColor(colorText.R, colorText.G, colorText.B)
	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(40, 6, "Test Target:", "", 0, "L", false, 0, "")
	pdf.SetFont("Arial", "", 12)
	pdf.CellFormat(0, 6, maskTargetHost(testRun.Host), "", 1, "L", false, 0, "")
	pdf.Ln(4)

	// Add summary text
	pdf.SetFillColor(colorPanel.R, colorPanel.G, colorPanel.B)
	pdf.SetDrawColor(colorBorder.R, colorBorder.G, colorBorder.B)
	pdf.Rect(pdf.GetX(), pdf.GetY(), 180, 0, "D")
	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(colorText.R, colorText.G, colorText.B)
	summary := generateTestSummary(testRun)
	pdf.MultiCell(180, 5, summary, "", "L", false)
	pdf.Ln(2)
}

func renderOverview(pdf *gofpdf.Fpdf, testRun *TestRun) {
	renderSectionHeader(pdf, "Test Overview")

	rows := []kvRow{
		{Label: "Status", Value: titleCase(testRun.Status)},
		{Label: "Concurrent Users", Value: fmt.Sprintf("%d users", testRun.TotalUsers)},
		{Label: "Ramp-up Time", Value: formatDurationFromSeconds(testRun.RampUpSec)},
		{Label: "Planned Duration", Value: formatDurationFromSeconds(testRun.Duration)},
		{Label: "Actual Duration", Value: formatActualDuration(testRun)},
		{Label: "Run Window", Value: formatTimeWindow(testRun.StartedAt, testRun.CompletedAt)},
	}

	renderKeyValueRows(pdf, rows)
}

func renderMetricCards(pdf *gofpdf.Fpdf, testRun *TestRun, summary timeSeriesSummary) {
	renderSectionHeader(pdf, "Performance Summary")

	successRate := calculatePercentage(testRun.SuccessCount, testRun.TotalRequests)
	errorRate := calculatePercentage(testRun.ErrorCount, testRun.TotalRequests)

	metrics := []metricCard{
		{
			Label:  "Total Requests",
			Value:  formatWithCommas(testRun.TotalRequests),
			Helper: fmt.Sprintf("%s successes · %s errors", formatWithCommas(testRun.SuccessCount), formatWithCommas(testRun.ErrorCount)),
		},
		{
			Label:  "Success Rate",
			Value:  formatPercentage(successRate, 2),
			Helper: fmt.Sprintf("%s / %s requests", formatWithCommas(testRun.SuccessCount), formatWithCommas(testRun.TotalRequests)),
		},
		{
			Label:  "Error Rate",
			Value:  formatPercentage(errorRate, 2),
			Helper: fmt.Sprintf("%s / %s requests", formatWithCommas(testRun.ErrorCount), formatWithCommas(testRun.TotalRequests)),
		},
		{
			Label:  "Average Latency",
			Value:  formatLatencyValue(testRun.AvgLatency),
			Helper: formatLatencyRange(testRun.MinLatency, testRun.MaxLatency),
		},
		{
			Label:  "P95 Latency",
			Value:  formatLatencyValue(latencyPercentile(summary, "p95")),
			Helper: fmt.Sprintf("P99 %s · P50 %s", formatLatencyValue(latencyPercentile(summary, "p99")), formatLatencyValue(latencyPercentile(summary, "p50"))),
		},
		{
			Label:  "Peak RPS",
			Value:  formatFloat(summary.PeakRPS, 2),
			Helper: fmt.Sprintf("Avg %s · Reported %s", formatFloat(summary.AvgRPS, 2), formatFloat(testRun.RPS, 2)),
		},
	}

	pageWidth, _ := pdf.GetPageSize()
	left, _, right, _ := pdf.GetMargins()
	usableWidth := pageWidth - left - right
	gap := 6.0
	colWidth := (usableWidth - gap) / 2
	cardHeight := 24.0
	startX := pdf.GetX()

	for idx, metric := range metrics {
		x := pdf.GetX()
		y := pdf.GetY()

		pdf.SetDrawColor(colorBorder.R, colorBorder.G, colorBorder.B)
		pdf.SetFillColor(colorPanel.R, colorPanel.G, colorPanel.B)
		pdf.Rect(x, y, colWidth, cardHeight, "FD")

		pdf.SetFont("Arial", "", 8)
		pdf.SetTextColor(colorMuted.R, colorMuted.G, colorMuted.B)
		pdf.SetXY(x+6, y+7)
		pdf.CellFormat(colWidth-12, 4, strings.ToUpper(metric.Label), "", 0, "L", false, 0, "")

		pdf.SetFont("Arial", "B", 12)
		pdf.SetTextColor(colorText.R, colorText.G, colorText.B)
		pdf.SetXY(x+6, y+14)
		pdf.CellFormat(colWidth-12, 6, metric.Value, "", 0, "L", false, 0, "")

		if metric.Helper != "" {
			pdf.SetFont("Arial", "", 8)
			pdf.SetTextColor(colorMuted.R, colorMuted.G, colorMuted.B)
			pdf.SetXY(x+6, y+20)
			pdf.CellFormat(colWidth-12, 4, metric.Helper, "", 0, "L", false, 0, "")
		}

		if idx%2 == 0 {
			pdf.SetXY(x+colWidth+gap, y)
		} else {
			pdf.SetXY(startX, y+cardHeight+gap)
		}
	}

	if len(metrics)%2 != 0 {
		pdf.SetXY(startX, pdf.GetY()+cardHeight+gap)
	}

	pdf.Ln(2)
}

func renderTimeSeriesInsights(pdf *gofpdf.Fpdf, summary timeSeriesSummary) {
	renderSectionHeader(pdf, "Time Series Insights")

	window := formatTimestampRange(summary.Start, summary.End)
	rows := []kvRow{
		{Label: "Samples Collected", Value: fmt.Sprintf("%d data points", summary.SampleCount)},
		{Label: "Observation Window", Value: window},
	}

	if summary.Duration > 0 {
		rows = append(rows, kvRow{Label: "Observed Duration", Value: formatDurationHuman(summary.Duration)})
	}

	rows = append(rows,
		kvRow{Label: "Average Success Rate", Value: formatPercentage(summary.AvgSuccessRate, 2)},
		kvRow{Label: "Average RPS", Value: formatFloat(summary.AvgRPS, 2)},
		kvRow{Label: "Median RPS", Value: formatFloat(summary.MedianRPS, 2)},
		kvRow{Label: "Peak RPS", Value: formatFloat(summary.PeakRPS, 2)},
		kvRow{Label: "Median Latency", Value: formatLatencyValue(latencyPercentile(summary, "p50"))},
		kvRow{Label: "P95 Latency", Value: formatLatencyValue(latencyPercentile(summary, "p95"))},
		kvRow{Label: "P99 Latency", Value: formatLatencyValue(latencyPercentile(summary, "p99"))},
	)

	renderKeyValueRows(pdf, rows)
}

func renderTimeSeriesTable(pdf *gofpdf.Fpdf, points []TimeSeriesPoint) {
	if len(points) == 0 {
		return
	}

	renderSectionHeader(pdf, "Sampled Time Series")

	colWidths := []float64{40, 28, 38, 36, 38}
	headers := []string{"Timestamp", "RPS", "Avg Latency", "Success %", "Requests"}

	pdf.SetFont("Arial", "B", 9)
	pdf.SetFillColor(colorSectionFill.R, colorSectionFill.G, colorSectionFill.B)
	for idx, header := range headers {
		ln := 0
		if idx == len(headers)-1 {
			ln = 1
		}
		pdf.CellFormat(colWidths[idx], 6, header, "1", ln, "C", true, 0, "")
	}

	pdf.SetFont("Arial", "", 8)
	pdf.SetFillColor(255, 255, 255)

	maxRows := 24
	step := int(math.Ceil(float64(len(points)) / float64(maxRows)))
	if step < 1 {
		step = 1
	}

	// Pre-allocate cells array to reduce allocations
	cells := make([]string, 5)

	for i := 0; i < len(points); i += step {
		point := &points[i] // Use pointer to avoid copying
		cells[0] = point.Timestamp.Local().Format("15:04:05")
		cells[1] = formatFloat(point.RPS, 2)
		cells[2] = formatLatencyValue(point.AvgLatency)
		cells[3] = formatPercentage(point.SuccessRate, 1)
		cells[4] = formatWithCommas(point.Requests)

		for col := range headers {
			ln := 0
			if col == len(headers)-1 {
				ln = 1
			}
			pdf.CellFormat(colWidths[col], 5, cells[col], "1", ln, "C", false, 0, "")
		}
	}

	pdf.Ln(4)
}

func renderNoTimeSeriesMessage(pdf *gofpdf.Fpdf) {
	renderSectionHeader(pdf, "Time Series Insights")
	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(colorMuted.R, colorMuted.G, colorMuted.B)
	pdf.MultiCell(0, 6, "No time-series metrics were captured for this run. The report includes aggregate statistics only.", "", "L", false)
	pdf.SetTextColor(colorText.R, colorText.G, colorText.B)
	pdf.Ln(2)
}

func renderFooter(pdf *gofpdf.Fpdf) {
	pdf.SetY(-15)
	pdf.SetFont("Arial", "I", 8)
	pdf.SetTextColor(100, 116, 139)
	pdf.Cell(0, 10, fmt.Sprintf("Generated by PipeOps Load Tester • %s • pipeops.io", time.Now().Format("2006-01-02 15:04:05")))
}

func renderSectionHeader(pdf *gofpdf.Fpdf, title string) {
	pdf.SetFillColor(colorSectionFill.R, colorSectionFill.G, colorSectionFill.B)
	pdf.SetTextColor(colorText.R, colorText.G, colorText.B)
	pdf.SetFont("Arial", "B", 13)
	pdf.CellFormat(180, 8, title, "", 1, "L", true, 0, "")
	pdf.Ln(3)
}

func renderKeyValueRows(pdf *gofpdf.Fpdf, rows []kvRow) {
	pdf.SetFont("Arial", "", 10)
	for _, row := range rows {
		if strings.TrimSpace(row.Value) == "" {
			continue
		}
		pdf.SetTextColor(colorMuted.R, colorMuted.G, colorMuted.B)
		pdf.CellFormat(50, 6, row.Label, "", 0, "L", false, 0, "")
		pdf.SetTextColor(colorText.R, colorText.G, colorText.B)
		pdf.MultiCell(0, 6, row.Value, "", "L", false)
	}
	pdf.Ln(2)
}

func analyzeTimeSeries(points []TimeSeriesPoint) timeSeriesSummary {
	summary := timeSeriesSummary{
		LatencyPercentiles: make(map[string]float64, 3),
	}

	if len(points) == 0 {
		return summary
	}

	summary.HasData = true
	summary.SampleCount = len(points)
	summary.Start = points[0].Timestamp
	summary.End = points[len(points)-1].Timestamp
	if summary.End.After(summary.Start) {
		summary.Duration = summary.End.Sub(summary.Start)
	} else if summary.SampleCount > 1 {
		summary.Duration = time.Duration(summary.SampleCount-1) * time.Second
	}

	// Pre-allocate slices with exact capacity
	latencies := make([]float64, 0, len(points))
	rpsValues := make([]float64, 0, len(points))
	successRates := make([]float64, 0, len(points))

	// Single loop with direct append
	for i := range points {
		point := &points[i] // Use pointer to avoid copying
		if point.RPS > summary.PeakRPS {
			summary.PeakRPS = point.RPS
		}
		if point.RPS > 0 {
			rpsValues = append(rpsValues, point.RPS)
		}
		if point.AvgLatency > 0 {
			latencies = append(latencies, point.AvgLatency)
			summary.AvgLatency += point.AvgLatency
		}
		if point.SuccessRate >= 0 {
			successRates = append(successRates, point.SuccessRate)
			summary.AvgSuccessRate += point.SuccessRate
		}
	}

	if len(latencies) > 0 {
		summary.AvgLatency /= float64(len(latencies))
	} else {
		summary.AvgLatency = 0
	}

	if len(successRates) > 0 {
		summary.AvgSuccessRate /= float64(len(successRates))
	}

	if len(rpsValues) > 0 {
		var rpsSum float64
		for _, v := range rpsValues {
			rpsSum += v
		}
		summary.AvgRPS = rpsSum / float64(len(rpsValues))
		sort.Float64s(rpsValues)
		summary.MedianRPS = computePercentileValue(rpsValues, 0.50)
	}

	if len(latencies) > 0 {
		sort.Float64s(latencies)
		summary.LatencyPercentiles["p50"] = computePercentileValue(latencies, 0.50)
		summary.LatencyPercentiles["p95"] = computePercentileValue(latencies, 0.95)
		summary.LatencyPercentiles["p99"] = computePercentileValue(latencies, 0.99)
	}

	return summary
}

func computePercentileValue(sortedValues []float64, percentile float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}
	if percentile <= 0 {
		return sortedValues[0]
	}
	if percentile >= 1 {
		return sortedValues[len(sortedValues)-1]
	}

	position := percentile * float64(len(sortedValues)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return sortedValues[lower]
	}

	weight := position - float64(lower)
	return sortedValues[lower] + (sortedValues[upper]-sortedValues[lower])*weight
}

func maskTargetHost(host string) string {
	if strings.TrimSpace(host) == "" {
		return "-"
	}

	normalized := normalizeHost(host)
	parsed, err := url.Parse(normalized)
	if err != nil {
		return host
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "https"
	}

	sanitizedHost := parsed.Host
	if at := strings.LastIndex(sanitizedHost, "@"); at != -1 {
		sanitizedHost = sanitizedHost[at+1:]
	}
	sanitizedHost = strings.ToLower(sanitizedHost)

	base := fmt.Sprintf("%s://%s", strings.ToLower(parsed.Scheme), sanitizedHost)
	if parsed.Path != "" && parsed.Path != "/" {
		return base + "/…"
	}
	return base
}

func formatDurationFromSeconds(seconds int) string {
	if seconds <= 0 {
		return "0s"
	}
	return formatDurationHuman(time.Duration(seconds) * time.Second)
}

func formatActualDuration(testRun *TestRun) string {
	if testRun.CompletedAt == nil {
		elapsed := time.Since(testRun.StartedAt)
		if elapsed < 0 {
			elapsed = 0
		}
		return fmt.Sprintf("In progress (elapsed %s)", formatDurationHuman(elapsed))
	}

	duration := testRun.CompletedAt.Sub(testRun.StartedAt)
	if duration < 0 {
		duration = 0
	}
	return formatDurationHuman(duration)
}

func formatDurationHuman(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", int(d/time.Millisecond))
	}

	hours := d / time.Hour
	minutes := (d % time.Hour) / time.Minute
	seconds := (d % time.Minute) / time.Second

	parts := make([]string, 0, 3)
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	return strings.Join(parts, " ")
}

func formatLatencyValue(value float64) string {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return "—"
	}
	return fmt.Sprintf("%.2f ms", value)
}

func formatLatencyRange(min, max float64) string {
	minText := formatLatencyValue(min)
	maxText := formatLatencyValue(max)

	switch {
	case minText == "—" && maxText == "—":
		return "Latency range unavailable"
	case minText == "—":
		return fmt.Sprintf("max %s", maxText)
	case maxText == "—":
		return fmt.Sprintf("min %s", minText)
	default:
		return fmt.Sprintf("min %s · max %s", minText, maxText)
	}
}

func formatWithCommas(value int64) string {
	negative := value < 0
	if negative {
		value = -value
	}

	s := strconv.FormatInt(value, 10)
	if len(s) <= 3 {
		if negative {
			return "-" + s
		}
		return s
	}

	// Use pooled string builder
	sb := getStringBuilder()
	defer putStringBuilder(sb)

	if negative {
		sb.WriteByte('-')
	}

	prefix := len(s) % 3
	if prefix == 0 {
		prefix = 3
	}
	sb.WriteString(s[:prefix])
	for i := prefix; i < len(s); i += 3 {
		sb.WriteByte(',')
		sb.WriteString(s[i : i+3])
	}

	return sb.String()
}

func calculatePercentage(part, total int64) float64 {
	if total == 0 {
		return 0
	}
	return (float64(part) / float64(total)) * 100
}

func formatPercentage(value float64, decimals int) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "—"
	}
	format := fmt.Sprintf("%%.%df%%%%", decimals)
	return fmt.Sprintf(format, value)
}

func formatFloat(value float64, decimals int) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "—"
	}
	format := fmt.Sprintf("%%.%df", decimals)
	return fmt.Sprintf(format, value)
}

func latencyPercentile(summary timeSeriesSummary, key string) float64 {
	if summary.LatencyPercentiles == nil {
		return 0
	}
	return summary.LatencyPercentiles[key]
}

func titleCase(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	lowered := strings.ToLower(value)
	return strings.ToUpper(lowered[:1]) + lowered[1:]
}

func formatTimeWindow(start time.Time, end *time.Time) string {
	startText := start.Local().Format("02 Jan 2006 15:04:05 MST")
	if end == nil {
		return startText + " → ongoing"
	}
	return fmt.Sprintf("%s → %s", startText, end.Local().Format("02 Jan 2006 15:04:05 MST"))
}

func formatTimestampRange(start, end time.Time) string {
	if start.IsZero() {
		return "-"
	}
	startText := start.Local().Format("02 Jan 2006 15:04:05")
	if end.IsZero() || end.Equal(start) {
		return startText
	}
	return fmt.Sprintf("%s → %s", startText, end.Local().Format("02 Jan 2006 15:04:05"))
}

func generateTestSummary(testRun *TestRun) string {
	successRate := calculatePercentage(testRun.SuccessCount, testRun.TotalRequests)
	userText := "user"
	if testRun.TotalUsers != 1 {
		userText = "users"
	}

	// Use string builder for better performance
	sb := getStringBuilder()
	defer putStringBuilder(sb)

	sb.WriteString("Tested ")
	sb.WriteString(maskTargetHost(testRun.Host))
	sb.WriteString(" with ")
	sb.WriteString(strconv.Itoa(testRun.TotalUsers))
	sb.WriteString(" virtual ")
	sb.WriteString(userText)
	sb.WriteString(" for ")
	sb.WriteString(formatDurationFromSeconds(testRun.Duration))
	sb.WriteString(" - ")
	sb.WriteString(fmt.Sprintf("%.1f", successRate))
	sb.WriteString("% success rate, ")
	sb.WriteString(fmt.Sprintf("%.2f", testRun.RPS))
	sb.WriteString(" RPS, ")
	sb.WriteString(fmt.Sprintf("%.2f", testRun.AvgLatency))
	sb.WriteString(" ms avg latency")

	return sb.String()
}
