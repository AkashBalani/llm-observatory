package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/AkashBalani/llm-observatory/internal/cost"
	"github.com/AkashBalani/llm-observatory/internal/logger"
	"github.com/AkashBalani/llm-observatory/internal/metrics"
)

var tracer = otel.Tracer("llm-observatory")

type Handler struct {
	provider   string
	targetBase string
	stripPath  string
	log        *logger.Client
}

func newHandler(provider, targetBase, stripPath string, log *logger.Client) *Handler {
	return &Handler{provider: provider, targetBase: targetBase, stripPath: stripPath, log: log}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	model := extractModel(bodyBytes)
	if model == "" {
		model = "unknown"
	}

	streaming := isStreaming(bodyBytes)

	// OpenAI doesn't include usage in streaming by default; inject the option.
	if streaming && h.provider == "openai" {
		bodyBytes = injectOpenAIStreamOptions(bodyBytes)
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

	for k, vals := range r.Header {
		for _, v := range vals {
			upstreamReq.Header.Add(k, v)
		}
	}

	start := time.Now()
	resp, err := http.DefaultClient.Do(upstreamReq)
	if err != nil {
		metrics.ErrorsTotal.WithLabelValues(h.provider, model, "upstream_request").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	if streaming && resp.StatusCode == http.StatusOK {
		h.serveStream(w, resp.Body, model, start, span)
	} else {
		h.serveBuffered(w, resp.Body, resp.StatusCode, model, start, span)
	}
}

// serveStream pipes SSE chunks to the client while capturing them to extract usage.
func (h *Handler) serveStream(w http.ResponseWriter, body io.Reader, model string, start time.Time, span trace.Span) {
	flusher, canFlush := w.(http.Flusher)

	var captured bytes.Buffer
	scanner := bufio.NewScanner(io.TeeReader(body, &captured))
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	for scanner.Scan() {
		w.Write(scanner.Bytes())
		w.Write([]byte("\n"))
		if canFlush {
			flusher.Flush()
		}
	}
	// SSE spec: events are separated by blank lines
	w.Write([]byte("\n"))
	if canFlush {
		flusher.Flush()
	}

	duration := time.Since(start).Seconds()
	metrics.RequestTotal.WithLabelValues(h.provider, model, "200").Inc()
	metrics.RequestDuration.WithLabelValues(h.provider, model).Observe(duration)

	inputTokens, outputTokens := extractStreamUsage(h.provider, captured.Bytes())
	h.recordUsage(model, inputTokens, outputTokens, duration, http.StatusOK, span)
}

// serveBuffered reads the full response body, records metrics, then writes to client.
func (h *Handler) serveBuffered(w http.ResponseWriter, body io.Reader, statusCode int, model string, start time.Time, span trace.Span) {
	respBytes, err := io.ReadAll(body)
	duration := time.Since(start).Seconds()

	statusLabel := strconv.Itoa(statusCode)
	metrics.RequestTotal.WithLabelValues(h.provider, model, statusLabel).Inc()
	metrics.RequestDuration.WithLabelValues(h.provider, model).Observe(duration)

	if err != nil {
		metrics.ErrorsTotal.WithLabelValues(h.provider, model, "response_read").Inc()
		return
	}

	if statusCode >= 400 {
		metrics.ErrorsTotal.WithLabelValues(h.provider, model, "http_"+statusLabel).Inc()
		span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", statusCode))
	}

	inputTokens, outputTokens := extractUsage(h.provider, respBytes)
	h.recordUsage(model, inputTokens, outputTokens, duration, statusCode, span)
	w.Write(respBytes)
}

func (h *Handler) recordUsage(model string, inputTokens, outputTokens int, duration float64, statusCode int, span trace.Span) {
	if inputTokens == 0 && outputTokens == 0 {
		return
	}
	metrics.TokensTotal.WithLabelValues(h.provider, model, "input").Add(float64(inputTokens))
	metrics.TokensTotal.WithLabelValues(h.provider, model, "output").Add(float64(outputTokens))

	estimatedCost := cost.Calculate(model, inputTokens, outputTokens)
	metrics.CostDollarsTotal.WithLabelValues(h.provider, model).Add(estimatedCost)

	span.SetAttributes(
		attribute.Int("llm.input_tokens", inputTokens),
		attribute.Int("llm.output_tokens", outputTokens),
		attribute.Float64("llm.cost_usd", estimatedCost),
		attribute.Float64("llm.duration_seconds", duration),
		attribute.Int("http.status_code", statusCode),
	)

	level := "info"
	if statusCode >= 400 {
		level = "error"
	}

	labels := map[string]string{
		"provider": h.provider,
		"model":    model,
	}
	fields := map[string]any{
		"status":        statusCode,
		"duration_s":    fmt.Sprintf("%.3f", duration),
		"input_tokens":  inputTokens,
		"output_tokens": outputTokens,
		"cost_usd":      fmt.Sprintf("%.6f", estimatedCost),
	}

	if level == "error" {
		h.log.Error("request completed", labels, fields)
	} else {
		h.log.Info("request completed", labels, fields)
	}
}

func isStreaming(body []byte) bool {
	var payload struct {
		Stream bool `json:"stream"`
	}
	return json.Unmarshal(body, &payload) == nil && payload.Stream
}

// injectOpenAIStreamOptions adds stream_options.include_usage so the final
// chunk carries token counts even in streaming mode.
func injectOpenAIStreamOptions(body []byte) []byte {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	m["stream_options"] = json.RawMessage(`{"include_usage":true}`)
	result, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return result
}

// extractStreamUsage scans SSE lines and pulls token counts from provider-specific events.
func extractStreamUsage(provider string, data []byte) (inputTokens, outputTokens int) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := line[len("data: "):]
		if payload == "[DONE]" {
			continue
		}

		switch provider {
		case "anthropic":
			inputTokens, outputTokens = parseAnthropicSSELine(payload, inputTokens, outputTokens)
		case "openai":
			inputTokens, outputTokens = parseOpenAISSELine(payload, inputTokens, outputTokens)
		}
	}
	return
}

func parseAnthropicSSELine(payload string, in, out int) (int, int) {
	// message_start carries input_tokens; message_delta carries output_tokens.
	var event struct {
		Type    string `json:"type"`
		Message struct {
			Usage struct {
				InputTokens int `json:"input_tokens"`
			} `json:"usage"`
		} `json:"message"`
		Usage struct {
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return in, out
	}
	switch event.Type {
	case "message_start":
		in = event.Message.Usage.InputTokens
	case "message_delta":
		out = event.Usage.OutputTokens
	}
	return in, out
}

func parseOpenAISSELine(payload string, in, out int) (int, int) {
	// With stream_options.include_usage, the last chunk has a non-null usage field.
	var chunk struct {
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil || chunk.Usage == nil {
		return in, out
	}
	return chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens
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
