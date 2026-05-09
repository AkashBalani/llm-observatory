# Changelog

All notable changes to LLM Observatory are documented here.  
Format: **What changed → Why → What we expect from it.**

---

## [Unreleased]

---

## 2026-05-09

### Add Loki structured logging — `9faee5f`

**What changed:**  
Added `internal/logger` package — a `Client` that writes structured JSON to stdout and asynchronously ships log entries to Loki's push API (`/loki/api/v1/push`). Entries are batched in a background goroutine and flushed every second or when 100 entries accumulate. Each entry carries labels (`service`, `provider`, `model`, `level`) so Grafana/Loki can filter by any dimension. The proxy handler now calls `log.Info` / `log.Error` instead of `log.Printf`, and `main.go` initialises the logger with the Loki URL from config. `LOKI_URL` added to config (default: `http://loki:3100`). Also updated `DECISIONS.md` with DD-008 (direct push over Promtail scraping) and DD-009 (staying on Go 1.20).

**Why:**  
Loki was running in the stack but receiving nothing. Metrics and traces answer "how many" and "how long" — logs answer "what exactly happened on this request". Structured labels make it possible to filter by model or provider in Grafana's Explore view without parsing text. Direct push was chosen over Promtail scraping because it avoids Docker socket mounting and regex pipeline config, and gives us clean label control from the application.

**Expected outcome:**  
Every completed request produces a JSON log line on stdout and a queryable entry in Loki labelled by provider and model. In Grafana → Explore → Loki, querying `{service="llm-observatory", provider="anthropic"}` returns a live feed of request logs with cost, duration, and token counts.

---

## 2026-05-09

### Add streaming support and README — `6fa2187`

**What changed:**  
The proxy now detects `"stream": true` in the request body and routes to a dedicated streaming path. SSE chunks are piped to the client line-by-line via `http.Flusher` as they arrive from the upstream API. Token counts are extracted from SSE events rather than the final JSON body: Anthropic's `message_start` event yields input tokens, `message_delta` yields output tokens. For OpenAI, `stream_options: {"include_usage": true}` is injected into the request so the final chunk carries usage. A `README.md` was also added covering quick start, architecture, environment variables, metrics reference, and pricing.

**Why:**  
The original buffered implementation worked for non-streaming requests but would hold the entire response in memory and deliver nothing to the client until the model finished generating — breaking streaming entirely. Most production LLM integrations use streaming for perceived responsiveness. This was a correctness issue, not a feature gap.

**Expected outcome:**  
Streaming responses flow to the client in real time with no buffering delay. Metrics (tokens, cost, latency, request count) are recorded correctly at stream end. Both streaming and non-streaming paths are now fully observable.

---

### Fix Jaeger trace export and Grafana datasource wiring — `5bafbc0`

**What changed:**  
`tracing.go` now parses the full `JAEGER_ENDPOINT` URL and extracts only `host:port` before passing it to `otlptracehttp.WithEndpoint`, which expects that format. Grafana's datasource provisioning YAML was updated to include explicit UIDs (`prometheus`, `jaeger`, `loki`). The dashboard JSON had all `${DS_PROMETHEUS}` references replaced with the hardcoded UID `prometheus`.

**Why:**  
Two separate bugs blocked observability on first run:
1. The OTel exporter was URL-encoding the full endpoint string and appending `/v1/traces` to it, producing an invalid URL like `http://http:%2F%2Fjaeger:...`. No traces were being exported.
2. `${DS_PROMETHEUS}` is a Grafana import-time variable that gets resolved interactively in the UI. File-provisioned dashboards never go through that flow, so all panels showed "datasource not found". Grafana also crashed on restart because the provisioner tried to create datasources with new UIDs that conflicted with the ones already stored in its SQLite DB from the first (UID-less) run.

**Expected outcome:**  
Jaeger receives traces on every request. Grafana loads cleanly on a fresh `docker compose up` with all panels bound to the correct Prometheus datasource.

---

### Add .env pattern to keep API keys out of git — `e68b12d`

**What changed:**  
Added `.gitignore` (excludes `.env`), `.env.example` (committed placeholder), updated `docker-compose.yml` to load secrets via `env_file: .env`, and updated the `make test-anthropic` target to source `.env` automatically before the curl command.

**Why:**  
Without this, the only way to pass an API key was to export it as an environment variable in the shell or inline it in a command — both of which risk leaking into shell history or being accidentally committed. GitHub bots scan public repos for exposed keys and can trigger charges within minutes of a leak.

**Expected outcome:**  
API keys live only in `.env` on the developer's machine. The git history and the running Docker containers never expose them. Onboarding is documented via `.env.example`.

---

## 2026-05-08

### Add Grafana dashboard: LLM Observatory overview — `3a71f4d`

**What changed:**  
Added `grafana/dashboards/llm-overview.json` — a fully provisioned Grafana dashboard with four rows:
- **Summary:** 6 stat panels (total requests, total cost, active requests, error rate, total tokens, cost/hour)
- **Request Metrics:** request rate by provider/model, status code breakdown with error overlay
- **Latency:** p50/p95/p99 timeseries, latency distribution histogram
- **Tokens & Cost:** token rate by type, cumulative cost by model, cost share donut, token share donut, cost-by-model table

Template variables allow filtering by provider and model.

**Why:**  
Raw Prometheus metrics require writing PromQL to get any insight. The dashboard translates metrics into the questions operators actually ask: "Which model is costing the most?", "Is latency spiking?", "What's my error rate right now?" It also demonstrates end-to-end observability stack ownership — from metric emission to dashboard provisioning in code.

**Expected outcome:**  
Any engineer cloning the repo gets a fully populated dashboard on first `docker compose up` — no manual Grafana configuration required.

---

### Initial scaffold: LLM Observatory — `bc2cc3e`

**What changed:**  
Full initial implementation of the proxy and observability stack:
- `cmd/proxy/main.go` — HTTP server, routes for `/anthropic/`, `/openai/`, `/metrics`, `/health`
- `internal/proxy/handler.go` — buffered proxy handler; reads body, extracts model, forwards to upstream, parses token usage from response JSON, records all metrics
- `internal/metrics/metrics.go` — Prometheus counters, histograms, and gauges for requests, latency, tokens, cost, errors, active requests
- `internal/cost/cost.go` — per-million-token pricing table for Anthropic and OpenAI models
- `internal/tracing/tracing.go` — OTel SDK init with OTLP HTTP export to Jaeger
- `internal/config/config.go` — environment variable based config
- `docker-compose.yml` — 6-service stack: proxy, Prometheus, Grafana, Jaeger, Loki, Promtail
- `Dockerfile` — multi-stage Go build

**Why:**  
Starting point for an LLM observability platform. The goal is to show a complete, production-minded observability stack built in Go — covering metrics, tracing, cost attribution, and log aggregation — without requiring application code changes.

**Expected outcome:**  
A working proxy that intercepts Anthropic and OpenAI HTTP calls, records structured telemetry, and surfaces everything in Grafana and Jaeger. Foundation for iterative feature additions.
