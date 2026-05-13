package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewHandoverManager(t *testing.T) {
	m := NewHandoverManager()
	if m == nil {
		t.Fatal("NewHandoverManager returned nil")
	}
	if len(m.Handovers) != 0 {
		t.Fatalf("expected 0 handovers, got %d", len(m.Handovers))
	}
}

func TestPrepareHandover(t *testing.T) {
	m := NewHandoverManager()

	messages := []Message{
		{Role: "user", Content: "Implement JWT authentication for the API"},
		{Role: "assistant", Content: "I'll implement JWT authentication. I'm deciding to use RS256 algorithm.\nCreated token validation in src/auth/token.go\nAdded middleware skeleton in src/middleware/auth.go"},
		{Role: "user", Content: "Looks good, what's left?"},
		{Role: "assistant", Content: "Still need to write unit tests for ValidateToken.\nWill need to add refresh token endpoint.\nTODO: Update API documentation"},
	}

	files := []string{"src/auth/token.go", "src/middleware/auth.go", "src/handler/api.go"}

	h := m.PrepareHandover("session-123", "claude-sonnet-4-6", messages, files)

	if h == nil {
		t.Fatal("PrepareHandover returned nil")
	}
	if h.SessionID != "session-123" {
		t.Errorf("expected session ID 'session-123', got %q", h.SessionID)
	}
	if h.FromModel != "claude-sonnet-4-6" {
		t.Errorf("expected from model 'claude-sonnet-4-6', got %q", h.FromModel)
	}
	if h.Status != "prepared" {
		t.Errorf("expected status 'prepared', got %q", h.Status)
	}
	if h.Context.Goal != "Implement JWT authentication for the API" {
		t.Errorf("unexpected goal: %q", h.Context.Goal)
	}
	if len(h.Context.FilesModified) != 3 {
		t.Errorf("expected 3 files modified, got %d", len(h.Context.FilesModified))
	}

	// Verify handover was tracked
	if len(m.Handovers) != 1 {
		t.Fatalf("expected 1 tracked handover, got %d", len(m.Handovers))
	}
}

func TestExtractGoal(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		want     string
	}{
		{
			name:     "empty messages",
			messages: nil,
			want:     "No goal identified",
		},
		{
			name: "simple goal",
			messages: []Message{
				{Role: "user", Content: "Fix the login bug"},
			},
			want: "Fix the login bug",
		},
		{
			name: "multiline goal uses first line",
			messages: []Message{
				{Role: "user", Content: "Implement caching\nUse Redis for storage\nAdd TTL support"},
			},
			want: "Implement caching",
		},
		{
			name: "skips system messages",
			messages: []Message{
				{Role: "system", Content: "You are a helpful assistant"},
				{Role: "user", Content: "Refactor the database layer"},
			},
			want: "Refactor the database layer",
		},
		{
			name: "long goal gets truncated",
			messages: []Message{
				{Role: "user", Content: strings.Repeat("a", 300)},
			},
			want: strings.Repeat("a", 200),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractGoal(tt.messages)
			if got != tt.want {
				t.Errorf("ExtractGoal() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractDecisions(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "Implement auth"},
		{Role: "assistant", Content: "I'll use RS256 for token signing.\nDecided to use 15-minute token expiry.\nThe function returns an error."},
		{Role: "assistant", Content: "Going with middleware-based auth checks."},
	}

	decisions := ExtractDecisions(messages)

	if len(decisions) == 0 {
		t.Fatal("expected at least one decision")
	}

	// Check that we found the RS256 decision
	foundRS256 := false
	for _, d := range decisions {
		if strings.Contains(d, "RS256") {
			foundRS256 = true
			break
		}
	}
	if !foundRS256 {
		t.Errorf("expected to find RS256 decision in: %v", decisions)
	}

	// Check middleware decision
	foundMiddleware := false
	for _, d := range decisions {
		if strings.Contains(d, "middleware") {
			foundMiddleware = true
			break
		}
	}
	if !foundMiddleware {
		t.Errorf("expected to find middleware decision in: %v", decisions)
	}
}

func TestExtractDecisions_NoDuplicates(t *testing.T) {
	messages := []Message{
		{Role: "assistant", Content: "I'll use RS256.\nI'll use RS256."},
	}

	decisions := ExtractDecisions(messages)
	seen := make(map[string]bool)
	for _, d := range decisions {
		if seen[d] {
			t.Errorf("duplicate decision found: %q", d)
		}
		seen[d] = true
	}
}

func TestExtractPendingTasks(t *testing.T) {
	messages := []Message{
		{Role: "assistant", Content: "Still need to write tests.\nTODO: Add error handling.\nWill need to update docs."},
	}

	tasks := ExtractPendingTasks(messages)

	if len(tasks) < 3 {
		t.Fatalf("expected at least 3 tasks, got %d: %v", len(tasks), tasks)
	}

	foundTests := false
	for _, task := range tasks {
		if strings.Contains(strings.ToLower(task), "test") {
			foundTests = true
			break
		}
	}
	if !foundTests {
		t.Errorf("expected to find test-related task in: %v", tasks)
	}
}

func TestExtractPendingTasks_Empty(t *testing.T) {
	messages := []Message{
		{Role: "assistant", Content: "Everything is done. All looks good."},
	}

	tasks := ExtractPendingTasks(messages)
	if len(tasks) != 0 {
		t.Errorf("expected no pending tasks from completed work, got: %v", tasks)
	}
}

func TestGenerateHandoverPrompt(t *testing.T) {
	h := &Handover{
		SessionID: "session-456",
		FromModel: "claude-sonnet-4-6",
		ToModel:   "claude-opus-4-6",
		Status:    "prepared",
		Context: HandoverContext{
			Goal:     "Implement JWT authentication for the API",
			Progress: "- Created token validation (src/auth/token.go)\n- Added middleware skeleton (src/middleware/auth.go)",
			KeyDecisions: []string{
				"Using RS256 algorithm (not HS256)",
				"Token expiry: 15 minutes",
			},
			PendingTasks: []string{
				"Write unit tests for ValidateToken",
				"Add refresh token endpoint",
				"Update API documentation",
			},
			Warnings: []string{
				"Don't modify go.mod (dependency freeze in effect)",
			},
			FilesModified: []string{
				"src/auth/token.go",
				"src/middleware/auth.go",
				"src/handler/api.go",
			},
		},
	}

	prompt := GenerateHandoverPrompt(h)

	// Verify essential sections exist
	if !strings.Contains(prompt, "## Session Handover") {
		t.Error("missing session handover header")
	}
	if !strings.Contains(prompt, "claude-sonnet-4-6") {
		t.Error("missing from model reference")
	}
	if !strings.Contains(prompt, "### Goal") {
		t.Error("missing goal section")
	}
	if !strings.Contains(prompt, "Implement JWT authentication") {
		t.Error("missing goal content")
	}
	if !strings.Contains(prompt, "### Progress") {
		t.Error("missing progress section")
	}
	if !strings.Contains(prompt, "### Key Decisions") {
		t.Error("missing key decisions section")
	}
	if !strings.Contains(prompt, "RS256") {
		t.Error("missing RS256 decision")
	}
	if !strings.Contains(prompt, "### Pending Tasks") {
		t.Error("missing pending tasks section")
	}
	if !strings.Contains(prompt, "Write unit tests") {
		t.Error("missing pending task content")
	}
	if !strings.Contains(prompt, "### Warnings") {
		t.Error("missing warnings section")
	}
	if !strings.Contains(prompt, "go.mod") {
		t.Error("missing warning content")
	}
	if !strings.Contains(prompt, "### Files Modified") {
		t.Error("missing files modified section")
	}
	if !strings.Contains(prompt, "src/auth/token.go") {
		t.Error("missing file in files modified")
	}
}

func TestGenerateHandoverPrompt_EmptyOptionalSections(t *testing.T) {
	h := &Handover{
		SessionID: "session-789",
		FromModel: "claude-haiku-3",
		Context: HandoverContext{
			Goal: "Simple fix",
		},
	}

	prompt := GenerateHandoverPrompt(h)

	// Should not contain sections with no data
	if strings.Contains(prompt, "### Key Decisions") {
		t.Error("should not include empty key decisions section")
	}
	if strings.Contains(prompt, "### Pending Tasks") {
		t.Error("should not include empty pending tasks section")
	}
	if strings.Contains(prompt, "### Warnings") {
		t.Error("should not include empty warnings section")
	}
	if strings.Contains(prompt, "### Files Modified") {
		t.Error("should not include empty files modified section")
	}
}

func TestAcceptHandover(t *testing.T) {
	m := NewHandoverManager()

	h := &Handover{
		SessionID: "session-accept",
		FromModel: "model-a",
		Status:    "prepared",
	}
	m.Handovers = append(m.Handovers, h)

	err := m.AcceptHandover(h, "model-b")
	if err != nil {
		t.Fatalf("AcceptHandover failed: %v", err)
	}
	if h.Status != "accepted" {
		t.Errorf("expected status 'accepted', got %q", h.Status)
	}
	if h.ToModel != "model-b" {
		t.Errorf("expected to model 'model-b', got %q", h.ToModel)
	}
}

func TestAcceptHandover_Nil(t *testing.T) {
	m := NewHandoverManager()
	err := m.AcceptHandover(nil, "model-b")
	if err == nil {
		t.Error("expected error for nil handover")
	}
}

func TestAcceptHandover_AlreadyAccepted(t *testing.T) {
	m := NewHandoverManager()
	h := &Handover{Status: "accepted"}

	err := m.AcceptHandover(h, "model-c")
	if err == nil {
		t.Error("expected error for already accepted handover")
	}
}

func TestAcceptHandover_Rejected(t *testing.T) {
	m := NewHandoverManager()
	h := &Handover{Status: "rejected"}

	err := m.AcceptHandover(h, "model-c")
	if err == nil {
		t.Error("expected error for rejected handover")
	}
}

func TestFormatHandover(t *testing.T) {
	h := &Handover{
		SessionID: "fmt-session",
		FromModel: "claude-sonnet-4-6",
		ToModel:   "claude-opus-4-6",
		Status:    "accepted",
		Context: HandoverContext{
			Goal:          "Build a REST API",
			FilesModified: []string{"main.go", "handler.go"},
			PendingTasks:  []string{"Add tests", "Deploy"},
		},
	}

	output := FormatHandover(h)

	if !strings.Contains(output, "fmt-session") {
		t.Error("missing session ID")
	}
	if !strings.Contains(output, "claude-sonnet-4-6") {
		t.Error("missing from model")
	}
	if !strings.Contains(output, "claude-opus-4-6") {
		t.Error("missing to model")
	}
	if !strings.Contains(output, "accepted") {
		t.Error("missing status")
	}
	if !strings.Contains(output, "Build a REST API") {
		t.Error("missing goal")
	}
	if !strings.Contains(output, "main.go") {
		t.Error("missing file")
	}
	if !strings.Contains(output, "Add tests") {
		t.Error("missing pending task")
	}
}

func TestFormatHandover_NoToModel(t *testing.T) {
	h := &Handover{
		SessionID: "no-to",
		FromModel: "model-a",
		Status:    "prepared",
		Context: HandoverContext{
			Goal: "Some task",
		},
	}

	output := FormatHandover(h)
	if strings.Contains(output, "To:") {
		t.Error("should not display To: line when ToModel is empty")
	}
}

func TestSaveAndLoadHandover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "handover.json")

	original := &Handover{
		SessionID: "save-load-test",
		FromModel: "claude-sonnet-4-6",
		ToModel:   "claude-opus-4-6",
		Status:    "accepted",
		Context: HandoverContext{
			Goal:                  "Implement feature X",
			Progress:              "- Done step 1\n- Done step 2",
			FilesModified:         []string{"a.go", "b.go"},
			PendingTasks:          []string{"Write tests"},
			Warnings:              []string{"Don't touch config"},
			KeyDecisions:          []string{"Using approach A"},
			CurrentState:          "in_progress",
			TokensBudgetRemaining: 50000,
		},
	}

	// Save
	if err := SaveHandover(original, path); err != nil {
		t.Fatalf("SaveHandover failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("handover file not created: %v", err)
	}

	// Load
	loaded, err := LoadHandover(path)
	if err != nil {
		t.Fatalf("LoadHandover failed: %v", err)
	}

	// Verify fields
	if loaded.SessionID != original.SessionID {
		t.Errorf("SessionID mismatch: got %q, want %q", loaded.SessionID, original.SessionID)
	}
	if loaded.FromModel != original.FromModel {
		t.Errorf("FromModel mismatch: got %q, want %q", loaded.FromModel, original.FromModel)
	}
	if loaded.ToModel != original.ToModel {
		t.Errorf("ToModel mismatch: got %q, want %q", loaded.ToModel, original.ToModel)
	}
	if loaded.Status != original.Status {
		t.Errorf("Status mismatch: got %q, want %q", loaded.Status, original.Status)
	}
	if loaded.Context.Goal != original.Context.Goal {
		t.Errorf("Goal mismatch: got %q, want %q", loaded.Context.Goal, original.Context.Goal)
	}
	if loaded.Context.Progress != original.Context.Progress {
		t.Errorf("Progress mismatch: got %q, want %q", loaded.Context.Progress, original.Context.Progress)
	}
	if len(loaded.Context.FilesModified) != 2 {
		t.Errorf("FilesModified count mismatch: got %d, want 2", len(loaded.Context.FilesModified))
	}
	if len(loaded.Context.PendingTasks) != 1 {
		t.Errorf("PendingTasks count mismatch: got %d, want 1", len(loaded.Context.PendingTasks))
	}
	if len(loaded.Context.Warnings) != 1 {
		t.Errorf("Warnings count mismatch: got %d, want 1", len(loaded.Context.Warnings))
	}
	if len(loaded.Context.KeyDecisions) != 1 {
		t.Errorf("KeyDecisions count mismatch: got %d, want 1", len(loaded.Context.KeyDecisions))
	}
	if loaded.Context.CurrentState != "in_progress" {
		t.Errorf("CurrentState mismatch: got %q", loaded.Context.CurrentState)
	}
	if loaded.Context.TokensBudgetRemaining != 50000 {
		t.Errorf("TokensBudgetRemaining mismatch: got %d", loaded.Context.TokensBudgetRemaining)
	}
}

func TestLoadHandover_NotFound(t *testing.T) {
	_, err := LoadHandover("/nonexistent/path/handover.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadHandover_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte("not valid json{{{"), 0o644)

	_, err := LoadHandover(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestPrepareHandover_ExtractsProgress(t *testing.T) {
	m := NewHandoverManager()
	messages := []Message{
		{Role: "user", Content: "Build a CLI tool"},
		{Role: "assistant", Content: "Created the main.go entry point.\nAdded cobra command structure."},
	}

	h := m.PrepareHandover("prog-test", "model-x", messages, nil)

	if h.Context.Progress == "" {
		t.Error("expected non-empty progress")
	}
	if !strings.Contains(h.Context.Progress, "main.go") {
		t.Errorf("expected progress to mention main.go, got: %q", h.Context.Progress)
	}
}

func TestHandoverManagerConcurrency(t *testing.T) {
	m := NewHandoverManager()
	messages := []Message{
		{Role: "user", Content: "Do work"},
		{Role: "assistant", Content: "Created something. TODO: finish it."},
	}

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(idx int) {
			defer func() { done <- struct{}{} }()
			h := m.PrepareHandover("concurrent-session", "model-a", messages, []string{"file.go"})
			_ = m.AcceptHandover(h, "model-b")
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if len(m.Handovers) != 10 {
		t.Errorf("expected 10 handovers, got %d", len(m.Handovers))
	}
}

func TestFullHandoverWorkflow(t *testing.T) {
	// Simulate a complete handover workflow
	m := NewHandoverManager()

	messages := []Message{
		{Role: "user", Content: "Implement JWT authentication for the API"},
		{Role: "assistant", Content: "I'll implement JWT auth. I'm going with RS256 for token signing.\nCreated src/auth/token.go with token validation logic.\nAdded middleware skeleton in src/middleware/auth.go."},
		{Role: "user", Content: "What's still pending?"},
		{Role: "assistant", Content: "Still need to write unit tests for ValidateToken.\nTODO: Add refresh token endpoint.\nWill need to update API documentation."},
	}

	files := []string{"src/auth/token.go", "src/middleware/auth.go"}

	// Step 1: Prepare
	h := m.PrepareHandover("workflow-test", "claude-sonnet-4-6", messages, files)
	if h.Status != "prepared" {
		t.Fatalf("expected prepared status, got %q", h.Status)
	}

	// Step 2: Generate prompt
	prompt := GenerateHandoverPrompt(h)
	if !strings.Contains(prompt, "JWT") {
		t.Error("prompt should reference JWT")
	}
	if !strings.Contains(prompt, "claude-sonnet-4-6") {
		t.Error("prompt should reference the originating model")
	}

	// Step 3: Save to disk
	dir := t.TempDir()
	path := filepath.Join(dir, "handover.json")
	if err := SaveHandover(h, path); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Step 4: Load from disk (simulating another machine/model)
	loaded, err := LoadHandover(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	// Step 5: Accept
	if err := m.AcceptHandover(loaded, "claude-opus-4-6"); err != nil {
		t.Fatalf("accept failed: %v", err)
	}
	if loaded.Status != "accepted" {
		t.Errorf("expected accepted, got %q", loaded.Status)
	}
	if loaded.ToModel != "claude-opus-4-6" {
		t.Errorf("expected to model claude-opus-4-6, got %q", loaded.ToModel)
	}

	// Step 6: Format for display
	output := FormatHandover(loaded)
	if !strings.Contains(output, "accepted") {
		t.Error("format output should show accepted status")
	}
}
