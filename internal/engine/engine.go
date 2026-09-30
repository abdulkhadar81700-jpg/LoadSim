package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const Version = "1.0"

type Config struct {
	URL         string
	Concurrency int
	Duration    time.Duration
	Requests    int64
	Max5xx      float64
	RampUp      int
	JSONOutput  string
}

type Metrics struct {
	mu sync.Mutex

	latencies []time.Duration

	started   int64
	completed int64
	active    int64
	success   int64

	statusCodes   map[int]int64
	networkErrors map[string]int64
}

type StageResult struct {
	Concurrency   int     `json:"concurrency"`
	Started       int64   `json:"started"`
	Completed     int64   `json:"completed"`
	Successful    int64   `json:"successful"`
	NetworkErrors int64   `json:"network_errors"`
	Status5xx     int64   `json:"status_5xx"`
	RPS           float64 `json:"rps"`
	P99           string  `json:"p99"`
	Degraded      bool    `json:"degraded"`
}

type JSONReport struct {
	Tool        string `json:"tool"`
	Version     string `json:"version"`
	Target      string `json:"target"`
	Concurrency int    `json:"concurrency"`
	Duration    string `json:"duration"`
	Requested   int64  `json:"requested_requests"`

	Started       int64 `json:"started"`
	Completed     int64 `json:"completed"`
	Successful    int64 `json:"successful"`
	NetworkErrors int64 `json:"network_errors"`
	Status5xx     int64 `json:"status_5xx"`

	RPS float64 `json:"rps"`

	P50 string `json:"p50"`
	P90 string `json:"p90"`
	P99 string `json:"p99"`

	StatusCodes         map[int]int64    `json:"status_codes"`
	NetworkErrorsByType map[string]int64 `json:"network_errors_by_type"`
	Stages              []StageResult    `json:"stages"`
}

type Snapshot struct {
	Started       int64
	Completed     int64
	Active        int64
	Successful    int64
	NetworkErrors int64
	Status5xx     int64

	RPS float64

	P50 time.Duration
	P90 time.Duration
	P99 time.Duration

	StatusCodes         map[int]int64
	NetworkErrorsByType map[string]int64

	SafetyStopped bool
	StopReason    string
	Degraded      bool
}

type resultSnapshot struct {
	started       int64
	completed     int64
	successful    int64
	networkErrors int64
	status5xx     int64
	latencies     []time.Duration
}

type Result struct {
	Report   JSONReport
	Snapshot Snapshot
}

func Run(ctx context.Context, cfg Config, onUpdate func(Snapshot)) Result {
	transport := &http.Transport{
		MaxIdleConns:        cfg.Concurrency + 100,
		MaxIdleConnsPerHost: cfg.Concurrency + 100,
		MaxConnsPerHost:     cfg.Concurrency + 100,
		IdleConnTimeout:     90 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	metrics := &Metrics{
		latencies:     make([]time.Duration, 0, 100000),
		statusCodes:   make(map[int]int64),
		networkErrors: make(map[string]int64),
	}

	runCtx, cancel := context.WithTimeout(ctx, cfg.Duration)
	defer cancel()

	stopReason := make(chan string, 1)

	startTime := time.Now()

	var stages []StageResult

	if cfg.RampUp > 0 {
		stages = runRampUp(
			runCtx,
			cancel,
			client,
			metrics,
			cfg,
			stopReason,
			onUpdate,
		)
	} else {
		runFixedConcurrency(
			runCtx,
			cancel,
			client,
			metrics,
			cfg,
			stopReason,
			onUpdate,
		)
	}

	elapsed := time.Since(startTime)

	started := atomic.LoadInt64(&metrics.started)
	completed := atomic.LoadInt64(&metrics.completed)
	successful := atomic.LoadInt64(&metrics.success)
	active := atomic.LoadInt64(&metrics.active)

	networkErrors := totalNetworkErrors(metrics)
	status5xx := total5xx(metrics)

	rps := 0.0

	if elapsed > 0 {
		rps = float64(completed) / elapsed.Seconds()
	}

	p50, p90, p99 := calculatePercentiles(metrics)

	reason := readStopReason(stopReason)

	snapshot := buildSnapshot(
		metrics,
		elapsed,
		reason != "",
		reason,
	)

	snapshot.Started = started
	snapshot.Completed = completed
	snapshot.Active = active
	snapshot.Successful = successful
	snapshot.NetworkErrors = networkErrors
	snapshot.Status5xx = status5xx
	snapshot.RPS = rps
	snapshot.P50 = p50
	snapshot.P90 = p90
	snapshot.P99 = p99

	snapshot.Degraded =
		networkErrors > 0 ||
			status5xx > 0 ||
			p99 >= 500*time.Millisecond

	report := JSONReport{
		Tool:                "LoadSim",
		Version:             Version,
		Target:              cfg.URL,
		Concurrency:         cfg.Concurrency,
		Duration:            elapsed.Round(time.Millisecond).String(),
		Requested:           cfg.Requests,
		Started:             started,
		Completed:           completed,
		Successful:          successful,
		NetworkErrors:       networkErrors,
		Status5xx:           status5xx,
		RPS:                 rps,
		P50:                 p50.String(),
		P90:                 p90.String(),
		P99:                 p99.String(),
		StatusCodes:         copyStatusCodes(metrics),
		NetworkErrorsByType: copyNetworkErrors(metrics),
		Stages:              stages,
	}

	if cfg.JSONOutput != "" {
		if err := writeJSONReport(cfg.JSONOutput, report); err != nil {
			fmt.Println("JSON report error:", err)
		}
	}

	if onUpdate != nil {
		onUpdate(snapshot)
	}

	return Result{
		Report:   report,
		Snapshot: snapshot,
	}
}

func runFixedConcurrency(
	ctx context.Context,
	cancel context.CancelFunc,
	client *http.Client,
	metrics *Metrics,
	cfg Config,
	stopReason chan<- string,
	onUpdate func(Snapshot),
) {
	var wg sync.WaitGroup

	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for {
				if ctx.Err() != nil {
					return
				}

				if !reserveRequest(metrics, cfg.Requests) {
					return
				}

				atomic.AddInt64(&metrics.active, 1)

				err := executeRequest(
					ctx,
					client,
					metrics,
					cfg.URL,
				)

				atomic.AddInt64(&metrics.active, -1)
				atomic.AddInt64(&metrics.completed, 1)

				if err != nil {
					classifyNetworkError(metrics, err)
				}

				if shouldAbort(metrics, cfg.Max5xx) {
					select {
					case stopReason <- safetyMessage(metrics, cfg.Max5xx):
						cancel()
					default:
					}

					return
				}

				sendPeriodicUpdate(metrics, onUpdate)
			}
		}()
	}

	wg.Wait()
}

func runRampUp(
	ctx context.Context,
	cancel context.CancelFunc,
	client *http.Client,
	metrics *Metrics,
	cfg Config,
	stopReason chan<- string,
	onUpdate func(Snapshot),
) []StageResult {
	stages := []int{
		100,
		500,
		1000,
		2000,
		3000,
		5000,
	}

	filtered := make([]int, 0)

	for _, stage := range stages {
		if stage <= cfg.Concurrency {
			filtered = append(filtered, stage)
		}
	}

	if len(filtered) == 0 ||
		filtered[len(filtered)-1] != cfg.Concurrency {
		filtered = append(filtered, cfg.Concurrency)
	}

	var wg sync.WaitGroup

	previousWorkers := 0

	results := make([]StageResult, 0, len(filtered))

	stageInterval := time.Duration(cfg.RampUp) *
		time.Second /
		time.Duration(len(filtered))

	if stageInterval < time.Second {
		stageInterval = time.Second
	}

	for _, stage := range filtered {
		if ctx.Err() != nil {
			break
		}

		if cfg.Requests > 0 &&
			atomic.LoadInt64(&metrics.started) >= cfg.Requests {
			break
		}

		addWorkers := stage - previousWorkers

		for i := 0; i < addWorkers; i++ {
			wg.Add(1)

			go func() {
				defer wg.Done()

				for {
					if ctx.Err() != nil {
						return
					}

					if !reserveRequest(metrics, cfg.Requests) {
						return
					}

					atomic.AddInt64(&metrics.active, 1)

					err := executeRequest(
						ctx,
						client,
						metrics,
						cfg.URL,
					)

					atomic.AddInt64(&metrics.active, -1)
					atomic.AddInt64(&metrics.completed, 1)

					if err != nil {
						classifyNetworkError(metrics, err)
					}

					if shouldAbort(metrics, cfg.Max5xx) {
						select {
						case stopReason <- safetyMessage(metrics, cfg.Max5xx):
							cancel()
						default:
						}

						return
					}

					sendPeriodicUpdate(metrics, onUpdate)
				}
			}()
		}

		previousWorkers = stage

		warmup := 250 * time.Millisecond

		select {
		case <-ctx.Done():
		case <-time.After(warmup):
		}

		if ctx.Err() != nil {
			break
		}

		before := snapshotMetrics(metrics)

		measurementDuration := stageInterval - warmup

		if measurementDuration < 500*time.Millisecond {
			measurementDuration = 500 * time.Millisecond
		}

		select {
		case <-ctx.Done():
		case <-time.After(measurementDuration):
		}

		after := snapshotMetrics(metrics)

		result := calculateStageResult(
			before,
			after,
			stage,
			measurementDuration,
		)

		results = append(results, result)

		if onUpdate != nil {
			onUpdate(
				buildSnapshot(
					metrics,
					0,
					false,
					"",
				),
			)
		}
	}

	cancel()
	wg.Wait()

	return results
}

func sendPeriodicUpdate(
	metrics *Metrics,
	onUpdate func(Snapshot),
) {
	if onUpdate == nil {
		return
	}

	completed := atomic.LoadInt64(&metrics.completed)

	if completed == 0 || completed%100 != 0 {
		return
	}

	onUpdate(
		buildSnapshot(
			metrics,
			0,
			false,
			"",
		),
	)
}

func reserveRequest(
	metrics *Metrics,
	requestTarget int64,
) bool {
	if requestTarget <= 0 {
		atomic.AddInt64(&metrics.started, 1)
		return true
	}

	for {
		current := atomic.LoadInt64(&metrics.started)

		if current >= requestTarget {
			return false
		}

		if atomic.CompareAndSwapInt64(
			&metrics.started,
			current,
			current+1,
		) {
			return true
		}
	}
}

func executeRequest(
	ctx context.Context,
	client *http.Client,
	metrics *Metrics,
	target string,
) error {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		target,
		nil,
	)

	if err != nil {
		return err
	}

	start := time.Now()

	resp, err := client.Do(req)

	latency := time.Since(start)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, resp.Body)

	metrics.mu.Lock()

	metrics.latencies = append(
		metrics.latencies,
		latency,
	)

	metrics.statusCodes[resp.StatusCode]++

	metrics.mu.Unlock()

	if resp.StatusCode >= 200 &&
		resp.StatusCode < 300 {
		atomic.AddInt64(&metrics.success, 1)
	}

	return nil
}

func snapshotMetrics(metrics *Metrics) resultSnapshot {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	latencies := make(
		[]time.Duration,
		len(metrics.latencies),
	)

	copy(latencies, metrics.latencies)

	var networkErrors int64

	for _, count := range metrics.networkErrors {
		networkErrors += count
	}

	var status5xx int64

	for code, count := range metrics.statusCodes {
		if code >= 500 && code <= 599 {
			status5xx += count
		}
	}

	return resultSnapshot{
		started:       atomic.LoadInt64(&metrics.started),
		completed:     atomic.LoadInt64(&metrics.completed),
		successful:    atomic.LoadInt64(&metrics.success),
		networkErrors: networkErrors,
		status5xx:     status5xx,
		latencies:     latencies,
	}
}

func calculateStageResult(
	before resultSnapshot,
	after resultSnapshot,
	concurrency int,
	duration time.Duration,
) StageResult {
	completed := after.completed - before.completed
	successful := after.successful - before.successful
	networkErrors := after.networkErrors - before.networkErrors
	status5xx := after.status5xx - before.status5xx

	stageLatencies := []time.Duration{}

	if len(after.latencies) > len(before.latencies) {
		stageLatencies = after.latencies[len(before.latencies):]
	}

	p99 := percentile(stageLatencies, 99)

	rps := 0.0

	if duration > 0 {
		rps = float64(completed) / duration.Seconds()
	}

	errorRate := 0.0

	if completed > 0 {
		errorRate = float64(networkErrors) /
			float64(completed) *
			100
	}

	degraded :=
		errorRate >= 5 ||
			status5xx >= 5 ||
			p99 >= 500*time.Millisecond

	return StageResult{
		Concurrency:   concurrency,
		Started:       after.started - before.started,
		Completed:     completed,
		Successful:    successful,
		NetworkErrors: networkErrors,
		Status5xx:     status5xx,
		RPS:           rps,
		P99:           p99.String(),
		Degraded:      degraded,
	}
}

func classifyNetworkError(
	metrics *Metrics,
	err error,
) {
	category := "other"

	if errors.Is(err, context.Canceled) {
		category = "canceled"
	} else if errors.Is(err, context.DeadlineExceeded) {
		category = "timeout"
	} else {
		var netErr net.Error

		if errors.As(err, &netErr) && netErr.Timeout() {
			category = "timeout"
		}

		lower := strings.ToLower(err.Error())

		switch {
		case strings.Contains(lower, "connection refused"):
			category = "connection_refused"

		case strings.Contains(lower, "connection reset"):
			category = "connection_reset"

		case strings.Contains(lower, "no such host"):
			category = "dns"

		case strings.Contains(lower, "dns"):
			category = "dns"

		case strings.Contains(lower, "too many open files"):
			category = "resource_limit"

		case strings.Contains(lower, "resource temporarily unavailable"):
			category = "resource_limit"
		}
	}

	metrics.mu.Lock()
	metrics.networkErrors[category]++
	metrics.mu.Unlock()
}

func shouldAbort(
	metrics *Metrics,
	max5xx float64,
) bool {
	totalCompleted := atomic.LoadInt64(&metrics.completed)

	if totalCompleted < 10 {
		return false
	}

	fiveXX := total5xx(metrics)

	rate := float64(fiveXX) /
		float64(totalCompleted) *
		100

	return rate > max5xx
}

func safetyMessage(
	metrics *Metrics,
	max5xx float64,
) string {
	completed := atomic.LoadInt64(&metrics.completed)
	fiveXX := total5xx(metrics)

	rate := 0.0

	if completed > 0 {
		rate = float64(fiveXX) /
			float64(completed) *
			100
	}

	return fmt.Sprintf(
		"5xx rate %.2f%% exceeded safety limit %.2f%%",
		rate,
		max5xx,
	)
}

func totalNetworkErrors(metrics *Metrics) int64 {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	var total int64

	for _, value := range metrics.networkErrors {
		total += value
	}

	return total
}

func total5xx(metrics *Metrics) int64 {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	var total int64

	for code, count := range metrics.statusCodes {
		if code >= 500 && code <= 599 {
			total += count
		}
	}

	return total
}

func calculatePercentiles(
	metrics *Metrics,
) (time.Duration, time.Duration, time.Duration) {
	metrics.mu.Lock()

	values := make(
		[]time.Duration,
		len(metrics.latencies),
	)

	copy(values, metrics.latencies)

	metrics.mu.Unlock()

	return percentile(values, 50),
		percentile(values, 90),
		percentile(values, 99)
}

func percentile(
	values []time.Duration,
	percent float64,
) time.Duration {
	if len(values) == 0 {
		return 0
	}

	sort.Slice(
		values,
		func(i, j int) bool {
			return values[i] < values[j]
		},
	)

	index := int(
		(percent / 100) *
			float64(len(values)-1),
	)

	return values[index]
}

func copyStatusCodes(
	metrics *Metrics,
) map[int]int64 {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	result := make(map[int]int64)

	for code, count := range metrics.statusCodes {
		result[code] = count
	}

	return result
}

func copyNetworkErrors(
	metrics *Metrics,
) map[string]int64 {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	result := make(map[string]int64)

	for category, count := range metrics.networkErrors {
		result[category] = count
	}

	return result
}

func buildSnapshot(
	metrics *Metrics,
	elapsed time.Duration,
	safetyStopped bool,
	stopReason string,
) Snapshot {
	p50, p90, p99 := calculatePercentiles(metrics)

	started := atomic.LoadInt64(&metrics.started)
	completed := atomic.LoadInt64(&metrics.completed)

	rps := 0.0

	if elapsed > 0 {
		rps = float64(completed) / elapsed.Seconds()
	}

	networkErrors := totalNetworkErrors(metrics)
	status5xx := total5xx(metrics)

	return Snapshot{
		Started:             started,
		Completed:           completed,
		Active:              atomic.LoadInt64(&metrics.active),
		Successful:          atomic.LoadInt64(&metrics.success),
		NetworkErrors:       networkErrors,
		Status5xx:           status5xx,
		RPS:                 rps,
		P50:                 p50,
		P90:                 p90,
		P99:                 p99,
		StatusCodes:         copyStatusCodes(metrics),
		NetworkErrorsByType: copyNetworkErrors(metrics),
		SafetyStopped:       safetyStopped,
		StopReason:          stopReason,
		Degraded: networkErrors > 0 ||
			status5xx > 0 ||
			p99 >= 500*time.Millisecond,
	}
}

func readStopReason(stopReason <-chan string) string {
	select {
	case reason := <-stopReason:
		return reason
	default:
		return ""
	}
}

func writeJSONReport(
	filename string,
	report JSONReport,
) error {
	data, err := json.MarshalIndent(
		report,
		"",
		"  ",
	)

	if err != nil {
		return err
	}

	return os.WriteFile(
		filename,
		data,
		0644,
	)
}
