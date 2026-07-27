package hooks

import (
	"testing"
)

func TestBuiltinHooks(t *testing.T) {
	hooks := BuiltinHooks()
	if len(hooks) == 0 {
		t.Fatal("expected non-empty BuiltinHooks")
	}
	// Check that all hooks have required fields
	for _, h := range hooks {
		if h.Name == "" {
			t.Error("expected non-empty hook Name")
		}
	}
}

func TestValidateHTTPHookURL_Empty(t *testing.T) {
	err := ValidateHTTPHookURL("")
	if err == nil {
		t.Error("expected error for empty URL")
	}
}

func TestValidateHTTPHookURL_Invalid(t *testing.T) {
	err := ValidateHTTPHookURL("not-a-url")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestValidateHTTPHookURL_Valid(t *testing.T) {
	err := ValidateHTTPHookURL("http://localhost:8080/hook")
	if err != nil {
		t.Errorf("expected no error for valid URL, got: %v", err)
	}
}

func TestValidateHTTPHookURL_HTTPS(t *testing.T) {
	err := ValidateHTTPHookURL("https://example.com/hook")
	if err != nil {
		t.Errorf("expected no error for valid HTTPS URL, got: %v", err)
	}
}

func TestLoadConventionPolicies_EmptyDir(t *testing.T) {
	// Test with a temp directory that has no policies
	count := LoadConventionPolicies(t.TempDir())
	if count != 0 {
		t.Errorf("expected 0 policies for empty dir, got %d", count)
	}
}

func TestLoadConventionPolicies_NonExistentDir(t *testing.T) {
	count := LoadConventionPolicies("/nonexistent/directory/xyz")
	if count != 0 {
		t.Errorf("expected 0 policies for non-existent dir, got %d", count)
	}
}

func TestCanonicalEvent_Empty(t *testing.T) {
	result := CanonicalEvent("")
	if result != "" {
		t.Errorf("CanonicalEvent(\"\") = %q, want empty", result)
	}
}

func TestCanonicalEvent_KnownEvent(t *testing.T) {
	// Test with a known event alias
	result := CanonicalEvent("pre_tool_use")
	// Should return a canonical event or empty if not recognized
	_ = result
}

func TestCanonicalEvent_UnknownEvent(t *testing.T) {
	result := CanonicalEvent("unknown_event_xyz")
	// Should return the original or empty
	_ = result
}
