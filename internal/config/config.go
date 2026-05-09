package config

import (
	"log"
	"os"
)

type Config struct {
	ListenAddr      string
	JaegerEndpoint  string
	LokiURL         string
	AnthropicTarget string
	OpenAITarget    string
}

func Load() *Config {
	return &Config{
		ListenAddr:      getEnv("LISTEN_ADDR", ":8080"),
		JaegerEndpoint:  getEnv("JAEGER_ENDPOINT", "http://jaeger:4318/v1/traces"),
		LokiURL:         getEnv("LOKI_URL", "http://loki:3100"),
		AnthropicTarget: getEnv("ANTHROPIC_TARGET", "https://api.anthropic.com"),
		OpenAITarget:    getEnv("OPENAI_TARGET", "https://api.openai.com"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	log.Printf("config: %s not set, using default: %s", key, fallback)
	return fallback
}
