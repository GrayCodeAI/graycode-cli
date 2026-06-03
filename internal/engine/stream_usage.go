package engine

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/types"

	analytics "github.com/GrayCodeAI/hawk/internal/observability"
)

// recordStreamUsage updates session billing/context counters and notifies the TUI.
func (s *Session) recordStreamUsage(ch chan<- StreamEvent, prompt, completion int, model, taskType string, apiStart time.Time) {
	if prompt <= 0 && completion <= 0 {
		return
	}
	if model != "" {
		s.mu.Lock()
		if strings.TrimSpace(s.Cost.Model) == "" {
			s.Cost.Model = model
		}
		s.mu.Unlock()
	}
	s.RecordAPIUsage(prompt, completion)
	s.Cost.Add(prompt, completion)
	if s.CostTracker != nil && model != "" {
		inPrice, outPrice := ModelPricing(model)
		cost := float64(prompt)*inPrice/1_000_000 + float64(completion)*outPrice/1_000_000
		_ = s.CostTracker.Record(analytics.CostEntry{
			Model:        model,
			TaskType:     taskType,
			InputTokens:  prompt,
			OutputTokens: completion,
			CostUSD:      cost,
			Duration:     time.Since(apiStart),
			Kept:         true,
		})
	}
	if ch != nil {
		ch <- StreamEvent{
			Type: "usage",
			Usage: &StreamUsage{
				PromptTokens:     prompt,
				CompletionTokens: completion,
			},
		}
	}
}

func estimateStreamCompletionTokens(text string, toolCalls []types.ToolCall) int {
	n := CountTokensFast(text)
	for _, tc := range toolCalls {
		if b, err := json.Marshal(tc.Arguments); err == nil {
			n += CountTokensFast(string(b))
		}
		n += CountTokensFast(tc.Name)
	}
	return n
}