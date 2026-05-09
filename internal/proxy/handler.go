package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/AkashBalani/llm-observatory/internal/cost"
	"github.com/AkashBalani/llm-observatory/internal/metrics"
)

var tracer = otel.Tracer("llm-observatory")

type Handler struct {
	provider   string
	targetBase string
	stripPath  string
}

func newHandler(provider, targetBase, stripPath string) *Handler {
	return &Handler{provider: provider, targetBase: targetBase, stripPath: stripPath}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Read and buffer the request body so we can forward it
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	// Extract model from request body for labeling
	model := extractModel(bodyBytes)
	if model == "" {
		model = "unknown"
	}

	ctx, span := tracer.Start(r.Context(), fmt.Sprintf("%s/%s", h.provider, model))
	defer span.End()

	span.SetAttributes(
		attribute.String("llm.provider", h.provider),
		attribute.String("llm.model", model),
		attribute.String("http.method", r.Method),
		attribute.String("http.path", r.URL.Path),
	)

	metrics.ActiveRequests.WithLabelValues(h.provider, model).Inc()
	defer metrics.ActiveRequests.WithLabelValues(h.provider, model).Dec()

	// Build upstream request
	upstreamPath := r.URL.Path
	if h.stripPath != "" {
		upstreamPath = upstreamPath[len(h.stripPath):]
	}
	upstreamURL := h.targetBase + upstreamPath
	if r.URL.RawQuery != "" {
		upstreamURL += "?" + r.URL.RawQuery
	}

	upstreamReq, err := http.NewRequestWithContext(ctx, r.Method, upstreamURL, bytes.NewReader(bodyBytes))
	if err != nil {
		metrics.ErrorsTotal.WithLabelValues(h.provider, model, "upstream_build").Inc()
		http.Error(w, "failed to build upstream request", http.StatusInternalServerError)
		return
	}

	// Forward headers (auth, content-type, etc.)
	for k, vals := range r.Header {
		for _, v := range vals {
			upstreamReq.Header.Add(k, v)
		}
	}

	start := time.Now()
	resp, err := http.DefaultClient.Do(upstreamReq)
	duration := time.Since(start).Seconds()

	if err != nil {
		metrics.ErrorsTotal.WithLabelValues(h.provider, model, "upstream_request").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Buffer response body to parse tokens before forwarding
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		metrics.ErrorsTotal.WithLabelValues(h.provider, model, "response_read").Inc()
		http.Error(w, "failed to read upstream response", http.StatusBadGateway)
		return
	}

	statusLabel := strconv.Itoa(resp.StatusCode)
	metrics.RequestTotal.WithLabelValues(h.provider, model, statusLabel).Inc()
	metrics.RequestDuration.WithLabelValues(h.provider, model).Observe(duration)

	span.SetAttributes(
		attribute.Int("http.status_code", resp.StatusCode),
		attribute.Float64("llm.duration_seconds", duration),
	)

	if resp.StatusCode >= 400 {
		metrics.ErrorsTotal.WithLabelValues(h.provider, model, "http_"+statusLabel).Inc()
		span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", resp.StatusCode))
	}

	// Parse usage from response
	inputTokens, outputTokens := extractUsage(h.provider, respBytes)
	if inputTokens > 0 || outputTokens > 0 {
		metrics.TokensTotal.WithLabelValues(h.provider, model, "input").Add(float64(inputTokens))
		metrics.TokensTotal.WithLabelValues(h.provider, model, "output").Add(float64(outputTokens))

		estimatedCost := cost.Calculate(model, inputTokens, outputTokens)
		metrics.CostDollarsTotal.WithLabelValues(h.provider, model).Add(estimatedCost)

		span.SetAttributes(
			attribute.Int("llm.input_tokens", inputTokens),
			attribute.Int("llm.output_tokens", outputTokens),
			attribute.Float64("llm.cost_usd", estimatedCost),
		)

		log.Printf("provider=%s model=%s status=%d duration=%.2fs input_tokens=%d output_tokens=%d cost=$%.6f",
			h.provider, model, resp.StatusCode, duration, inputTokens, outputTokens, estimatedCost)
	}

	// Forward response to client
	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBytes)
}

func extractModel(body []byte) string {
	var payload struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return payload.Model
}

// extractUsage parses input/output token counts from provider response bodies.
func extractUsage(provider string, body []byte) (int, int) {
	switch provider {
	case "anthropic":
		var resp struct {
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(body, &resp); err == nil {
			return resp.Usage.InputTokens, resp.Usage.OutputTokens
		}
	case "openai":
		var resp struct {
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(body, &resp); err == nil {
			return resp.Usage.PromptTokens, resp.Usage.CompletionTokens
		}
	}
	return 0, 0
}
