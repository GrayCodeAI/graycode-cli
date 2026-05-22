package observability

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewDebugRecorder(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	if dr == nil {
		t.Fatal("NewDebugRecorder returned nil")
	}
	if dr.Dir != "/tmp/test-debug" {
		t.Errorf("Dir = %q, want %q", dr.Dir, "/tmp/test-debug")
	}
	if dr.Sessions == nil {
		t.Fatal("Sessions should be initialized")
	}
	if len(dr.Sessions) != 0 {
		t.Errorf("Sessions length = %d, want 0", len(dr.Sessions))
	}
	if dr.ActiveSession != nil {
		t.Error("ActiveSession should be nil initially")
	}
}

func TestStartSession(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	session := dr.StartSession("API returns 500 on auth endpoint")

	if session == nil {
		t.Fatal("StartSession returned nil")
	}
	if session.Symptom != "API returns 500 on auth endpoint" {
		t.Errorf("Symptom = %q, want %q", session.Symptom, "API returns 500 on auth endpoint")
	}
	if !strings.HasPrefix(session.ID, "dbg_") {
		t.Errorf("ID = %q, should start with 'dbg_'", session.ID)
	}
	if session.StartTime.IsZero() {
		t.Error("StartTime should not be zero")
	}
	if session.EndTime != nil {
		t.Error("EndTime should be nil for active session")
	}
	if dr.ActiveSession != session {
		t.Error("ActiveSession should point to the new session")
	}
	if len(dr.Sessions) != 1 {
		t.Errorf("Sessions length = %d, want 1", len(dr.Sessions))
	}
}

func TestRecordStep(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	dr.StartSession("nil pointer dereference")

	dr.RecordStep("read", "src/auth/handler.go", "Found nil pointer on line 42", "handler not checking error")
	dr.RecordStep("grep", "token.Claims", "Used in 3 files", "widespread usage")

	if len(dr.ActiveSession.Steps) != 2 {
		t.Fatalf("Steps length = %d, want 2", len(dr.ActiveSession.Steps))
	}

	step1 := dr.ActiveSession.Steps[0]
	if step1.Index != 1 {
		t.Errorf("Step1.Index = %d, want 1", step1.Index)
	}
	if step1.Action != "read" {
		t.Errorf("Step1.Action = %q, want %q", step1.Action, "read")
	}
	if step1.Target != "src/auth/handler.go" {
		t.Errorf("Step1.Target = %q, want %q", step1.Target, "src/auth/handler.go")
	}
	if step1.Result != "Found nil pointer on line 42" {
		t.Errorf("Step1.Result = %q", step1.Result)
	}
	if step1.InsightGained != "handler not checking error" {
		t.Errorf("Step1.InsightGained = %q", step1.InsightGained)
	}
	if step1.Timestamp.IsZero() {
		t.Error("Step1.Timestamp should not be zero")
	}

	// Check files investigated
	if len(dr.ActiveSession.FilesInvestigated) != 2 {
		t.Errorf("FilesInvestigated length = %d, want 2", len(dr.ActiveSession.FilesInvestigated))
	}
}

func TestRecordStepNoActiveSession(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	// Should not panic when no active session
	dr.RecordStep("read", "foo.go", "result", "insight")
}

func TestRecordStepDeduplicatesFiles(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	dr.StartSession("test")

	dr.RecordStep("read", "foo.go", "result1", "")
	dr.RecordStep("read", "foo.go", "result2", "")

	if len(dr.ActiveSession.FilesInvestigated) != 1 {
		t.Errorf("FilesInvestigated length = %d, want 1 (deduplication)", len(dr.ActiveSession.FilesInvestigated))
	}
}

func TestAddHypothesis(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	dr.StartSession("test bug")

	dr.AddHypothesis("Claims struct missing required field")
	dr.AddHypothesis("Token parsing fails on expired tokens")

	if len(dr.ActiveSession.HypothesesTested) != 2 {
		t.Fatalf("HypothesesTested length = %d, want 2", len(dr.ActiveSession.HypothesesTested))
	}

	h := dr.ActiveSession.HypothesesTested[0]
	if h.Description != "Claims struct missing required field" {
		t.Errorf("Hypothesis description = %q", h.Description)
	}
	if h.Tested {
		t.Error("Hypothesis should not be tested initially")
	}
	if h.Confirmed {
		t.Error("Hypothesis should not be confirmed initially")
	}

	// Should also add a step
	if len(dr.ActiveSession.Steps) != 2 {
		t.Errorf("Steps length = %d, want 2 (one per hypothesis)", len(dr.ActiveSession.Steps))
	}
}

func TestAddHypothesisNoActiveSession(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	// Should not panic
	dr.AddHypothesis("some hypothesis")
}

func TestConfirmHypothesis(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	dr.StartSession("test bug")
	dr.AddHypothesis("Token parsing fails on expired tokens")

	dr.ConfirmHypothesis(0, "error swallowed at line 38")

	h := dr.ActiveSession.HypothesesTested[0]
	if !h.Tested {
		t.Error("Hypothesis should be tested")
	}
	if !h.Confirmed {
		t.Error("Hypothesis should be confirmed")
	}
	if h.Evidence != "error swallowed at line 38" {
		t.Errorf("Evidence = %q", h.Evidence)
	}
}

func TestConfirmHypothesisOutOfBounds(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	dr.StartSession("test")
	dr.AddHypothesis("h1")

	// Should not panic
	dr.ConfirmHypothesis(-1, "evidence")
	dr.ConfirmHypothesis(5, "evidence")
}

func TestRejectHypothesis(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	dr.StartSession("test bug")
	dr.AddHypothesis("Claims struct missing required field")

	dr.RejectHypothesis(0, "all fields present in struct")

	h := dr.ActiveSession.HypothesesTested[0]
	if !h.Tested {
		t.Error("Hypothesis should be tested")
	}
	if h.Confirmed {
		t.Error("Hypothesis should not be confirmed")
	}
	if h.Evidence != "all fields present in struct" {
		t.Errorf("Evidence = %q", h.Evidence)
	}
}

func TestRejectHypothesisNoActiveSession(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	// Should not panic
	dr.RejectHypothesis(0, "evidence")
}

func TestSetRootCause(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	dr.StartSession("test bug")

	dr.SetRootCause("Expired token error was swallowed")

	if dr.ActiveSession.RootCause != "Expired token error was swallowed" {
		t.Errorf("RootCause = %q", dr.ActiveSession.RootCause)
	}
}

func TestSetRootCauseNoActiveSession(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	// Should not panic
	dr.SetRootCause("some cause")
}

func TestSetResolution(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	dr.StartSession("test bug")

	dr.SetResolution("Added explicit error check before accessing Claims fields")

	if dr.ActiveSession.Resolution != "Added explicit error check before accessing Claims fields" {
		t.Errorf("Resolution = %q", dr.ActiveSession.Resolution)
	}
}

func TestSetResolutionNoActiveSession(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	// Should not panic
	dr.SetResolution("some resolution")
}

func TestEndSession(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	dr.StartSession("test bug")

	dr.EndSession(true)

	if dr.ActiveSession != nil {
		t.Error("ActiveSession should be nil after ending")
	}

	session := dr.Sessions[0]
	if session.EndTime == nil {
		t.Fatal("EndTime should be set")
	}
	if !session.Successful {
		t.Error("Session should be marked successful")
	}
}

func TestEndSessionUnsuccessful(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	dr.StartSession("test bug")

	dr.EndSession(false)

	session := dr.Sessions[0]
	if session.Successful {
		t.Error("Session should not be marked successful")
	}
}

func TestEndSessionNoActiveSession(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	// Should not panic
	dr.EndSession(true)
}

func TestFormatSession(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	session := dr.StartSession("API returns 500 on /api/auth/login")

	dr.RecordStep("read", "src/auth/handler.go", "Found nil pointer on line 42", "")
	dr.RecordStep("grep", "token.Claims", "Used in 3 files", "")
	dr.AddHypothesis("Claims struct missing required field")
	dr.RejectHypothesis(0, "")
	dr.AddHypothesis("Token parsing fails on expired tokens")
	dr.ConfirmHypothesis(1, "error swallowed at line 38")
	dr.RecordStep("fix_attempt", "Added error check", "Tests pass", "")
	dr.SetRootCause("Expired token error was swallowed, nil Claims passed to handler")
	dr.SetResolution("Added explicit error check before accessing Claims fields")

	// Set a fixed end time for predictable output
	endTime := session.StartTime.Add(8*time.Minute + 23*time.Second)
	session.EndTime = &endTime
	session.Successful = true

	output := dr.FormatSession(session)

	// Verify key parts of the output
	if !strings.Contains(output, `Debug Session: "API returns 500 on /api/auth/login"`) {
		t.Error("Missing session title")
	}
	if !strings.Contains(output, "8m 23s") {
		t.Errorf("Missing duration, got:\n%s", output)
	}
	if !strings.Contains(output, "RESOLVED") {
		t.Error("Missing RESOLVED status")
	}
	if !strings.Contains(output, "Symptom: API returns 500 on /api/auth/login") {
		t.Error("Missing symptom")
	}
	if !strings.Contains(output, "[read] src/auth/handler.go") {
		t.Error("Missing read step")
	}
	if !strings.Contains(output, "[grep] token.Claims") {
		t.Error("Missing grep step")
	}
	if !strings.Contains(output, "REJECTED") {
		t.Error("Missing REJECTED hypothesis")
	}
	if !strings.Contains(output, "CONFIRMED") {
		t.Error("Missing CONFIRMED hypothesis")
	}
	if !strings.Contains(output, "Evidence: error swallowed at line 38") {
		t.Error("Missing evidence")
	}
	if !strings.Contains(output, "Root Cause: Expired token error was swallowed") {
		t.Error("Missing root cause")
	}
	if !strings.Contains(output, "Resolution: Added explicit error check") {
		t.Error("Missing resolution")
	}
	if !strings.Contains(output, "src/auth/handler.go") {
		t.Error("Missing file in files list")
	}
}

func TestFormatSessionNil(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	output := dr.FormatSession(nil)
	if output != "" {
		t.Errorf("FormatSession(nil) = %q, want empty", output)
	}
}

func TestFormatSessionUnresolved(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	session := dr.StartSession("test issue")
	dr.EndSession(false)

	output := dr.FormatSession(session)
	if !strings.Contains(output, "UNRESOLVED") {
		t.Error("Missing UNRESOLVED status")
	}
}

func TestSearchSessions(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")

	// Create several sessions
	dr.StartSession("API returns 500 on auth endpoint")
	dr.SetRootCause("nil pointer in token handler")
	dr.EndSession(true)

	dr.StartSession("Database connection timeout")
	dr.SetRootCause("connection pool exhausted")
	dr.EndSession(true)

	dr.StartSession("Auth token validation fails")
	dr.SetRootCause("expired token not handled")
	dr.EndSession(true)

	dr.StartSession("CSS layout broken on mobile")
	dr.EndSession(false)

	// Search for auth-related sessions
	results := dr.SearchSessions("auth endpoint returns error")
	if len(results) < 2 {
		t.Errorf("Expected at least 2 results for auth search, got %d", len(results))
	}

	// Search for token-related sessions
	results = dr.SearchSessions("token parsing error")
	if len(results) < 1 {
		t.Errorf("Expected at least 1 result for token search, got %d", len(results))
	}

	// Search for something unrelated
	results = dr.SearchSessions("kubernetes deployment")
	if len(results) != 0 {
		t.Errorf("Expected 0 results for unrelated search, got %d", len(results))
	}
}

func TestSearchSessionsSubstring(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")

	dr.StartSession("API returns 500 on auth endpoint")
	dr.EndSession(true)

	// Exact substring match
	results := dr.SearchSessions("API returns 500 on auth endpoint")
	if len(results) != 1 {
		t.Errorf("Expected 1 result for exact match, got %d", len(results))
	}
}

func TestBuildDebugContext(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")

	dr.StartSession("API returns 500 on auth endpoint")
	dr.RecordStep("read", "handler.go", "found issue", "nil check missing")
	dr.SetRootCause("nil pointer")
	dr.SetResolution("added nil check")
	dr.EndSession(true)

	ctx := dr.BuildDebugContext("auth endpoint error 500")
	if ctx == "" {
		t.Fatal("BuildDebugContext returned empty string")
	}
	if !strings.Contains(ctx, "Relevant Past Debug Sessions") {
		t.Error("Missing header")
	}
	if !strings.Contains(ctx, "auth endpoint") {
		t.Error("Missing session content")
	}
}

func TestBuildDebugContextNoMatches(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")

	dr.StartSession("CSS layout issue")
	dr.EndSession(true)

	ctx := dr.BuildDebugContext("kubernetes deployment failure")
	if ctx != "" {
		t.Errorf("Expected empty context for unrelated search, got %q", ctx)
	}
}

func TestBuildDebugContextLimit(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")

	// Create more than 5 matching sessions
	for i := 0; i < 8; i++ {
		dr.StartSession("auth error variant")
		dr.EndSession(true)
	}

	ctx := dr.BuildDebugContext("auth error")
	// Should contain at most 5 session outputs
	count := strings.Count(ctx, "Debug Session:")
	if count > 5 {
		t.Errorf("Expected at most 5 sessions in context, got %d", count)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	// Create recorder with sessions
	dr := NewDebugRecorder(dir)
	dr.StartSession("test bug")
	dr.RecordStep("read", "foo.go", "found issue", "missing nil check")
	dr.AddHypothesis("nil pointer")
	dr.ConfirmHypothesis(0, "confirmed in debugger")
	dr.SetRootCause("nil pointer dereference")
	dr.SetResolution("added nil check")
	dr.EndSession(true)

	// Save
	if err := dr.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "debug_sessions.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Sessions file not created: %v", err)
	}

	// Load into new recorder
	dr2 := NewDebugRecorder(dir)
	if err := dr2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(dr2.Sessions) != 1 {
		t.Fatalf("Loaded sessions length = %d, want 1", len(dr2.Sessions))
	}

	session := dr2.Sessions[0]
	if session.Symptom != "test bug" {
		t.Errorf("Loaded Symptom = %q", session.Symptom)
	}
	if session.RootCause != "nil pointer dereference" {
		t.Errorf("Loaded RootCause = %q", session.RootCause)
	}
	if session.Resolution != "added nil check" {
		t.Errorf("Loaded Resolution = %q", session.Resolution)
	}
	if !session.Successful {
		t.Error("Loaded session should be successful")
	}
	if len(session.Steps) != 2 { // read + hypothesis
		t.Errorf("Loaded steps length = %d, want 2", len(session.Steps))
	}
	if len(session.HypothesesTested) != 1 {
		t.Errorf("Loaded hypotheses length = %d, want 1", len(session.HypothesesTested))
	}
	if !session.HypothesesTested[0].Confirmed {
		t.Error("Loaded hypothesis should be confirmed")
	}
}

func TestLoadNoFile(t *testing.T) {
	dir := t.TempDir()
	dr := NewDebugRecorder(dir)

	// Should not error when file doesn't exist
	if err := dr.Load(); err != nil {
		t.Fatalf("Load should not error on missing file: %v", err)
	}
}

func TestSaveNoDir(t *testing.T) {
	dr := NewDebugRecorder("")
	if err := dr.Save(); err == nil {
		t.Error("Save with empty dir should return error")
	}
}

func TestLoadNoDir(t *testing.T) {
	dr := NewDebugRecorder("")
	if err := dr.Load(); err == nil {
		t.Error("Load with empty dir should return error")
	}
}

func TestSaveCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "debug")
	dr := NewDebugRecorder(dir)
	dr.StartSession("test")
	dr.EndSession(true)

	if err := dr.Save(); err != nil {
		t.Fatalf("Save should create nested directories: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "debug_sessions.json")); err != nil {
		t.Fatalf("File not created in nested dir: %v", err)
	}
}

func TestMultipleSessions(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")

	// Session 1
	dr.StartSession("bug 1")
	dr.RecordStep("read", "a.go", "ok", "")
	dr.EndSession(true)

	// Session 2
	dr.StartSession("bug 2")
	dr.RecordStep("grep", "pattern", "found", "")
	dr.EndSession(false)

	if len(dr.Sessions) != 2 {
		t.Errorf("Sessions length = %d, want 2", len(dr.Sessions))
	}
	if dr.Sessions[0].Symptom != "bug 1" {
		t.Errorf("Session 0 symptom = %q", dr.Sessions[0].Symptom)
	}
	if dr.Sessions[1].Symptom != "bug 2" {
		t.Errorf("Session 1 symptom = %q", dr.Sessions[1].Symptom)
	}
}

func TestDebugRecorderConcurrentAccess(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	dr.StartSession("concurrent test")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			dr.RecordStep("read", "file.go", "result", "insight")
		}(i)
	}
	wg.Wait()

	if len(dr.ActiveSession.Steps) != 50 {
		t.Errorf("Steps length = %d, want 50", len(dr.ActiveSession.Steps))
	}
}

func TestFormatSessionInProgress(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	session := dr.StartSession("ongoing issue")

	output := dr.FormatSession(session)
	if !strings.Contains(output, "in progress") {
		t.Error("Missing 'in progress' for session without EndTime")
	}
}

func TestFormatSessionShortDuration(t *testing.T) {
	dr := NewDebugRecorder("/tmp/test-debug")
	session := dr.StartSession("quick fix")

	endTime := session.StartTime.Add(45 * time.Second)
	session.EndTime = &endTime
	session.Successful = true

	output := dr.FormatSession(session)
	if !strings.Contains(output, "45s") {
		t.Errorf("Missing short duration, got:\n%s", output)
	}
}
