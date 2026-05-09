package proxy

import (
	"net/http"

	"github.com/AkashBalani/llm-observatory/internal/config"
)

func NewAnthropicProxy(cfg *config.Config) http.Handler {
	return newHandler("anthropic", cfg.AnthropicTarget, "/anthropic")
}

func NewOpenAIProxy(cfg *config.Config) http.Handler {
	return newHandler("openai", cfg.OpenAITarget, "/openai")
}
