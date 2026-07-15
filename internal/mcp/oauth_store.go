package mcp

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/GrayCodeAI/hawk/internal/auth"
)

// oauthTokenService is the keychain/keyring service name tokens are stored
// under, keyed per-server by the account parameter.
// Keychain service name — a fixed OAuth storage label, not a secret value.
const oauthTokenService = "hawk-mcp-oauth" // #nosec G101 -- fixed storage label, not a credential

// StoredToken is what gets persisted per MCP server. auth.SecureStorage
// only stores a single string value per account, so this is JSON-marshaled
// into that string — not auth.TokenStore, which (as of this writing) has
// no-op Load/Save and doesn't actually persist anything.
type StoredToken struct {
	AccessToken   string    `json:"access_token"`
	RefreshToken  string    `json:"refresh_token,omitempty"`
	ExpiresAt     time.Time `json:"expires_at,omitempty"`
	ClientID      string    `json:"client_id"`
	TokenEndpoint string    `json:"token_endpoint"`
}

// NeedsRefresh reports whether the access token is expired or close enough
// to expiring (30s) that it should be refreshed before use. A token with a
// zero ExpiresAt (the server didn't return expires_in) is treated as never
// needing a refresh purely on expiry grounds.
func (t *StoredToken) NeedsRefresh() bool {
	if t.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(t.ExpiresAt.Add(-30 * time.Second))
}

// tokenBackend is the minimal surface SaveToken/LoadToken need from a
// credential store — satisfied by *auth.SecureStorage. Kept as an
// interface (rather than calling auth.NewSecureStorage directly) so tests
// can swap in a fake and never touch the real OS keychain.
type tokenBackend interface {
	Get(account string) (string, error)
	Set(account, value string) error
}

var newTokenStorage = func() tokenBackend { return auth.NewSecureStorage(oauthTokenService) }

// SetTokenBackendForTesting overrides the credential store SaveToken/
// LoadToken use, for tests in other packages that exercise code paths
// eventually calling SaveToken/LoadToken (e.g. internal/tool's OAuth flow
// orchestration) and must never touch the real OS keychain. Returns a
// restore function; production code must never call this.
func SetTokenBackendForTesting(backend interface {
	Get(account string) (string, error)
	Set(account, value string) error
},
) func() {
	orig := newTokenStorage
	newTokenStorage = func() tokenBackend { return backend }
	return func() { newTokenStorage = orig }
}

// SaveToken persists tok for serverName.
func SaveToken(serverName string, tok *StoredToken) error {
	data, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	return newTokenStorage().Set(serverName, string(data))
}

// LoadToken returns the stored token for serverName, if any.
func LoadToken(serverName string) (*StoredToken, error) {
	raw, err := newTokenStorage().Get(serverName)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, fmt.Errorf("no token stored for %q", serverName)
	}
	var tok StoredToken
	if err := json.Unmarshal([]byte(raw), &tok); err != nil {
		return nil, fmt.Errorf("stored token for %q is corrupt: %w", serverName, err)
	}
	return &tok, nil
}
