# LLM Observatory

A transparent observability proxy for LLM APIs. Drop it in front of Anthropic or OpenAI calls to get real-time metrics, distributed traces, and cost tracking — without changing your application code.

## What it does

- **Metrics** (Prometheus) — request rate, latency percentiles, token usage, cost per model
- **Traces** (OpenTelemetry → Jaeger) — end-to-end span per request with token and cost attributes
- **Streaming support** — SSE streams are proxied in real-time; token counts are extracted from stream events
- **Cost tracking** — per-request USD cost calculated from token usage and current model pricing

## Architecture

```
Your App → LLM Observatory (proxy) → Anthropic / OpenAI
                  ↓                        ↑
            Prometheus ← scrape     response forwarded
            Jaeger   ← OTLP traces
            Grafana  ← dashboards
            Loki     ← logs
```

## Quick start

**1. Clone and configure**
```bash
git clone https://github.com/AkashBalani/llm-observatory.git
cd llm-observatory
cp .env.example .env
# edit .env and add your API keys
```

**2. Start the stack**
```bash
docker compose up --build -d
```

**3. Point your app at the proxy**

| Provider  | Original endpoint               | Proxy endpoint                       |
|-----------|---------------------------------|--------------------------------------|
| Anthropic | `https://api.anthropic.com`     | `http://localhost:8080/anthropic`    |
| OpenAI    | `https://api.openai.com`        | `http://localhost:8080/openai`       |

Everything else (headers, auth, request body) stays the same.

**4. Fire a test request**
```bash
make test-anthropic
```

## Dashboards & UIs

| Service    | URL                      | Credentials   |
|------------|--------------------------|---------------|
| Grafana    | http://localhost:3000    | admin / admin |
| Prometheus | http://localhost:9090    | —             |
| Jaeger     | http://localhost:16686   | —             |

The **LLM Observatory** dashboard in Grafana shows:
- Total requests, cost, active requests, error rate, total tokens, cost/hour
- Request rate by provider/model
- Latency percentiles (p50/p95/p99)
- Token usage rate (input vs output)
- Cumulative cost by model
- Cost share and token share pie charts

## Environment variables

| Variable           | Default                          | Description                   |
|--------------------|----------------------------------|-------------------------------|
| `LISTEN_ADDR`      | `:8080`                          | Proxy listen address          |
| `JAEGER_ENDPOINT`  | `http://jaeger:4318/v1/traces`   | OTLP HTTP trace endpoint      |
| `ANTHROPIC_TARGET` | `https://api.anthropic.com`      | Anthropic upstream URL        |
| `OPENAI_TARGET`    | `https://api.openai.com`         | OpenAI upstream URL           |
| `ANTHROPIC_API_KEY`| —                                | Your Anthropic API key        |
| `OPENAI_API_KEY`   | —                                | Your OpenAI API key           |

## Prometheus metrics

| Metric                        | Type      | Labels                        |
|-------------------------------|-----------|-------------------------------|
| `llm_requests_total`          | Counter   | provider, model, status       |
| `llm_request_duration_seconds`| Histogram | provider, model               |
| `llm_tokens_total`            | Counter   | provider, model, type         |
| `llm_cost_dollars_total`      | Counter   | provider, model               |
| `llm_active_requests`         | Gauge     | provider, model               |
| `llm_errors_total`            | Counter   | provider, model, error_type   |

## Supported models and pricing

| Model                    | Input ($/1M tokens) | Output ($/1M tokens) |
|--------------------------|---------------------|----------------------|
| claude-opus-4-7          | $15.00              | $75.00               |
| claude-sonnet-4-6        | $3.00               | $15.00               |
| claude-haiku-4-5         | $0.25               | $1.25                |
| gpt-4o                   | $5.00               | $15.00               |
| gpt-4o-mini              | $0.15               | $0.60                |

## Development

```bash
make run      # run proxy locally (no Docker)
make test     # run unit tests
make lint     # golangci-lint
make down     # stop the stack
```

## Tech stack

- **Go** — proxy, metrics, tracing
- **Prometheus** — metrics storage
- **Grafana** — dashboards
- **Jaeger** — distributed tracing (OTLP)
- **Loki + Promtail** — log aggregation
- **Docker Compose** — local orchestration
