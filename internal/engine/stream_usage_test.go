package engine

import (
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/types"
)

func TestStreamUsageLedgerRecordsDoneOnlyUsage(t *testing.T) {
	var ledger streamUsageLedger
	terminal := &types.EyrieUsage{PromptTokens: 13, CompletionTokens: 7, TotalTokens: 20}

	if !ledger.shouldRecord(terminal, true) {
		t.Fatal("done-only usage was dropped")
	}
}

func TestStreamUsageLedgerDoesNotDoubleCountRepeatedDoneUsage(t *testing.T) {
	var ledger streamUsageLedger
	usage := &types.EyrieUsage{PromptTokens: 13, CompletionTokens: 7, TotalTokens: 20, CacheReadTokens: 4}

	if !ledger.shouldRecord(usage, false) {
		t.Fatal("usage event was dropped")
	}
	if ledger.shouldRecord(cloneStreamUsage(usage), true) {
		t.Fatal("identical terminal usage would be double-counted")
	}
}

func TestStreamUsageLedgerPreservesDistinctContinuationUsage(t *testing.T) {
	var ledger streamUsageLedger
	first := &types.EyrieUsage{PromptTokens: 13, CompletionTokens: 7, TotalTokens: 20}
	second := &types.EyrieUsage{PromptTokens: 20, CompletionTokens: 5, TotalTokens: 25}

	if !ledger.shouldRecord(first, false) || !ledger.shouldRecord(second, false) {
		t.Fatal("distinct continuation usage was dropped")
	}
	if ledger.shouldRecord(cloneStreamUsage(second), true) {
		t.Fatal("terminal repeat of the last continuation usage would be double-counted")
	}
}

func TestUpdateResolvedRoutePreservesMissingFields(t *testing.T) {
	provider, model, changed := updateResolvedRoute("openai", "openai/gpt-5", &types.ResolvedRoute{Model: "openai/gpt-5-mini"})
	if !changed || provider != "openai" || model != "openai/gpt-5-mini" {
		t.Fatalf("resolved route = %q/%q changed=%v", provider, model, changed)
	}
}

func TestRecordStreamUsageAttributesResolvedRoute(t *testing.T) {
	const (
		resolvedProvider = "openai"
		resolvedModel    = "openai/fallback-test-model"
	)
	RegisterLivePricing(resolvedModel, 2, 4)
	sess := NewSessionWithClient(NewMockClientForTest(), "anthropic", "anthropic/requested-test-model", "", nil, true)
	ch := make(chan StreamEvent, 1)

	sess.recordStreamUsage(ch, 1_000_000, 1_000_000, resolvedProvider, resolvedModel, "test", time.Now())

	if sess.Cost.Model != resolvedModel {
		t.Fatalf("cost model = %q, want resolved model %q", sess.Cost.Model, resolvedModel)
	}
	if got := sess.Cost.Total(); got != 6 {
		t.Fatalf("resolved-route cost = %v, want 6", got)
	}
	event := <-ch
	if event.Usage == nil || event.Usage.Provider != resolvedProvider || event.Usage.Model != resolvedModel {
		t.Fatalf("usage event lost resolved route: %+v", event.Usage)
	}
}
