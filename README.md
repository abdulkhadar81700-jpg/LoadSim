# LoadSim

## Lightweight HTTP Load \& Latency Benchmarking System

LoadSim is a high-concurrency HTTP benchmarking tool built in Go for measuring how HTTP endpoints behave under controlled load.

It provides latency percentiles, throughput, status-code distribution, network-error analysis, controlled ramp-up, degradation detection, and automatic safety stopping when the configured 5xx threshold is exceeded.

\---

# 1\. Problem

Developers need a simple and reliable way to understand how an HTTP service behaves when request load increases.

Traditional testing can make it difficult to quickly identify:

* Latency degradation under load
* Throughput limits
* High-tail latency such as P99
* HTTP 5xx failures
* Network-level failures
* The concurrency level where degradation begins
* Whether an endpoint is becoming unstable

LoadSim addresses these problems with a lightweight Go-based benchmarking engine and an easy-to-use web dashboard.

\---

# 2\. Solution

LoadSim generates controlled HTTP traffic against a target endpoint and continuously collects performance metrics.

The system supports:

* Controlled concurrency
* High-volume request workloads
* HTTP keep-alive connection pooling
* Concurrency ramp-up
* Latency percentile calculation
* Throughput measurement
* HTTP status-code tracking
* Network-error classification
* 5xx safety monitoring
* Degradation detection
* JSON result reports
* Browser-based monitoring

The same core benchmarking engine is used by both the CLI and web application.

\---

# 3\. Features

## High-Concurrency HTTP Engine

* Supports up to 5,000 configured concurrent workers
* Worker-pool based execution
* Efficient HTTP transport
* HTTP keep-alive connection reuse
* Controlled request scheduling

## Benchmark Metrics

LoadSim calculates:

* P50 latency
* P90 latency
* P99 latency
* Requests per second (RPS)
* Total requests started
* Total requests completed
* Successful requests
* Network errors
* HTTP 5xx responses
* HTTP status-code distribution

## Controlled Ramp-Up

LoadSim can gradually increase concurrency instead of immediately applying maximum load.

Example:

```text
100 → 500 → 1000 → 2000 → 3000 → 5000
```

## Automatic Safety Stop

LoadSim monitors HTTP 5xx responses during a benchmark.

If the configured 5xx threshold is exceeded, the benchmark can stop automatically.

Default threshold:

```text
10%
```

## Network Error Classification

Network failures can be classified into categories including:

* Timeout
* Connection refused
* Connection reset
* DNS error
* Resource limit
* Cancelled request
* Other

## Degradation Detection

LoadSim analyses benchmark behaviour using latency, throughput, and error information to identify significant performance degradation.

## JSON Reports

Benchmark results can be exported as JSON for analysis, documentation, and reproducibility.

## Web Dashboard

The browser dashboard provides:

* Benchmark configuration
* Live progress
* Started requests
* Completed requests
* Active requests
* Successful requests
* Network errors
* HTTP 5xx responses
* P50 / P90 / P99 latency
* RPS
* HTTP status distribution
* Network-error distribution
* Safety status
* Degradation status

\---

# 4\. Tech Stack

## Backend

* Go
* Go `net/http`
* Goroutines
* HTTP Transport
* Concurrent worker pools
* Atomic counters
* Context cancellation

## Frontend

* HTML5
* CSS3
* JavaScript
* Fetch API

## Development and Version Control

* Git
* GitHub
* PowerShell
* Go toolchain

\---

# 5\. Architecture

```text
                         ┌──────────────────────┐
                         │    Web Dashboard     │
                         │    HTML/CSS/JS       │
                         └──────────┬───────────┘
                                    │
                                    ▼
                         ┌──────────────────────┐
                         │      Go Web API      │
                         │ Start / Status /     │
                         │ Result / Stop        │
                         └──────────┬───────────┘
                                    │
                                    ▼
                         ┌──────────────────────┐
                         │    LoadSim Engine    │
                         └──────────┬───────────┘
                                    │
              ┌─────────────────────┼─────────────────────┐
              │                     │                     │
              ▼                     ▼                     ▼
      ┌──────────────┐      ┌──────────────┐      ┌──────────────┐
      │ Ramp         │      │ Worker Pool  │      │ Safety       │
      │ Controller   │      │              │      │ Controller   │
      └──────────────┘      └──────┬───────┘      └──────────────┘
                                   │
                                   ▼
                         ┌──────────────────────┐
                         │ HTTP Transport       │
                         │ Keep-Alive Pool      │
                         └──────────┬───────────┘
                                    │
                                    ▼
                         ┌──────────────────────┐
                         │ Target HTTP Endpoint │
                         └──────────┬───────────┘
                                    │
                                    ▼
                         ┌──────────────────────┐
                         │   Metrics Engine     │
                         │ P50 / P90 / P99      │
                         │ RPS / Status / Error │
                         └──────────┬───────────┘
                                    │
                                    ▼
                         ┌──────────────────────┐
                         │ Degradation Detector │
                         │ + JSON Reporter      │
                         └──────────────────────┘
```

\---

# 6\. Project Structure

```text
LoadSim/
├── cmd/
│   ├── loadsim/
│   │   └── main.go
│   └── loadsim-web/
│       └── main.go
├── internal/
│   └── engine/
│       └── engine.go
├── testserver/
│   └── main.go
├── web/
│   ├── index.html
│   ├── styles.css
│   └── app.js
├── final-50k-report.json
├── final-ramp-report-v4.json
├── safety-report.json
├── go.mod
├── .gitignore
└── README.md
```

\---

# 7\. Setup

## Requirements

* Go 1.20 or newer
* Git
* Windows, Linux, or macOS

Clone the repository:

```bash
git clone https://github.com/abdulkhadar81700-jpg/LoadSim.git
cd LoadSim
```

Verify the Go installation:

```bash
go version
```

Run the project tests:

```bash
go test ./...
```

\---

# 8\. Environment Variables

The current local demonstration does not require API keys, passwords, or external credentials.

Local services use:

```text
Web Dashboard / API: 127.0.0.1:8090
Test Server:         127.0.0.1:8080
```

No private credentials are stored in the repository.

\---

# 9\. Run Instructions

## Step 1 — Start the Test Server

From the project root:

```bash
go run ./testserver
```

The test server runs on:

```text
http://127.0.0.1:8080
```

Available endpoints:

```text
/fast
/slow
/unstable
```

### Fast endpoint

`/fast` returns a fast HTTP 200 response and is useful for throughput testing.

### Slow endpoint

`/slow` introduces an artificial response delay and is useful for demonstrating latency behaviour.

### Unstable endpoint

`/unstable` generates controlled HTTP 500 responses for demonstrating the safety-stop mechanism.

## Step 2 — Run the CLI

Example:

```bash
go run ./cmd/loadsim -url http://127.0.0.1:8080/fast -concurrency 100 -requests 50000 -duration 10
```

## Step 3 — Run a Ramp-Up Benchmark

```bash
go run ./cmd/loadsim -url http://127.0.0.1:8080/fast -concurrency 5000 -requests 50000 -duration 10 -ramp-up 10
```

## Step 4 — Generate a JSON Report

```bash
go run ./cmd/loadsim -url http://127.0.0.1:8080/fast -concurrency 100 -requests 50000 -json report.json
```

## Step 5 — Start the Web Dashboard

```bash
go run ./cmd/loadsim-web
```

Open:

```text
http://127.0.0.1:8090/
```

The web dashboard communicates with the Go backend, which invokes the same LoadSim engine used by the CLI.

\---

# 10\. Demo Link

## GitHub Repository

https://github.com/abdulkhadar81700-jpg/LoadSim

## Local Demo

```text
http://127.0.0.1:8090/
```

## Live Demo

A public deployment URL can be added here if a cloud deployment is created.

```text
LIVE DEMO: TO BE ADDED
```

\---

# 11\. Team Members

## LoadSim Team

|Name|Role|
|-|-|
|Shaik Abdulkhadar|Team Lead / System \& Backend Development|
|U. Harshitha|Team Member|
|S. Jayasri|Team Member|
|Ch. V. Kotti Reddy|Team Member|

\---

# 12\. Known Limitations

* The current web API restricts benchmark targets to localhost/loopback addresses for the demonstration environment.
* Very high concurrency can be affected by operating-system, CPU, memory, socket, and target-server limitations.
* Network-error behaviour can vary between operating systems and environments.
* Benchmark results depend on the hardware and target environment.
* Benchmark measurements should not be interpreted as universal server-capacity guarantees.
* The current implementation focuses on HTTP benchmarking rather than distributed multi-machine load generation.
* A public hosted benchmarking service would require additional security controls before allowing arbitrary external targets.

\---

# 13\. AI / Tool Disclosure

AI-assisted development tools were used during the project for permitted development activities including:

* Brainstorming
* Architecture discussion
* Technical explanations
* Code suggestions
* Debugging
* Testing assistance
* Documentation
* Troubleshooting

AI-generated suggestions were reviewed and tested during development.

The team is responsible for understanding the submitted architecture, source code, implementation decisions, testing process, and benchmark results.

No private passwords, API keys, or sensitive credentials are included in the repository.

\---

# Benchmark Evidence

LoadSim was tested using a local HTTP test server.

## 50,000 Request Benchmark

Example recorded benchmark:

```text
Target:              http://127.0.0.1:8080/fast
Concurrency:         100
Requests:            50,000
Started:             50,000
Completed:           50,000
Successful:          50,000
Network Errors:      0
5xx Responses:       0

RPS:                 9,273.29

P50:                 3.4469 ms
P90:                 22.784 ms
P99:                 126.3931 ms

Network Error Rate:  0%
```

The JSON benchmark report is stored in:

```text
final-50k-report.json
```

Additional benchmark reports:

```text
final-ramp-report-v4.json
safety-report.json
```

These are measurements from the development environment. Results can vary depending on hardware, operating system, target server, and network conditions.

\---

# Safety Demonstration

The `/unstable` test endpoint is included to demonstrate LoadSim's safety system.

When the configured HTTP 5xx threshold is exceeded, LoadSim can stop the benchmark.

This demonstrates that the system monitors target health while generating load.

\---

# Testing

Run the complete Go test suite:

```bash
go test ./...
```

The project contains:

```text
cmd/loadsim
cmd/loadsim-web
internal/engine
testserver
```

\---

# Repository

GitHub:

https://github.com/abdulkhadar81700-jpg/LoadSim

\---

# License

MIT



