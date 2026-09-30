const API = window.location.origin;

const $ = (id) => document.getElementById(id);

let pollTimer = null;

function setError(message) {
    const box = $("errorBox");
    if (!box) return;

    box.textContent = message || "";
    box.style.display = message ? "block" : "none";
}

function setBackendStatus(connected) {
    const el = $("backendStatus");
    if (!el) return;

    if (connected) {
        el.textContent = "● Backend connected";
        el.classList.remove("offline");
        el.classList.add("online");
    } else {
        el.textContent = "● Backend disconnected";
        el.classList.remove("online");
        el.classList.add("offline");
    }
}

async function api(path, options = {}) {
    const response = await fetch(`${API}${path}`, {
        ...options,
        headers: {
            "Content-Type": "application/json",
            ...(options.headers || {})
        }
    });

    const text = await response.text();

    let data;

    try {
        data = text ? JSON.parse(text) : {};
    } catch {
        throw new Error(
            `Backend returned invalid JSON (${response.status}): ${text.slice(0, 200)}`
        );
    }

    if (!response.ok) {
        throw new Error(data.error || `Request failed (${response.status})`);
    }

    return data;
}

async function checkBackend() {
    try {
        const data = await api("/api/health");
        setBackendStatus(data.status === "ok");
    } catch (error) {
        setBackendStatus(false);
        console.error(error);
    }
}

function getNumber(id, fallback) {
    const value = Number($(id)?.value);
    return Number.isFinite(value) ? value : fallback;
}

function getFormData() {
    return {
        url: $("targetUrl").value.trim(),
        concurrency: getNumber("concurrency", 100),
        requests: getNumber("requests", 50000),
        duration: getNumber("duration", 10),
        max5xx: getNumber("max5xx", 10),
        rampUp: getNumber("rampUp", 0)
    };
}

function formatNumber(value) {
    if (value === undefined || value === null) return "0";
    return Number(value).toLocaleString();
}

function formatDuration(value) {
    if (!value) return "0 ms";
    return value;
}

function updateText(id, value) {
    const el = $(id);
    if (el) el.textContent = value;
}

function updateSnapshot(snapshot) {
    if (!snapshot) return;

    updateText("started", formatNumber(snapshot.Started));
    updateText("completed", formatNumber(snapshot.Completed));
    updateText("active", formatNumber(snapshot.Active));
    updateText("successful", formatNumber(snapshot.Successful));
    updateText("networkErrors", formatNumber(snapshot.NetworkErrors));
    updateText("status5xx", formatNumber(snapshot.Status5xx));

    updateText("rps", Number(snapshot.RPS || 0).toFixed(2));
    updateText("p50", formatDuration(snapshot.P50));
    updateText("p90", formatDuration(snapshot.P90));
    updateText("p99", formatDuration(snapshot.P99));

    const requested = Number(getNumber("requests", 0));
    const completed = Number(snapshot.Completed || 0);

    const percent =
        requested > 0
            ? Math.min(100, (completed / requested) * 100)
            : 0;

    const progress = $("progressBar");
    if (progress) {
        progress.style.width = `${percent}%`;
    }

    const state = $("runState");

    if (state) {
        if (snapshot.SafetyStopped) {
            state.textContent = "SAFETY STOP";
            state.className = "status danger";
        } else if (snapshot.Degraded) {
            state.textContent = "DEGRADED";
            state.className = "status warning";
        } else {
            state.textContent = "RUNNING";
            state.className = "status running";
        }
    }
}

function updateResult(report) {
    if (!report) return;

    updateText("started", formatNumber(report.started));
    updateText("completed", formatNumber(report.completed));
    updateText("active", "0");
    updateText("successful", formatNumber(report.successful));
    updateText("networkErrors", formatNumber(report.network_errors));
    updateText("status5xx", formatNumber(report.status_5xx));

    updateText("rps", Number(report.rps || 0).toFixed(2));
    updateText("p50", report.p50 || "0");
    updateText("p90", report.p90 || "0");
    updateText("p99", report.p99 || "0");

    const requested = Number(report.requested_requests || 0);

    const progress = $("progressBar");

    if (progress) {
        progress.style.width = requested > 0 ? "100%" : "0%";
    }

    const state = $("runState");

    if (state) {
        if (report.safety_stopped) {
            state.textContent = "SAFETY STOP";
            state.className = "status danger";
        } else if (report.degraded) {
            state.textContent = "DEGRADED";
            state.className = "status warning";
        } else {
            state.textContent = "COMPLETED";
            state.className = "status success";
        }
    }

    renderStatusCodes(report.status_codes);
    renderNetworkErrors(report.network_errors_by_type);

    const safety = $("safetyMessage");

    if (safety) {
        if (report.safety_stopped) {
            safety.textContent =
                report.safety_reason || "Benchmark stopped by safety threshold.";
        } else if (report.degraded) {
            safety.textContent =
                "Performance degradation detected during the benchmark.";
        } else {
            safety.textContent =
                "No significant network degradation observed.";
        }
    }

    const summary = $("resultSummary");

    if (summary) {
        summary.style.display = "block";
    }
}

function renderStatusCodes(codes) {
    const container = $("statusDistribution");
    if (!container) return;

    container.innerHTML = "";

    if (!codes || Object.keys(codes).length === 0) {
        container.innerHTML = "<span>No status codes</span>";
        return;
    }

    Object.entries(codes)
        .sort((a, b) => Number(a[0]) - Number(b[0]))
        .forEach(([code, count]) => {
            const item = document.createElement("div");
            item.className = "distribution-item";

            item.innerHTML = `
                <span>${code}</span>
                <strong>${formatNumber(count)}</strong>
            `;

            container.appendChild(item);
        });
}

function renderNetworkErrors(errors) {
    const container = $("networkDistribution");
    if (!container) return;

    container.innerHTML = "";

    if (!errors || Object.keys(errors).length === 0) {
        container.innerHTML = "<span>No network errors</span>";
        return;
    }

    Object.entries(errors).forEach(([type, count]) => {
        const item = document.createElement("div");
        item.className = "distribution-item";

        item.innerHTML = `
            <span>${type}</span>
            <strong>${formatNumber(count)}</strong>
        `;

        container.appendChild(item);
    });
}

async function pollStatus() {
    try {
        const data = await api("/api/benchmark/status");

        setBackendStatus(true);

        updateSnapshot(data.snapshot);

        if (data.running) {
            setRunButtons(true);

            pollTimer = setTimeout(pollStatus, 500);
        } else {
            setRunButtons(false);

            try {
                const result = await api("/api/benchmark/result");
                updateResult(result);
            } catch (error) {
                console.log("No final result yet.");
            }
        }
    } catch (error) {
        console.error(error);
        setBackendStatus(false);
        setError(error.message);

        setRunButtons(false);
    }
}

function setRunButtons(running) {
    const run = $("runButton");
    const stop = $("stopButton");

    if (run) {
        run.disabled = running;
        run.textContent = running ? "Benchmark Running..." : "Run Benchmark";
    }

    if (stop) {
        stop.disabled = !running;
    }
}

async function startBenchmark() {
    setError("");

    const config = getFormData();

    if (!config.url) {
        setError("Target URL is required.");
        return;
    }

    try {
        setRunButtons(true);

        const state = $("runState");

        if (state) {
            state.textContent = "STARTING";
            state.className = "status running";
        }

        const result = await api("/api/benchmark/start", {
            method: "POST",
            body: JSON.stringify(config)
        });

        console.log("Benchmark started:", result);

        resetDisplay();

        pollStatus();
    } catch (error) {
        console.error(error);

        setError(error.message);

        setRunButtons(false);

        const state = $("runState");

        if (state) {
            state.textContent = "ERROR";
            state.className = "status danger";
        }
    }
}

async function stopBenchmark() {
    try {
        await api("/api/benchmark/stop", {
            method: "POST",
            body: JSON.stringify({})
        });

        setError("Stop requested.");
    } catch (error) {
        setError(error.message);
    }
}

function resetDisplay() {
    updateText("started", "0");
    updateText("completed", "0");
    updateText("active", "0");
    updateText("successful", "0");
    updateText("networkErrors", "0");
    updateText("status5xx", "0");

    updateText("rps", "0.00");
    updateText("p50", "0");
    updateText("p90", "0");
    updateText("p99", "0");

    const progress = $("progressBar");

    if (progress) {
        progress.style.width = "0%";
    }

    const result = $("resultSummary");

    if (result) {
        result.style.display = "none";
    }

    const status = $("runState");

    if (status) {
        status.textContent = "RUNNING";
        status.className = "status running";
    }
}

function connectEvents() {
    const run = $("runButton");
    const stop = $("stopButton");

    if (run) {
        run.addEventListener("click", startBenchmark);
    }

    if (stop) {
        stop.addEventListener("click", stopBenchmark);
    }
}

document.addEventListener("DOMContentLoaded", () => {
    connectEvents();
    checkBackend();
});