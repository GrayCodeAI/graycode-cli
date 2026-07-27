package jsonc

import (
	"testing"
)

func TestValidateHooks_NotObject(t *testing.T) {
	r := &ValidationResult{}
	validateHooks("not-an-object", r)
	if r.Valid() {
		t.Error("expected validation error for non-object hooks")
	}
}

func TestValidateHooks_InvalidArray(t *testing.T) {
	r := &ValidationResult{}
	validateHooks(map[string]interface{}{
		"event": "not-an-array",
	}, r)
	if r.Valid() {
		t.Error("expected validation error for non-array hooks")
	}
}

func TestValidateHooks_InvalidItem(t *testing.T) {
	r := &ValidationResult{}
	validateHooks(map[string]interface{}{
		"event": []interface{}{"not-an-object"},
	}, r)
	if r.Valid() {
		t.Error("expected validation error for non-object hook item")
	}
}

func TestValidateHooks_InvalidMatcher(t *testing.T) {
	r := &ValidationResult{}
	validateHooks(map[string]interface{}{
		"event": []interface{}{
			map[string]interface{}{
				"matcher": 123,
			},
		},
	}, r)
	if r.Valid() {
		t.Error("expected validation error for non-string matcher")
	}
}

func TestValidateHooks_InvalidMatcherRegex(t *testing.T) {
	r := &ValidationResult{}
	validateHooks(map[string]interface{}{
		"event": []interface{}{
			map[string]interface{}{
				"matcher": "[invalid",
			},
		},
	}, r)
	if r.Valid() {
		t.Error("expected validation error for invalid regex matcher")
	}
}

func TestValidateHooks_InvalidHooksField(t *testing.T) {
	r := &ValidationResult{}
	validateHooks(map[string]interface{}{
		"event": []interface{}{
			map[string]interface{}{
				"hooks": "not-an-array",
			},
		},
	}, r)
	if r.Valid() {
		t.Error("expected validation error for non-array hooks field")
	}
}

func TestValidateHookEntry_NotObject(t *testing.T) {
	r := &ValidationResult{}
	validateHookEntry("path", "not-an-object", r)
	if r.Valid() {
		t.Error("expected validation error for non-object hook entry")
	}
}

func TestValidateHookEntry_NoType(t *testing.T) {
	r := &ValidationResult{}
	validateHookEntry("path", map[string]interface{}{}, r)
	if r.Valid() {
		t.Error("expected validation error for missing type")
	}
}

func TestValidateHookEntry_CommandEmpty(t *testing.T) {
	r := &ValidationResult{}
	validateHookEntry("path", map[string]interface{}{
		"type":    "command",
		"command": "",
	}, r)
	if r.Valid() {
		t.Error("expected validation error for empty command")
	}
}

func TestValidateHookEntry_CommandMissing(t *testing.T) {
	r := &ValidationResult{}
	validateHookEntry("path", map[string]interface{}{
		"type": "command",
	}, r)
	if r.Valid() {
		t.Error("expected validation error for missing command")
	}
}

func TestValidateHookEntry_CommandValid(t *testing.T) {
	r := &ValidationResult{}
	validateHookEntry("path", map[string]interface{}{
		"type":    "command",
		"command": "echo hello",
	}, r)
	if !r.Valid() {
		t.Errorf("expected no errors for valid command, got: %v", r.Errors)
	}
}

func TestValidateHookEntry_PromptInvalid(t *testing.T) {
	r := &ValidationResult{}
	validateHookEntry("path", map[string]interface{}{
		"type":    "prompt",
		"prompt":  123,
	}, r)
	if r.Valid() {
		t.Error("expected validation error for non-string prompt")
	}
}

func TestValidateHookEntry_PromptValid(t *testing.T) {
	r := &ValidationResult{}
	validateHookEntry("path", map[string]interface{}{
		"type":   "prompt",
		"prompt": "hello",
	}, r)
	if !r.Valid() {
		t.Errorf("expected no errors for valid prompt, got: %v", r.Errors)
	}
}

func TestValidateHookEntry_AgentNoPrompt(t *testing.T) {
	r := &ValidationResult{}
	validateHookEntry("path", map[string]interface{}{
		"type": "agent",
	}, r)
	if r.Valid() {
		t.Error("expected validation error for agent without prompt")
	}
}

func TestValidateHookEntry_AgentEmptyPrompt(t *testing.T) {
	r := &ValidationResult{}
	validateHookEntry("path", map[string]interface{}{
		"type":   "agent",
		"prompt": "",
	}, r)
	if r.Valid() {
		t.Error("expected validation error for agent with empty prompt")
	}
}

func TestValidateHookEntry_AgentValid(t *testing.T) {
	r := &ValidationResult{}
	validateHookEntry("path", map[string]interface{}{
		"type":   "agent",
		"prompt": "be helpful",
	}, r)
	if !r.Valid() {
		t.Errorf("expected no errors for valid agent, got: %v", r.Errors)
	}
}

func TestValidateHookEntry_InvalidType(t *testing.T) {
	r := &ValidationResult{}
	validateHookEntry("path", map[string]interface{}{
		"type": "invalid",
	}, r)
	if r.Valid() {
		t.Error("expected validation error for invalid type")
	}
}

func TestValidateMCPServers_NotObject(t *testing.T) {
	r := &ValidationResult{}
	validateMCPServers("not-an-object", r)
	if r.Valid() {
		t.Error("expected validation error for non-object mcpServers")
	}
}

func TestValidateMCPServers_InvalidEntry(t *testing.T) {
	r := &ValidationResult{}
	validateMCPServers(map[string]interface{}{
		"server1": "not-an-object",
	}, r)
	if r.Valid() {
		t.Error("expected validation error for non-object server entry")
	}
}

func TestValidateMCPServers_NoType(t *testing.T) {
	r := &ValidationResult{}
	validateMCPServers(map[string]interface{}{
		"server1": map[string]interface{}{},
	}, r)
	if r.Valid() {
		t.Error("expected validation error for missing type")
	}
}

func TestValidMatcherRegex_Valid(t *testing.T) {
	if !validMatcherRegex(".*") {
		t.Error("expected .* to be valid regex")
	}
	if !validMatcherRegex("Bash.*") {
		t.Error("expected Bash.* to be valid regex")
	}
}

func TestValidMatcherRegex_Invalid(t *testing.T) {
	if validMatcherRegex("[invalid") {
		t.Error("expected [invalid to be invalid regex")
	}
}

func TestAddErr_Nil(t *testing.T) {
	r := &ValidationResult{}
	r.AddErr(nil)
	if len(r.Errors) != 0 {
		t.Errorf("expected 0 errors after AddErr(nil), got %d", len(r.Errors))
	}
}

func TestAddErr_NonNil(t *testing.T) {
	r := &ValidationResult{}
	r.AddErr(&ErrValidation{Field: "test", Reason: "test error"})
	if len(r.Errors) != 1 {
		t.Errorf("expected 1 error after AddErr, got %d", len(r.Errors))
	}
}
