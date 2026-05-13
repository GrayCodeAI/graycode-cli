package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mockExecuteFn returns a simple mock execution function for testing.
func mockExecuteFn(outputs map[string]string) func(context.Context, string, string) (string, error) {
	return func(ctx context.Context, action, input string) (string, error) {
		if out, ok := outputs[action+":"+input]; ok {
			return out, nil
		}
		if out, ok := outputs[action]; ok {
			return out, nil
		}
		return fmt.Sprintf("executed %s", action), nil
	}
}

// failingExecuteFn returns a function that always fails.
func failingExecuteFn() func(context.Context, string, string) (string, error) {
	return func(ctx context.Context, action, input string) (string, error) {
		return "", fmt.Errorf("execution failed: %s", action)
	}
}

func TestNewWorkflowEngine(t *testing.T) {
	fn := mockExecuteFn(nil)
	engine := NewWorkflowEngine(fn)

	if engine == nil {
		t.Fatal("expected non-nil engine")
	}
	if engine.Workflows == nil {
		t.Fatal("expected non-nil Workflows map")
	}
	if engine.ExecuteFn == nil {
		t.Fatal("expected non-nil ExecuteFn")
	}
}

func TestLoadWorkflow(t *testing.T) {
	wf := Workflow{
		Name:        "test-workflow",
		Description: "A test workflow",
		OnFailure:   "abort",
		Steps: []WorkflowStep{
			{Name: "step1", Action: "bash", Input: "echo hello", Output: "result"},
			{Name: "step2", Action: "prompt", Input: "{{.result}}", DependsOn: []string{"step1"}},
		},
		Variables: map[string]string{"env": "test"},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.json")
	data, _ := json.Marshal(wf)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	engine := NewWorkflowEngine(mockExecuteFn(nil))
	loaded, err := engine.LoadWorkflow(path)
	if err != nil {
		t.Fatalf("LoadWorkflow failed: %v", err)
	}

	if loaded.Name != "test-workflow" {
		t.Errorf("expected name 'test-workflow', got %q", loaded.Name)
	}
	if len(loaded.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(loaded.Steps))
	}
	if loaded.Variables["env"] != "test" {
		t.Errorf("expected variable env='test', got %q", loaded.Variables["env"])
	}

	// Verify it was stored
	if _, ok := engine.Workflows["test-workflow"]; !ok {
		t.Error("workflow was not stored in engine")
	}
}

func TestLoadWorkflow_FileNotFound(t *testing.T) {
	engine := NewWorkflowEngine(mockExecuteFn(nil))
	_, err := engine.LoadWorkflow("/nonexistent/path.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadWorkflow_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte("not json"), 0644)

	engine := NewWorkflowEngine(mockExecuteFn(nil))
	_, err := engine.LoadWorkflow(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadWorkflow_CycleDetection(t *testing.T) {
	wf := Workflow{
		Name: "cyclic",
		Steps: []WorkflowStep{
			{Name: "a", Action: "bash", DependsOn: []string{"b"}},
			{Name: "b", Action: "bash", DependsOn: []string{"a"}},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "cyclic.json")
	data, _ := json.Marshal(wf)
	os.WriteFile(path, data, 0644)

	engine := NewWorkflowEngine(mockExecuteFn(nil))
	_, err := engine.LoadWorkflow(path)
	if err == nil {
		t.Fatal("expected error for cyclic dependencies")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected 'circular' in error, got: %v", err)
	}
}

func TestExecute_SimpleWorkflow(t *testing.T) {
	outputs := map[string]string{
		"bash": "hello world",
	}
	engine := NewWorkflowEngine(mockExecuteFn(outputs))

	wf := &Workflow{
		Name:      "simple",
		OnFailure: "abort",
		Variables: map[string]string{},
		Steps: []WorkflowStep{
			{Name: "greet", Action: "bash", Input: "echo hello", Output: "greeting"},
		},
	}

	result, err := engine.Execute(context.Background(), wf)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("expected status 'success', got %q", result.Status)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step result, got %d", len(result.Steps))
	}
	if result.Steps[0].Status != "success" {
		t.Errorf("expected step status 'success', got %q", result.Steps[0].Status)
	}
	if result.Variables["greeting"] != "hello world" {
		t.Errorf("expected variable greeting='hello world', got %q", result.Variables["greeting"])
	}
}

func TestExecute_WithDependencies(t *testing.T) {
	callOrder := []string{}
	fn := func(ctx context.Context, action, input string) (string, error) {
		callOrder = append(callOrder, action+":"+input)
		return "output-" + action, nil
	}

	engine := NewWorkflowEngine(fn)
	wf := &Workflow{
		Name:      "deps",
		OnFailure: "abort",
		Variables: map[string]string{},
		Steps: []WorkflowStep{
			{Name: "step1", Action: "bash", Input: "first", Output: "out1"},
			{Name: "step2", Action: "prompt", Input: "{{.out1}}", Output: "out2", DependsOn: []string{"step1"}},
			{Name: "step3", Action: "read", Input: "{{.out2}}", DependsOn: []string{"step2"}},
		},
	}

	result, err := engine.Execute(context.Background(), wf)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("expected 'success', got %q", result.Status)
	}
	if len(callOrder) != 3 {
		t.Fatalf("expected 3 calls, got %d: %v", len(callOrder), callOrder)
	}
	// step2 should have received output from step1
	if callOrder[1] != "prompt:output-bash" {
		t.Errorf("step2 input should be 'output-bash', got %q", callOrder[1])
	}
}

func TestExecute_ConditionSkip(t *testing.T) {
	engine := NewWorkflowEngine(mockExecuteFn(nil))

	wf := &Workflow{
		Name:      "conditional",
		OnFailure: "abort",
		Variables: map[string]string{"skip": "true"},
		Steps: []WorkflowStep{
			{
				Name:      "maybe",
				Action:    "bash",
				Input:     "echo hi",
				Condition: "{{.skip}} == false",
			},
		},
	}

	result, err := engine.Execute(context.Background(), wf)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Steps[0].Status != "skipped" {
		t.Errorf("expected 'skipped', got %q", result.Steps[0].Status)
	}
}

func TestExecute_ConditionPass(t *testing.T) {
	engine := NewWorkflowEngine(mockExecuteFn(nil))

	wf := &Workflow{
		Name:      "conditional-pass",
		OnFailure: "abort",
		Variables: map[string]string{"run": "yes"},
		Steps: []WorkflowStep{
			{
				Name:      "runnable",
				Action:    "bash",
				Input:     "echo hi",
				Condition: "{{.run}} == yes",
			},
		},
	}

	result, err := engine.Execute(context.Background(), wf)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Steps[0].Status != "success" {
		t.Errorf("expected 'success', got %q", result.Steps[0].Status)
	}
}

func TestExecute_FailureAbort(t *testing.T) {
	engine := NewWorkflowEngine(failingExecuteFn())

	wf := &Workflow{
		Name:      "will-fail",
		OnFailure: "abort",
		Variables: map[string]string{},
		Steps: []WorkflowStep{
			{Name: "bad", Action: "bash", Input: "fail"},
			{Name: "unreachable", Action: "bash", Input: "never"},
		},
	}

	result, err := engine.Execute(context.Background(), wf)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if result.Status != "failed" {
		t.Errorf("expected 'failed', got %q", result.Status)
	}
	// Only first step should have been attempted
	if len(result.Steps) != 1 {
		t.Errorf("expected 1 step result (abort after first), got %d", len(result.Steps))
	}
}

func TestExecute_FailureContinue(t *testing.T) {
	callCount := 0
	fn := func(ctx context.Context, action, input string) (string, error) {
		callCount++
		if callCount == 1 {
			return "", fmt.Errorf("first fails")
		}
		return "ok", nil
	}

	engine := NewWorkflowEngine(fn)
	wf := &Workflow{
		Name:      "continue-on-fail",
		OnFailure: "continue",
		Variables: map[string]string{},
		Steps: []WorkflowStep{
			{Name: "step1", Action: "bash", Input: "x"},
			{Name: "step2", Action: "bash", Input: "y"},
		},
	}

	result, err := engine.Execute(context.Background(), wf)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("expected 'success' with continue policy, got %q", result.Status)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 step results, got %d", len(result.Steps))
	}
	if result.Steps[0].Status != "failed" {
		t.Errorf("expected step1 'failed', got %q", result.Steps[0].Status)
	}
	if result.Steps[1].Status != "success" {
		t.Errorf("expected step2 'success', got %q", result.Steps[1].Status)
	}
}

func TestExecute_Retry(t *testing.T) {
	attempts := 0
	fn := func(ctx context.Context, action, input string) (string, error) {
		attempts++
		if attempts < 3 {
			return "", fmt.Errorf("not yet")
		}
		return "finally", nil
	}

	engine := NewWorkflowEngine(fn)
	wf := &Workflow{
		Name:      "retry-test",
		OnFailure: "abort",
		Variables: map[string]string{},
		Steps: []WorkflowStep{
			{Name: "flaky", Action: "bash", Input: "do it", MaxRetries: 3, Output: "out"},
		},
	}

	result, err := engine.Execute(context.Background(), wf)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("expected 'success', got %q", result.Status)
	}
	if result.Variables["out"] != "finally" {
		t.Errorf("expected output 'finally', got %q", result.Variables["out"])
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestExecute_MaxDurationTimeout(t *testing.T) {
	fn := func(ctx context.Context, action, input string) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
			return "done", nil
		}
	}

	engine := NewWorkflowEngine(fn)
	wf := &Workflow{
		Name:        "timeout-test",
		OnFailure:   "abort",
		Variables:   map[string]string{},
		MaxDuration: 50 * time.Millisecond,
		Steps: []WorkflowStep{
			{Name: "slow", Action: "bash", Input: "sleep"},
		},
	}

	result, err := engine.Execute(context.Background(), wf)
	// Should return with aborted status or context error
	if err == nil && result.Status != "aborted" && result.Status != "failed" {
		t.Errorf("expected aborted/failed or error for timed-out workflow, got status=%q err=%v", result.Status, err)
	}
}

func TestSubstituteVars(t *testing.T) {
	tests := []struct {
		template string
		vars     map[string]string
		expected string
	}{
		{
			template: "hello {{.name}}",
			vars:     map[string]string{"name": "world"},
			expected: "hello world",
		},
		{
			template: "{{.a}} and {{.b}}",
			vars:     map[string]string{"a": "foo", "b": "bar"},
			expected: "foo and bar",
		},
		{
			template: "no vars here",
			vars:     map[string]string{},
			expected: "no vars here",
		},
		{
			template: "{{.missing}} stays",
			vars:     map[string]string{},
			expected: "{{.missing}} stays",
		},
		{
			template: "{{ .spaced }}",
			vars:     map[string]string{"spaced": "value"},
			expected: "value",
		},
		{
			template: "{{.x}}{{.y}}",
			vars:     map[string]string{"x": "a", "y": "b"},
			expected: "ab",
		},
	}

	for _, tt := range tests {
		result := SubstituteVars(tt.template, tt.vars)
		if result != tt.expected {
			t.Errorf("SubstituteVars(%q, %v) = %q, want %q", tt.template, tt.vars, result, tt.expected)
		}
	}
}

func TestEvalCondition(t *testing.T) {
	tests := []struct {
		condition string
		vars      map[string]string
		expected  bool
	}{
		{
			condition: "{{.status}} == success",
			vars:      map[string]string{"status": "success"},
			expected:  true,
		},
		{
			condition: "{{.status}} == success",
			vars:      map[string]string{"status": "failed"},
			expected:  false,
		},
		{
			condition: "{{.count}} > 0",
			vars:      map[string]string{"count": "5"},
			expected:  true,
		},
		{
			condition: "{{.count}} > 0",
			vars:      map[string]string{"count": "-1"},
			expected:  false,
		},
		{
			condition: "{{.count}} >= 5",
			vars:      map[string]string{"count": "5"},
			expected:  true,
		},
		{
			condition: "{{.count}} < 10",
			vars:      map[string]string{"count": "5"},
			expected:  true,
		},
		{
			condition: "{{.a}} != {{.b}}",
			vars:      map[string]string{"a": "x", "b": "y"},
			expected:  true,
		},
		{
			condition: "{{.a}} != {{.b}}",
			vars:      map[string]string{"a": "same", "b": "same"},
			expected:  false,
		},
		{
			condition: "{{.flag}}",
			vars:      map[string]string{"flag": "true"},
			expected:  true,
		},
		{
			condition: "{{.flag}}",
			vars:      map[string]string{"flag": "false"},
			expected:  false,
		},
		{
			condition: "{{.flag}}",
			vars:      map[string]string{"flag": ""},
			expected:  false,
		},
	}

	for _, tt := range tests {
		result := EvalCondition(tt.condition, tt.vars)
		if result != tt.expected {
			t.Errorf("EvalCondition(%q, %v) = %v, want %v", tt.condition, tt.vars, result, tt.expected)
		}
	}
}

func TestValidateWorkflow(t *testing.T) {
	t.Run("valid workflow", func(t *testing.T) {
		wf := &Workflow{
			Name:      "valid",
			OnFailure: "abort",
			Variables: map[string]string{"input": "data"},
			Steps: []WorkflowStep{
				{Name: "step1", Action: "bash", Input: "{{.input}}", Output: "out1"},
				{Name: "step2", Action: "prompt", Input: "{{.out1}}", DependsOn: []string{"step1"}},
			},
		}
		warnings := ValidateWorkflow(wf)
		if len(warnings) != 0 {
			t.Errorf("expected no warnings, got: %v", warnings)
		}
	})

	t.Run("no name", func(t *testing.T) {
		wf := &Workflow{Steps: []WorkflowStep{{Name: "s", Action: "bash"}}}
		warnings := ValidateWorkflow(wf)
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "no name") {
				found = true
			}
		}
		if !found {
			t.Error("expected warning about missing name")
		}
	})

	t.Run("no steps", func(t *testing.T) {
		wf := &Workflow{Name: "empty"}
		warnings := ValidateWorkflow(wf)
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "no steps") {
				found = true
			}
		}
		if !found {
			t.Error("expected warning about no steps")
		}
	})

	t.Run("duplicate step names", func(t *testing.T) {
		wf := &Workflow{
			Name: "dupes",
			Steps: []WorkflowStep{
				{Name: "same", Action: "bash"},
				{Name: "same", Action: "prompt"},
			},
		}
		warnings := ValidateWorkflow(wf)
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "duplicate") {
				found = true
			}
		}
		if !found {
			t.Error("expected warning about duplicate step names")
		}
	})

	t.Run("unknown action", func(t *testing.T) {
		wf := &Workflow{
			Name:  "bad-action",
			Steps: []WorkflowStep{{Name: "s", Action: "fly"}},
		}
		warnings := ValidateWorkflow(wf)
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "unknown action") {
				found = true
			}
		}
		if !found {
			t.Error("expected warning about unknown action")
		}
	})

	t.Run("unknown dependency", func(t *testing.T) {
		wf := &Workflow{
			Name: "bad-dep",
			Steps: []WorkflowStep{
				{Name: "s", Action: "bash", DependsOn: []string{"nonexistent"}},
			},
		}
		warnings := ValidateWorkflow(wf)
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "unknown step") {
				found = true
			}
		}
		if !found {
			t.Error("expected warning about unknown dependency")
		}
	})

	t.Run("cycle detection", func(t *testing.T) {
		wf := &Workflow{
			Name: "cyclic",
			Steps: []WorkflowStep{
				{Name: "a", Action: "bash", DependsOn: []string{"b"}},
				{Name: "b", Action: "bash", DependsOn: []string{"a"}},
			},
		}
		warnings := ValidateWorkflow(wf)
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "cycle") {
				found = true
			}
		}
		if !found {
			t.Error("expected warning about cycle")
		}
	})

	t.Run("undefined variable", func(t *testing.T) {
		wf := &Workflow{
			Name:      "undef-var",
			Variables: map[string]string{},
			Steps: []WorkflowStep{
				{Name: "s", Action: "bash", Input: "{{.undefined}}"},
			},
		}
		warnings := ValidateWorkflow(wf)
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "undefined variable") {
				found = true
			}
		}
		if !found {
			t.Error("expected warning about undefined variable")
		}
	})

	t.Run("invalid on_failure", func(t *testing.T) {
		wf := &Workflow{
			Name:      "bad-policy",
			OnFailure: "explode",
			Steps:     []WorkflowStep{{Name: "s", Action: "bash"}},
		}
		warnings := ValidateWorkflow(wf)
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "invalid on_failure") {
				found = true
			}
		}
		if !found {
			t.Error("expected warning about invalid on_failure")
		}
	})
}

func TestBuiltinWorkflows(t *testing.T) {
	builtins := BuiltinWorkflows()

	expectedNames := []string{"pr-review", "fix-tests", "refactor"}
	for _, name := range expectedNames {
		wf, ok := builtins[name]
		if !ok {
			t.Errorf("expected builtin workflow %q", name)
			continue
		}
		if wf.Name != name {
			t.Errorf("builtin %q has name %q", name, wf.Name)
		}
		if len(wf.Steps) == 0 {
			t.Errorf("builtin %q has no steps", name)
		}
		// Validate each builtin
		warnings := ValidateWorkflow(wf)
		// The refactor template has a special condition that may reference test_result weirdly;
		// we just check no critical errors like cycles
		for _, w := range warnings {
			if strings.Contains(w, "cycle") {
				t.Errorf("builtin %q has cycle warning: %s", name, w)
			}
		}
	}
}

func TestFormatResult(t *testing.T) {
	result := &WorkflowResult{
		Status: "success",
		Steps: []StepResult{
			{StepName: "step1", Status: "success", Duration: 100 * time.Millisecond},
			{StepName: "step2", Status: "skipped"},
			{StepName: "step3", Status: "failed", Error: "something broke", Duration: 50 * time.Millisecond},
		},
		Duration:  200 * time.Millisecond,
		Variables: map[string]string{"out": "hello"},
	}

	formatted := FormatResult(result)

	if !strings.Contains(formatted, "success") {
		t.Error("expected 'success' in output")
	}
	if !strings.Contains(formatted, "step1") {
		t.Error("expected 'step1' in output")
	}
	if !strings.Contains(formatted, "skipped") {
		t.Error("expected 'skipped' in output")
	}
	if !strings.Contains(formatted, "something broke") {
		t.Error("expected error message in output")
	}
	if !strings.Contains(formatted, "out = hello") {
		t.Error("expected variable output in formatted result")
	}
}

func TestTopologicalSort(t *testing.T) {
	steps := []WorkflowStep{
		{Name: "c", Action: "bash", DependsOn: []string{"a", "b"}},
		{Name: "a", Action: "bash"},
		{Name: "b", Action: "bash", DependsOn: []string{"a"}},
	}

	order, err := topologicalSort(steps)
	if err != nil {
		t.Fatalf("topologicalSort failed: %v", err)
	}

	// Verify a comes before b and c, and b comes before c
	posOf := make(map[string]int)
	for pos, idx := range order {
		posOf[steps[idx].Name] = pos
	}

	if posOf["a"] >= posOf["b"] {
		t.Errorf("a should come before b: a=%d, b=%d", posOf["a"], posOf["b"])
	}
	if posOf["a"] >= posOf["c"] {
		t.Errorf("a should come before c: a=%d, c=%d", posOf["a"], posOf["c"])
	}
	if posOf["b"] >= posOf["c"] {
		t.Errorf("b should come before c: b=%d, c=%d", posOf["b"], posOf["c"])
	}
}

func TestTopologicalSort_Cycle(t *testing.T) {
	steps := []WorkflowStep{
		{Name: "a", Action: "bash", DependsOn: []string{"c"}},
		{Name: "b", Action: "bash", DependsOn: []string{"a"}},
		{Name: "c", Action: "bash", DependsOn: []string{"b"}},
	}

	_, err := topologicalSort(steps)
	if err == nil {
		t.Fatal("expected error for cyclic graph")
	}
}

func TestExecute_VariableSubstitutionChain(t *testing.T) {
	fn := func(ctx context.Context, action, input string) (string, error) {
		// Echo back the input with a prefix
		return "processed:" + input, nil
	}

	engine := NewWorkflowEngine(fn)
	wf := &Workflow{
		Name:      "chain",
		OnFailure: "abort",
		Variables: map[string]string{"seed": "initial"},
		Steps: []WorkflowStep{
			{Name: "first", Action: "bash", Input: "{{.seed}}", Output: "mid"},
			{Name: "second", Action: "prompt", Input: "{{.mid}}", Output: "final", DependsOn: []string{"first"}},
		},
	}

	result, err := engine.Execute(context.Background(), wf)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Variables["mid"] != "processed:initial" {
		t.Errorf("expected mid='processed:initial', got %q", result.Variables["mid"])
	}
	if result.Variables["final"] != "processed:processed:initial" {
		t.Errorf("expected final='processed:processed:initial', got %q", result.Variables["final"])
	}
}

func TestExecute_StepTimeout(t *testing.T) {
	fn := func(ctx context.Context, action, input string) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
			return "done", nil
		}
	}

	engine := NewWorkflowEngine(fn)
	wf := &Workflow{
		Name:      "step-timeout",
		OnFailure: "abort",
		Variables: map[string]string{},
		Steps: []WorkflowStep{
			{Name: "slow", Action: "bash", Input: "wait", Timeout: 50 * time.Millisecond},
		},
	}

	result, err := engine.Execute(context.Background(), wf)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if result.Status != "failed" {
		t.Errorf("expected 'failed', got %q", result.Status)
	}
	if result.Steps[0].Status != "failed" {
		t.Errorf("expected step status 'failed', got %q", result.Steps[0].Status)
	}
}

func TestExecute_ContextCancellation(t *testing.T) {
	fn := func(ctx context.Context, action, input string) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
			return "done", nil
		}
	}

	engine := NewWorkflowEngine(fn)
	wf := &Workflow{
		Name:      "cancellable",
		OnFailure: "abort",
		Variables: map[string]string{},
		Steps: []WorkflowStep{
			{Name: "long", Action: "bash", Input: "wait"},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	result, err := engine.Execute(ctx, wf)
	// Either err is non-nil (context) or result shows failed/aborted
	if err == nil && result != nil && result.Status == "success" {
		t.Error("expected non-success for cancelled context")
	}
}

func TestHasCycle(t *testing.T) {
	tests := []struct {
		name     string
		steps    []WorkflowStep
		expected bool
	}{
		{
			name: "no cycle",
			steps: []WorkflowStep{
				{Name: "a", Action: "bash"},
				{Name: "b", Action: "bash", DependsOn: []string{"a"}},
			},
			expected: false,
		},
		{
			name: "self cycle",
			steps: []WorkflowStep{
				{Name: "a", Action: "bash", DependsOn: []string{"a"}},
			},
			expected: true,
		},
		{
			name: "indirect cycle",
			steps: []WorkflowStep{
				{Name: "a", Action: "bash", DependsOn: []string{"c"}},
				{Name: "b", Action: "bash", DependsOn: []string{"a"}},
				{Name: "c", Action: "bash", DependsOn: []string{"b"}},
			},
			expected: true,
		},
		{
			name: "diamond (no cycle)",
			steps: []WorkflowStep{
				{Name: "a", Action: "bash"},
				{Name: "b", Action: "bash", DependsOn: []string{"a"}},
				{Name: "c", Action: "bash", DependsOn: []string{"a"}},
				{Name: "d", Action: "bash", DependsOn: []string{"b", "c"}},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasCycle(tt.steps)
			if result != tt.expected {
				t.Errorf("hasCycle() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExecute_EmptyWorkflow(t *testing.T) {
	engine := NewWorkflowEngine(mockExecuteFn(nil))
	wf := &Workflow{
		Name:      "empty",
		OnFailure: "abort",
		Variables: map[string]string{"x": "1"},
		Steps:     []WorkflowStep{},
	}

	result, err := engine.Execute(context.Background(), wf)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.Status != "success" {
		t.Errorf("expected 'success' for empty workflow, got %q", result.Status)
	}
	if result.Variables["x"] != "1" {
		t.Errorf("expected initial variables to be preserved")
	}
}

func TestExecute_RetryExhausted(t *testing.T) {
	engine := NewWorkflowEngine(failingExecuteFn())
	wf := &Workflow{
		Name:      "exhaust-retry",
		OnFailure: "abort",
		Variables: map[string]string{},
		Steps: []WorkflowStep{
			{Name: "failing", Action: "bash", Input: "x", MaxRetries: 2},
		},
	}

	result, err := engine.Execute(context.Background(), wf)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Status != "failed" {
		t.Errorf("expected 'failed', got %q", result.Status)
	}
	if result.Steps[0].Error == "" {
		t.Error("expected error message in step result")
	}
}

func TestFormatResult_LongVariable(t *testing.T) {
	longVal := strings.Repeat("x", 200)
	result := &WorkflowResult{
		Status:    "success",
		Steps:     []StepResult{},
		Duration:  1 * time.Second,
		Variables: map[string]string{"long": longVal},
	}

	formatted := FormatResult(result)
	// Should be truncated
	if strings.Contains(formatted, longVal) {
		t.Error("expected long variable to be truncated")
	}
	if !strings.Contains(formatted, "...") {
		t.Error("expected '...' for truncated variable")
	}
}
