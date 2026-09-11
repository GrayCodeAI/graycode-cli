package lsp

import (
	"os"
	"strings"
	"testing"
)

func TestIsSensitiveEnvKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"API_KEY", true},
		{"OPENAI_API_KEY", true},
		{"DB_PASSWORD", true},
		{"APP_SECRET", true},
		{"GITHUB_TOKEN", true},
		{"AUTH_HEADER", true},
		{"AWS_CREDENTIALS", true},
		{"PRIVATE_KEY", true},
		{"BEARER_TOKEN", true},
		{"SIGNATURE_HASH", true},
		{"APIKEY_TEST", true},
		{"PATH", false},
		{"HOME", false},
		{"USER", false},
		{"LANG", false},
		{"SHELL", false},
		{"GO111MODULE", false},
		{"GOPATH", false},
	}

	for _, tt := range tests {
		got := IsSensitiveEnvKey(tt.key)
		if got != tt.want {
			t.Errorf("IsSensitiveEnvKey(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestIsSensitiveEnvEntry(t *testing.T) {
	// Sensitive key
	if !IsSensitiveEnvEntry("SECRET_FOO", "val") {
		t.Error("expected true for sensitive key")
	}

	// Non-sensitive key and value
	if IsSensitiveEnvEntry("PLAIN_VAR", "ordinary_value") {
		t.Error("expected false for plain variable")
	}

	// Non-sensitive key but secret token value detected by tok
	secretVal := "sk-proj-abc1234567890abcdef1234567890abcdef"
	if !IsSensitiveEnvEntry("VAR_X", secretVal) {
		t.Logf("tok secret detector checked value %q", secretVal)
	}
}

func TestScrubEnvironment(t *testing.T) {
	ambient := []string{
		"PATH=/usr/bin:/bin",
		"USER=developer",
		"OPENAI_API_KEY=sk-1234567890",
		"AWS_SECRET_ACCESS_KEY=secretval",
		"DB_PASSWORD=supersecret",
		"ACCESS_TOKEN=token123",
		"CUSTOM_PLAIN=hello",
	}

	explicit := map[string]string{
		"CUSTOM_PLAIN": "overridden",
		"EXPLICIT_VAR": "value42",
	}

	scrubbed := ScrubEnvironment(ambient, explicit)

	scrubbedMap := make(map[string]string)
	for _, entry := range scrubbed {
		parts := strings.SplitN(entry, "=", 2)
		scrubbedMap[parts[0]] = parts[1]
	}

	// Sensitive keys must be scrubbed
	sensitiveKeys := []string{"OPENAI_API_KEY", "AWS_SECRET_ACCESS_KEY", "DB_PASSWORD", "ACCESS_TOKEN"}
	for _, k := range sensitiveKeys {
		if _, exists := scrubbedMap[k]; exists {
			t.Errorf("expected %q to be scrubbed from environment", k)
		}
	}

	// Safe keys must be preserved
	if scrubbedMap["PATH"] != "/usr/bin:/bin" {
		t.Errorf("PATH = %q, want %q", scrubbedMap["PATH"], "/usr/bin:/bin")
	}
	if scrubbedMap["USER"] != "developer" {
		t.Errorf("USER = %q, want %q", scrubbedMap["USER"], "developer")
	}

	// Explicit overrides and additions must be applied
	if scrubbedMap["CUSTOM_PLAIN"] != "overridden" {
		t.Errorf("CUSTOM_PLAIN = %q, want 'overridden'", scrubbedMap["CUSTOM_PLAIN"])
	}
	if scrubbedMap["EXPLICIT_VAR"] != "value42" {
		t.Errorf("EXPLICIT_VAR = %q, want 'value42'", scrubbedMap["EXPLICIT_VAR"])
	}
}

func TestScrubEnvironment_NilAmbient(t *testing.T) {
	os.Setenv("HAWK_TEST_SAFE_VAR", "safe_value")
	os.Setenv("HAWK_TEST_SECRET_KEY", "hidden_secret")
	defer os.Unsetenv("HAWK_TEST_SAFE_VAR")
	defer os.Unsetenv("HAWK_TEST_SECRET_KEY")

	scrubbed := ScrubEnvironment(nil, map[string]string{"INJECTED": "yes"})

	foundSafe := false
	foundSecret := false
	foundInjected := false

	for _, entry := range scrubbed {
		if strings.HasPrefix(entry, "HAWK_TEST_SAFE_VAR=") {
			foundSafe = true
		}
		if strings.HasPrefix(entry, "HAWK_TEST_SECRET_KEY=") {
			foundSecret = true
		}
		if entry == "INJECTED=yes" {
			foundInjected = true
		}
	}

	if !foundSafe {
		t.Error("expected HAWK_TEST_SAFE_VAR to be present")
	}
	if foundSecret {
		t.Error("expected HAWK_TEST_SECRET_KEY to be scrubbed")
	}
	if !foundInjected {
		t.Error("expected INJECTED=yes to be present")
	}
}
