package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockProvider is a test helper that implements ContextProvider.
type mockProvider struct {
	name        string
	description string
	budget      int
	items       []ContextItem
	err         error
	gatherDelay time.Duration
	called      int32 // atomic counter
}

func (m *mockProvider) Name() string        { return m.name }
func (m *mockProvider) Description() string { return m.description }
func (m *mockProvider) TokenBudget() int    { return m.budget }

func (m *mockProvider) Gather(ctx context.Context, query string) ([]ContextItem, error) {
	atomic.AddInt32(&m.called, 1)
	if m.gatherDelay > 0 {
		time.Sleep(m.gatherDelay)
	}
	if m.err != nil {
		return nil, m.err
	}
	return m.items, nil
}

func (m *mockProvider) callCount() int {
	return int(atomic.LoadInt32(&m.called))
}

func TestRegistrationAndRetrieval(t *testing.T) {
	cm := NewContextManager(1000)

	if len(cm.Providers) != 0 {
		t.Fatalf("expected empty providers, got %d", len(cm.Providers))
	}

	p1 := &mockProvider{name: "test1", description: "Test 1", budget: 100}
	p2 := &mockProvider{name: "test2", description: "Test 2", budget: 200}

	cm.Register(p1)
	cm.Register(p2)

	if len(cm.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(cm.Providers))
	}

	if cm.Providers[0].Name() != "test1" {
		t.Errorf("expected first provider name 'test1', got '%s'", cm.Providers[0].Name())
	}
	if cm.Providers[1].Name() != "test2" {
		t.Errorf("expected second provider name 'test2', got '%s'", cm.Providers[1].Name())
	}
}

func TestGatherAllCallsAllProviders(t *testing.T) {
	cm := NewContextManager(5000)

	p1 := &mockProvider{
		name: "p1", budget: 100,
		items: []ContextItem{{Source: "p1", Title: "Item1", Content: "content1", Relevance: 0.9, TokenCount: 10}},
	}
	p2 := &mockProvider{
		name: "p2", budget: 100,
		items: []ContextItem{{Source: "p2", Title: "Item2", Content: "content2", Relevance: 0.8, TokenCount: 10}},
	}
	p3 := &mockProvider{
		name: "p3", budget: 100,
		items: []ContextItem{{Source: "p3", Title: "Item3", Content: "content3", Relevance: 0.7, TokenCount: 10}},
	}

	cm.Register(p1)
	cm.Register(p2)
	cm.Register(p3)

	items, err := cm.GatherAll(context.Background(), "test query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p1.callCount() != 1 {
		t.Errorf("expected p1 called once, got %d", p1.callCount())
	}
	if p2.callCount() != 1 {
		t.Errorf("expected p2 called once, got %d", p2.callCount())
	}
	if p3.callCount() != 1 {
		t.Errorf("expected p3 called once, got %d", p3.callCount())
	}

	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	// Should be sorted by relevance
	if items[0].Source != "p1" || items[1].Source != "p2" || items[2].Source != "p3" {
		t.Errorf("items not sorted by relevance: %v", items)
	}
}

func TestBudgetEnforcement(t *testing.T) {
	cm := NewContextManager(50) // very small budget

	items := make([]ContextItem, 10)
	for i := range items {
		items[i] = ContextItem{
			Source:     "big",
			Title:      fmt.Sprintf("Item%d", i),
			Content:    strings.Repeat("x", 100), // ~25 tokens each
			Relevance:  float64(10-i) / 10.0,
			TokenCount: 25,
		}
	}

	p := &mockProvider{name: "big", budget: 1000, items: items}
	cm.Register(p)

	result, err := cm.GatherAll(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Budget is 50, each item is 25 tokens, so max 2 items
	if len(result) > 2 {
		t.Errorf("expected at most 2 items within budget, got %d", len(result))
	}

	// Verify total tokens within budget
	total := 0
	for _, item := range result {
		total += item.TokenCount
	}
	if total > 50 {
		t.Errorf("total tokens %d exceeds budget 50", total)
	}
}

func TestContextProviderFormatContext(t *testing.T) {
	items := []ContextItem{
		{Source: "git", Title: "Current Branch", Content: "main", Relevance: 0.9},
		{Source: "files", Title: "Recent Files", Content: "file1.go\nfile2.go", Relevance: 0.7},
	}

	output := FormatContextItems(items)

	if !strings.Contains(output, "## Relevant Context") {
		t.Error("missing header")
	}
	if !strings.Contains(output, "### [Source: git] Current Branch") {
		t.Error("missing git source header")
	}
	if !strings.Contains(output, "main") {
		t.Error("missing branch content")
	}
	if !strings.Contains(output, "### [Source: files] Recent Files") {
		t.Error("missing files source header")
	}
	if !strings.Contains(output, "file1.go\nfile2.go") {
		t.Error("missing files content")
	}
}

func TestFormatContextEmpty(t *testing.T) {
	output := FormatContextItems(nil)
	if output != "" {
		t.Errorf("expected empty string for nil items, got %q", output)
	}

	output = FormatContextItems([]ContextItem{})
	if output != "" {
		t.Errorf("expected empty string for empty items, got %q", output)
	}
}

func TestPrioritizeItemsSelection(t *testing.T) {
	items := []ContextItem{
		{Source: "a", Title: "Low", Content: "xx", Relevance: 0.1, TokenCount: 10},
		{Source: "a", Title: "High", Content: "xx", Relevance: 0.9, TokenCount: 10},
		{Source: "a", Title: "Med", Content: "xx", Relevance: 0.5, TokenCount: 10},
	}

	result := PrioritizeItems(items, 20)

	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result))
	}

	// Highest relevance items should be selected first
	if result[0].Relevance < result[1].Relevance {
		t.Error("items not in relevance order")
	}
	if result[0].Title != "High" {
		t.Errorf("expected 'High' first, got %q", result[0].Title)
	}
}

func TestPrioritizeItemsDiversity(t *testing.T) {
	// Create items where one source dominates by relevance
	items := []ContextItem{
		{Source: "dominant", Title: "D1", Content: "xx", Relevance: 0.99, TokenCount: 10},
		{Source: "dominant", Title: "D2", Content: "xx", Relevance: 0.98, TokenCount: 10},
		{Source: "dominant", Title: "D3", Content: "xx", Relevance: 0.97, TokenCount: 10},
		{Source: "dominant", Title: "D4", Content: "xx", Relevance: 0.96, TokenCount: 10},
		{Source: "dominant", Title: "D5", Content: "xx", Relevance: 0.95, TokenCount: 10},
		{Source: "other", Title: "O1", Content: "xx", Relevance: 0.5, TokenCount: 10},
		{Source: "other", Title: "O2", Content: "xx", Relevance: 0.4, TokenCount: 10},
	}

	result := PrioritizeItems(items, 1000) // Large budget so diversity is the constraint

	// Count per source
	sourceCounts := make(map[string]int)
	for _, item := range result {
		sourceCounts[item.Source]++
	}

	// The "dominant" source should not take all slots
	if sourceCounts["other"] == 0 {
		t.Error("diversity enforcement failed: 'other' source has no items")
	}

	// dominant should be limited
	if sourceCounts["dominant"] > sourceCounts["other"]+3 {
		t.Errorf("dominant source too many items: dominant=%d other=%d",
			sourceCounts["dominant"], sourceCounts["other"])
	}
}

func TestPrioritizeItemsEmptyAndZeroBudget(t *testing.T) {
	result := PrioritizeItems(nil, 100)
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}

	result = PrioritizeItems([]ContextItem{{Source: "a", TokenCount: 10}}, 0)
	if result != nil {
		t.Errorf("expected nil for zero budget, got %v", result)
	}
}

func TestEmptyProviderList(t *testing.T) {
	cm := NewContextManager(1000)

	items, err := cm.GatherAll(context.Background(), "query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items != nil {
		t.Errorf("expected nil items for empty provider list, got %v", items)
	}
}

func TestProviderErrorHandling(t *testing.T) {
	cm := NewContextManager(5000)

	goodProvider := &mockProvider{
		name: "good", budget: 100,
		items: []ContextItem{{Source: "good", Title: "Good Item", Content: "works", Relevance: 0.8, TokenCount: 5}},
	}
	badProvider := &mockProvider{
		name: "bad", budget: 100,
		err: fmt.Errorf("provider failed"),
	}
	anotherGood := &mockProvider{
		name: "good2", budget: 100,
		items: []ContextItem{{Source: "good2", Title: "Also Good", Content: "also works", Relevance: 0.7, TokenCount: 5}},
	}

	cm.Register(goodProvider)
	cm.Register(badProvider)
	cm.Register(anotherGood)

	items, err := cm.GatherAll(context.Background(), "")
	if err != nil {
		t.Fatalf("GatherAll should not return error when individual providers fail: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items (from non-failing providers), got %d", len(items))
	}

	// Verify the bad provider was still called
	if badProvider.callCount() != 1 {
		t.Errorf("expected bad provider called once, got %d", badProvider.callCount())
	}
}

func TestEnvironmentContextProviderReturnsRealData(t *testing.T) {
	provider := &EnvironmentContextProvider{}

	items, err := provider.Gather(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0]
	if item.Source != "environment" {
		t.Errorf("expected source 'environment', got %q", item.Source)
	}

	// Should contain OS info
	if !strings.Contains(item.Content, "OS:") {
		t.Error("missing OS info in environment context")
	}

	// Should contain Working Dir
	if !strings.Contains(item.Content, "Working Dir:") {
		t.Error("missing working dir in environment context")
	}
}

func TestConcurrentGathering(t *testing.T) {
	cm := NewContextManager(5000)

	// Create providers with delays to test parallel execution
	numProviders := 5
	for i := 0; i < numProviders; i++ {
		p := &mockProvider{
			name:        fmt.Sprintf("slow%d", i),
			budget:      100,
			gatherDelay: 50 * time.Millisecond,
			items: []ContextItem{{
				Source:     fmt.Sprintf("slow%d", i),
				Title:      fmt.Sprintf("Item%d", i),
				Content:    "content",
				Relevance:  float64(i) / float64(numProviders),
				TokenCount: 10,
			}},
		}
		cm.Register(p)
	}

	start := time.Now()
	items, err := cm.GatherAll(context.Background(), "")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != numProviders {
		t.Errorf("expected %d items, got %d", numProviders, len(items))
	}

	// If run sequentially, would take ~250ms. Parallel should be much less.
	// Allow generous margin for CI environments.
	if elapsed > 200*time.Millisecond {
		t.Errorf("gathering took %v, expected parallel execution under 200ms", elapsed)
	}
}

func TestConcurrentRegistration(t *testing.T) {
	cm := NewContextManager(5000)

	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			p := &mockProvider{name: fmt.Sprintf("concurrent%d", idx), budget: 100}
			cm.Register(p)
		}(i)
	}

	wg.Wait()

	if len(cm.Providers) != numGoroutines {
		t.Errorf("expected %d providers after concurrent registration, got %d", numGoroutines, len(cm.Providers))
	}
}

func TestErrorContextProvider(t *testing.T) {
	// Create a temp directory with error logs
	tmpDir, err := os.MkdirTemp("", "error-context-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create some log files
	os.WriteFile(filepath.Join(tmpDir, "build.log"), []byte("error: undefined variable x"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "test.err"), []byte("FAIL: TestFoo timeout"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("not an error file"), 0o644)

	provider := &ErrorContextProvider{LogDir: tmpDir}

	items, err := provider.Gather(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should pick up .log and .err files but not .txt
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	for _, item := range items {
		if item.Source != "errors" {
			t.Errorf("expected source 'errors', got %q", item.Source)
		}
	}
}

func TestErrorContextProviderNonexistentDir(t *testing.T) {
	provider := &ErrorContextProvider{LogDir: "/nonexistent/path/xyz"}

	items, err := provider.Gather(context.Background(), "")
	if err != nil {
		t.Fatalf("should not error for nonexistent dir, got: %v", err)
	}
	if items != nil {
		t.Errorf("expected nil items for nonexistent dir, got %v", items)
	}
}

func TestDependencyContextProvider(t *testing.T) {
	// Create a temp directory with a go.mod
	tmpDir, err := os.MkdirTemp("", "dep-context-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	goMod := `module example.com/test

go 1.21

require (
	github.com/stretchr/testify v1.8.0
)
`
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0o644)

	provider := &DependencyContextProvider{ProjectDir: tmpDir}

	items, err := provider.Gather(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item (go.mod only), got %d", len(items))
	}

	if !strings.Contains(items[0].Content, "github.com/stretchr/testify") {
		t.Error("expected go.mod content in item")
	}
}

func TestFileContextProvider(t *testing.T) {
	// Create a temp directory with some files
	tmpDir, err := os.MkdirTemp("", "file-context-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create source files with different mod times
	files := []string{"a.go", "b.go", "c.js"}
	for i, name := range files {
		path := filepath.Join(tmpDir, name)
		os.WriteFile(path, []byte("package main"), 0o644)
		// Stagger modification times
		modTime := time.Now().Add(time.Duration(-i) * time.Hour)
		os.Chtimes(path, modTime, modTime)
	}

	// Create a non-source file that should be excluded
	os.WriteFile(filepath.Join(tmpDir, "data.txt"), []byte("not source"), 0o644)

	provider := &FileContextProvider{RepoDir: tmpDir, MaxFiles: 10}

	items, err := provider.Gather(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 context item, got %d", len(items))
	}

	content := items[0].Content
	if !strings.Contains(content, "a.go") {
		t.Error("expected a.go in file list")
	}
	if !strings.Contains(content, "b.go") {
		t.Error("expected b.go in file list")
	}
	if !strings.Contains(content, "c.js") {
		t.Error("expected c.js in file list")
	}
	if strings.Contains(content, "data.txt") {
		t.Error("data.txt should not be in file list")
	}
}

func TestGitContextProviderInterface(t *testing.T) {
	provider := &GitContextProvider{RepoDir: "/tmp", MaxCommits: 5}

	if provider.Name() != "git" {
		t.Errorf("expected name 'git', got %q", provider.Name())
	}
	if provider.TokenBudget() != 500 {
		t.Errorf("expected budget 500, got %d", provider.TokenBudget())
	}
	if provider.Description() == "" {
		t.Error("description should not be empty")
	}
}
