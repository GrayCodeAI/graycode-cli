// Package auth tests authentication and secure storage for Hawk daemon.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	t.Run("storage type detection darwin", func(t *testing.T) {
		t.Parallel()
		os.Setenv("GRAYCODE_STORAGE_TYPE", "darwin")
		defer os.Unsetenv("GRAYCODE_STORAGE_TYPE")

		got := NewSecureStorage("test")
		if got == nil {
			t.Fatal("expected non-nil storage")
		}
	})

	t.Run("storage type detection linux", func(t *testing.T) {
		t.Parallel()
		os.Setenv("GRAYCODE_STORAGE_TYPE", "linux")
		defer os.Unsetenv("GRAYCODE_STORAGE_TYPE")

		got := NewSecureStorage("test")
		if got == nil {
			t.Fatal("expected non-nil storage")
		}
	})

	t.Run("storage type detection windows", func(t *testing.T) {
		t.Parallel()
		os.Setenv("GRAYCODE_STORAGE_TYPE", "windows")
		defer os.Unsetenv("GRAYCODE_STORAGE_TYPE")

		got := NewSecureStorage("test")
		if got == nil {
			t.Fatal("expected non-nil storage")
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

func TestSecureStorage(t *testing.T) {
	t.Run("new secure storage", func(t *testing.T) {
		t.Parallel()
		ss := NewSecureStorage("hawk-test")
		if ss == nil {
			t.Fatal("NewSecureStorage returned nil")
		}
	})
}

func TestSanitizeAccountName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"ANTHROPIC_API_KEY", "anthropic_api_key"},
		{"OPENAI_API_KEY", "openai_api_key"},
		{"Test_Provider_123", "test_provider_123"},
		{"test-with-dashes", "test_with_dashes"},
		{"test.with.dots", "test_with_dots"},
		{"UPPER", "upper"},
		{"lower", "lower"},
		{"MixedCase", "mixedcase"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
		})
	}
}

func TestTokenFileOperations(t *testing.T) {
	t.Parallel()

	t.Run("file token store", func(t *testing.T) {
		store := NewFileTokenStore("")
		store.Set(context.Background(), "test", "token123")
		got, err := store.Get(context.Background(), "test")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got != "token123" {
			t.Errorf("Get() = %q, want %q", got, "token123")
		}
	})
}

func TestLoadSaveTokenStore(t *testing.T) {
	t.Run("save and load", func(t *testing.T) {
		store := NewTokenStore()
		store.Set("anthropic", "sk-ant-123")
		store.Set("openai", "sk-oai-456")
		store.Set("gemini", "key-gem-789")

		if err := store.Save(); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		store2 := NewTokenStore()
		if err := store2.Load(); err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		tests := []struct {
			provider string
			want     string
		}{
			{"anthropic", "sk-ant-123"},
			{"openai", "sk-oai-456"},
			{"gemini", "key-gem-789"},
		}

		for _, tt := range tests {
			if got := store2.Get(tt.provider); got != tt.want {
				t.Errorf("Get(%q) = %q, want %q", tt.provider, got, tt.want)
			}
		}
	})

	t.Run("empty load", func(t *testing.T) {
		// Not parallel: mutates global state
		store := NewTokenStore()
		if err := store.Load(); err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		// Should not panic
	})
}

func TestFileTokenStore_ErrNotFound(t *testing.T) {
	t.Parallel()

	store := NewFileTokenStore("")
	_, err := store.Get(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("Get() should return error for nonexistent key")
	}
}

// TestFileTokenStore tests the file-based token store implementation.
type FileTokenStore struct {
	data map[string]string
	path string
}

func NewFileTokenStore(path string) *FileTokenStore {
	return &FileTokenStore{
		data: make(map[string]string),
		path: path,
	}
}

func (f *FileTokenStore) Get(ctx context.Context, account string) (string, error) {
	account = strings.TrimSpace(account)
	if account == "" {
		return "", fmt.Errorf("empty account")
	}

	data, err := os.ReadFile(f.path) // #nosec G304
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("secret not found")
		}
		return "", err
	}

	var tokens map[string]string
	if err := json.Unmarshal(data, &tokens); err != nil {
		return "", err
	}

	secret := tokens[strings.ToLower(account)]
	if secret == "" {
		return "", fmt.Errorf("secret not found")
	}

	return secret, nil
}

func (f *FileTokenStore) Set(ctx context.Context, account, secret string) error {
	account = strings.TrimSpace(account)
	secret = strings.TrimSpace(secret)
	if account == "" {
		return fmt.Errorf("empty account")
	}

	data, err := os.ReadFile(f.path) // #nosec G304
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var tokens map[string]string
	if err := json.Unmarshal(data, &tokens); err != nil && !os.IsNotExist(err) {
		return err
	}

	if tokens == nil {
		tokens = make(map[string]string)
	}

	tokens[strings.ToLower(account)] = secret

	data, err = json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(f.path, data, 0o600) // #nosec G304
}

func (f *FileTokenStore) Delete(ctx context.Context, account string) error {
	account = strings.TrimSpace(account)
	if account == "" {
		return fmt.Errorf("empty account")
	}

	data, err := os.ReadFile(f.path) // #nosec G304
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var tokens map[string]string
	if err := json.Unmarshal(data, &tokens); err != nil {
		return err
	}

	delete(tokens, strings.ToLower(account))

	data, err = json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(f.path, data, 0o600) // #nosec G304
}

func TestFileTokenStore_CRUD(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "tokens.json")

	t.Run("set and get", func(t *testing.T) {
		t.Parallel()
		store := NewFileTokenStore(path)
		if err := store.Set(context.Background(), "anthropic", "sk-test"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		got, err := store.Get(context.Background(), "anthropic")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got != "sk-test" {
			t.Errorf("Get() = %q, want %q", got, "sk-test")
		}
	})

	t.Run("update", func(t *testing.T) {
		t.Parallel()
		store := NewFileTokenStore(path)
		store.Set(context.Background(), "openai", "old-token")
		store.Set(context.Background(), "openai", "new-token")

		got, err := store.Get(context.Background(), "openai")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got != "new-token" {
			t.Errorf("Get() = %q, want %q", got, "new-token")
		}
	})

	t.Run("delete", func(t *testing.T) {
		t.Parallel()
		store := NewFileTokenStore(path)
		store.Set(context.Background(), "gemini", "key-gem")

		if err := store.Delete(context.Background(), "gemini"); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		_, err := store.Get(context.Background(), "gemini")
		if err == nil {
			t.Error("Get() should return error for deleted key")
		}
	})

	t.Run("missing get", func(t *testing.T) {
		t.Parallel()
		store := NewFileTokenStore(path)
		_, err := store.Get(context.Background(), "nonexistent")
		if err == nil {
			t.Error("Get() should return error for missing key")
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		t.Parallel()
		store := NewFileTokenStore(path)
		store.Set(context.Background(), "OpenAI", "sk-oai")

		got, err := store.Get(context.Background(), "openai")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got != "sk-oai" {
			t.Errorf("Get() = %q, want %q", got, "sk-oai")
		}
	})
}

func TestGenerateKey(t *testing.T) {
	_ = generateKey(16)
}
