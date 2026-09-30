*# LoadSim*



*Lightweight, high-concurrency HTTP endpoint benchmarking CLI built in Go.*



*LoadSim is designed to measure HTTP endpoint performance under increasing concurrency while providing latency percentiles, throughput, status-code distribution, network-error analysis, degradation detection, and automatic safety stopping.*



*---*



*## Problem*



*Developers need a lightweight way to understand how an HTTP endpoint behaves under increasing load.*



*LoadSim provides a command-line load-testing engine that can:*



*- Generate high-concurrency HTTP traffic*

*- Support up to 5,000 configured concurrent workers*

*- Use HTTP keep-alive connection pooling*

*- Perform controlled concurrency ramp-up*

*- Measure p50, p90, and p99 latency*

*- Measure requests per second (RPS)*

*- Track HTTP status codes*

*- Track and classify network errors*

*- Detect performance degradation*

*- Automatically stop when the configured 5xx safety threshold is exceeded*

*- Export benchmark results as JSON*



*---*



*## Architecture*



*```text*

&#x20;               *┌──────────────────┐*

&#x20;               *│   CLI Arguments  │*

&#x20;               *└────────┬─────────┘*

&#x20;                        *│*

&#x20;                        *▼*

&#x20;               *┌──────────────────┐*

&#x20;               *│ Safety Controller│*

&#x20;               *│   5xx Monitor   │*

&#x20;               *└────────┬─────────┘*

&#x20;                        *│*

&#x20;                        *▼*

&#x20;               *┌──────────────────┐*

&#x20;               *│  Ramp Controller │*

&#x20;               *│100→500→1K→2K→3K │*

&#x20;               *│      →5K         │*

&#x20;               *└────────┬─────────┘*

&#x20;                        *│*

&#x20;                        *▼*

&#x20;               *┌──────────────────┐*

&#x20;               *│ Concurrent Worker│*

&#x20;               *│      Pool        │*

&#x20;               *└────────┬─────────┘*

&#x20;                        *│*

&#x20;                        *▼*

&#x20;               *┌──────────────────┐*

&#x20;               *│ HTTP Transport   │*

&#x20;               *│ Keep-Alive Pool  │*

&#x20;               *└────────┬─────────┘*

&#x20;                        *│*

&#x20;                        *▼*

&#x20;               *┌──────────────────┐*

&#x20;               *│   Target HTTP    │*

&#x20;               *│     Endpoint     │*

&#x20;               *└────────┬─────────┘*

&#x20;                        *│*

&#x20;                        *▼*

&#x20;               *┌──────────────────┐*

&#x20;               *│ Metrics Engine   │*

&#x20;               *│ p50/p90/p99/RPS  │*

&#x20;               *│ Status/Errors    │*

&#x20;               *└────────┬─────────┘*

&#x20;                        *│*

&#x20;                        *▼*

&#x20;               *┌──────────────────┐*

&#x20;               *│ Degradation +    │*

&#x20;               *│ JSON Reporter    │*

&#x20;               *└──────────────────┘*

