package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGeneratePKCE(t *testing.T) {
	pkce, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() error: %v", err)
	}
	if pkce.CodeVerifier == "" {
		t.Error("code verifier is empty")
	}
	if pkce.CodeChallenge == "" {
		t.Error("code challenge is empty")
	}
	if pkce.State == "" {
		t.Error("state is empty")
	}
	if pkce.CodeVerifier == pkce.CodeChallenge {
		t.Error("verifier and challenge should differ")
	}

	// Generate another to ensure randomness
	pkce2, _ := GeneratePKCE()
	if pkce.CodeVerifier == pkce2.CodeVerifier {
		t.Error("two PKCE generations should not produce same verifier")
	}
}

func TestBuildAuthURL(t *testing.T) {
	cfg := &OAuthConfig{
		ClientID:     "test-client",
		AuthorizeURL: "https://auth.example.com/authorize",
		Scopes:       []string{"read", "write"},
	}
	pkce := &PKCEParams{
		CodeVerifier:  "verifier123",
		CodeChallenge: "challenge456",
		State:         "state789",
	}

	url := cfg.BuildAuthURL(pkce, 8080)

	if !strings.Contains(url, "https://auth.example.com/authorize?") {
		t.Errorf("URL should start with authorize endpoint, got %s", url)
	}
	if !strings.Contains(url, "client_id=test-client") {
		t.Error("URL should contain client_id")
	}
	if !strings.Contains(url, "code_challenge=challenge456") {
		t.Error("URL should contain code_challenge")
	}
	if !strings.Contains(url, "code_challenge_method=S256") {
		t.Error("URL should contain S256 method")
	}
	if !strings.Contains(url, "state=state789") {
		t.Error("URL should contain state")
	}
	if !strings.Contains(url, "redirect_uri=http") {
		t.Error("URL should contain redirect_uri")
	}
	if !strings.Contains(url, "scope=read+write") {
		t.Error("URL should contain scopes")
	}
}

func TestExchangeCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("expected form content type, got %s", ct)
		}

		r.ParseForm()
		if r.FormValue("grant_type") != "authorization_code" {
			t.Errorf("expected grant_type=authorization_code")
		}
		if r.FormValue("code") != "test-code" {
			t.Errorf("expected code=test-code")
		}
		if r.FormValue("code_verifier") != "test-verifier" {
			t.Errorf("expected code_verifier=test-verifier")
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "access123",
			"refresh_token": "refresh456",
			"expires_in":    3600,
			"token_type":    "Bearer",
			"scope":         "read write",
		})
	}))
	defer server.Close()

	cfg := &OAuthConfig{
		ClientID: "test-client",
		TokenURL: server.URL + "/token",
	}

	tokens, err := cfg.ExchangeCode(context.Background(), "test-code", "test-verifier", 8080)
	if err != nil {
		t.Fatalf("ExchangeCode error: %v", err)
	}
	if tokens.AccessToken != "access123" {
		t.Errorf("expected access123, got %s", tokens.AccessToken)
	}
	if tokens.RefreshToken != "refresh456" {
		t.Errorf("expected refresh456, got %s", tokens.RefreshToken)
	}
	if len(tokens.Scopes) != 2 {
		t.Errorf("expected 2 scopes, got %d", len(tokens.Scopes))
	}
}

func TestRefreshAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.FormValue("grant_type") != "refresh_token" {
			t.Errorf("expected grant_type=refresh_token")
		}
		if r.FormValue("refresh_token") != "old-refresh" {
			t.Errorf("expected old refresh token")
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"expires_in":    7200,
			"token_type":    "Bearer",
		})
	}))
	defer server.Close()

	cfg := &OAuthConfig{
		ClientID: "test-client",
		TokenURL: server.URL + "/token",
	}

	tokens, err := cfg.RefreshAccessToken(context.Background(), "old-refresh")
	if err != nil {
		t.Fatalf("RefreshAccessToken error: %v", err)
	}
	if tokens.AccessToken != "new-access" {
		t.Errorf("expected new-access, got %s", tokens.AccessToken)
	}
	if tokens.RefreshToken != "new-refresh" {
		t.Errorf("expected new-refresh, got %s", tokens.RefreshToken)
	}
}

func TestIsExpired(t *testing.T) {
	tests := []struct {
		name    string
		tokens  *OAuthTokens
		expired bool
	}{
		{"nil tokens", nil, true},
		{"empty token", &OAuthTokens{}, true},
		{"expired", &OAuthTokens{AccessToken: "x", ExpiresAt: time.Now().Add(-1 * time.Hour)}, true},
		{"about to expire (within buffer)", &OAuthTokens{AccessToken: "x", ExpiresAt: time.Now().Add(2 * time.Minute)}, true},
		{"valid", &OAuthTokens{AccessToken: "x", ExpiresAt: time.Now().Add(1 * time.Hour)}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsExpired(tt.tokens); got != tt.expired {
				t.Errorf("IsExpired() = %v, want %v", got, tt.expired)
			}
		})
	}
}

func TestCallbackServer(t *testing.T) {
	state := "test-state-123"
	srv := NewCallbackServer(state)
	port, err := srv.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer srv.Close()

	if port == 0 {
		t.Error("port should be non-zero")
	}

	// Simulate OAuth redirect
	go func() {
		resp, err := http.Get(
			"http://localhost:" + itoa(port) + "/callback?code=auth-code-123&state=" + state,
		)
		if err != nil {
			t.Errorf("callback request failed: %v", err)
			return
		}
		resp.Body.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	code, err := srv.WaitForCode(ctx)
	if err != nil {
		t.Fatalf("WaitForCode() error: %v", err)
	}
	if code != "auth-code-123" {
		t.Errorf("expected auth-code-123, got %s", code)
	}
}

func TestCallbackServer_StateMismatch(t *testing.T) {
	srv := NewCallbackServer("correct-state")
	port, _ := srv.Start()
	defer srv.Close()

	go func() {
		http.Get("http://localhost:" + itoa(port) + "/callback?code=x&state=wrong-state")
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := srv.WaitForCode(ctx)
	if err == nil {
		t.Error("expected error for state mismatch")
	}
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
