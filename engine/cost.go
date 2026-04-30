package engine

import (
	"fmt"
	"sync"
)

// Cost tracks token usage and estimated cost.
type Cost struct {
	mu               sync.Mutex
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
	// Rough estimate: $3/M input, $15/M output (Sonnet-level pricing)
	c.TotalCostUSD += float64(prompt)*3.0/1_000_000 + float64(completion)*15.0/1_000_000
}

// Summary returns a formatted cost string.
func (c *Cost) Summary() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return fmt.Sprintf("Tokens: %d in / %d out | Cost: $%.4f",
		c.PromptTokens, c.CompletionTokens, c.TotalCostUSD)
}
