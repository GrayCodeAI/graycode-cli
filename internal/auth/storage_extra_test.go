package auth

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSecureStorage_GetSet_Fallback tests the file-based fallback storage
// used on Linux when no OS keyring is available.
func TestSecureStorage_GetSet_Fallback(t *testing.T) {
	// Override config dir to use a temp directory
	tmpDir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", tmpDir)

	ss := &SecureStorage{service: "test-service"}

	// Set a token
	err := ss.Set("test-account", "secret-token")
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get the token back
	token, err := ss.Get("test-account")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if token != "secret-token" {
		t.Errorf("expected 'secret-token', got %q", token)
	}
}

// TestSecureStorage_GetSet_MultipleAccounts tests storing multiple accounts.
func TestSecureStorage_GetSet_MultipleAccounts(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", tmpDir)

	ss := &SecureStorage{service: "test-service"}

	// Set multiple tokens
	if err := ss.Set("account1", "token1"); err != nil {
		t.Fatalf("Set account1 failed: %v", err)
	}
	if err := ss.Set("account2", "token2"); err != nil {
		t.Fatalf("Set account2 failed: %v", err)
	}

	// Retrieve both
	t1, err := ss.Get("account1")
	if err != nil {
		t.Fatalf("Get account1 failed: %v", err)
	}
	if t1 != "token1" {
		t.Errorf("expected 'token1', got %q", t1)
	}

	t2, err := ss.Get("account2")
	if err != nil {
		t.Fatalf("Get account2 failed: %v", err)
	}
	if t2 != "token2" {
		t.Errorf("expected 'token2', got %q", t2)
	}
}

// TestSecureStorage_Get_NonExistent tests getting a token that doesn't exist.
func TestSecureStorage_Get_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", tmpDir)

	ss := &SecureStorage{service: "test-service"}

	// Get should fail because no token file exists
	_, err := ss.Get("non-existent-account")
	if err == nil {
		t.Error("expected error for non-existent account")
	}
}

// TestSecureStorage_Set_OverwritesExisting tests that Set overwrites existing tokens.
func TestSecureStorage_Set_OverwritesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", tmpDir)

	ss := &SecureStorage{service: "test-service"}

	// Set initial token
	if err := ss.Set("account", "old-token"); err != nil {
		t.Fatalf("Set old-token failed: %v", err)
	}

	// Overwrite with new token
	if err := ss.Set("account", "new-token"); err != nil {
		t.Fatalf("Set new-token failed: %v", err)
	}

	// Verify new token
	token, err := ss.Get("account")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if token != "new-token" {
		t.Errorf("expected 'new-token', got %q", token)
	}
}

// TestSecureStorage_GetFile_CorruptJSON tests that corrupt JSON returns an error.
func TestSecureStorage_GetFile_CorruptJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", tmpDir)

	// Write corrupt JSON to the token file
	tokenFile := filepath.Join(tmpDir, ".tokens")
	if err := os.WriteFile(tokenFile, []byte("not valid json{"), 0o600); err != nil {
		t.Fatal(err)
	}

	ss := &SecureStorage{service: "test-service"}
	_, err := ss.Get("any-account")
	if err == nil {
		t.Error("expected error for corrupt JSON")
	}
}

// TestSecurityQuote tests the securityQuote helper function.
func TestSecurityQuote_Extra(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", `"simple"`},
		{"with\"quote", `"with\"quote"`},
		{"with\\backslash", `"with\\backslash"`},
		{"", `""`},
	}

	for _, tc := range tests {
		result := securityQuote(tc.input)
		if result != tc.expected {
			t.Errorf("securityQuote(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

// TestPowershellQuote tests the powershellQuote helper function.
func TestPowershellQuote_Extra(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with'quote", "with''quote"},
		{"multiple'quotes'here", "multiple''quotes''here"},
		{"", ""},
	}

	for _, tc := range tests {
		result := powershellQuote(tc.input)
		if result != tc.expected {
			t.Errorf("powershellQuote(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

// TestNewSecureStorage_Extra tests the constructor.
func TestNewSecureStorage_Extra(t *testing.T) {
	ss := NewSecureStorage("my-service")
	if ss.service != "my-service" {
		t.Errorf("expected service 'my-service', got %q", ss.service)
	}
}
