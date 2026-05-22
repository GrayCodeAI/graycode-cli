package planning

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestNewActionManager(t *testing.T) {
	am := NewActionManager(func(a *ActionRequired) (*FormResponse, error) {
		return nil, nil
	})
	if am == nil {
		t.Fatal("expected non-nil ActionManager")
	}
	if len(am.Pending) != 0 {
		t.Errorf("expected 0 pending, got %d", len(am.Pending))
	}
	if len(am.History) != 0 {
		t.Errorf("expected 0 history, got %d", len(am.History))
	}
}

func TestRequest_BasicFlow(t *testing.T) {
	am := NewActionManager(func(a *ActionRequired) (*FormResponse, error) {
		return &FormResponse{
			Values:      map[string]string{"host": "localhost", "port": "5432"},
			SubmittedAt: time.Now(),
		}, nil
	})

	action := &ActionRequired{
		ID:    "test-1",
		Title: "Configure DB",
		Fields: []FormField{
			{Name: "host", Label: "Host", Type: "text", Required: true},
			{Name: "port", Label: "Port", Type: "number", Default: "5432"},
		},
	}

	resp, err := am.Request(action)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Values["host"] != "localhost" {
		t.Errorf("expected host=localhost, got %q", resp.Values["host"])
	}
	if resp.Values["port"] != "5432" {
		t.Errorf("expected port=5432, got %q", resp.Values["port"])
	}
	if !action.Resolved {
		t.Error("expected action to be resolved")
	}
	if len(am.Pending) != 0 {
		t.Errorf("expected 0 pending after resolve, got %d", len(am.Pending))
	}
	if len(am.History) != 1 {
		t.Errorf("expected 1 in history, got %d", len(am.History))
	}
}

func TestRequest_Timeout(t *testing.T) {
	am := NewActionManager(func(a *ActionRequired) (*FormResponse, error) {
		time.Sleep(200 * time.Millisecond)
		return &FormResponse{
			Values:      map[string]string{"value": "late"},
			SubmittedAt: time.Now(),
		}, nil
	})

	action := &ActionRequired{
		ID:      "timeout-1",
		Title:   "Slow prompt",
		Timeout: 50 * time.Millisecond,
		Fields: []FormField{
			{Name: "value", Label: "Value", Type: "text"},
		},
	}

	resp, err := am.Request(action)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response on timeout")
	}
	if !resp.TimedOut {
		t.Error("expected TimedOut to be true")
	}
}

func TestRequest_ValidationError(t *testing.T) {
	am := NewActionManager(func(a *ActionRequired) (*FormResponse, error) {
		return &FormResponse{
			Values:      map[string]string{"email": "not-an-email"},
			SubmittedAt: time.Now(),
		}, nil
	})

	action := &ActionRequired{
		ID:    "validate-1",
		Title: "Email",
		Fields: []FormField{
			{Name: "email", Label: "Email", Type: "text", Required: true, Validation: `^[^@]+@[^@]+\.[^@]+$`},
		},
	}

	_, err := am.Request(action)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "does not match pattern") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRequest_RequiredFieldMissing(t *testing.T) {
	am := NewActionManager(func(a *ActionRequired) (*FormResponse, error) {
		return &FormResponse{
			Values:      map[string]string{},
			SubmittedAt: time.Now(),
		}, nil
	})

	action := &ActionRequired{
		ID:    "required-1",
		Title: "Required Field",
		Fields: []FormField{
			{Name: "name", Label: "Name", Type: "text", Required: true},
		},
	}

	_, err := am.Request(action)
	if err == nil {
		t.Fatal("expected validation error for missing required field")
	}
	if !strings.Contains(err.Error(), "is required") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRequest_PromptError(t *testing.T) {
	am := NewActionManager(func(a *ActionRequired) (*FormResponse, error) {
		return nil, fmt.Errorf("user cancelled")
	})

	action := &ActionRequired{
		ID:    "error-1",
		Title: "Will fail",
		Fields: []FormField{
			{Name: "x", Label: "X", Type: "text"},
		},
	}

	_, err := am.Request(action)
	if err == nil {
		t.Fatal("expected error from prompt function")
	}
	if !strings.Contains(err.Error(), "user cancelled") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRequestText(t *testing.T) {
	am := NewActionManager(func(a *ActionRequired) (*FormResponse, error) {
		return &FormResponse{
			Values:      map[string]string{"value": "hello world"},
			SubmittedAt: time.Now(),
		}, nil
	})

	val, err := am.RequestText("Enter name", "What is your name?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "hello world" {
		t.Errorf("expected 'hello world', got %q", val)
	}
}

func TestRequestChoice(t *testing.T) {
	am := NewActionManager(func(a *ActionRequired) (*FormResponse, error) {
		return &FormResponse{
			Values:      map[string]string{"value": "blue"},
			SubmittedAt: time.Now(),
		}, nil
	})

	val, err := am.RequestChoice("Pick color", []string{"red", "blue", "green"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "blue" {
		t.Errorf("expected 'blue', got %q", val)
	}
}

func TestRequestChoice_InvalidChoice(t *testing.T) {
	am := NewActionManager(func(a *ActionRequired) (*FormResponse, error) {
		return &FormResponse{
			Values:      map[string]string{"value": "purple"},
			SubmittedAt: time.Now(),
		}, nil
	})

	_, err := am.RequestChoice("Pick color", []string{"red", "blue", "green"})
	if err == nil {
		t.Fatal("expected validation error for invalid choice")
	}
	if !strings.Contains(err.Error(), "invalid choice") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRequestConfirm(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"yes", true},
		{"y", true},
		{"false", false},
		{"no", false},
		{"n", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			am := NewActionManager(func(a *ActionRequired) (*FormResponse, error) {
				return &FormResponse{
					Values:      map[string]string{"value": tt.input},
					SubmittedAt: time.Now(),
				}, nil
			})
			val, err := am.RequestConfirm("Proceed?")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if val != tt.expected {
				t.Errorf("input %q: expected %v, got %v", tt.input, tt.expected, val)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	action := &ActionRequired{
		Fields: []FormField{
			{Name: "name", Label: "Name", Type: "text", Required: true},
			{Name: "age", Label: "Age", Type: "number", Validation: `^\d+$`},
			{Name: "mode", Label: "Mode", Type: "choice", Choices: []string{"fast", "slow"}},
		},
	}

	// All valid.
	resp := &FormResponse{Values: map[string]string{"name": "Alice", "age": "30", "mode": "fast"}}
	errs := Validate(action, resp)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}

	// Missing required.
	resp2 := &FormResponse{Values: map[string]string{"age": "30"}}
	errs2 := Validate(action, resp2)
	if len(errs2) != 1 {
		t.Errorf("expected 1 error, got %d: %v", len(errs2), errs2)
	}

	// Regex fail.
	resp3 := &FormResponse{Values: map[string]string{"name": "Bob", "age": "abc"}}
	errs3 := Validate(action, resp3)
	if len(errs3) != 1 {
		t.Errorf("expected 1 error, got %d: %v", len(errs3), errs3)
	}

	// Invalid choice.
	resp4 := &FormResponse{Values: map[string]string{"name": "Carol", "mode": "medium"}}
	errs4 := Validate(action, resp4)
	if len(errs4) != 1 {
		t.Errorf("expected 1 error, got %d: %v", len(errs4), errs4)
	}
}

func TestBuildFormPrompt(t *testing.T) {
	action := &ActionRequired{
		Title:       "Configure Database Connection",
		Description: "Enter database details below.",
		Fields: []FormField{
			{Name: "host", Label: "Host", Type: "text", Required: true},
			{Name: "port", Label: "Port", Type: "number", Default: "5432"},
			{Name: "db", Label: "Database", Type: "text", Required: true},
			{Name: "ssl", Label: "SSL Mode", Type: "choice", Choices: []string{"disable", "require", "verify-full"}},
		},
	}

	output := BuildFormPrompt(action)

	if !strings.Contains(output, "Action Required") {
		t.Error("missing Action Required header")
	}
	if !strings.Contains(output, "Configure Database Connection") {
		t.Error("missing title")
	}
	if !strings.Contains(output, "Host [text, required]") {
		t.Error("missing Host field")
	}
	if !strings.Contains(output, "default: 5432") {
		t.Error("missing default for port")
	}
	if !strings.Contains(output, "disable / require / verify-full") {
		t.Error("missing choices for SSL Mode")
	}
	if !strings.Contains(output, "Please provide:") {
		t.Error("missing 'Please provide' section")
	}
}

func TestFormatResponse(t *testing.T) {
	// nil response.
	out := FormatResponse(nil)
	if out != "<no response>" {
		t.Errorf("expected '<no response>', got %q", out)
	}

	// Timed out.
	out2 := FormatResponse(&FormResponse{TimedOut: true})
	if out2 != "<timed out>" {
		t.Errorf("expected '<timed out>', got %q", out2)
	}

	// Normal response.
	ts := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	out3 := FormatResponse(&FormResponse{
		Values:      map[string]string{"host": "db.example.com"},
		SubmittedAt: ts,
	})
	if !strings.Contains(out3, "2026-01-15") {
		t.Error("missing date in formatted response")
	}
	if !strings.Contains(out3, "host: db.example.com") {
		t.Error("missing host value in formatted response")
	}
}

func TestGetPending(t *testing.T) {
	blockCh := make(chan struct{})
	am := NewActionManager(func(a *ActionRequired) (*FormResponse, error) {
		<-blockCh
		return &FormResponse{
			Values:      map[string]string{"value": "done"},
			SubmittedAt: time.Now(),
		}, nil
	})

	// Start a request in background.
	go func() {
		am.Request(&ActionRequired{
			ID:    "pending-1",
			Title: "Blocked",
			Fields: []FormField{
				{Name: "value", Label: "Value", Type: "text"},
			},
		})
	}()

	// Give goroutine time to add to pending.
	time.Sleep(20 * time.Millisecond)

	pending := am.GetPending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if pending[0].ID != "pending-1" {
		t.Errorf("expected ID 'pending-1', got %q", pending[0].ID)
	}

	// Unblock.
	close(blockCh)
	time.Sleep(20 * time.Millisecond)

	pending2 := am.GetPending()
	if len(pending2) != 0 {
		t.Errorf("expected 0 pending after resolve, got %d", len(pending2))
	}
}

func TestCancel(t *testing.T) {
	blockCh := make(chan struct{})
	am := NewActionManager(func(a *ActionRequired) (*FormResponse, error) {
		<-blockCh
		return &FormResponse{Values: map[string]string{}, SubmittedAt: time.Now()}, nil
	})

	// Start a request in background.
	go func() {
		am.Request(&ActionRequired{
			ID:    "cancel-1",
			Title: "To be cancelled",
			Fields: []FormField{
				{Name: "value", Label: "Value", Type: "text"},
			},
		})
	}()

	time.Sleep(20 * time.Millisecond)

	am.Cancel("cancel-1")

	pending := am.GetPending()
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after cancel, got %d", len(pending))
	}

	if len(am.History) != 1 {
		t.Errorf("expected 1 in history after cancel, got %d", len(am.History))
	}
	if !am.History[0].Resolved {
		t.Error("expected cancelled action to be marked resolved")
	}

	// Unblock so goroutine can exit.
	close(blockCh)
	time.Sleep(20 * time.Millisecond)
}

func TestValidate_EmptyOptionalField(t *testing.T) {
	action := &ActionRequired{
		Fields: []FormField{
			{Name: "notes", Label: "Notes", Type: "text", Required: false, Validation: `^\w+$`},
		},
	}
	// Empty optional field should not trigger regex validation.
	resp := &FormResponse{Values: map[string]string{"notes": ""}}
	errs := Validate(action, resp)
	if len(errs) != 0 {
		t.Errorf("expected no errors for empty optional field, got: %v", errs)
	}
}

func TestRequest_SetsCreatedAt(t *testing.T) {
	am := NewActionManager(func(a *ActionRequired) (*FormResponse, error) {
		return &FormResponse{Values: map[string]string{}, SubmittedAt: time.Now()}, nil
	})

	action := &ActionRequired{
		ID:     "time-1",
		Title:  "Test",
		Fields: []FormField{},
	}

	before := time.Now()
	am.Request(action)
	after := time.Now()

	if action.CreatedAt.Before(before) || action.CreatedAt.After(after) {
		t.Errorf("CreatedAt %v not between %v and %v", action.CreatedAt, before, after)
	}
}
