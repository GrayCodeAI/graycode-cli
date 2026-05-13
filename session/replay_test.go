package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewReplay(t *testing.T) {
	r := NewReplay("session-123")

	if r.SessionID != "session-123" {
		t.Errorf("expected SessionID 'session-123', got %q", r.SessionID)
	}
	if r.Speed != 1.0 {
		t.Errorf("expected Speed 1.0, got %f", r.Speed)
	}
	if r.Status != "idle" {
		t.Errorf("expected Status 'idle', got %q", r.Status)
	}
	if r.CurrentStep != 0 {
		t.Errorf("expected CurrentStep 0, got %d", r.CurrentStep)
	}
	if len(r.Steps) != 0 {
		t.Errorf("expected empty Steps, got %d", len(r.Steps))
	}
	if len(r.Breakpoints) != 0 {
		t.Errorf("expected empty Breakpoints, got %d", len(r.Breakpoints))
	}
}

func TestLoadFromExport(t *testing.T) {
	ts := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	lines := []string{
		mustMarshal(map[string]interface{}{
			"seq": 1, "timestamp": ts.Format(time.RFC3339),
			"delta_ms": 0, "role": "user", "content": "hello",
		}),
		mustMarshal(map[string]interface{}{
			"seq": 2, "timestamp": ts.Add(500 * time.Millisecond).Format(time.RFC3339),
			"delta_ms": 500, "role": "assistant", "content": "hi there",
		}),
		mustMarshal(map[string]interface{}{
			"seq": 3, "timestamp": ts.Add(2 * time.Second).Format(time.RFC3339),
			"delta_ms": 1500, "role": "user", "content": "how are you?",
		}),
	}

	data := strings.Join(lines, "\n")
	r, err := LoadFromExport(data)
	if err != nil {
		t.Fatalf("LoadFromExport failed: %v", err)
	}

	if len(r.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(r.Steps))
	}
	if r.Steps[0].Role != "user" {
		t.Errorf("step 0 role: expected 'user', got %q", r.Steps[0].Role)
	}
	if r.Steps[0].Content != "hello" {
		t.Errorf("step 0 content: expected 'hello', got %q", r.Steps[0].Content)
	}
	if r.Steps[1].Role != "assistant" {
		t.Errorf("step 1 role: expected 'assistant', got %q", r.Steps[1].Role)
	}
	if r.Steps[1].OriginalDuration != 500*time.Millisecond {
		t.Errorf("step 1 duration: expected 500ms, got %v", r.Steps[1].OriginalDuration)
	}
	if r.Steps[2].Content != "how are you?" {
		t.Errorf("step 2 content: expected 'how are you?', got %q", r.Steps[2].Content)
	}
	if r.Status != "idle" {
		t.Errorf("expected status 'idle', got %q", r.Status)
	}
}

func TestLoadFromExportEmpty(t *testing.T) {
	_, err := LoadFromExport("")
	if err == nil {
		t.Error("expected error for empty data")
	}
}

func TestLoadFromExportInvalidJSON(t *testing.T) {
	_, err := LoadFromExport("not valid json\n")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadFromExportWithToolInfo(t *testing.T) {
	ts := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	lines := []string{
		mustMarshal(map[string]interface{}{
			"seq": 1, "timestamp": ts.Format(time.RFC3339),
			"delta_ms": 0, "role": "user", "content": "run ls",
		}),
		mustMarshal(map[string]interface{}{
			"seq": 2, "timestamp": ts.Add(time.Second).Format(time.RFC3339),
			"delta_ms": 1000, "role": "assistant", "content": "running ls",
			"tool_name": "bash", "tool_result": "file1.go\nfile2.go",
		}),
	}

	data := strings.Join(lines, "\n")
	r, err := LoadFromExport(data)
	if err != nil {
		t.Fatalf("LoadFromExport failed: %v", err)
	}

	if r.Steps[1].ToolName != "bash" {
		t.Errorf("expected tool_name 'bash', got %q", r.Steps[1].ToolName)
	}
	if r.Steps[1].ToolArgs != "file1.go\nfile2.go" {
		t.Errorf("expected tool_args to contain tool_result, got %q", r.Steps[1].ToolArgs)
	}
}

func TestPlayBasic(t *testing.T) {
	r := NewReplay("test-session")
	r.Speed = 0 // instant replay
	r.Steps = []ReplayStep{
		{Index: 0, Role: "user", Content: "hello"},
		{Index: 1, Role: "assistant", Content: "hi there"},
		{Index: 2, Role: "user", Content: "bye"},
		{Index: 3, Role: "assistant", Content: "goodbye"},
	}

	callCount := 0
	executeFn := func(prompt string) (string, error) {
		callCount++
		switch prompt {
		case "hello":
			return "hi there", nil
		case "bye":
			return "goodbye", nil
		default:
			return "", fmt.Errorf("unexpected prompt: %s", prompt)
		}
	}

	ctx := context.Background()
	result, err := r.Play(ctx, executeFn)
	if err != nil {
		t.Fatalf("Play failed: %v", err)
	}

	if result.OriginalSteps != 4 {
		t.Errorf("expected OriginalSteps=4, got %d", result.OriginalSteps)
	}
	if result.ReplayedSteps != 2 {
		t.Errorf("expected ReplayedSteps=2, got %d", result.ReplayedSteps)
	}
	if len(result.Divergences) != 0 {
		t.Errorf("expected no divergences, got %d", len(result.Divergences))
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls to executeFn, got %d", callCount)
	}
	if r.Status != "done" {
		t.Errorf("expected status 'done', got %q", r.Status)
	}
}

func TestPlayWithDivergence(t *testing.T) {
	r := NewReplay("test-session")
	r.Speed = 0
	r.Steps = []ReplayStep{
		{Index: 0, Role: "user", Content: "hello"},
		{Index: 1, Role: "assistant", Content: "expected response"},
	}

	executeFn := func(prompt string) (string, error) {
		return "different response", nil
	}

	ctx := context.Background()
	result, err := r.Play(ctx, executeFn)
	if err != nil {
		t.Fatalf("Play failed: %v", err)
	}

	if len(result.Divergences) != 1 {
		t.Fatalf("expected 1 divergence, got %d", len(result.Divergences))
	}
	if result.Divergences[0].Type != "content_mismatch" {
		t.Errorf("expected type 'content_mismatch', got %q", result.Divergences[0].Type)
	}
	if result.Divergences[0].Expected != "expected response" {
		t.Errorf("expected Expected='expected response', got %q", result.Divergences[0].Expected)
	}
	if result.Divergences[0].Got != "different response" {
		t.Errorf("expected Got='different response', got %q", result.Divergences[0].Got)
	}
}

func TestPlayWithError(t *testing.T) {
	r := NewReplay("test-session")
	r.Speed = 0
	r.Steps = []ReplayStep{
		{Index: 0, Role: "user", Content: "hello"},
		{Index: 1, Role: "assistant", Content: "expected"},
	}

	executeFn := func(prompt string) (string, error) {
		return "", fmt.Errorf("model unavailable")
	}

	ctx := context.Background()
	result, err := r.Play(ctx, executeFn)
	if err != nil {
		t.Fatalf("Play failed: %v", err)
	}

	if len(result.Divergences) != 1 {
		t.Fatalf("expected 1 divergence, got %d", len(result.Divergences))
	}
	if result.Divergences[0].Type != "error" {
		t.Errorf("expected type 'error', got %q", result.Divergences[0].Type)
	}
	if !strings.Contains(result.Divergences[0].Got, "model unavailable") {
		t.Errorf("expected error message in Got, got %q", result.Divergences[0].Got)
	}
}

func TestPlayContextCancellation(t *testing.T) {
	r := NewReplay("test-session")
	r.Speed = 0
	r.Steps = []ReplayStep{
		{Index: 0, Role: "user", Content: "hello"},
		{Index: 1, Role: "assistant", Content: "hi"},
		{Index: 2, Role: "user", Content: "second"},
		{Index: 3, Role: "assistant", Content: "response"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	callCount := 0

	executeFn := func(prompt string) (string, error) {
		callCount++
		if callCount == 1 {
			cancel() // Cancel after first execution.
		}
		return "response", nil
	}

	result, err := r.Play(ctx, executeFn)
	if err == nil {
		t.Error("expected context cancellation error")
	}
	if result == nil {
		t.Fatal("expected non-nil result even on cancellation")
	}
	if r.Status != "stopped" {
		t.Errorf("expected status 'stopped', got %q", r.Status)
	}
}

func TestPlayStep(t *testing.T) {
	r := NewReplay("test-session")
	r.Steps = []ReplayStep{
		{Index: 0, Role: "user", Content: "what is 2+2?"},
		{Index: 1, Role: "assistant", Content: "4"},
	}

	executeFn := func(prompt string) (string, error) {
		if prompt == "what is 2+2?" {
			return "4", nil
		}
		return "", fmt.Errorf("unexpected")
	}

	ctx := context.Background()
	result, err := r.PlayStep(ctx, 0, executeFn)
	if err != nil {
		t.Fatalf("PlayStep failed: %v", err)
	}

	if result.Role != "assistant" {
		t.Errorf("expected role 'assistant', got %q", result.Role)
	}
	if result.Content != "4" {
		t.Errorf("expected content '4', got %q", result.Content)
	}
}

func TestPlayStepNonUser(t *testing.T) {
	r := NewReplay("test-session")
	r.Steps = []ReplayStep{
		{Index: 0, Role: "assistant", Content: "I am assistant"},
	}

	executeFn := func(prompt string) (string, error) {
		t.Error("executeFn should not be called for non-user steps")
		return "", nil
	}

	ctx := context.Background()
	result, err := r.PlayStep(ctx, 0, executeFn)
	if err != nil {
		t.Fatalf("PlayStep failed: %v", err)
	}

	if result.Content != "I am assistant" {
		t.Errorf("expected original step content, got %q", result.Content)
	}
}

func TestPlayStepOutOfRange(t *testing.T) {
	r := NewReplay("test-session")
	r.Steps = []ReplayStep{
		{Index: 0, Role: "user", Content: "hello"},
	}

	executeFn := func(prompt string) (string, error) {
		return "hi", nil
	}

	ctx := context.Background()
	_, err := r.PlayStep(ctx, 5, executeFn)
	if err == nil {
		t.Error("expected error for out-of-range step")
	}

	_, err = r.PlayStep(ctx, -1, executeFn)
	if err == nil {
		t.Error("expected error for negative step index")
	}
}

func TestSetBreakpointAndRemove(t *testing.T) {
	r := NewReplay("test-session")

	r.SetBreakpoint(3)
	r.SetBreakpoint(7)
	r.SetBreakpoint(3) // duplicate, should not be added

	if len(r.Breakpoints) != 2 {
		t.Errorf("expected 2 breakpoints, got %d", len(r.Breakpoints))
	}

	r.RemoveBreakpoint(3)
	if len(r.Breakpoints) != 1 {
		t.Errorf("expected 1 breakpoint after removal, got %d", len(r.Breakpoints))
	}
	if r.Breakpoints[0] != 7 {
		t.Errorf("expected remaining breakpoint at 7, got %d", r.Breakpoints[0])
	}

	// Remove non-existent breakpoint should be no-op.
	r.RemoveBreakpoint(99)
	if len(r.Breakpoints) != 1 {
		t.Errorf("expected 1 breakpoint, got %d", len(r.Breakpoints))
	}
}

func TestPauseResumeStop(t *testing.T) {
	r := NewReplay("test-session")

	// Pause when not playing should be no-op.
	r.Pause()
	if r.Status != "idle" {
		t.Errorf("expected status 'idle' (pause on non-playing), got %q", r.Status)
	}

	// Simulate playing state.
	r.mu.Lock()
	r.Status = "playing"
	r.mu.Unlock()

	r.Pause()
	if r.Status != "paused" {
		t.Errorf("expected status 'paused', got %q", r.Status)
	}

	r.Resume()
	if r.Status != "playing" {
		t.Errorf("expected status 'playing' after resume, got %q", r.Status)
	}

	r.Stop()
	if r.Status != "stopped" {
		t.Errorf("expected status 'stopped', got %q", r.Status)
	}

	// Resume when stopped should be no-op (only resumes from paused).
	r.Resume()
	if r.Status != "stopped" {
		t.Errorf("expected status still 'stopped', got %q", r.Status)
	}
}

func TestPlayWithBreakpoint(t *testing.T) {
	r := NewReplay("test-session")
	r.Speed = 0
	r.Steps = []ReplayStep{
		{Index: 0, Role: "user", Content: "first"},
		{Index: 1, Role: "assistant", Content: "response1"},
		{Index: 2, Role: "user", Content: "second"},
		{Index: 3, Role: "assistant", Content: "response2"},
	}

	r.SetBreakpoint(2) // Breakpoint at the second user message.

	var mu sync.Mutex
	calls := []string{}

	executeFn := func(prompt string) (string, error) {
		mu.Lock()
		calls = append(calls, prompt)
		mu.Unlock()
		switch prompt {
		case "first":
			return "response1", nil
		case "second":
			return "response2", nil
		}
		return "", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start play in goroutine since it will pause at breakpoint.
	var result *ReplayResult
	var playErr error
	done := make(chan struct{})

	go func() {
		result, playErr = r.Play(ctx, executeFn)
		close(done)
	}()

	// Wait for it to hit breakpoint.
	deadline := time.After(1 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for breakpoint")
		default:
		}
		r.mu.Lock()
		s := r.Status
		r.mu.Unlock()
		if s == "paused" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Resume to continue execution.
	r.Resume()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for play to finish")
	}

	if playErr != nil {
		t.Fatalf("Play failed: %v", playErr)
	}

	mu.Lock()
	callCount := len(calls)
	mu.Unlock()

	if callCount != 2 {
		t.Errorf("expected 2 executed prompts, got %d", callCount)
	}
	if result.ReplayedSteps != 2 {
		t.Errorf("expected ReplayedSteps=2, got %d", result.ReplayedSteps)
	}
}

func TestPlayStop(t *testing.T) {
	r := NewReplay("test-session")
	r.Speed = 0
	r.Steps = []ReplayStep{
		{Index: 0, Role: "user", Content: "first"},
		{Index: 1, Role: "assistant", Content: "response1"},
		{Index: 2, Role: "user", Content: "second"},
		{Index: 3, Role: "assistant", Content: "response2"},
	}

	callCount := 0
	executeFn := func(prompt string) (string, error) {
		callCount++
		if callCount == 1 {
			r.Stop()
		}
		return "response", nil
	}

	ctx := context.Background()
	result, err := r.Play(ctx, executeFn)
	if err != nil {
		t.Fatalf("Play failed: %v", err)
	}

	if r.Status != "stopped" {
		t.Errorf("expected status 'stopped', got %q", r.Status)
	}
	// Should have executed at most 1 prompt before stopping.
	if result.ReplayedSteps > 1 {
		t.Errorf("expected at most 1 replayed step, got %d", result.ReplayedSteps)
	}
}

func TestDetectDivergenceNone(t *testing.T) {
	div := DetectDivergence("hello world", "hello world")
	if div != nil {
		t.Error("expected nil divergence for matching strings")
	}
}

func TestDetectDivergenceWhitespace(t *testing.T) {
	div := DetectDivergence("  hello world  ", "hello world")
	if div != nil {
		t.Error("expected nil divergence when only whitespace differs")
	}
}

func TestDetectDivergenceContentMismatch(t *testing.T) {
	div := DetectDivergence("expected output", "actual output")
	if div == nil {
		t.Fatal("expected non-nil divergence")
	}
	if div.Type != "content_mismatch" {
		t.Errorf("expected type 'content_mismatch', got %q", div.Type)
	}
	if div.Expected != "expected output" {
		t.Errorf("expected Expected='expected output', got %q", div.Expected)
	}
	if div.Got != "actual output" {
		t.Errorf("expected Got='actual output', got %q", div.Got)
	}
}

func TestDetectDivergenceToolMismatch(t *testing.T) {
	expected := `{"name": "read_file", "tool_use": true}`
	actual := `{"name": "write_file", "tool_use": true}`

	div := DetectDivergence(expected, actual)
	if div == nil {
		t.Fatal("expected non-nil divergence")
	}
	if div.Type != "tool_mismatch" {
		t.Errorf("expected type 'tool_mismatch', got %q", div.Type)
	}
}

func TestDetectDivergenceSameToolDifferentContent(t *testing.T) {
	expected := `{"name": "bash", "tool_use": true, "args": "ls"}`
	actual := `{"name": "bash", "tool_use": true, "args": "pwd"}`

	div := DetectDivergence(expected, actual)
	if div == nil {
		t.Fatal("expected non-nil divergence")
	}
	// Same tool name, so it should be content_mismatch, not tool_mismatch.
	if div.Type != "content_mismatch" {
		t.Errorf("expected type 'content_mismatch', got %q", div.Type)
	}
}

func TestFormatReplayStatus(t *testing.T) {
	r := NewReplay("test-session")
	r.Steps = []ReplayStep{
		{Index: 0, Role: "user", Content: "a"},
		{Index: 1, Role: "assistant", Content: "b"},
		{Index: 2, Role: "user", Content: "c"},
		{Index: 3, Role: "assistant", Content: "d"},
	}
	r.Status = "playing"
	r.CurrentStep = 1

	status := r.FormatReplayStatus()
	if !strings.Contains(status, "playing") {
		t.Errorf("expected status to contain 'playing', got %q", status)
	}
	if !strings.Contains(status, "2/4") {
		t.Errorf("expected status to contain '2/4', got %q", status)
	}
	if !strings.Contains(status, "1.0x") {
		t.Errorf("expected status to contain '1.0x', got %q", status)
	}
}

func TestFormatReplayStatusInstantSpeed(t *testing.T) {
	r := NewReplay("test-session")
	r.Steps = []ReplayStep{{Index: 0, Role: "user", Content: "a"}}
	r.Speed = 0
	r.Status = "idle"

	status := r.FormatReplayStatus()
	if !strings.Contains(status, "instant") {
		t.Errorf("expected status to contain 'instant', got %q", status)
	}
}

func TestFormatReplayStatusWithBreakpoints(t *testing.T) {
	r := NewReplay("test-session")
	r.Steps = []ReplayStep{{Index: 0, Role: "user", Content: "a"}}
	r.Status = "idle"
	r.SetBreakpoint(5)

	status := r.FormatReplayStatus()
	if !strings.Contains(status, "breakpoints") {
		t.Errorf("expected status to contain 'breakpoints', got %q", status)
	}
}

func TestFormatReplayStatusNoSteps(t *testing.T) {
	r := NewReplay("test-session")
	status := r.FormatReplayStatus()
	if !strings.Contains(status, "no steps") {
		t.Errorf("expected status to indicate no steps, got %q", status)
	}
}

func TestFormatDivergences(t *testing.T) {
	divs := []Divergence{
		{StepIndex: 2, Expected: "foo", Got: "bar", Type: "content_mismatch"},
		{StepIndex: 5, Expected: "read_file", Got: "error: timeout", Type: "error"},
	}

	output := FormatDivergences(divs)
	if !strings.Contains(output, "2 divergence(s)") {
		t.Errorf("expected '2 divergence(s)' in output, got %q", output)
	}
	if !strings.Contains(output, "step 2") {
		t.Errorf("expected 'step 2' in output, got %q", output)
	}
	if !strings.Contains(output, "step 5") {
		t.Errorf("expected 'step 5' in output, got %q", output)
	}
	if !strings.Contains(output, "content_mismatch") {
		t.Errorf("expected 'content_mismatch' in output, got %q", output)
	}
	if !strings.Contains(output, "error") {
		t.Errorf("expected 'error' type in output, got %q", output)
	}
}

func TestFormatDivergencesEmpty(t *testing.T) {
	output := FormatDivergences(nil)
	if !strings.Contains(output, "No divergences") {
		t.Errorf("expected 'No divergences' for empty list, got %q", output)
	}
}

func TestExtractUserPrompts(t *testing.T) {
	steps := []ReplayStep{
		{Index: 0, Role: "user", Content: "first prompt"},
		{Index: 1, Role: "assistant", Content: "first response"},
		{Index: 2, Role: "user", Content: "second prompt"},
		{Index: 3, Role: "assistant", Content: "second response"},
		{Index: 4, Role: "user", Content: "third prompt"},
	}

	prompts := ExtractUserPrompts(steps)
	if len(prompts) != 3 {
		t.Fatalf("expected 3 prompts, got %d", len(prompts))
	}
	if prompts[0] != "first prompt" {
		t.Errorf("expected 'first prompt', got %q", prompts[0])
	}
	if prompts[1] != "second prompt" {
		t.Errorf("expected 'second prompt', got %q", prompts[1])
	}
	if prompts[2] != "third prompt" {
		t.Errorf("expected 'third prompt', got %q", prompts[2])
	}
}

func TestExtractUserPromptsEmpty(t *testing.T) {
	prompts := ExtractUserPrompts(nil)
	if len(prompts) != 0 {
		t.Errorf("expected 0 prompts for nil input, got %d", len(prompts))
	}

	prompts = ExtractUserPrompts([]ReplayStep{
		{Index: 0, Role: "assistant", Content: "no user messages"},
	})
	if len(prompts) != 0 {
		t.Errorf("expected 0 prompts when no user role, got %d", len(prompts))
	}
}

func TestReplayConcurrentAccess(t *testing.T) {
	r := NewReplay("concurrent-test")
	r.Steps = []ReplayStep{
		{Index: 0, Role: "user", Content: "hello"},
		{Index: 1, Role: "assistant", Content: "hi"},
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			r.SetBreakpoint(n)
			r.FormatReplayStatus()
			r.RemoveBreakpoint(n)
		}(i)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		r.Pause()
		r.Resume()
		r.Stop()
	}()

	wg.Wait()
}

func TestPlayWithSpeedDelay(t *testing.T) {
	r := NewReplay("test-session")
	r.Speed = 100.0 // 100x speed to make delays very short.
	r.Steps = []ReplayStep{
		{Index: 0, Role: "user", Content: "hello", OriginalDuration: 100 * time.Millisecond},
		{Index: 1, Role: "assistant", Content: "hi"},
	}

	executeFn := func(prompt string) (string, error) {
		return "hi", nil
	}

	ctx := context.Background()
	start := time.Now()
	result, err := r.Play(ctx, executeFn)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Play failed: %v", err)
	}
	if result.ReplayedSteps != 1 {
		t.Errorf("expected 1 replayed step, got %d", result.ReplayedSteps)
	}
	// At 100x speed, 100ms becomes 1ms. Should complete quickly.
	if elapsed > 500*time.Millisecond {
		t.Errorf("expected fast execution at 100x speed, took %v", elapsed)
	}
}

func TestFindExpectedResponse(t *testing.T) {
	steps := []ReplayStep{
		{Index: 0, Role: "user", Content: "hello"},
		{Index: 1, Role: "assistant", Content: "hi there"},
		{Index: 2, Role: "user", Content: "bye"},
		{Index: 3, Role: "assistant", Content: "goodbye"},
	}

	// After user at index 0, expect assistant at index 1.
	expected := findExpectedResponse(steps, 0)
	if expected != "hi there" {
		t.Errorf("expected 'hi there', got %q", expected)
	}

	// After user at index 2, expect assistant at index 3.
	expected = findExpectedResponse(steps, 2)
	if expected != "goodbye" {
		t.Errorf("expected 'goodbye', got %q", expected)
	}

	// No response after last assistant.
	expected = findExpectedResponse(steps, 3)
	if expected != "" {
		t.Errorf("expected empty, got %q", expected)
	}
}

func TestFindExpectedResponseConsecutiveUsers(t *testing.T) {
	steps := []ReplayStep{
		{Index: 0, Role: "user", Content: "first"},
		{Index: 1, Role: "user", Content: "second"},
		{Index: 2, Role: "assistant", Content: "response"},
	}

	// First user should not find response (next is another user).
	expected := findExpectedResponse(steps, 0)
	if expected != "" {
		t.Errorf("expected empty (consecutive users), got %q", expected)
	}

	// Second user should find the assistant response.
	expected = findExpectedResponse(steps, 1)
	if expected != "response" {
		t.Errorf("expected 'response', got %q", expected)
	}
}

func TestLooksLikeToolCall(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{`{"name": "bash", "tool_use": true}`, true},
		{`calling tool_call now`, true},
		{`using function_call here`, true},
		{`plain text response`, false},
		{`{"message": "hello"}`, false},
	}

	for _, tc := range tests {
		got := looksLikeToolCall(tc.input)
		if got != tc.expected {
			t.Errorf("looksLikeToolCall(%q) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

func TestExtractToolName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`{"name": "bash", "args": "ls"}`, "bash"},
		{`{"name": "read_file", "tool_use": true}`, "read_file"},
		{`plain text`, ""},
		{`{"message": "hello"}`, ""},
	}

	for _, tc := range tests {
		got := extractToolName(tc.input)
		if got != tc.expected {
			t.Errorf("extractToolName(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"this is a longer string", 10, "this is a ..."},
		{"exact", 5, "exact"},
		{"multi\nline\ntext", 20, "multi line text"},
	}

	for _, tc := range tests {
		got := truncate(tc.input, tc.maxLen)
		if got != tc.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.expected)
		}
	}
}

func TestIntegrationLoadAndPlay(t *testing.T) {
	// Build a replay export like ExportReplay would produce.
	ts := time.Date(2025, 3, 10, 14, 0, 0, 0, time.UTC)

	entries := []replayEntry{
		{Seq: 1, Timestamp: ts, DeltaMs: 0, Role: "user", Content: "What is Go?"},
		{Seq: 2, Timestamp: ts.Add(2 * time.Second), DeltaMs: 2000, Role: "assistant", Content: "Go is a programming language."},
		{Seq: 3, Timestamp: ts.Add(5 * time.Second), DeltaMs: 3000, Role: "user", Content: "Who created it?"},
		{Seq: 4, Timestamp: ts.Add(7 * time.Second), DeltaMs: 2000, Role: "assistant", Content: "Go was created at Google."},
	}

	var b strings.Builder
	for _, e := range entries {
		data, _ := json.Marshal(e)
		b.Write(data)
		b.WriteByte('\n')
	}

	// Load from export.
	replay, err := LoadFromExport(b.String())
	if err != nil {
		t.Fatalf("LoadFromExport failed: %v", err)
	}

	if len(replay.Steps) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(replay.Steps))
	}

	// Extract user prompts.
	prompts := ExtractUserPrompts(replay.Steps)
	if len(prompts) != 2 {
		t.Fatalf("expected 2 user prompts, got %d", len(prompts))
	}
	if prompts[0] != "What is Go?" {
		t.Errorf("expected 'What is Go?', got %q", prompts[0])
	}
	if prompts[1] != "Who created it?" {
		t.Errorf("expected 'Who created it?', got %q", prompts[1])
	}

	// Play with instant speed and matching responses.
	replay.Speed = 0
	executeFn := func(prompt string) (string, error) {
		switch prompt {
		case "What is Go?":
			return "Go is a programming language.", nil
		case "Who created it?":
			return "Go was created at Google.", nil
		}
		return "", fmt.Errorf("unexpected: %s", prompt)
	}

	ctx := context.Background()
	result, err := replay.Play(ctx, executeFn)
	if err != nil {
		t.Fatalf("Play failed: %v", err)
	}

	if result.OriginalSteps != 4 {
		t.Errorf("expected OriginalSteps=4, got %d", result.OriginalSteps)
	}
	if result.ReplayedSteps != 2 {
		t.Errorf("expected ReplayedSteps=2, got %d", result.ReplayedSteps)
	}
	if len(result.Divergences) != 0 {
		t.Errorf("expected no divergences, got %d: %v", len(result.Divergences), result.Divergences)
	}
}

func TestIntegrationLoadAndPlayWithDivergence(t *testing.T) {
	ts := time.Date(2025, 3, 10, 14, 0, 0, 0, time.UTC)

	entries := []replayEntry{
		{Seq: 1, Timestamp: ts, DeltaMs: 0, Role: "user", Content: "What is Go?"},
		{Seq: 2, Timestamp: ts.Add(2 * time.Second), DeltaMs: 2000, Role: "assistant", Content: "Go is a systems programming language."},
	}

	var b strings.Builder
	for _, e := range entries {
		data, _ := json.Marshal(e)
		b.Write(data)
		b.WriteByte('\n')
	}

	replay, err := LoadFromExport(b.String())
	if err != nil {
		t.Fatalf("LoadFromExport failed: %v", err)
	}
	replay.Speed = 0

	// Respond with something different.
	executeFn := func(prompt string) (string, error) {
		return "Go is an open-source language developed at Google.", nil
	}

	ctx := context.Background()
	result, err := replay.Play(ctx, executeFn)
	if err != nil {
		t.Fatalf("Play failed: %v", err)
	}

	if len(result.Divergences) != 1 {
		t.Fatalf("expected 1 divergence, got %d", len(result.Divergences))
	}

	div := result.Divergences[0]
	if div.StepIndex != 0 {
		t.Errorf("expected divergence at step 0, got %d", div.StepIndex)
	}
	if div.Type != "content_mismatch" {
		t.Errorf("expected type content_mismatch, got %q", div.Type)
	}

	// Format the divergences.
	formatted := FormatDivergences(result.Divergences)
	if !strings.Contains(formatted, "1 divergence") {
		t.Errorf("expected '1 divergence' in formatted output, got %q", formatted)
	}
}

// mustMarshal is a test helper that panics on marshal failure.
func mustMarshal(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}
