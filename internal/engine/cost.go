package engine

import (
	"fmt"
	"sync"
)

func pricingForModel(model string) (float64, float64) {
	return ModelPricing(model)
}

// Cost tracks token usage and estimated cost.
type Cost struct {
	mu               sync.Mutex
	Model            string
	PromptTokens     int
	CompletionTokens int
	CacheReadTokens  int
	CacheWriteTokens int
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

// AddCacheTokens records cache token usage (priced at ~10% of input).
func (c *Cost) AddCacheTokens(read, write int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.CacheReadTokens += read
	c.CacheWriteTokens += write
	inPrice, _ := pricingForModel(c.Model)
	c.TotalCostUSD += float64(read) * inPrice * 0.1 / 1_000_000   // cache reads are ~10% of input price
	c.TotalCostUSD += float64(write) * inPrice * 1.25 / 1_000_000 // cache writes are ~125% of input price
}

// Total returns the estimated total cost in USD.
func (c *Cost) Total() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.TotalCostUSD
}

// TotalUSD returns the estimated total cost (same as Total — unified pricing).
func (c *Cost) TotalUSD() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.TotalCostUSD
}

// Summary returns a formatted cost string.
func (c *Cost) Summary() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := fmt.Sprintf("Tokens: %d in / %d out", c.PromptTokens, c.CompletionTokens)
	if c.CacheReadTokens > 0 || c.CacheWriteTokens > 0 {
		s += fmt.Sprintf(" | Cache: %d read / %d write", c.CacheReadTokens, c.CacheWriteTokens)
	}
	s += fmt.Sprintf(" | Cost: $%.4f | Model: %s", c.TotalCostUSD, c.Model)
	return s
}
