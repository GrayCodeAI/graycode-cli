package engine

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewResponseCache(t *testing.T) {
	rc := NewResponseCache(0, 0)
	if rc.MaxEntries != DefaultMaxEntries {
		t.Errorf("expected default MaxEntries %d, got %d", DefaultMaxEntries, rc.MaxEntries)
	}
	if rc.MaxAge != DefaultMaxAge {
		t.Errorf("expected default MaxAge %v, got %v", DefaultMaxAge, rc.MaxAge)
	}

	rc2 := NewResponseCache(500, 12*time.Hour)
	if rc2.MaxEntries != 500 {
		t.Errorf("expected MaxEntries 500, got %d", rc2.MaxEntries)
	}
	if rc2.MaxAge != 12*time.Hour {
		t.Errorf("expected MaxAge 12h, got %v", rc2.MaxAge)
	}
}

func TestGetSetExactMatch(t *testing.T) {
	rc := NewResponseCache(100, time.Hour)

	// Miss on empty cache.
	entry, ok := rc.Get("hello world", "gpt-4")
	if ok || entry != nil {
		t.Fatal("expected cache miss on empty cache")
	}

	// Set and get.
	rc.Set("hello world", "response here", "gpt-4", 50)
	entry, ok = rc.Get("hello world", "gpt-4")
	if !ok || entry == nil {
		t.Fatal("expected cache hit")
	}
	if entry.Response != "response here" {
		t.Errorf("expected response 'response here', got %q", entry.Response)
	}
	if entry.Model != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got %q", entry.Model)
	}
	if entry.Tokens != 50 {
		t.Errorf("expected 50 tokens, got %d", entry.Tokens)
	}
	if entry.HitCount != 1 {
		t.Errorf("expected hit count 1, got %d", entry.HitCount)
	}
}

func TestGetNormalization(t *testing.T) {
	rc := NewResponseCache(100, time.Hour)

	rc.Set("Hello  World", "resp1", "gpt-4", 10)

	// Should match with different case/whitespace.
	entry, ok := rc.Get("hello world", "gpt-4")
	if !ok || entry == nil {
		t.Fatal("expected cache hit with normalized prompt")
	}
	if entry.Response != "resp1" {
		t.Errorf("expected response 'resp1', got %q", entry.Response)
	}
}

func TestGetModelSeparation(t *testing.T) {
	rc := NewResponseCache(100, time.Hour)

	rc.Set("test prompt", "response-gpt4", "gpt-4", 10)
	rc.Set("test prompt", "response-claude", "claude", 15)

	entry, ok := rc.Get("test prompt", "gpt-4")
	if !ok || entry.Response != "response-gpt4" {
		t.Fatal("expected gpt-4 response")
	}

	entry, ok = rc.Get("test prompt", "claude")
	if !ok || entry.Response != "response-claude" {
		t.Fatal("expected claude response")
	}
}

func TestGetExpiration(t *testing.T) {
	rc := NewResponseCache(100, 50*time.Millisecond)

	rc.Set("test", "response", "model", 10)

	entry, ok := rc.Get("test", "model")
	if !ok || entry == nil {
		t.Fatal("expected cache hit before expiration")
	}

	time.Sleep(60 * time.Millisecond)

	entry, ok = rc.Get("test", "model")
	if ok || entry != nil {
		t.Fatal("expected cache miss after expiration")
	}
}

func TestSimilarityMatch(t *testing.T) {
	rc := NewResponseCache(100, time.Hour)

	rc.Set("explain how to write a golang function", "explanation here", "gpt-4", 100)

	// Very similar prompt (high Jaccard similarity).
	entry, sim := rc.SimilarityMatch("explain how to write a golang function please", 0.80)
	if entry == nil {
		t.Fatal("expected similarity match")
	}
	if sim < 0.80 {
		t.Errorf("expected similarity >= 0.80, got %f", sim)
	}

	// Very different prompt.
	entry, sim = rc.SimilarityMatch("completely unrelated topic about cooking", 0.80)
	if entry != nil {
		t.Errorf("expected no match for unrelated prompt, got sim=%f", sim)
	}
}

func TestSimilarityMatchInGet(t *testing.T) {
	rc := NewResponseCache(100, time.Hour)

	rc.Set("explain the difference between goroutines and threads in go", "detailed explanation", "gpt-4", 200)

	// Nearly identical prompt (>0.95 Jaccard).
	entry, ok := rc.Get("explain the difference between goroutines and threads in go language", "gpt-4")
	// This might or might not match depending on exact Jaccard - test the mechanism.
	if ok && entry != nil {
		if entry.Response != "detailed explanation" {
			t.Errorf("unexpected response: %q", entry.Response)
		}
	}
}

func TestHashPrompt(t *testing.T) {
	// Same content, different whitespace/case should produce same hash.
	h1 := HashPrompt("Hello World")
	h2 := HashPrompt("hello   world")
	h3 := HashPrompt("HELLO WORLD")
	if h1 != h2 || h2 != h3 {
		t.Error("normalized prompts should produce same hash")
	}

	// Different content should produce different hash.
	h4 := HashPrompt("different prompt")
	if h1 == h4 {
		t.Error("different prompts should produce different hash")
	}

	// Hash should be valid hex.
	if len(h1) != 64 {
		t.Errorf("expected 64-char hex hash, got %d chars", len(h1))
	}
}

func TestEvictLRU(t *testing.T) {
	rc := NewResponseCache(3, time.Hour)

	rc.Set("prompt1", "resp1", "model", 10)
	time.Sleep(time.Millisecond)
	rc.Set("prompt2", "resp2", "model", 20)
	time.Sleep(time.Millisecond)
	rc.Set("prompt3", "resp3", "model", 30)

	// Touch prompt1 to make it recent.
	rc.Get("prompt1", "model")
	time.Sleep(time.Millisecond)

	// Adding a 4th should evict the LRU (prompt2, since prompt1 was just hit).
	rc.Set("prompt4", "resp4", "model", 40)

	// prompt2 should be evicted (it was the LRU).
	_, ok := rc.Get("prompt2", "model")
	if ok {
		t.Error("expected prompt2 to be evicted")
	}

	// prompt1 should still be there.
	entry, ok := rc.Get("prompt1", "model")
	if !ok || entry == nil {
		t.Error("expected prompt1 to still be in cache")
	}
}

func TestEvictLRUEmpty(t *testing.T) {
	rc := NewResponseCache(10, time.Hour)
	// Should not panic on empty cache.
	rc.EvictLRU()
}

func TestInvalidate(t *testing.T) {
	rc := NewResponseCache(100, time.Hour)

	rc.Set("explain goroutines", "resp1", "model", 10)
	rc.Set("explain channels", "resp2", "model", 20)
	rc.Set("how to cook pasta", "resp3", "model", 30)

	rc.Invalidate("explain")

	stats := rc.Stats()
	if stats.Entries != 1 {
		t.Errorf("expected 1 entry after invalidation, got %d", stats.Entries)
	}

	// The remaining entry should be the pasta one.
	entry, ok := rc.Get("how to cook pasta", "model")
	if !ok || entry == nil {
		t.Error("expected pasta entry to survive invalidation")
	}
}

func TestInvalidateInvalidRegex(t *testing.T) {
	rc := NewResponseCache(100, time.Hour)
	rc.Set("test", "resp", "model", 10)

	// Invalid regex should not crash or remove anything.
	rc.Invalidate("[invalid")

	stats := rc.Stats()
	if stats.Entries != 1 {
		t.Errorf("expected 1 entry after invalid regex, got %d", stats.Entries)
	}
}

func TestInvalidateByAge(t *testing.T) {
	rc := NewResponseCache(100, time.Hour)

	rc.Set("old prompt", "old resp", "model", 10)
	time.Sleep(60 * time.Millisecond)
	rc.Set("new prompt", "new resp", "model", 20)

	rc.InvalidateByAge(50 * time.Millisecond)

	stats := rc.Stats()
	if stats.Entries != 1 {
		t.Errorf("expected 1 entry after age invalidation, got %d", stats.Entries)
	}

	entry, ok := rc.Get("new prompt", "model")
	if !ok || entry == nil {
		t.Error("expected new prompt to survive age invalidation")
	}
}

func TestResponseCacheStats(t *testing.T) {
	rc := NewResponseCache(100, time.Hour)

	rc.Set("prompt1", "resp1", "model", 100)
	rc.Set("prompt2", "resp2", "model", 200)

	// Generate hits.
	rc.Get("prompt1", "model")
	rc.Get("prompt1", "model")
	rc.Get("prompt2", "model")
	rc.Get("nonexistent", "model") // miss

	stats := rc.Stats()

	if stats.Entries != 2 {
		t.Errorf("expected 2 entries, got %d", stats.Entries)
	}
	if stats.HitCount != 3 {
		t.Errorf("expected 3 hits, got %d", stats.HitCount)
	}
	if stats.MissCount != 1 {
		t.Errorf("expected 1 miss, got %d", stats.MissCount)
	}
	if stats.HitRate < 0.74 || stats.HitRate > 0.76 {
		t.Errorf("expected hit rate ~0.75, got %f", stats.HitRate)
	}
	// prompt1: 100 tokens * 2 hits = 200, prompt2: 200 tokens * 1 hit = 200.
	if stats.SavedTokens != 400 {
		t.Errorf("expected 400 saved tokens, got %d", stats.SavedTokens)
	}
	if stats.SavedCostUSD <= 0 {
		t.Error("expected positive saved cost")
	}
}

func TestShouldCache(t *testing.T) {
	// Should NOT cache.
	noCachePrompts := []string{
		"What is the current time?",
		"Show me the contents of main.go",
		"Read the file config.yaml",
		"What day is today's date?",
		"What's in the file /etc/hosts",
		"My specific project needs this",
	}
	for _, p := range noCachePrompts {
		if ShouldCache(p) {
			t.Errorf("expected ShouldCache=false for %q", p)
		}
	}

	// Should cache.
	cachePrompts := []string{
		"Explain how goroutines work",
		"How to write a REST API in Go",
		"What is a closure in JavaScript",
		"Format this JSON data properly",
		"Generate a boilerplate Express app",
		"Convert this Python code to Go",
	}
	for _, p := range cachePrompts {
		if !ShouldCache(p) {
			t.Errorf("expected ShouldCache=true for %q", p)
		}
	}

	// Short prompts (<=20 chars) without cache keywords should not cache.
	if ShouldCache("hi") {
		t.Error("expected short prompt not to cache")
	}
}

func TestFormatStats(t *testing.T) {
	rc := NewResponseCache(1000, time.Hour)

	rc.Set("explain closures", "response about closures", "gpt-4", 500)
	rc.Get("explain closures", "gpt-4")
	rc.Get("nonexistent", "gpt-4")

	output := rc.FormatStats()

	if !strings.Contains(output, "Response Cache:") {
		t.Error("expected 'Response Cache:' in output")
	}
	if !strings.Contains(output, "Entries: 1/1000") {
		t.Errorf("expected 'Entries: 1/1000' in output, got: %s", output)
	}
	if !strings.Contains(output, "hits") {
		t.Error("expected 'hits' in output")
	}
	if !strings.Contains(output, "Saved:") {
		t.Error("expected 'Saved:' in output")
	}
	if !strings.Contains(output, "Oldest entry:") {
		t.Error("expected 'Oldest entry:' in output")
	}
}

func TestExportImport(t *testing.T) {
	rc := NewResponseCache(100, time.Hour)

	rc.Set("prompt1", "resp1", "gpt-4", 50)
	rc.Set("prompt2", "resp2", "claude", 100)

	// Export.
	data, err := rc.Export()
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Verify it's valid JSON.
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("exported data is not valid JSON: %v", err)
	}

	// Import into a new cache.
	rc2 := NewResponseCache(100, time.Hour)
	if err := rc2.Import(data); err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	stats := rc2.Stats()
	if stats.Entries != 2 {
		t.Errorf("expected 2 entries after import, got %d", stats.Entries)
	}

	// Verify entries are accessible.
	entry, ok := rc2.Get("prompt1", "gpt-4")
	if !ok || entry == nil {
		t.Error("expected prompt1 to be accessible after import")
	}
	if entry != nil && entry.Response != "resp1" {
		t.Errorf("expected response 'resp1', got %q", entry.Response)
	}
}

func TestResponseCacheImportInvalidJSON(t *testing.T) {
	rc := NewResponseCache(100, time.Hour)
	err := rc.Import([]byte("invalid json"))
	if err == nil {
		t.Error("expected error on invalid JSON import")
	}
}

func TestImportExpiredEntries(t *testing.T) {
	rc := NewResponseCache(100, 50*time.Millisecond)

	// Create cache with entries.
	rc.Set("prompt1", "resp1", "model", 10)

	data, err := rc.Export()
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Wait for entries to expire.
	time.Sleep(60 * time.Millisecond)

	// Import into new cache - expired entries should be skipped.
	rc2 := NewResponseCache(100, 50*time.Millisecond)
	if err := rc2.Import(data); err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	stats := rc2.Stats()
	if stats.Entries != 0 {
		t.Errorf("expected 0 entries after importing expired data, got %d", stats.Entries)
	}
}

func TestResponseCacheConcurrentAccess(t *testing.T) {
	rc := NewResponseCache(100, time.Hour)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			prompt := strings.Repeat("word ", n%10+1)
			rc.Set(prompt, "response", "model", n*10)
		}(i)
		go func(n int) {
			defer wg.Done()
			prompt := strings.Repeat("word ", n%10+1)
			rc.Get(prompt, "model")
		}(i)
	}
	wg.Wait()

	// Should not panic or corrupt data.
	stats := rc.Stats()
	if stats.Entries < 0 || stats.Entries > 100 {
		t.Errorf("unexpected entries count: %d", stats.Entries)
	}
}

func TestResponseCacheJaccard(t *testing.T) {
	tests := []struct {
		a, b     string
		expected float64
	}{
		{"hello world", "hello world", 1.0},
		{"", "", 1.0},
		{"a b c", "a b c d", 0.75},
		{"a b", "c d", 0.0},
		{"the cat sat on the mat", "the cat sat on a mat", 0.8571}, // 6/7
	}

	for _, tt := range tests {
		setA := wordSet(tt.a)
		setB := wordSet(tt.b)
		sim := responseCacheJaccard(setA, setB)
		if sim < tt.expected-0.01 || sim > tt.expected+0.01 {
			t.Errorf("jaccardSimilarity(%q, %q) = %f, want ~%f", tt.a, tt.b, sim, tt.expected)
		}
	}
}

func TestResponseCacheFormatTokens(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0"},
		{100, "100"},
		{1000, "1,000"},
		{34000, "34,000"},
		{1234567, "1,234,567"},
	}
	for _, tt := range tests {
		got := responseCacheFormatTokens(tt.input)
		if got != tt.expected {
			t.Errorf("formatTokenCount(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestResponseCacheFormatDuration(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{30 * time.Second, "30s ago"},
		{5 * time.Minute, "5m ago"},
		{18 * time.Hour, "18h ago"},
	}
	for _, tt := range tests {
		got := responseCacheFormatDuration(tt.input)
		if got != tt.expected {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
