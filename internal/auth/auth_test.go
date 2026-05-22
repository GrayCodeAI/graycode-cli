package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTokenStore(t *testing.T) {
	t.Parallel()

	t.Run("set and get", func(t *testing.T) {
		t.Parallel()
		store := NewTokenStore()
		store.Set("anthropic", "sk-test-123")

		if got := store.Get("anthropic"); got != "sk-test-123" {
			t.Errorf("Get() = %q, want %q", got, "sk-test-123")
		}
	})

	t.Run("has existing", func(t *testing.T) {
		t.Parallel()
		store := NewTokenStore()
		store.Set("openai", "sk-abc")

		if !store.Has("openai") {
			t.Error("Has() = false, want true")
		}
	})

	t.Run("has missing", func(t *testing.T) {
		t.Parallel()
		store := NewTokenStore()

		if store.Has("nonexistent") {
			t.Error("Has() = true, want false for missing provider")
		}
	})

	t.Run("get missing returns empty", func(t *testing.T) {
		t.Parallel()
		store := NewTokenStore()

		if got := store.Get("missing"); got != "" {
			t.Errorf("Get() = %q, want empty string", got)
		}
	})

	t.Run("overwrite existing", func(t *testing.T) {
		t.Parallel()
		store := NewTokenStore()
		store.Set("anthropic", "old-token")
		store.Set("anthropic", "new-token")

		if got := store.Get("anthropic"); got != "new-token" {
			t.Errorf("Get() = %q, want %q after overwrite", got, "new-token")
		}
	})

	t.Run("multiple providers", func(t *testing.T) {
		t.Parallel()
		store := NewTokenStore()
		store.Set("anthropic", "sk-ant")
		store.Set("openai", "sk-oai")
		store.Set("gemini", "key-gem")

		tests := []struct {
			provider string
			want     string
		}{
			{"anthropic", "sk-ant"},
			{"openai", "sk-oai"},
			{"gemini", "key-gem"},
		}
		for _, tt := range tests {
			if got := store.Get(tt.provider); got != tt.want {
				t.Errorf("Get(%q) = %q, want %q", tt.provider, got, tt.want)
			}
		}
	})

	t.Run("load and save are no-ops", func(t *testing.T) {
		t.Parallel()
		store := NewTokenStore()
		if err := store.Load(); err != nil {
			t.Errorf("Load() error = %v", err)
		}
		if err := store.Save(); err != nil {
			t.Errorf("Save() error = %v", err)
		}
	})
}

func TestGenerateNonce(t *testing.T) {
	t.Parallel()

	t.Run("uniqueness", func(t *testing.T) {
		t.Parallel()
		seen := make(map[string]bool)
		for i := 0; i < 100; i++ {
			nonce := GenerateNonce()
			if seen[nonce] {
				t.Fatalf("duplicate nonce generated on iteration %d", i)
			}
			seen[nonce] = true
		}
	})

	t.Run("non-empty", func(t *testing.T) {
		t.Parallel()
		nonce := GenerateNonce()
		if len(nonce) == 0 {
			t.Fatal("nonce should not be empty")
		}
	})

	t.Run("sufficient length", func(t *testing.T) {
		t.Parallel()
		nonce := GenerateNonce()
		if len(nonce) < 16 {
			t.Errorf("nonce length = %d, want >= 16", len(nonce))
		}
	})
}

func TestSecureStorage(t *testing.T) {
	t.Run("new secure storage", func(t *testing.T) {
		t.Parallel()
		ss := NewSecureStorage("hawk-test")
		if ss == nil {
			t.Fatal("NewSecureStorage returned nil")
		}
		if ss.service != "hawk-test" {
			t.Errorf("service = %q, want %q", ss.service, "hawk-test")
		}
	})

	t.Run("file-based get missing", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("HOME", dir)

		ss := NewSecureStorage("hawk-test")
		_, err := ss.getFile("nonexistent")
		if err == nil {
			t.Error("getFile() should return error for missing file")
		}
	})

	t.Run("file-based set and get", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("HOME", dir)

		hawkDir := filepath.Join(dir, ".hawk")
		if err := os.MkdirAll(hawkDir, 0o755); err != nil {
			t.Fatal(err)
		}

		ss := NewSecureStorage("hawk-test")
		if err := ss.setFile("anthropic", "sk-test-token"); err != nil {
			t.Fatalf("setFile() error = %v", err)
		}

		got, err := ss.getFile("anthropic")
		if err != nil {
			t.Fatalf("getFile() error = %v", err)
		}
		if got != "sk-test-token" {
			t.Errorf("getFile() = %q, want %q", got, "sk-test-token")
		}
	})

	t.Run("file-based overwrite", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("HOME", dir)

		hawkDir := filepath.Join(dir, ".hawk")
		if err := os.MkdirAll(hawkDir, 0o755); err != nil {
			t.Fatal(err)
		}

		ss := NewSecureStorage("hawk-test")
		if err := ss.setFile("provider", "old-token"); err != nil {
			t.Fatal(err)
		}
		if err := ss.setFile("provider", "new-token"); err != nil {
			t.Fatal(err)
		}

		got, err := ss.getFile("provider")
		if err != nil {
			t.Fatalf("getFile() error = %v", err)
		}
		if got != "new-token" {
			t.Errorf("getFile() = %q, want %q", got, "new-token")
		}
	})

	t.Run("file permissions are restrictive", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("HOME", dir)

		hawkDir := filepath.Join(dir, ".hawk")
		if err := os.MkdirAll(hawkDir, 0o755); err != nil {
			t.Fatal(err)
		}

		ss := NewSecureStorage("hawk-test")
		if err := ss.setFile("test", "secret"); err != nil {
			t.Fatal(err)
		}

		path := filepath.Join(hawkDir, ".tokens")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		perm := info.Mode().Perm()
		if perm != 0o600 {
			t.Errorf("file permissions = %o, want 0600", perm)
		}
	})

	t.Run("file stores valid JSON", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("HOME", dir)

		hawkDir := filepath.Join(dir, ".hawk")
		if err := os.MkdirAll(hawkDir, 0o755); err != nil {
			t.Fatal(err)
		}

		ss := NewSecureStorage("hawk-test")
		if err := ss.setFile("provider1", "token1"); err != nil {
			t.Fatal(err)
		}
		if err := ss.setFile("provider2", "token2"); err != nil {
			t.Fatal(err)
		}

		data, err := os.ReadFile(filepath.Join(hawkDir, ".tokens"))
		if err != nil {
			t.Fatal(err)
		}

		var tokens map[string]string
		if err := json.Unmarshal(data, &tokens); err != nil {
			t.Fatalf("stored file is not valid JSON: %v", err)
		}
		if tokens["provider1"] != "token1" || tokens["provider2"] != "token2" {
			t.Errorf("tokens = %v, want both providers", tokens)
		}
	})
}

func TestExecCommand(t *testing.T) {
	t.Parallel()
	out, err := execCommand("echo", "test")
	if err != nil {
		t.Fatalf("execCommand(\"echo\", \"test\") error = %v", err)
	}
	if out != "test" {
		t.Errorf("execCommand(\"echo\", \"test\") = %q, want %q", out, "test")
	}
}

func TestExecCommand_NotFound(t *testing.T) {
	t.Parallel()
	_, err := execCommand("nonexistent-command-xyz")
	if err == nil {
		t.Error("execCommand() should return error for missing command")
	}
}
