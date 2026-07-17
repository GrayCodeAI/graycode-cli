package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

// TokenStore manages authentication tokens.
type TokenStore struct {
	tokens map[string]string // provider -> token
}

// NewTokenStore creates a new token store.
func NewTokenStore() *TokenStore {
	return &TokenStore{tokens: make(map[string]string)}
}

// Load loads tokens from secure storage.
// Deprecated: Use SecureStorage directly to load tokens. This stub always
// returns an empty token map. Migrate callers to SecureStorage.Get/Set.
func (t *TokenStore) Load() error {
	// Stub: no-op. Existing callers that relied on this get an empty token
	// map. New code should use SecureStorage directly.
	t.tokens = make(map[string]string)
	return nil
}

// Save saves tokens to secure storage.
// Deprecated: Use SecureStorage directly to persist tokens. This stub is a
// no-op. Migrate callers to SecureStorage.Set.
func (t *TokenStore) Save() error {
	// Stub: no-op. Tokens held in-memory only; they are lost on process exit.
	// Use SecureStorage for persistent, OS-keychain-backed storage.
	return nil
}

// Get returns a token for a provider.
func (t *TokenStore) Get(provider string) string {
	return t.tokens[provider]
}

// Set sets a token for a provider.
func (t *TokenStore) Set(provider, token string) {
	t.tokens[provider] = token
}

// Has returns true if a token exists for a provider.
func (t *TokenStore) Has(provider string) bool {
	_, ok := t.tokens[provider]
	return ok
}

// SecureStorage handles secure token storage using OS keychain/keyring.
type SecureStorage struct {
	service string
}

// NewSecureStorage creates a new secure storage.
func NewSecureStorage(service string) *SecureStorage {
	return &SecureStorage{service: service}
}

// Get retrieves a token from secure storage.
func (s *SecureStorage) Get(account string) (string, error) {
	if runtime.GOOS == "darwin" {
		return s.getMacOS(account)
	}
	// Fallback to file-based storage
	return s.getFile(account)
}

// Set stores a token in secure storage.
func (s *SecureStorage) Set(account, token string) error {
	if runtime.GOOS == "darwin" {
		return s.setMacOS(account, token)
	}
	return s.setFile(account, token)
}

func (s *SecureStorage) getMacOS(account string) (string, error) {
	// Use security command to get from keychain
	data, err := execCommand("security", "find-generic-password", "-s", s.service, "-a", account, "-w")
	if err != nil {
		return "", err
	}
	return data, nil
}

func (s *SecureStorage) setMacOS(account, token string) error {
	// Feed the command to `security -i` via stdin so the secret never appears
	// in the process argument list (argv is visible to all local users via ps
	// for the lifetime of the call).
	if strings.ContainsAny(s.service+account+token, "\n\r") {
		return fmt.Errorf("keychain values must not contain newlines")
	}
	cmd := exec.CommandContext(context.Background(), "security", "-i")
	cmd.Stdin = strings.NewReader("add-generic-password -U -s " + securityQuote(s.service) +
		" -a " + securityQuote(account) + " -w " + securityQuote(token) + "\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("security add-generic-password: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// securityQuote quotes an argument for the `security -i` interactive command
// parser: wraps it in double quotes and escapes backslashes and double quotes.
func securityQuote(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(v) + `"`
}

func (s *SecureStorage) getFile(account string) (string, error) {
	path := filepath.Join(storage.ConfigDir(), ".tokens")
	data, err := os.ReadFile(path) // #nosec G304 -- path is the fixed internal token store location, not external input
	if err != nil {
		return "", err
	}
	var tokens map[string]string
	if err := json.Unmarshal(data, &tokens); err != nil {
		return "", err
	}
	return tokens[account], nil
}

func (s *SecureStorage) setFile(account, token string) error {
	path := filepath.Join(storage.ConfigDir(), ".tokens")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	var tokens map[string]string
	// #nosec G304 -- path is the fixed internal token store location, not external input
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &tokens)
	}
	if tokens == nil {
		tokens = make(map[string]string)
	}
	tokens[account] = token
	data, _ := json.Marshal(tokens)
	return os.WriteFile(path, data, 0o600)
}

// GenerateNonce generates a random nonce for OAuth.
func GenerateNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func execCommand(name string, args ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), name, args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return strings.TrimSpace(string(out)), nil
}
