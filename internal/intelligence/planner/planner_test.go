package planner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerate_ProducesPrompt(t *testing.T) {
	prompt := Generate("Add user authentication", "Go web app with REST API")

	if prompt == nil {
		t.Fatal("expected non-nil PlanPrompt")
	}
	if prompt.System == "" {
		t.Error("expected non-empty system prompt")
	}
	if !strings.Contains(prompt.User, "Add user authentication") {
		t.Error("expected feature description in user prompt")
	}
	if !strings.Contains(prompt.User, "Go web app with REST API") {
		t.Error("expected repo context in user prompt")
	}
}

func TestParsePlan_ValidJSON(t *testing.T) {
	response := `{
		"title": "Add Auth",
		"summary": "Implement JWT-based authentication",
		"tasks": [
			{"id": 1, "description": "Create auth middleware", "file": "middleware/auth.go", "status": "pending"},
			{"id": 2, "description": "Add login endpoint", "file": "handlers/login.go", "status": "pending", "depends": [1]},
			{"id": 3, "description": "Write tests", "status": "pending", "depends": [1, 2]}
		],
		"design": "Use JWT tokens with refresh mechanism",
		"risk_notes": "Token expiration handling needs care"
	}`

	plan, err := ParsePlan(response)
	if err != nil {
		t.Fatalf("ParsePlan failed: %v", err)
	}

	if plan.Title != "Add Auth" {
		t.Errorf("expected title 'Add Auth', got %q", plan.Title)
	}
	if plan.Summary != "Implement JWT-based authentication" {
		t.Errorf("unexpected summary: %q", plan.Summary)
	}
	if len(plan.Tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(plan.Tasks))
	}
	if plan.Tasks[0].File != "middleware/auth.go" {
		t.Errorf("expected task 1 file 'middleware/auth.go', got %q", plan.Tasks[0].File)
	}
	if len(plan.Tasks[1].Depends) != 1 || plan.Tasks[1].Depends[0] != 1 {
		t.Errorf("expected task 2 to depend on [1], got %v", plan.Tasks[1].Depends)
	}
	if len(plan.Tasks[2].Depends) != 2 {
		t.Errorf("expected task 3 to depend on [1, 2], got %v", plan.Tasks[2].Depends)
	}
	if plan.Design != "Use JWT tokens with refresh mechanism" {
		t.Errorf("unexpected design: %q", plan.Design)
	}
	if plan.RiskNotes != "Token expiration handling needs care" {
		t.Errorf("unexpected risk_notes: %q", plan.RiskNotes)
	}
	if plan.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestParsePlan_WithMarkdownFences(t *testing.T) {
	response := "```json\n{\"title\": \"Test Plan\", \"summary\": \"A test\", \"tasks\": [], \"design\": \"\", \"risk_notes\": \"\"}\n```"

	plan, err := ParsePlan(response)
	if err != nil {
		t.Fatalf("ParsePlan with fences failed: %v", err)
	}
	if plan.Title != "Test Plan" {
		t.Errorf("expected title 'Test Plan', got %q", plan.Title)
	}
}

func TestParsePlan_SetsDefaultStatus(t *testing.T) {
	response := `{
		"title": "Defaults",
		"summary": "Test defaults",
		"tasks": [
			{"id": 1, "description": "Task without status"}
		],
		"design": "",
		"risk_notes": ""
	}`

	plan, err := ParsePlan(response)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Tasks[0].Status != "pending" {
		t.Errorf("expected default status 'pending', got %q", plan.Tasks[0].Status)
	}
}

func TestParsePlan_SetsDefaultIDs(t *testing.T) {
	response := `{
		"title": "IDs",
		"summary": "Test IDs",
		"tasks": [
			{"description": "First task"},
			{"description": "Second task"}
		],
		"design": "",
		"risk_notes": ""
	}`

	plan, err := ParsePlan(response)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Tasks[0].ID != 1 {
		t.Errorf("expected task 1 ID=1, got %d", plan.Tasks[0].ID)
	}
	if plan.Tasks[1].ID != 2 {
		t.Errorf("expected task 2 ID=2, got %d", plan.Tasks[1].ID)
	}
}

func TestParsePlan_InvalidJSON(t *testing.T) {
	_, err := ParsePlan("this is not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	plan := &Plan{
		Title:   "Test Save Load",
		Summary: "Verify round-trip",
		Tasks: []Task{
			{ID: 1, Description: "Step one", File: "main.go", Status: "pending"},
			{ID: 2, Description: "Step two", Status: "done", Depends: []int{1}},
		},
		Design:    "Simple approach",
		RiskNotes: "None identified",
		CreatedAt: time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC),
	}

	path, err := Save(dir, plan)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if !strings.HasSuffix(path, ".json") {
		t.Errorf("expected .json extension, got %q", path)
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("saved file does not exist: %s", path)
	}

	// Load it back
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Title != plan.Title {
		t.Errorf("title mismatch: %q vs %q", loaded.Title, plan.Title)
	}
	if loaded.Summary != plan.Summary {
		t.Errorf("summary mismatch: %q vs %q", loaded.Summary, plan.Summary)
	}
	if len(loaded.Tasks) != len(plan.Tasks) {
		t.Fatalf("task count mismatch: %d vs %d", len(loaded.Tasks), len(plan.Tasks))
	}
	if loaded.Tasks[1].Status != "done" {
		t.Errorf("expected task 2 status 'done', got %q", loaded.Tasks[1].Status)
	}
}

func TestSave_CreatesDirectories(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deep", "nested")

	plan := &Plan{
		Title:     "Nested",
		Summary:   "Test dir creation",
		Tasks:     []Task{},
		CreatedAt: time.Now(),
	}

	path, err := Save(dir, plan)
	if err != nil {
		t.Fatalf("Save should create nested directories: %v", err)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected file to exist")
	}
}

func TestSave_SanitizesFilename(t *testing.T) {
	dir := t.TempDir()

	plan := &Plan{
		Title:     "Add Auth: User/Session Management?",
		Summary:   "Test filename sanitization",
		Tasks:     []Task{},
		CreatedAt: time.Now(),
	}

	path, err := Save(dir, plan)
	if err != nil {
		t.Fatal(err)
	}

	filename := filepath.Base(path)
	if strings.ContainsAny(filename, "/\\:*?\"<>|") {
		t.Errorf("filename contains unsafe characters: %q", filename)
	}
}

func TestLoad_NonexistentFile(t *testing.T) {
	_, err := Load("/nonexistent/path/plan.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestMarkDone(t *testing.T) {
	plan := &Plan{
		Tasks: []Task{
			{ID: 1, Description: "Task 1", Status: "pending"},
			{ID: 2, Description: "Task 2", Status: "pending"},
			{ID: 3, Description: "Task 3", Status: "pending"},
		},
	}

	MarkDone(plan, 2)

	if plan.Tasks[0].Status != "pending" {
		t.Error("task 1 should still be pending")
	}
	if plan.Tasks[1].Status != "done" {
		t.Errorf("task 2 should be done, got %q", plan.Tasks[1].Status)
	}
	if plan.Tasks[2].Status != "pending" {
		t.Error("task 3 should still be pending")
	}
}

func TestMarkDone_NonexistentTask(t *testing.T) {
	plan := &Plan{
		Tasks: []Task{
			{ID: 1, Description: "Task 1", Status: "pending"},
		},
	}

	// Should be a no-op, not panic
	MarkDone(plan, 99)

	if plan.Tasks[0].Status != "pending" {
		t.Error("task 1 should still be pending")
	}
}

func TestMarkSkipped(t *testing.T) {
	plan := &Plan{
		Tasks: []Task{
			{ID: 1, Description: "Task 1", Status: "pending"},
		},
	}

	MarkSkipped(plan, 1)

	if plan.Tasks[0].Status != "skipped" {
		t.Errorf("expected status 'skipped', got %q", plan.Tasks[0].Status)
	}
}

func TestPendingTasks(t *testing.T) {
	plan := &Plan{
		Tasks: []Task{
			{ID: 1, Description: "Done task", Status: "done"},
			{ID: 2, Description: "Pending task", Status: "pending"},
			{ID: 3, Description: "Skipped task", Status: "skipped"},
			{ID: 4, Description: "Another pending", Status: "pending"},
		},
	}

	pending := PendingTasks(plan)
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending tasks, got %d", len(pending))
	}
	if pending[0].ID != 2 || pending[1].ID != 4 {
		t.Errorf("unexpected pending tasks: %v", pending)
	}
}

func TestFormatMarkdown(t *testing.T) {
	plan := &Plan{
		Title:   "Auth Feature",
		Summary: "Add JWT authentication to the API.",
		Tasks: []Task{
			{ID: 1, Description: "Create middleware", File: "middleware/auth.go", Status: "done"},
			{ID: 2, Description: "Add login endpoint", File: "handlers/login.go", Status: "pending", Depends: []int{1}},
			{ID: 3, Description: "Write tests", Status: "skipped"},
		},
		Design:    "Use standard JWT library.",
		RiskNotes: "Token invalidation is complex.",
		CreatedAt: time.Date(2026, 5, 2, 10, 30, 0, 0, time.UTC),
	}

	md := FormatMarkdown(plan)

	// Check title
	if !strings.Contains(md, "# Auth Feature") {
		t.Error("expected title in markdown")
	}

	// Check summary
	if !strings.Contains(md, "Add JWT authentication to the API.") {
		t.Error("expected summary in markdown")
	}

	// Check design section
	if !strings.Contains(md, "## Design") {
		t.Error("expected design section")
	}
	if !strings.Contains(md, "Use standard JWT library.") {
		t.Error("expected design content")
	}

	// Check tasks section
	if !strings.Contains(md, "## Tasks") {
		t.Error("expected tasks section")
	}

	// Check done task has [x]
	if !strings.Contains(md, "[x] **1.**") {
		t.Error("expected done checkbox for task 1")
	}

	// Check pending task has [ ]
	if !strings.Contains(md, "[ ] **2.**") {
		t.Error("expected pending checkbox for task 2")
	}

	// Check skipped task has [-]
	if !strings.Contains(md, "[-] **3.**") {
		t.Error("expected skipped checkbox for task 3")
	}

	// Check file annotation
	if !strings.Contains(md, "(`middleware/auth.go`)") {
		t.Error("expected file annotation for task 1")
	}

	// Check dependency annotation
	if !strings.Contains(md, "[depends: #1]") {
		t.Error("expected dependency annotation for task 2")
	}

	// Check risks section
	if !strings.Contains(md, "## Risks") {
		t.Error("expected risks section")
	}

	// Check created timestamp
	if !strings.Contains(md, "2026-05-02 10:30") {
		t.Error("expected created timestamp")
	}
}

func TestFormatMarkdown_EmptyPlan(t *testing.T) {
	plan := &Plan{
		Title:     "Empty",
		CreatedAt: time.Now(),
	}

	md := FormatMarkdown(plan)
	if !strings.Contains(md, "# Empty") {
		t.Error("expected title even for empty plan")
	}
	// Should not contain tasks section when there are none
	if strings.Contains(md, "## Tasks") {
		t.Error("should not have tasks section when empty")
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Simple Title", "simple-title"},
		{"With/Slashes", "with-slashes"},
		{"With: Colons", "with-colons"},
		{"Multiple   Spaces", "multiple-spaces"},
		{"Special*?\"<>|Chars", "specialchars"},
		{"", "untitled-plan"},
		{"  spaces  ", "spaces"},
	}

	for _, tc := range tests {
		result := sanitizeFilename(tc.input)
		if result != tc.expected {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestPlanJSON_RoundTrip(t *testing.T) {
	plan := &Plan{
		Title:   "Round Trip",
		Summary: "Test JSON serialization",
		Tasks: []Task{
			{ID: 1, Description: "First", Status: "pending"},
		},
		Design:    "Simple",
		RiskNotes: "None",
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}

	var decoded Plan
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.Title != plan.Title {
		t.Errorf("title mismatch after round-trip")
	}
	if len(decoded.Tasks) != 1 {
		t.Errorf("expected 1 task after round-trip, got %d", len(decoded.Tasks))
	}
}
