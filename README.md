*# LoadSim*



*## Lightweight HTTP Load \& Latency Benchmarking System*



*LoadSim is a high-concurrency HTTP benchmarking tool built in Go for measuring how HTTP endpoints behave under controlled load.*



*It provides latency percentiles, throughput, status-code distribution, network-error analysis, controlled ramp-up, degradation detection, and automatic safety stopping when the configured 5xx threshold is exceeded.*



*---*



*# 1. Problem*



*Developers need a simple and reliable way to understand how an HTTP service behaves when request load increases.*



*Traditional testing can make it difficult to quickly identify:*



*- Latency degradation under load*

*- Throughput limits*

*- High-tail latency such as P99*

*- HTTP 5xx failures*

*- Network-level failures*

*- The concurrency level where degradation begins*

*- Whether an endpoint is becoming unstable*



*LoadSim addresses these problems with a lightweight Go-based benchmarking engine and an easy-to-use web dashboard.*



*---*



*# 2. Solution*



*LoadSim generates controlled HTTP traffic against a target endpoint and continuously collects performance metrics.*



*The system supports:*



*- Controlled concurrency*

*- High-volume request workloads*

*- HTTP keep-alive connection pooling*

*- Concurrency ramp-up*

*- Latency percentile calculation*

*- Throughput measurement*

*- HTTP status-code tracking*

*- Network-error classification*

*- 5xx safety monitoring*

*- Degradation detection*

*- JSON result reports*

*- Browser-based monitoring*



*The same core benchmarking engine is used by both the CLI and web application.*



*---*



*# 3. Features*



*## High-Concurrency HTTP Engine*



*- Supports up to 5,000 configured concurrent workers*

*- Worker-pool based execution*

*- Efficient HTTP transport*

*- HTTP keep-alive connection reuse*

*- Controlled request scheduling*



*## Benchmark Metrics*



*LoadSim calculates:*



*- P50 latency*

*- P90 latency*

*- P99 latency*

*- Requests per second (RPS)*

*- Total requests started*

*- Total requests completed*

*- Successful requests*

*- Network errors*

*- HTTP 5xx responses*

*- HTTP status-code distribution*



*## Controlled Ramp-Up*



*LoadSim can gradually increase concurrency instead of immediately applying maximum load.*



*Example:*



*```text*

*100 → 500 → 1000 → 2000 → 3000 → 5000*

