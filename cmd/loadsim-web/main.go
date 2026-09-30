package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"loadsim/internal/engine"
)

type Job struct {
	mu       sync.RWMutex
	Running  bool
	Result   *engine.Result
	Snapshot engine.Snapshot
}

var job = &Job{}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", healthHandler)
	mux.HandleFunc("/api/benchmark/start", startHandler)
	mux.HandleFunc("/api/benchmark/status", statusHandler)
	mux.HandleFunc("/api/benchmark/result", resultHandler)
	mux.HandleFunc("/api/benchmark/stop", stopHandler)

	webDir, err := filepath.Abs("./web")
	if err != nil {
		fmt.Println("Could not locate web directory:", err)
		return
	}

	fmt.Println("Website directory:", webDir)

	if _, err := os.Stat(filepath.Join(webDir, "index.html")); err != nil {
		fmt.Println("ERROR: web/index.html not found")
		fmt.Println("Expected:", filepath.Join(webDir, "index.html"))
		return
	}

	mux.Handle("/", http.FileServer(http.Dir(webDir)))

	server := &http.Server{
		Addr:    "127.0.0.1:8090",
		Handler: withCORS(mux),
	}

	fmt.Println("====================================")
	fmt.Println("       LoadSim Web Backend")
	fmt.Println("====================================")
	fmt.Println("Running on http://127.0.0.1:8090")
	fmt.Println()
	fmt.Println("Website:")
	fmt.Println("  http://127.0.0.1:8090/")
	fmt.Println()
	fmt.Println("API:")
	fmt.Println("  GET  /api/health")
	fmt.Println("  POST /api/benchmark/start")
	fmt.Println("  GET  /api/benchmark/status")
	fmt.Println("  GET  /api/benchmark/result")
	fmt.Println("  POST /api/benchmark/stop")
	fmt.Println("====================================")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Println("Server error:", err)
	}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"service": "LoadSim Web Backend",
		"status":  "ok",
	})
}

type BenchmarkRequest struct {
	URL         string  `json:"url"`
	Concurrency int     `json:"concurrency"`
	Requests    int64   `json:"requests"`
	Duration    int     `json:"duration"`
	Max5xx      float64 `json:"max5xx"`
	RampUp      int     `json:"rampUp"`
}

func startHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "POST required",
		})
		return
	}

	var req BenchmarkRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON",
		})
		return
	}

	if err := validateRequest(req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	job.mu.Lock()

	if job.Running {
		job.mu.Unlock()

		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "benchmark already running",
		})
		return
	}

	job.Running = true
	job.Result = nil
	job.Snapshot = engine.Snapshot{}

	job.mu.Unlock()

	cfg := engine.Config{
		URL:         req.URL,
		Concurrency: req.Concurrency,
		Requests:    req.Requests,
		Duration:    time.Duration(req.Duration) * time.Second,
		Max5xx:      req.Max5xx,
		RampUp:      req.RampUp,
	}

	go func() {
		ctx := context.Background()

		result := engine.Run(ctx, cfg, func(snapshot engine.Snapshot) {
			job.mu.Lock()
			job.Snapshot = snapshot
			job.mu.Unlock()
		})

		job.mu.Lock()
		job.Result = &result
		job.Snapshot = result.Snapshot
		job.Running = false
		job.mu.Unlock()
	}()

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"status": "started",
	})
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	job.mu.RLock()
	defer job.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"running":  job.Running,
		"snapshot": job.Snapshot,
	})
}

func resultHandler(w http.ResponseWriter, r *http.Request) {
	job.mu.RLock()
	defer job.mu.RUnlock()

	if job.Result == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "no benchmark result available",
		})
		return
	}

	writeJSON(w, http.StatusOK, job.Result.Report)
}

func stopHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "stop requested",
	})
}

func validateRequest(req BenchmarkRequest) error {
	if req.URL == "" {
		return fmt.Errorf("URL is required")
	}

	parsed, err := url.ParseRequestURI(req.URL)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("URL must use http or https")
	}

	if parsed.Hostname() == "" {
		return fmt.Errorf("invalid URL hostname")
	}

	// Local HTTP targets are allowed.
	// External/public targets must use HTTPS.
	if parsed.Scheme == "http" {
		host := parsed.Hostname()
		ip := net.ParseIP(host)

		if host != "localhost" &&
			host != "127.0.0.1" &&
			host != "::1" &&
			(ip == nil || !ip.IsLoopback()) {
			return fmt.Errorf("external targets must use HTTPS")
		}
	}

	if req.Concurrency < 1 || req.Concurrency > 5000 {
		return fmt.Errorf("concurrency must be between 1 and 5000")
	}

	if req.Requests < 1 || req.Requests > 1000000 {
		return fmt.Errorf("requests must be between 1 and 1000000")
	}

	if req.Duration < 1 || req.Duration > 600 {
		return fmt.Errorf("duration must be between 1 and 600 seconds")
	}

	if req.Max5xx < 0 || req.Max5xx > 100 {
		return fmt.Errorf("max5xx must be between 0 and 100")
	}

	if req.RampUp < 0 || req.RampUp > 600 {
		return fmt.Errorf("rampUp must be between 0 and 600 seconds")
	}

	return nil
}
	if req.URL == "" {
		return fmt.Errorf("URL is required")
	}

	parsed, err := url.ParseRequestURI(req.URL)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("URL must use http or https")
	}

	if parsed.Hostname() == "" {
		return fmt.Errorf("invalid URL hostname")
	}

	// Allow local HTTP targets and HTTPS targets.
	// HTTPS is required for external/public targets.
	if parsed.Scheme == "http" {
		host := parsed.Hostname()
		ip := net.ParseIP(host)

		if host != "localhost" &&
			host != "127.0.0.1" &&
			host != "::1" &&
			(ip == nil || !ip.IsLoopback()) {
			return fmt.Errorf("external targets must use HTTPS")
		}
	}

	if req.Concurrency < 1 || req.Concurrency > 5000 {
		return fmt.Errorf("concurrency must be between 1 and 5000")
	}

	if req.Requests < 1 || req.Requests > 1000000 {
		return fmt.Errorf("requests must be between 1 and 1000000")
	}

	if req.Duration < 1 || req.Duration > 600 {
		return fmt.Errorf("duration must be between 1 and 600 seconds")
	}

	if req.Max5xx < 0 || req.Max5xx > 100 {
		return fmt.Errorf("max5xx must be between 0 and 100")
	}

	if req.RampUp < 0 || req.RampUp > 600 {
		return fmt.Errorf("rampUp must be between 0 and 600 seconds")
	}

	return nil
}
	if req.URL == "" {
		return fmt.Errorf("URL is required")
	}

	parsed, err := url.ParseRequestURI(req.URL)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}

	host := parsed.Hostname()

	if host == "" {
		return fmt.Errorf("invalid URL hostname")
	}

	ip := net.ParseIP(host)

	if host != "localhost" &&
		host != "127.0.0.1" &&
		host != "::1" &&
		ip == nil {
		return fmt.Errorf("target must be localhost or loopback")
	}

	if ip != nil && !ip.IsLoopback() {
		return fmt.Errorf("target must be loopback")
	}

	if req.Concurrency < 1 || req.Concurrency > 5000 {
		return fmt.Errorf("concurrency must be between 1 and 5000")
	}

	if req.Requests < 1 || req.Requests > 1000000 {
		return fmt.Errorf("requests must be between 1 and 1000000")
	}

	if req.Duration < 1 || req.Duration > 600 {
		return fmt.Errorf("duration must be between 1 and 600 seconds")
	}

	if req.Max5xx < 0 || req.Max5xx > 100 {
		return fmt.Errorf("max5xx must be between 0 and 100")
	}

	if req.RampUp < 0 || req.RampUp > 600 {
		return fmt.Errorf("rampUp must be between 0 and 600 seconds")
	}

	return nil
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(value)
}
