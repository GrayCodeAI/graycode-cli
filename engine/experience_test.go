package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewExperienceStore(t *testing.T) {
	store := NewExperienceStore("/tmp/test-exp")
	if store == nil {
		t.Fatal("expected non-nil store")
	}
	if store.Dir != "/tmp/test-exp" {
		t.Errorf("expected dir /tmp/test-exp, got %s", store.Dir)
	}
	if len(store.Experiences) != 0 {
		t.Errorf("expected empty experiences, got %d", len(store.Experiences))
	}
}

func TestRecord(t *testing.T) {
	store := NewExperienceStore("")

	exp := store.Record(
		"Implement JWT authentication middleware",
		"Created middleware function, added token validation, integrated with router",
		"success",
		[]string{"Read existing auth code", "Write middleware", "Add tests", "Verify token expiry checked before claims"},
		[]string{"Read", "Edit", "Bash"},
		[]string{"pkg/auth/middleware.go", "pkg/auth/middleware_test.go"},
		1500,
		3*time.Minute,
	)

	if exp == nil {
		t.Fatal("expected non-nil experience")
	}
	if exp.ID == "" {
		t.Error("expected non-empty ID")
	}
	if exp.Task != "Implement JWT authentication middleware" {
		t.Errorf("unexpected task: %s", exp.Task)
	}
	if exp.Score != 1.0 {
		t.Errorf("expected score 1.0 for success, got %f", exp.Score)
	}
	if len(exp.Tags) == 0 {
		t.Error("expected tags to be auto-generated")
	}
	// Should have "jwt", "auth", "implement", "middleware"
	hasJWT := false
	hasAuth := false
	for _, tag := range exp.Tags {
		if tag == "jwt" {
			hasJWT = true
		}
		if tag == "auth" {
			hasAuth = true
		}
	}
	if !hasJWT {
		t.Error("expected 'jwt' tag")
	}
	if !hasAuth {
		t.Error("expected 'auth' tag")
	}

	if len(store.Experiences) != 1 {
		t.Errorf("expected 1 experience, got %d", len(store.Experiences))
	}
}

func TestRecordOutcomeScoring(t *testing.T) {
	store := NewExperienceStore("")

	tests := []struct {
		outcome  string
		expected float64
	}{
		{"success", 1.0},
		{"Success - all tests pass", 1.0},
		{"partial", 0.5},
		{"Partial success", 0.5},
		{"failure", 0.0},
		{"failed to compile", 0.0},
		{"unknown outcome", 0.5},
	}

	for _, tt := range tests {
		exp := store.Record(tt.outcome, "", tt.outcome, nil, nil, nil, 0, 0)
		if exp.Score != tt.expected {
			t.Errorf("outcome %q: expected score %f, got %f", tt.outcome, tt.expected, exp.Score)
		}
	}
}

func TestFindRelevant(t *testing.T) {
	store := NewExperienceStore("")

	// Record several experiences
	store.Record(
		"Implement JWT authentication middleware",
		"Created middleware, added token validation",
		"success",
		[]string{"Read code", "Write middleware"},
		[]string{"Read", "Edit"},
		[]string{"auth.go"},
		1000, 2*time.Minute,
	)

	store.Record(
		"Fix race condition in HTTP handler",
		"Added mutex, used defer for unlock",
		"success",
		[]string{"Identified shared state", "Added sync.Mutex"},
		[]string{"Read", "Edit", "Bash"},
		[]string{"handler.go"},
		800, 1*time.Minute,
	)

	store.Record(
		"Add database migration for users table",
		"Created migration file, ran migrate up",
		"success",
		[]string{"Write migration SQL", "Test rollback"},
		[]string{"Write", "Bash"},
		[]string{"migrations/001_users.sql"},
		500, 30*time.Second,
	)

	// Search for auth-related task
	results := store.FindRelevant("Add OAuth token authentication", 2)
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	// The JWT auth experience should be the top match
	if results[0].Task != "Implement JWT authentication middleware" {
		t.Errorf("expected JWT auth as top match, got: %s", results[0].Task)
	}

	// Search for race condition task
	results = store.FindRelevant("Fix data race in concurrent handler", 2)
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Task != "Fix race condition in HTTP handler" {
		t.Errorf("expected race condition as top match, got: %s", results[0].Task)
	}
}

func TestFindRelevantEmpty(t *testing.T) {
	store := NewExperienceStore("")
	results := store.FindRelevant("anything", 5)
	if results != nil {
		t.Errorf("expected nil results for empty store, got %v", results)
	}
}

func TestFindRelevantZeroLimit(t *testing.T) {
	store := NewExperienceStore("")
	store.Record("test task", "approach", "success", nil, nil, nil, 100, time.Second)
	results := store.FindRelevant("test task", 0)
	if results != nil {
		t.Errorf("expected nil results for zero limit, got %v", results)
	}
}

func TestBuildExperienceContext(t *testing.T) {
	store := NewExperienceStore("")

	store.Record(
		"Implement JWT auth",
		"Created middleware, added to router, wrote tests",
		"success",
		[]string{"Always validate token expiry before claims access"},
		[]string{"Read", "Edit", "Bash"},
		[]string{"auth.go"},
		1000, 3*time.Minute,
	)

	ctx := store.BuildExperienceContext("Add JWT token validation", 500)
	if ctx == "" {
		t.Fatal("expected non-empty context")
	}

	if !containsSubstring(ctx, "Relevant Past Experiences") {
		t.Error("expected header in context")
	}
	if !containsSubstring(ctx, "JWT auth") {
		t.Error("expected task name in context")
	}
	if !containsSubstring(ctx, "success") {
		t.Error("expected outcome in context")
	}
	if !containsSubstring(ctx, "Read, Edit, Bash") {
		t.Error("expected tools in context")
	}
}

func TestBuildExperienceContextEmpty(t *testing.T) {
	store := NewExperienceStore("")
	ctx := store.BuildExperienceContext("unrelated task xyz", 500)
	if ctx != "" {
		t.Errorf("expected empty context for unrelated task, got: %s", ctx)
	}
}

func TestBuildExperienceContextTokenLimit(t *testing.T) {
	store := NewExperienceStore("")

	// Add many experiences
	for i := 0; i < 20; i++ {
		store.Record(
			"Implement feature with testing and validation",
			"Long approach description that takes up space in the context window",
			"success",
			[]string{"Step one", "Step two", "Step three"},
			[]string{"Read", "Edit", "Bash", "Write"},
			[]string{"file1.go", "file2.go"},
			1000, time.Minute,
		)
	}

	// Very small token limit should truncate
	ctx := store.BuildExperienceContext("Implement feature with testing", 50)
	// Should contain at least the header
	if !containsSubstring(ctx, "Relevant Past Experiences") {
		t.Error("expected header even with small token limit")
	}
}

func TestGeneralize(t *testing.T) {
	store := NewExperienceStore("")

	exp := &Experience{
		ID:       "exp-123",
		Task:     "Fix bug in user service",
		Approach: "Modified pkg/users/service.go to fix nil pointer",
		Steps:    []string{"Read pkg/users/service.go", "Found nil check missing"},
		Outcome:  "success",
		ToolsUsed: []string{"Read", "Edit"},
		FilesModified: []string{
			"pkg/users/service.go",
			"internal/handler/auth.go",
			"cmd/server/main.go",
		},
		Score:     1.0,
		Tags:      []string{"fix", "bug"},
		CreatedAt: time.Now(),
	}

	generalized := store.Generalize(exp)

	if generalized.ID != "exp-123-generalized" {
		t.Errorf("unexpected ID: %s", generalized.ID)
	}

	// Check files are generalized
	for _, f := range generalized.FilesModified {
		if containsSubstring(f, "users") || containsSubstring(f, "auth") {
			t.Errorf("file path not generalized: %s", f)
		}
	}

	// Check approach is generalized
	if containsSubstring(generalized.Approach, "pkg/users/service.go") {
		t.Error("approach should have file paths generalized")
	}
}

func TestDeduplicate(t *testing.T) {
	store := NewExperienceStore("")

	// Add near-duplicate experiences (very similar text to exceed 0.8 Jaccard)
	store.Record(
		"Implement JWT authentication middleware for API",
		"Created auth middleware with token validation for routes",
		"success",
		nil, nil, nil, 1000, time.Minute,
	)

	store.Record(
		"Implement JWT authentication middleware for API",
		"Created auth middleware with token validation for routes",
		"partial",
		nil, nil, nil, 1200, 2*time.Minute,
	)

	// Add a different experience
	store.Record(
		"Add database migration for orders",
		"Created SQL migration file",
		"success",
		nil, nil, nil, 500, 30*time.Second,
	)

	removed := store.Deduplicate()
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	// Should keep the higher-scoring duplicate (the success one)
	if len(store.Experiences) != 2 {
		t.Errorf("expected 2 experiences after dedup, got %d", len(store.Experiences))
	}

	// The success one should remain
	found := false
	for _, exp := range store.Experiences {
		if exp.Score == 1.0 && containsSubstring(exp.Task, "JWT") {
			found = true
		}
	}
	if !found {
		t.Error("expected the higher-scoring JWT experience to remain")
	}
}

func TestDeduplicateEmpty(t *testing.T) {
	store := NewExperienceStore("")
	removed := store.Deduplicate()
	if removed != 0 {
		t.Errorf("expected 0 removed for empty store, got %d", removed)
	}
}

func TestPrune(t *testing.T) {
	store := NewExperienceStore("")

	// Add experiences with different ages and scores
	store.mu.Lock()
	store.Experiences = append(store.Experiences, &Experience{
		ID:        "old-success",
		Task:      "Old successful task",
		Score:     1.0,
		CreatedAt: time.Now().Add(-60 * 24 * time.Hour), // 60 days old
	})
	store.Experiences = append(store.Experiences, &Experience{
		ID:        "recent-failure",
		Task:      "Recent failed task",
		Score:     0.0,
		CreatedAt: time.Now().Add(-1 * time.Hour), // 1 hour old
	})
	store.Experiences = append(store.Experiences, &Experience{
		ID:        "recent-success",
		Task:      "Recent successful task",
		Score:     1.0,
		CreatedAt: time.Now().Add(-1 * time.Hour), // 1 hour old
	})
	store.mu.Unlock()

	// Prune old (>30 days) and low-score (<0.3)
	pruned := store.Prune(30*24*time.Hour, 0.3)
	if pruned != 2 {
		t.Errorf("expected 2 pruned, got %d", pruned)
	}

	if len(store.Experiences) != 1 {
		t.Errorf("expected 1 remaining, got %d", len(store.Experiences))
	}
	if store.Experiences[0].ID != "recent-success" {
		t.Errorf("expected recent-success to remain, got %s", store.Experiences[0].ID)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	// Create store and record experiences
	store := NewExperienceStore(dir)
	store.Record(
		"Implement feature X",
		"Used approach Y",
		"success",
		[]string{"Step 1", "Step 2"},
		[]string{"Read", "Edit"},
		[]string{"file.go"},
		1000,
		2*time.Minute,
	)
	store.Record(
		"Fix bug Z",
		"Applied fix W",
		"partial",
		[]string{"Debug", "Fix"},
		[]string{"Bash"},
		[]string{"handler.go"},
		500,
		time.Minute,
	)

	// Save
	if err := store.Save(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "experiences.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}

	// Load into new store
	store2 := NewExperienceStore(dir)
	if err := store2.Load(); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if len(store2.Experiences) != 2 {
		t.Fatalf("expected 2 experiences after load, got %d", len(store2.Experiences))
	}

	if store2.Experiences[0].Task != "Implement feature X" {
		t.Errorf("unexpected task after load: %s", store2.Experiences[0].Task)
	}
	if store2.Experiences[0].Score != 1.0 {
		t.Errorf("unexpected score after load: %f", store2.Experiences[0].Score)
	}
	if store2.Experiences[1].Outcome != "partial" {
		t.Errorf("unexpected outcome after load: %s", store2.Experiences[1].Outcome)
	}
}

func TestSaveNoDir(t *testing.T) {
	store := NewExperienceStore("")
	err := store.Save()
	if err == nil {
		t.Error("expected error when saving with empty dir")
	}
}

func TestLoadNoDir(t *testing.T) {
	store := NewExperienceStore("")
	err := store.Load()
	if err == nil {
		t.Error("expected error when loading with empty dir")
	}
}

func TestLoadNoFile(t *testing.T) {
	dir := t.TempDir()
	store := NewExperienceStore(dir)
	// Should not error when file doesn't exist
	if err := store.Load(); err != nil {
		t.Errorf("expected no error for missing file, got: %v", err)
	}
}

func TestStats(t *testing.T) {
	store := NewExperienceStore("")

	store.Record("Task A", "Approach A", "success",
		nil, []string{"Read", "Edit"}, nil, 1000, time.Minute)
	store.Record("Task B", "Approach B", "success",
		nil, []string{"Read", "Bash"}, nil, 2000, 2*time.Minute)
	store.Record("Task C", "Approach C", "failure",
		nil, []string{"Read"}, nil, 500, 30*time.Second)

	stats := store.Stats()

	if stats.TotalExperiences != 3 {
		t.Errorf("expected 3 total, got %d", stats.TotalExperiences)
	}

	if stats.ByOutcome["success"] != 2 {
		t.Errorf("expected 2 successes, got %d", stats.ByOutcome["success"])
	}
	if stats.ByOutcome["failure"] != 1 {
		t.Errorf("expected 1 failure, got %d", stats.ByOutcome["failure"])
	}

	// Average tokens: (1000+2000+500)/3 = 1166
	if stats.AvgTokens != 1166 {
		t.Errorf("expected avg tokens 1166, got %d", stats.AvgTokens)
	}

	if len(stats.TopTools) == 0 {
		t.Error("expected top tools to be populated")
	}
	// Read should be the top tool (used 3 times)
	if stats.TopTools[0] != "Read" {
		t.Errorf("expected Read as top tool, got %s", stats.TopTools[0])
	}
}

func TestStatsEmpty(t *testing.T) {
	store := NewExperienceStore("")
	stats := store.Stats()
	if stats.TotalExperiences != 0 {
		t.Errorf("expected 0 total, got %d", stats.TotalExperiences)
	}
	if stats.AvgScore != 0 {
		t.Errorf("expected 0 avg score, got %f", stats.AvgScore)
	}
}

func TestJaccardSimilarity(t *testing.T) {
	tests := []struct {
		a, b     []string
		expected float64
	}{
		{nil, nil, 0},
		{[]string{"a", "b"}, []string{"a", "b"}, 1.0},
		{[]string{"a", "b"}, []string{"c", "d"}, 0.0},
		{[]string{"a", "b", "c"}, []string{"b", "c", "d"}, 0.5},
	}

	for _, tt := range tests {
		got := jaccardSimilarity(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("jaccard(%v, %v) = %f, want %f", tt.a, tt.b, got, tt.expected)
		}
	}
}

func TestTokenize(t *testing.T) {
	words := tokenize("Implement JWT auth-middleware for HTTP handler")
	// Should get: implement, jwt, auth, middleware, for, http, handler
	// "for" is 3 chars, should be included
	found := map[string]bool{}
	for _, w := range words {
		found[w] = true
	}

	expected := []string{"implement", "jwt", "auth", "middleware", "for", "http", "handler"}
	for _, e := range expected {
		if !found[e] {
			t.Errorf("expected word %q in tokenize output, got %v", e, words)
		}
	}
}

func TestGeneralizePath(t *testing.T) {
	tests := []struct {
		input    string
		contains string
	}{
		{"pkg/auth/middleware.go", "pkg"},
		{"internal/handler/user.go", "internal"},
		{"cmd/server/main.go", "cmd"},
	}

	for _, tt := range tests {
		result := generalizePath(tt.input)
		if !containsSubstring(result, tt.contains) {
			t.Errorf("generalizePath(%q) = %q, expected to contain %q", tt.input, result, tt.contains)
		}
		// Should have wildcard or generic pattern
		if !containsSubstring(result, "*") && !containsSubstring(result, "<") {
			t.Errorf("generalizePath(%q) = %q, expected generic pattern", tt.input, result)
		}
	}
}

func TestExtractTags(t *testing.T) {
	tags := extractTags("Implement JWT authentication middleware with cache layer")
	tagSet := make(map[string]bool)
	for _, tag := range tags {
		tagSet[tag] = true
	}

	if !tagSet["jwt"] {
		t.Error("expected 'jwt' tag")
	}
	if !tagSet["auth"] {
		t.Error("expected 'auth' tag")
	}
	if !tagSet["implement"] {
		t.Error("expected 'implement' tag")
	}
	if !tagSet["middleware"] {
		t.Error("expected 'middleware' tag")
	}
	if !tagSet["cache"] {
		t.Error("expected 'cache' tag")
	}
}

func TestConcurrentAccess(t *testing.T) {
	store := NewExperienceStore("")

	// Concurrent writes and reads
	done := make(chan struct{})

	go func() {
		for i := 0; i < 100; i++ {
			store.Record("concurrent task", "approach", "success",
				nil, nil, nil, 100, time.Millisecond)
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 100; i++ {
			store.FindRelevant("concurrent task", 5)
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 100; i++ {
			store.Stats()
		}
		done <- struct{}{}
	}()

	<-done
	<-done
	<-done
}

func TestUsedCountIncrement(t *testing.T) {
	store := NewExperienceStore("")

	store.Record(
		"Implement authentication",
		"Created auth module",
		"success",
		[]string{"insight"},
		[]string{"Read"},
		nil, 100, time.Second,
	)

	// Building context should increment usage
	store.BuildExperienceContext("Implement authentication", 500)
	store.BuildExperienceContext("Implement authentication", 500)

	store.mu.RLock()
	count := store.Experiences[0].UsedCount
	store.mu.RUnlock()

	if count != 2 {
		t.Errorf("expected UsedCount 2, got %d", count)
	}
}

// containsSubstring checks if s contains substr.
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
