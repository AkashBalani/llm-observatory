package cost

// Pricing in USD per million tokens (as of 2026)
var modelPricing = map[string][2]float64{
	// Anthropic — [input, output] per million tokens
	"claude-opus-4-7":           {15.00, 75.00},
	"claude-sonnet-4-6":         {3.00, 15.00},
	"claude-haiku-4-5":          {0.25, 1.25},
	"claude-haiku-4-5-20251001": {0.25, 1.25},

	// OpenAI
	"gpt-4o":      {5.00, 15.00},
	"gpt-4o-mini": {0.15, 0.60},
	"gpt-4-turbo": {10.00, 30.00},
}

// Calculate returns estimated cost in USD for a given model and token counts.
func Calculate(model string, inputTokens, outputTokens int) float64 {
	pricing, ok := modelPricing[model]
	if !ok {
		return 0
	}
	inputCost := float64(inputTokens) / 1_000_000 * pricing[0]
	outputCost := float64(outputTokens) / 1_000_000 * pricing[1]
	return inputCost + outputCost
}
