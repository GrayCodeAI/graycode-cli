package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	sightLib "github.com/GrayCodeAI/sight"
)

func TestReviewStore_CreateAndGet(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".hawk"), 0o755)

	store, err := OpenReviewStore(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	id, err := store.Create("abc123def456")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive ID, got %d", id)
	}

	r, err := store.Get(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if r.SHA != "abc123def456" {
		t.Errorf("expected sha abc123def456, got %s", r.SHA)
	}
	if r.Status != ReviewStatusPending {
		t.Errorf("expected pending status, got %s", r.Status)
	}
}

func TestReviewStore_Update(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".hawk"), 0o755)

	store, err := OpenReviewStore(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	id, _ := store.Create("sha123")

	result := &sightLib.Result{
		Findings: []sightLib.Finding{
			{Concern: "security", Severity: sightLib.SeverityHigh, File: "main.go", Line: 10, Message: "SQL injection"},
			{Concern: "bugs", Severity: sightLib.SeverityMedium, File: "handler.go", Line: 20, Message: "nil deref"},
		},
		Stats: sightLib.Stats{TokensUsed: 500},
	}

	err = store.Update(id, ReviewStatusOpen, result)
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	r, _ := store.Get(id)
	if r.Status != ReviewStatusOpen {
		t.Errorf("expected open, got %s", r.Status)
	}
	if len(r.Findings) != 2 {
		t.Errorf("expected 2 findings, got %d", len(r.Findings))
	}
	if r.MaxSeverity != "high" {
		t.Errorf("expected max severity high, got %s", r.MaxSeverity)
	}
	if r.TokensUsed != 500 {
		t.Errorf("expected 500 tokens, got %d", r.TokensUsed)
	}
}

func TestReviewStore_GetBySHA(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".hawk"), 0o755)

	store, err := OpenReviewStore(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	store.Create("sha_first")
	store.Create("sha_second")

	r, err := store.GetBySHA("sha_second")
	if err != nil {
		t.Fatalf("get by sha: %v", err)
	}
	if r.SHA != "sha_second" {
		t.Errorf("expected sha_second, got %s", r.SHA)
	}
}

func TestReviewStore_ListOpen(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".hawk"), 0o755)

	store, err := OpenReviewStore(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	id1, _ := store.Create("sha1")
	id2, _ := store.Create("sha2")
	id3, _ := store.Create("sha3")

	store.SetStatus(id1, ReviewStatusOpen)
	store.SetStatus(id2, ReviewStatusPassed)
	store.SetStatus(id3, ReviewStatusOpen)

	open, err := store.ListOpen()
	if err != nil {
		t.Fatalf("list open: %v", err)
	}
	if len(open) != 2 {
		t.Errorf("expected 2 open reviews, got %d", len(open))
	}
}

func TestReviewStore_Summary(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".hawk"), 0o755)

	store, err := OpenReviewStore(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	id1, _ := store.Create("s1")
	id2, _ := store.Create("s2")
	id3, _ := store.Create("s3")

	store.SetStatus(id1, ReviewStatusOpen)
	store.SetStatus(id2, ReviewStatusPassed)
	store.SetStatus(id3, ReviewStatusOpen)

	summary, err := store.Summary()
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary[ReviewStatusOpen] != 2 {
		t.Errorf("expected 2 open, got %d", summary[ReviewStatusOpen])
	}
	if summary[ReviewStatusPassed] != 1 {
		t.Errorf("expected 1 passed, got %d", summary[ReviewStatusPassed])
	}
}

func TestReviewStore_SetStatus(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".hawk"), 0o755)

	store, err := OpenReviewStore(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	id, _ := store.Create("sha_close")
	store.SetStatus(id, ReviewStatusOpen)
	store.SetStatus(id, ReviewStatusClosed)

	r, _ := store.Get(id)
	if r.Status != ReviewStatusClosed {
		t.Errorf("expected closed, got %s", r.Status)
	}
}

func TestBuildFixPrompt(t *testing.T) {
	r := &ReviewRecord{
		SHA: "abc12345deadbeef0000000000000000000000ff",
		Findings: []sightLib.Finding{
			{Severity: sightLib.SeverityHigh, File: "main.go", Line: 10, Message: "SQL injection", Fix: "use parameterized query"},
		},
	}

	prompt := buildFixPrompt(r)
	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
	if !strings.Contains(prompt, "abc12345") {
		t.Error("prompt should contain short SHA")
	}
	if !strings.Contains(prompt, "SQL injection") {
		t.Error("prompt should contain finding message")
	}
	if !strings.Contains(prompt, "parameterized query") {
		t.Error("prompt should contain suggested fix")
	}
}

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		status ReviewStatus
		want   string
	}{
		{ReviewStatusPassed, "✓"},
		{ReviewStatusOpen, "⚠"},
		{ReviewStatusFailed, "✗"},
		{ReviewStatusRunning, "⟳"},
		{ReviewStatusPending, "·"},
	}
	for _, tt := range tests {
		got := statusIcon(tt.status)
		if got != tt.want {
			t.Errorf("statusIcon(%s) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestAnalysisPrompts_AllTypesExist(t *testing.T) {
	expected := []string{"security", "duplication", "complexity", "dead-code", "refactor", "test-fixtures"}
	for _, typ := range expected {
		if _, ok := analysisPrompts[typ]; !ok {
			t.Errorf("missing analysis prompt for type %q", typ)
		}
	}
}

func TestHookScript_ContainsHawkReview(t *testing.T) {
	if !strings.Contains(hookScript, "hawk review") {
		t.Error("hook script should contain 'hawk review'")
	}
	if !strings.Contains(hookScript, "git rev-parse HEAD") {
		t.Error("hook script should get HEAD sha")
	}
}
