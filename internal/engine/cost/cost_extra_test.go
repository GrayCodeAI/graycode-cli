package cost

import (
	"strings"
	"testing"
)

func TestCost_Add(t *testing.T) {
	c := &Cost{}
	c.Add(100, 50)
	if c.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d, want 100", c.PromptTokens)
	}
	if c.CompletionTokens != 50 {
		t.Errorf("CompletionTokens = %d, want 50", c.CompletionTokens)
	}
}

func TestCost_AddForModel(t *testing.T) {
	c := &Cost{}
	c.AddForModel("gpt-4", 100, 50)
	if c.Model != "gpt-4" {
		t.Errorf("Model = %q, want %q", c.Model, "gpt-4")
	}
	if c.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d, want 100", c.PromptTokens)
	}
}

func TestCost_AddForModel_EmptyModel(t *testing.T) {
	c := &Cost{Model: "existing-model"}
	c.AddForModel("", 100, 50)
	// Model should not change when empty
	if c.Model != "existing-model" {
		t.Errorf("Model = %q, want %q", c.Model, "existing-model")
	}
}

func TestCost_AddForModel_WhitespaceModel(t *testing.T) {
	c := &Cost{Model: "existing-model"}
	c.AddForModel("  ", 100, 50)
	// Model should not change when whitespace
	if c.Model != "existing-model" {
		t.Errorf("Model = %q, want %q", c.Model, "existing-model")
	}
}

func TestCost_AddCacheTokens(t *testing.T) {
	c := &Cost{Model: "gpt-4"}
	c.AddCacheTokens(1000, 500)
	if c.CacheReadTokens != 1000 {
		t.Errorf("CacheReadTokens = %d, want 1000", c.CacheReadTokens)
	}
	if c.CacheWriteTokens != 500 {
		t.Errorf("CacheWriteTokens = %d, want 500", c.CacheWriteTokens)
	}
}

func TestCost_Total(t *testing.T) {
	c := &Cost{TotalCostUSD: 1.50}
	if c.Total() != 1.50 {
		t.Errorf("Total() = %f, want %f", c.Total(), 1.50)
	}
}

func TestCost_TotalUSD(t *testing.T) {
	c := &Cost{TotalCostUSD: 2.50}
	if c.TotalUSD() != 2.50 {
		t.Errorf("TotalUSD() = %f, want %f", c.TotalUSD(), 2.50)
	}
}

func TestCost_Summary(t *testing.T) {
	c := &Cost{
		PromptTokens:     100,
		CompletionTokens: 50,
		CacheReadTokens:  10,
		CacheWriteTokens: 5,
		TotalCostUSD:     0.0015,
		Model:            "gpt-4",
	}
	summary := c.Summary()
	if !strings.Contains(summary, "100 in / 50 out") {
		t.Errorf("Summary should contain token counts, got %q", summary)
	}
	if !strings.Contains(summary, "Cache: 10 read / 5 write") {
		t.Errorf("Summary should contain cache info, got %q", summary)
	}
	if !strings.Contains(summary, "$0.0015") {
		t.Errorf("Summary should contain cost, got %q", summary)
	}
	if !strings.Contains(summary, "gpt-4") {
		t.Errorf("Summary should contain model, got %q", summary)
	}
}

func TestCost_Summary_NoCache(t *testing.T) {
	c := &Cost{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalCostUSD:     0.0015,
		Model:            "gpt-4",
	}
	summary := c.Summary()
	if strings.Contains(summary, "Cache:") {
		t.Errorf("Summary should not contain cache info, got %q", summary)
	}
}

func TestCost_AddMultiple(t *testing.T) {
	c := &Cost{Model: "gpt-4"}
	c.Add(100, 50)
	c.Add(200, 100)
	if c.PromptTokens != 300 {
		t.Errorf("PromptTokens = %d, want 300", c.PromptTokens)
	}
	if c.CompletionTokens != 150 {
		t.Errorf("CompletionTokens = %d, want 150", c.CompletionTokens)
	}
}
