package cost

import (
	"fmt"
	"strings"
	"sync"
)

type Cost struct {
	mu               sync.Mutex
	Model            string
	PromptTokens     int
	CompletionTokens int
	CacheReadTokens  int
	CacheWriteTokens int
	TotalCostUSD     float64
}

// Snapshot is a race-free view of the accumulated session cost.
type Snapshot struct {
	Model            string
	PromptTokens     int
	CompletionTokens int
	CacheReadTokens  int
	CacheWriteTokens int
	TotalCostUSD     float64
}

// SetModel updates the model used for subsequent pricing calculations.
func (c *Cost) SetModel(model string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Model = strings.TrimSpace(model)
}

// Snapshot returns a consistent view of all cost fields.
func (c *Cost) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Snapshot{
		Model:            c.Model,
		PromptTokens:     c.PromptTokens,
		CompletionTokens: c.CompletionTokens,
		CacheReadTokens:  c.CacheReadTokens,
		CacheWriteTokens: c.CacheWriteTokens,
		TotalCostUSD:     c.TotalCostUSD,
	}
}

func (c *Cost) Add(prompt, completion int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.addLocked(prompt, completion)
}

// AddForModel records usage against the concrete model selected by the provider
// engine. The model update and price lookup are atomic with the token/cost
// update, so a routed fallback cannot be billed using the requested model.
func (c *Cost) AddForModel(model string, prompt, completion int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if model = strings.TrimSpace(model); model != "" {
		c.Model = model
	}
	c.addLocked(prompt, completion)
}

func (c *Cost) addLocked(prompt, completion int) {
	c.PromptTokens += prompt
	c.CompletionTokens += completion
	inPrice, outPrice := ModelPricing(c.Model)
	c.TotalCostUSD += float64(prompt)*inPrice/1_000_000 + float64(completion)*outPrice/1_000_000
}

func (c *Cost) AddCacheTokens(read, write int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.CacheReadTokens += read
	c.CacheWriteTokens += write
	inPrice, _ := ModelPricing(c.Model)
	c.TotalCostUSD += float64(read) * inPrice * 0.1 / 1_000_000
	c.TotalCostUSD += float64(write) * inPrice * 1.25 / 1_000_000
}

func (c *Cost) Total() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.TotalCostUSD
}

func (c *Cost) TotalUSD() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.TotalCostUSD
}

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
