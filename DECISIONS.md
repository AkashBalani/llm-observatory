# Design Decisions

Major architectural and technical decisions made during the project. Updated whenever a significant choice is made or revisited.

---

## DD-001 — Go as the implementation language

**Date:** 2026-05-08  
**Status:** Active

**Decision:** Build the proxy in Go rather than Java, Python, or Node.

**Reasoning:**
- Go's `net/http` standard library is purpose-built for proxies — low-level control without framework overhead.
- Single static binary; no runtime dependencies to manage in Docker.
- Goroutines make concurrent request handling cheap and straightforward.
- Strengthens portfolio breadth: Akash's primary stack is Java/cloud; Go is a natural adjacent skill valued in infrastructure and platform roles.

**Trade-offs:** Go's ecosystem for LLM SDKs is thinner than Python's. Accepted because we're building a proxy, not a consumer — we speak raw HTTP, not SDK abstractions.

---

## DD-002 — Transparent proxy pattern (not SDK wrapper)

**Date:** 2026-05-08  
**Status:** Active

**Decision:** Intercept raw HTTP at the transport layer rather than wrapping provider SDKs.

**Reasoning:**
- Zero changes required in the calling application — just swap the base URL.
- Works with any language or SDK automatically.
- Captures the true wire-level request and response, including headers and error payloads.

**Trade-offs:** We have to parse provider-specific JSON ourselves (usage fields differ between Anthropic and OpenAI). Acceptable cost for the flexibility gained.

---

## DD-003 — Prometheus + Grafana for metrics (not a SaaS)

**Date:** 2026-05-08  
**Status:** Active

**Decision:** Self-hosted Prometheus and Grafana rather than Datadog, New Relic, or similar SaaS.

**Reasoning:**
- No external accounts, no data leaving the local machine — important for proxies that touch API keys and prompts.
- Full control over retention and dashboard design.
- Demonstrates infrastructure-as-code skills: provisioning datasources and dashboards from config files, not UI clicks.

**Trade-offs:** No built-in alerting infrastructure or long-term storage. Acceptable for the current scope; can layer on Alertmanager later.

---

## DD-004 — OpenTelemetry + Jaeger for tracing

**Date:** 2026-05-08  
**Status:** Active

**Decision:** Use the OpenTelemetry SDK with OTLP HTTP export to Jaeger rather than a vendor-specific tracing library.

**Reasoning:**
- OTel is the industry standard; the same instrumentation code works with any OTLP-compatible backend (Jaeger, Tempo, Honeycomb, Datadog).
- Jaeger runs in a single Docker container with no external dependencies.
- Decouples the application from the observability backend.

**Trade-offs:** OTel's `WithEndpoint` expects `host:port` not a full URL — required a URL-parsing workaround in `tracing.go`. Documented and handled.

---

## DD-005 — Buffered response for non-streaming, real-time pipe for streaming

**Date:** 2026-05-09  
**Status:** Active

**Decision:** Two distinct code paths in the handler: buffer the full response body for non-streaming requests; stream SSE chunks line-by-line via `http.Flusher` for streaming requests.

**Reasoning:**
- Non-streaming: buffering lets us parse the full JSON response in one pass before forwarding. Simple and reliable.
- Streaming: buffering would defeat the purpose — the client would see no output until the model finished generating. We pipe chunks immediately and parse usage from SSE events in parallel using `io.TeeReader`.

**Trade-offs:** Streaming usage parsing is provider-specific (Anthropic: `message_start`/`message_delta` events; OpenAI: inject `stream_options.include_usage` and read the final chunk). More code, but necessary for correctness.

---

## DD-006 — API keys via .env file, never in source

**Date:** 2026-05-09  
**Status:** Active

**Decision:** Store all secrets in a `.env` file that is gitignored. Docker Compose loads it via `env_file`; the Makefile sources it before curl commands.

**Reasoning:**
- Bots scan GitHub constantly for leaked keys. A single accidental commit can result in unexpected API charges within minutes.
- `.env.example` committed as a template makes onboarding clear without exposing real values.

**Trade-offs:** Developers must manually create `.env` from `.env.example` before running. Acceptable — it is a one-time step and the README documents it.

---

## DD-007 — Grafana provisioning via config files, not UI

**Date:** 2026-05-08  
**Status:** Active

**Decision:** Provision Grafana datasources and dashboards entirely through YAML/JSON config files mounted into the container, rather than configuring them through the Grafana web UI.

**Reasoning:**
- Config lives in git — the full stack is reproducible with `docker compose up`, no manual steps.
- Datasource UIDs are explicit and stable, avoiding the "datasource not found" issue that occurs when Grafana generates random UIDs across restarts.

**Trade-offs:** Dashboard JSON is verbose and harder to edit by hand. Workaround: design in the UI, export JSON, commit it.
