package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"loadsim/internal/engine"
)

type BenchmarkRequest struct {
	URL         string  `json:"url"`
	Concurrency int     `json:"concurrency"`
	Requests    int64   `json:"requests"`
	Duration    string  `json:"duration"`
	Max5xx      float64 `json:"max5xx"`
	RampUp      int     `json:"rampUp"`
}

type Job struct {
	ID         string
	Status     string
	Error      string
	StartedAt  time.Time
	FinishedAt time.Time

	mu     sync.RWMutex
	cancel context.CancelFunc
	result *engine.Result
}

type Server struct {
	mu      sync.RWMutex
	current *Job
}

type resultResponse struct {
	Tool                string               `json:"tool"`
	Version             string               `json:"version"`
	Target              string               `json:"target"`
	Concurrency         int                  `json:"concurrency"`
	Duration            string               `json:"duration"`
	RequestedRequests   int64                `json:"requested_requests"`
	Started             int64                `json:"started"`
	Completed           int64                `json:"completed"`
	Successful          int64                `json:"successful"`
	NetworkErrors       int64                `json:"network_errors"`
	Status5xx            int64                `json:"status_5xx"`
	RPS                 float64              `json:"rps"`
	P50                 string               `json:"p50"`
	P90                 string               `json:"p90"`
	P99                 string               `json:"p99"`
	StatusCodes         map[int]int64        `json:"status_codes"`
	NetworkErrorsByType map[string]int64     `json:"network_errors_by_type"`
	Stages              []engine.StageResult `json:"stages"`
	SafetyStopped       bool                 `json:"safety_stopped"`
	SafetyReason        string               `json:"safety_reason"`
	Degraded            bool                 `json:"degraded"`
	NetworkErrorRate    float64              `json:"network_error_rate"`
}

func main() {
	server := &Server{}

	http.HandleFunc("/api/health", server.handleHealth)
	http.HandleFunc("/api/benchmark/start", server.handleStart)
	http.HandleFunc("/api/benchmark/status", server.handleStatus)
	http.HandleFunc("/api/benchmark/result", server.handleResult)
	http.HandleFunc("/api/benchmark/stop", server.handleStop)

	// Serve the LoadSim website.
	http.Handle("/", http.FileServer(http.Dir("./web")))

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

	err := http.ListenAndServe("127.0.0.1:8090", nil)
	if err != nil {
		fmt.Println("Server error:", err)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "LoadSim Web Backend",
		"status":  "ok",
	})
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var req BenchmarkRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := validateRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	duration, err := time.ParseDuration(req.Duration)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid duration: "+err.Error())
		return
	}

	s.mu.Lock()

	if s.current != nil {
		s.current.mu.RLock()
		status := s.current.Status
		s.current.mu.RUnlock()

		if status == "running" {
			s.mu.Unlock()
			writeError(w, http.StatusConflict, "a benchmark is already running")
			return
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	job := &Job{
		ID:        strconv.FormatInt(time.Now().UnixNano(), 10),
		Status:    "started",
		StartedAt: time.Now(),
		cancel:    cancel,
	}

	s.current = job
	s.mu.Unlock()

	cfg := engine.Config{
		URL:         req.URL,
		Concurrency: req.Concurrency,
		Duration:    duration,
		Requests:    req.Requests,
		Max5xx:      req.Max5xx,
		RampUp:      req.RampUp,
		JSONOutput:  "",
	}

	job.mu.Lock()
	job.Status = "running"
	job.mu.Unlock()

	go func() {
		result := engine.Run(ctx, cfg, func(snapshot engine.Snapshot) {
			_ = snapshot
		})

		job.mu.Lock()
		job.result = &result
		job.FinishedAt = time.Now()

		if result.Snapshot.SafetyStopped {
			job.Status = "safety_stopped"
		} else {
			job.Status = "completed"
		}

		job.mu.Unlock()
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{
		"jobId":  job.ID,
		"status": "started",
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	job := s.current
	s.mu.RUnlock()

	if job == nil {
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "idle",
		})
		return
	}

	job.mu.RLock()
	defer job.mu.RUnlock()

	response := map[string]interface{}{
		"jobId":     job.ID,
		"status":    job.Status,
		"startedAt": job.StartedAt,
	}

	if !job.FinishedAt.IsZero() {
		response["finishedAt"] = job.FinishedAt
	}

	if job.Error != "" {
		response["error"] = job.Error
	}

	if job.result != nil {
		snapshot := job.result.Snapshot

		response["progress"] = map[string]interface{}{
			"started":       snapshot.Started,
			"completed":     snapshot.Completed,
			"active":        snapshot.Active,
			"successful":    snapshot.Successful,
			"networkErrors": snapshot.NetworkErrors,
			"status5xx":     snapshot.Status5xx,
			"rps":           snapshot.RPS,
			"p50":           snapshot.P50.String(),
			"p90":           snapshot.P90.String(),
			"p99":           snapshot.P99.String(),
			"safetyStopped": snapshot.SafetyStopped,
			"stopReason":    snapshot.StopReason,
			"degraded":      snapshot.Degraded,
		}
	}

	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleResult(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	job := s.current
	s.mu.RUnlock()

	if job == nil {
		writeError(w, http.StatusNotFound, "no benchmark exists")
		return
	}

	job.mu.RLock()
	defer job.mu.RUnlock()

	if job.result == nil {
		writeError(w, http.StatusConflict, "benchmark has not completed")
		return
	}

	report := job.result.Report
	snapshot := job.result.Snapshot

	errorRate := float64(0)

	if snapshot.Completed > 0 {
		errorRate =
			float64(snapshot.NetworkErrors) /
				float64(snapshot.Completed) * 100
	}

	response := resultResponse{
		Tool:                report.Tool,
		Version:             report.Version,
		Target:              report.Target,
		Concurrency:         report.Concurrency,
		Duration:            report.Duration,
		RequestedRequests:   report.Requested,
		Started:             snapshot.Started,
		Completed:           snapshot.Completed,
		Successful:          snapshot.Successful,
		NetworkErrors:       snapshot.NetworkErrors,
		Status5xx:            snapshot.Status5xx,
		RPS:                 snapshot.RPS,
		P50:                 snapshot.P50.String(),
		P90:                 snapshot.P90.String(),
		P99:                 snapshot.P99.String(),
		StatusCodes:         snapshot.StatusCodes,
		NetworkErrorsByType: snapshot.NetworkErrorsByType,
		Stages:              report.Stages,
		SafetyStopped:       snapshot.SafetyStopped,
		SafetyReason:        snapshot.StopReason,
		Degraded:            snapshot.Degraded,
		NetworkErrorRate:    errorRate,
	}

	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	s.mu.RLock()
	job := s.current
	s.mu.RUnlock()

	if job == nil {
		writeError(w, http.StatusNotFound, "no benchmark is running")
		return
	}

	job.mu.RLock()
	cancel := job.cancel
	status := job.Status
	job.mu.RUnlock()

	if status != "running" {
		writeJSON(w, http.StatusOK, map[string]string{
			"status": status,
		})
		return
	}

	if cancel != nil {
		cancel()
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "stopping",
	})
}

func validateRequest(req BenchmarkRequest) error {
	if strings.TrimSpace(req.URL) == "" {
		return fmt.Errorf("url is required")
	}

	parsed, err := url.Parse(req.URL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("only HTTP and HTTPS URLs are supported")
	}

	if !isLoopbackHost(parsed.Hostname()) {
		return fmt.Errorf(
			"for safety, LoadSim Web currently only allows localhost/loopback targets",
		)
	}

	if req.Concurrency < 1 {
		return fmt.Errorf("concurrency must be at least 1")
	}

	if req.Concurrency > 5000 {
		return fmt.Errorf("concurrency cannot exceed 5000")
	}

	if req.Requests < 0 {
		return fmt.Errorf("requests cannot be negative")
	}

	if req.Requests > 1000000 {
		return fmt.Errorf("requests cannot exceed 1000000")
	}

	if req.Duration == "" {
		req.Duration = "10s"
	}

	duration, err := time.ParseDuration(req.Duration)
	if err != nil {
		return fmt.Errorf("invalid duration: %w", err)
	}

	if duration <= 0 {
		return fmt.Errorf("duration must be greater than zero")
	}

	if duration > 10*time.Minute {
		return fmt.Errorf("duration cannot exceed 10 minutes")
	}

	if req.Max5xx < 0 || req.Max5xx > 100 {
		return fmt.Errorf("max5xx must be between 0 and 100")
	}

	if req.RampUp < 0 || req.RampUp > 600 {
		return fmt.Errorf("rampUp must be between 0 and 600 seconds")
	}

	return nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)

	if ip == nil {
		return false
	}

	return ip.IsLoopback()
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")

	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		fmt.Println("JSON response error:", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{
		"error": message,
	})
}