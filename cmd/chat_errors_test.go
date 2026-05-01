package cmd

import (
	"errors"
	"testing"
)

func TestFriendlyError(t *testing.T) {
	tests := []struct {
		name     string
		err      string
		contains string
	}{
		{"rate limit 429", "eyrie: openai stream request failed: max retries (3) exceeded: HTTP 429", "Rate limited"},
		{"unauthorized 401", "HTTP 401 Unauthorized", "Authentication failed"},
		{"forbidden 403", "HTTP 403 Forbidden", "Access denied"},
		{"not found 404", "model not found", "/model"},
		{"server error 500", "HTTP 500 Internal Server Error", "server error"},
		{"bad gateway 502", "HTTP 502 Bad Gateway", "temporarily unavailable"},
		{"service unavailable 503", "HTTP 503 Service Unavailable", "temporarily unavailable"},
		{"timeout", "context deadline exceeded", "timed out"},
		{"connection refused", "connection refused", "Connection refused"},
		{"unknown error", "something weird happened", "something weird happened"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := friendlyError(errors.New(tt.err))
			if !containsCI(got, tt.contains) {
				t.Errorf("friendlyError(%q) = %q, want it to contain %q", tt.err, got, tt.contains)
			}
		})
	}
}

func containsCI(s, substr string) bool {
	return len(s) >= len(substr) && contains(s, substr)
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
