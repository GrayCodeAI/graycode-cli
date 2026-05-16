package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/hawk/routing"
	"github.com/GrayCodeAI/inspect"
	"github.com/GrayCodeAI/sight"
	"github.com/GrayCodeAI/tok"
	"github.com/GrayCodeAI/yaad/engine"
	"github.com/GrayCodeAI/yaad/graph"
	"github.com/GrayCodeAI/yaad/storage"
)

// mockSightProvider implements sight.Provider for integration testing.
type mockSightProvider struct {
	response string
}

func (m *mockSightProvider) Chat(_ context.Context, _ []sight.Message, _ sight.ChatOpts) (*sight.Response, error) {
	return &sight.Response{Content: m.response, TokensUsed: 100}, nil
}

// setupYaad creates a yaad engine backed by a temp SQLite database.
func setupYaad(t *testing.T) *engine.Engine {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	g := graph.New(store, store.DB())
	return engine.New(store, g)
}

func TestIntegration_SightReviewStoreRecall(t *testing.T) {
	// 1. Set up mock LLM provider that returns a code review finding
	mockResp := `[{"severity":"high","file":"main.go","line":10,"message":"SQL injection vulnerability","fix":"Use parameterized queries","reasoning":"Direct string concatenation in SQL"}]`
	provider := &mockSightProvider{response: mockResp}

	// 2. Run sight review with mock provider
	diff := `--- a/main.go
+++ b/main.go
@@ -8,0 +9,3 @@
+func query(input string) {
+    db.Exec("SELECT * FROM users WHERE name = '" + input + "'")
+}`

	ctx := context.Background()
	result, err := sight.Review(ctx, diff, sight.WithProvider(provider))
	if err != nil {
		t.Fatalf("sight.Review failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// 3. Store the review findings in yaad
	eng := setupYaad(t)
	findingsJSON, _ := json.Marshal(result.Findings)
	node, err := eng.Remember(ctx, engine.RememberInput{
		Content: fmt.Sprintf("Code review findings: %s", string(findingsJSON)),
		Type:    "decision",
		Tags:    "review,security",
		Project: "/test/project",
	})
	if err != nil {
		t.Fatalf("eng.Remember failed: %v", err)
	}
	if node == nil || node.ID == "" {
		t.Fatal("expected stored node with ID")
	}

	// 4. Recall the stored findings
	recalled, err := eng.Recall(ctx, engine.RecallOpts{
		Query:   "SQL injection review",
		Limit:   5,
		Project: "/test/project",
	})
	if err != nil {
		t.Fatalf("eng.Recall failed: %v", err)
	}
	if len(recalled.Nodes) == 0 {
		t.Fatal("expected to recall at least one node")
	}

	found := false
	for _, n := range recalled.Nodes {
		if n.ID == node.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("recalled nodes should contain the review finding node")
	}
}

func TestIntegration_InspectScanHTTPTest(t *testing.T) {
	// 1. Start a test HTTP server with known issues
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// Intentionally missing security headers
		fmt.Fprint(w, `<!DOCTYPE html><html>
		<head><title>Test Site</title></head>
		<body>
			<a href="/broken-page">Broken Link</a>
			<img src="/missing.png">
			<form action="/submit"><input type="text" name="q"></form>
		</body></html>`)
	}))
	defer ts.Close()

	// 2. Run inspect scan
	ctx := context.Background()
	report, err := inspect.Scan(ctx, ts.URL, inspect.Quick)
	if err != nil {
		t.Fatalf("inspect.Scan failed: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Target != ts.URL {
		t.Fatalf("expected target=%s, got %s", ts.URL, report.Target)
	}

	// 3. Store findings in yaad
	eng := setupYaad(t)
	reportJSON, _ := json.Marshal(report.Stats)
	_, err = eng.Remember(ctx, engine.RememberInput{
		Content: fmt.Sprintf("Site audit of %s: %d findings. %s", ts.URL, report.Stats.FindingsTotal, string(reportJSON)),
		Type:    "bug",
		Tags:    "inspect,audit",
	})
	if err != nil {
		t.Fatalf("eng.Remember failed: %v", err)
	}

	// 4. Verify recall
	recalled, err := eng.Recall(ctx, engine.RecallOpts{Query: "site audit", Limit: 5})
	if err != nil {
		t.Fatalf("eng.Recall failed: %v", err)
	}
	if len(recalled.Nodes) == 0 {
		t.Fatal("expected to recall audit results")
	}
}

func TestIntegration_TokCompression(t *testing.T) {
	// Generate a large repetitive text (simulates verbose CLI output)
	var large string
	for i := 0; i < 100; i++ {
		large += fmt.Sprintf("[INFO] 2025-05-08 10:00:%02d Processing item %d of 100... status=ok duration=12ms\n", i%60, i)
	}
	large += "[ERROR] 2025-05-08 10:01:00 Failed to process item 101: connection timeout\n"
	large += "[WARN] 2025-05-08 10:01:01 Retrying with backoff...\n"

	// 1. Compress with tok
	compressed, stats := tok.Compress(large)
	if stats.ReductionPercent <= 0 {
		t.Fatalf("expected positive compression, got %.1f%%", stats.ReductionPercent)
	}
	if compressed == "" {
		t.Fatal("compressed output should not be empty")
	}
	if stats.OriginalTokens <= stats.FinalTokens {
		t.Fatalf("expected fewer final tokens (%d) than original (%d)", stats.FinalTokens, stats.OriginalTokens)
	}

	// 2. Verify token estimation works
	tokens := tok.EstimateTokens(large)
	if tokens == 0 {
		t.Fatal("EstimateTokens should return non-zero for non-empty text")
	}

	// 3. Store compression stats in yaad
	eng := setupYaad(t)
	ctx := context.Background()
	_, err := eng.Remember(ctx, engine.RememberInput{
		Content: fmt.Sprintf("Token compression: %d → %d tokens (%.1f%% reduction)", stats.OriginalTokens, stats.FinalTokens, stats.ReductionPercent),
		Type:    "convention",
		Tags:    "tok,performance",
	})
	if err != nil {
		t.Fatalf("eng.Remember failed: %v", err)
	}
}

func TestIntegration_CascadeRouting(t *testing.T) {
	roles := routing.ModelRoles{
		Planner:  "claude-opus-4-20250514",
		Coder:    "claude-sonnet-4-20250514",
		Reviewer: "claude-sonnet-4-20250514",
		Commit:   "claude-haiku-4-20250514",
	}
	cr := routing.NewCascadeRouter(roles)

	tests := []struct {
		msg       string
		wantType  routing.TaskType
		wantModel string
	}{
		{"Design the architecture for the payment system", routing.TaskPlanning, "claude-opus-4-20250514"},
		{"Implement the payment endpoint", routing.TaskCoding, "claude-sonnet-4-20250514"},
		{"Summarize what we did today", routing.TaskSummary, "claude-haiku-4-20250514"},
		{"Review this pull request for security issues", routing.TaskReview, "claude-sonnet-4-20250514"},
	}

	for _, tt := range tests {
		taskType := routing.ClassifyTask(tt.msg)
		if taskType != tt.wantType {
			t.Errorf("ClassifyTask(%q) = %s, want %s", tt.msg, taskType, tt.wantType)
		}

		model := cr.Route(tt.msg, "")
		if model != tt.wantModel {
			t.Errorf("Route(%q) = %s, want %s", tt.msg, model, tt.wantModel)
		}
	}
}

func TestIntegration_FullPipeline(t *testing.T) {
	ctx := context.Background()

	// 1. Initialize all components
	eng := setupYaad(t)

	// 2. Use tok to compress some context
	verbose := "This is a very long and verbose piece of text that contains a lot of redundant information that could be compressed significantly by removing unnecessary repetition and verbosity from the text content."
	compressed, _ := tok.Compress(verbose)
	if compressed == "" {
		t.Log("no compression applied, using original")
	}

	// 3. Use sight to review code (mock)
	mockResp := `[{"severity":"medium","file":"app.go","line":5,"message":"Unused variable","fix":"Remove unused var"}]`
	provider := &mockSightProvider{response: mockResp}
	diff := "--- a/app.go\n+++ b/app.go\n@@ -4,0 +5 @@\n+var unused = 42"
	reviewResult, err := sight.Review(ctx, diff, sight.WithProvider(provider))
	if err != nil {
		t.Fatalf("sight.Review: %v", err)
	}

	// 4. Store in yaad
	_, err = eng.Remember(ctx, engine.RememberInput{
		Content: fmt.Sprintf("Review: %d findings in app.go", len(reviewResult.Findings)),
		Type:    "bug",
		Project: "/test",
	})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	// 5. Use inspect on a test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>OK</title></head><body>OK</body></html>`)
	}))
	defer ts.Close()

	report, err := inspect.Scan(ctx, ts.URL, inspect.Quick)
	if err != nil {
		t.Fatalf("inspect.Scan: %v", err)
	}

	// 6. Store inspect result
	_, err = eng.Remember(ctx, engine.RememberInput{
		Content: fmt.Sprintf("Audit %s: %d pages, %d issues", ts.URL, report.Stats.PagesScanned, report.Stats.FindingsTotal),
		Type:    "spec",
		Project: "/test",
	})
	if err != nil {
		t.Fatalf("Remember audit: %v", err)
	}

	// 7. Use cascade routing to determine model
	roles := routing.ModelRoles{Planner: "opus", Coder: "sonnet", Commit: "haiku", Reviewer: "sonnet"}
	cr := routing.NewCascadeRouter(roles)
	model := cr.Route("Summarize what we did today", "")
	if model != "haiku" {
		t.Fatalf("expected haiku for summary, got %s", model)
	}

	// 8. Verify all data is recallable
	status, err := eng.Status(ctx, "/test")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Nodes < 2 {
		t.Fatalf("expected at least 2 nodes, got %d", status.Nodes)
	}

	// 9. Recall and verify cross-module data flows
	recalled, err := eng.Recall(ctx, engine.RecallOpts{Query: "findings", Project: "/test", Limit: 10})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(recalled.Nodes) == 0 {
		t.Fatal("expected recalled nodes from full pipeline")
	}
}
