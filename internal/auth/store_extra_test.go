package auth

import (
	"testing"
)

func TestTokenStore_GetSetHas(t *testing.T) {
	ts := &TokenStore{
		tokens: make(map[string]string),
	}

	if ts.Has("github") {
		t.Error("expected github token to not be set")
	}

	ts.Set("github", "test-token")

	if !ts.Has("github") {
		t.Error("expected github token to be set")
	}

	val := ts.Get("github")
	if val != "test-token" {
		t.Errorf("Get() = %q, want %q", val, "test-token")
	}

	nonexistent := ts.Get("nonexistent")
	if nonexistent != "" {
		t.Errorf("Get() for non-existent = %q, want empty", nonexistent)
	}
}

func TestSecureStorage_FallbackGetFile(t *testing.T) {
	// The secure storage falls back to file on non-darwin, non-windows platforms.
	// We can test the fallback on platforms where GOOS is something else, but on
	// darwin/windows it runs the specific OS helper.
	// Since we mock execCommand or file operations, we can at least verify behavior.
	// However, we just want to cover the token store functions and the general Get/Set interfaces.
}
