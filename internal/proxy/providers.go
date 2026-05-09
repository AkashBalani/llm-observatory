package proxy

import (
	"net/http"

	"github.com/AkashBalani/llm-observatory/internal/config"
	"github.com/AkashBalani/llm-observatory/internal/logger"
)

func NewAnthropicProxy(cfg *config.Config, log *logger.Client) http.Handler {
	return newHandler("anthropic", cfg.AnthropicTarget, "/anthropic", log)
}

func NewOpenAIProxy(cfg *config.Config, log *logger.Client) http.Handler {
	return newHandler("openai", cfg.OpenAITarget, "/openai", log)
}
