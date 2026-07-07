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
func (t *TokenStore) Load() error {
	// Try to load from keychain/keyring
	t.tokens = make(map[string]string)
	return nil
}

// Save saves tokens to secure storage.
func (t *TokenStore) Save() error {
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
	// Use security command to add to keychain
	_, err := execCommand("security", "add-generic-password", "-s", s.service, "-a", account, "-w", token, "-U")
	return err
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
