package engine

import (
	"fmt"
	"strings"
	"sync"
)

// modelPricing maps model prefixes to (input $/M, output $/M).
var modelPricing = map[string][2]float64{
	"claude-3-5-sonnet":    {3.0, 15.0},
	"claude-sonnet-4":      {3.0, 15.0},
	"claude-3-5-haiku":     {0.80, 4.0},
	"claude-3-opus":        {15.0, 75.0},
	"claude-3-haiku":       {0.25, 1.25},
	"gpt-4o":               {2.50, 10.0},
	"gpt-4o-mini":          {0.15, 0.60},
	"gpt-4-turbo":          {10.0, 30.0},
	"gpt-4":                {30.0, 60.0},
	"gpt-3.5":              {0.50, 1.50},
	"o1":                   {15.0, 60.0},
	"o1-mini":              {3.0, 12.0},
	"o3":                   {10.0, 40.0},
	"o3-mini":              {1.10, 4.40},
	"o4-mini":              {1.10, 4.40},
	"gemini-2.5-pro":       {1.25, 10.0},
	"gemini-2.5-flash":     {0.15, 0.60},
	"gemini-2.0-flash":     {0.10, 0.40},
	"gemini-1.5-pro":       {1.25, 5.0},
	"deepseek-chat":        {0.14, 0.28},
	"deepseek-reasoner":    {0.55, 2.19},
	"llama-3":              {0.20, 0.20},
	"mistral-large":        {2.0, 6.0},
	"mistral-small":        {0.20, 0.60},
	"qwen":                 {0.15, 0.60},
}

func pricingForModel(model string) (float64, float64) {
	lower := strings.ToLower(model)
	for prefix, prices := range modelPricing {
		if strings.Contains(lower, prefix) {
			return prices[0], prices[1]
		}
	}
	return 3.0, 15.0 // default fallback
}

// Cost tracks token usage and estimated cost.
type Cost struct {
	mu               sync.Mutex
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalCostUSD     float64
}

// Add records token usage from a response.
func (c *Cost) Add(prompt, completion int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.PromptTokens += prompt
	c.CompletionTokens += completion
	inPrice, outPrice := pricingForModel(c.Model)
	c.TotalCostUSD += float64(prompt)*inPrice/1_000_000 + float64(completion)*outPrice/1_000_000
}

// Summary returns a formatted cost string.
func (c *Cost) Summary() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return fmt.Sprintf("Tokens: %d in / %d out | Cost: $%.4f | Model: %s",
		c.PromptTokens, c.CompletionTokens, c.TotalCostUSD, c.Model)
}
