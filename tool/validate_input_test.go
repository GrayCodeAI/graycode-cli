package tool

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateToolInput_MissingRequired(t *testing.T) {
	err := ValidateToolInput("Bash", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing required field")
	}
	if !strings.Contains(err.Error(), "command") {
		t.Fatalf("expected error about 'command', got: %v", err)
	}
}

func TestValidateToolInput_EmptyRequired(t *testing.T) {
	err := ValidateToolInput("Bash", json.RawMessage(`{"command":""}`))
	if err == nil {
		t.Fatal("expected error for empty required field")
	}
}

func TestValidateToolInput_Valid(t *testing.T) {
	err := ValidateToolInput("Bash", json.RawMessage(`{"command":"echo hello"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateToolInput_UnknownTool(t *testing.T) {
	err := ValidateToolInput("UnknownToolXYZ", json.RawMessage(`{"foo":"bar"}`))
	if err != nil {
		t.Fatalf("unexpected error for unknown tool: %v", err)
	}
}

func TestValidateToolInput_InvalidJSON(t *testing.T) {
	err := ValidateToolInput("Bash", json.RawMessage(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
