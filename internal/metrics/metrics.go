package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	RequestTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "llm_requests_total",
		Help: "Total number of LLM API requests",
	}, []string{"provider", "model", "status"})

	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "llm_request_duration_seconds",
		Help:    "LLM API request latency in seconds",
		Buckets: []float64{0.1, 0.25, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0},
	}, []string{"provider", "model"})

	TokensTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "llm_tokens_total",
		Help: "Total tokens consumed",
	}, []string{"provider", "model", "type"}) // type: input | output

	CostDollarsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "llm_cost_dollars_total",
		Help: "Estimated cumulative cost in USD",
	}, []string{"provider", "model"})

	ActiveRequests = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "llm_active_requests",
		Help: "Number of in-flight LLM requests",
	}, []string{"provider", "model"})

	ErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "llm_errors_total",
		Help: "Total number of LLM API errors",
	}, []string{"provider", "model", "error_type"})
)
