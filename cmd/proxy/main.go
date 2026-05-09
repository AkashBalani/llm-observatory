package main

import (
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/AkashBalani/llm-observatory/internal/config"
	"github.com/AkashBalani/llm-observatory/internal/proxy"
	"github.com/AkashBalani/llm-observatory/internal/tracing"
)

func main() {
	cfg := config.Load()

	shutdown, err := tracing.Init(cfg.JaegerEndpoint)
	if err != nil {
		log.Fatalf("tracing init failed: %v", err)
	}
	defer shutdown()

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/anthropic/", proxy.NewAnthropicProxy(cfg))
	mux.Handle("/openai/", proxy.NewOpenAIProxy(cfg))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	log.Printf("LLM Observatory listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
