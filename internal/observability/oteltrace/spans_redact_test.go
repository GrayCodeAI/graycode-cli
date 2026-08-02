package oteltrace

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

// TestRedactErrorMessage_SecretsRemoved verifies the H15 fix: error messages
// carrying secret values are redacted before they appear in span tags.
func TestRedactErrorMessage_SecretsRemoved(t *testing.T) {
	redactorOnce = sync.Once{} // reset for deterministic state
	redactor = nil

	tests := []struct {
		name string
		err  error
	}{
		{name: "anthropic key", err: errors.New(`http call failed: sk-ant-api03-AbcDefGhiJklMnoPqrStuVwxYz1234567890`)},
		{name: "openai key", err: errors.New(`unauthorized: sk-a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6`)},
		{name: "aws key", err: errors.New(`access denied: AKIAIOSFODNN7EXAMPLE`)},
		{name: "github token", err: errors.New(`auth error: ghp_abcdefghijklmnopqrstuvwxyz123456abcdefghijklmno`)},
		{name: "password in url", err: errors.New(`connect failed: postgres://user:supersecret@dbhost:5432/app`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactErrorMessage(tt.err)
			if strings.Contains(got, "sk-ant-api") || strings.Contains(got, "sk-a1b2c3d4e5f6") ||
				strings.Contains(got, "AKIAIOSFODNN7EXAMPLE") || strings.Contains(got, "ghp_abcdefghijklmnopqrstuvwxyz123456") ||
				strings.Contains(got, "supersecret") {
				t.Errorf("error message was not redacted: %q", got)
			}
			if !strings.Contains(got, "[REDACTED") {
				t.Errorf("expected redaction marker in %q", got)
			}
		})
	}
}

// TestRedactErrorMessage_PassesThrough innocuous errors unmodified.
func TestRedactErrorMessage_PassesThrough(t *testing.T) {
	redactorOnce = sync.Once{}
	redactor = nil
	err := errors.New("context deadline exceeded")
	got := redactErrorMessage(err)
	if got != "context deadline exceeded" {
		t.Errorf("expected error to pass through unmodified, got %q", got)
	}
}
