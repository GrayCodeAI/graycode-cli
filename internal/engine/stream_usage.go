package engine

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/types"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"

	analytics "github.com/GrayCodeAI/hawk/internal/observability"
)

// streamUsageLedger prevents providers that repeat their final usage on the
// done event from being billed twice. Distinct usage events are preserved so
// multi-request continuation streams still account for every provider call.
type streamUsageLedger struct {
	lastUsageEvent *types.EyrieUsage
}

func (l *streamUsageLedger) shouldRecord(usage *types.EyrieUsage, terminal bool) bool {
	if usage == nil {
		return false
	}
	if terminal {
		duplicate := equalStreamUsage(l.lastUsageEvent, usage)
		l.lastUsageEvent = nil
		return !duplicate
	}
	l.lastUsageEvent = cloneStreamUsage(usage)
	return true
}

func (l *streamUsageLedger) reset() {
	l.lastUsageEvent = nil
}

func equalStreamUsage(a, b *types.EyrieUsage) bool {
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func cloneStreamUsage(usage *types.EyrieUsage) *types.EyrieUsage {
	if usage == nil {
		return nil
	}
	cloned := *usage
	return &cloned
}

func updateResolvedRoute(provider, model string, route *types.ResolvedRoute) (string, string, bool) {
	if route == nil {
		return provider, model, false
	}
	nextProvider := strings.TrimSpace(route.Provider)
	nextModel := strings.TrimSpace(route.Model)
	if nextProvider == "" {
		nextProvider = provider
	}
	if nextModel == "" {
		nextModel = model
	}
	changed := nextProvider != provider || nextModel != model
	return nextProvider, nextModel, changed
}

// recordStreamUsage updates session billing/context counters and notifies the TUI.
// Provider and model are the concrete route selected by Eyrie, not merely the
// user's preference, so fallback usage is attributed to the transport that ran.
func (s *Session) recordStreamUsage(ch chan<- StreamEvent, prompt, completion int, provider, model, taskType string, apiStart time.Time) {
	if prompt <= 0 && completion <= 0 {
		return
	}
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	s.RecordAPIUsage(prompt, completion)
	costBefore := s.CostValue().Total()
	s.CostValue().AddForModel(model, prompt, completion)
	requestCost := s.CostValue().Total() - costBefore
	if tracker := s.LifecycleSvc().CostTracker(); tracker != nil && model != "" {
		_ = tracker.Record(analytics.CostEntry{
			Model:        model,
			TaskType:     taskType,
			InputTokens:  prompt,
			OutputTokens: completion,
			CostUSD:      requestCost,
			Duration:     time.Since(apiStart),
			Kept:         true,
		})
	}
	s.recordTokUsageBudgetObservation(
		prompt+completion,
		requestCost,
		provider,
		model,
	)
	if ch != nil {
		ch <- StreamEvent{
			Type: "usage",
			Usage: &StreamUsage{
				PromptTokens:     prompt,
				CompletionTokens: completion,
				Provider:         provider,
				Model:            model,
			},
		}
		if tracker := s.currentTokUsageTracker(); tracker != nil {
			for _, alert := range tracker.DrainAlerts() {
				ch <- StreamEvent{
					Type:    "content",
					Content: fmt.Sprintf("\n%s Usage %s: %s\n", icons.Alert(), alert.Level, alert.Message),
				}
			}
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
