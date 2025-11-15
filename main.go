package main

import (
	"fmt"
	"log"
	"net/http"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// Initialize database
	db, err := InitDB()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Create test manager
	testManager := NewTestManager(db)

	// Setup routes
	http.HandleFunc("/", serveIndex)
	http.HandleFunc("/api/start", testManager.HandleStartTest)
	http.HandleFunc("/api/status/", testManager.HandleGetStatus)
	http.HandleFunc("/api/metrics/", testManager.HandleGetMetrics)
	http.HandleFunc("/api/timeseries/", testManager.HandleGetTimeSeries)
	http.HandleFunc("/api/history", testManager.HandleGetHistory)
	http.HandleFunc("/api/historical-metrics/", testManager.HandleGetHistoricalMetrics)
	http.HandleFunc("/api/stop/", testManager.HandleStopTest)
	http.HandleFunc("/api/report/", testManager.HandleGenerateReport)

	// Serve static files
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	port := ":8080"
	fmt.Printf("PipeOps Load Tester running on http://localhost%s\n", port)
	log.Fatal(http.ListenAndServe(port, nil))
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/index.html")
}
