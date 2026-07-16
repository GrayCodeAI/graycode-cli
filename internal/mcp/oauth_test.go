package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestDiscoverOAuthMetadata_ProtectedResourceThenAuthServer(t *testing.T) {
	var authServer *httptest.Server
	authServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/oauth-authorization-server" {
			_ = json.NewEncoder(w).Encode(AuthServerMetadata{
				Issuer:                authServer.URL,
				AuthorizationEndpoint: authServer.URL + "/authorize",
				TokenEndpoint:         authServer.URL + "/token",
				RegistrationEndpoint:  authServer.URL + "/register",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer authServer.Close()

	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/oauth-protected-resource" {
			_ = json.NewEncoder(w).Encode(ProtectedResourceMetadata{
				AuthorizationServers: []string{authServer.URL},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mcpServer.Close()

	meta, err := DiscoverOAuthMetadata(context.Background(), mcpServer.URL+"/mcp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.AuthorizationEndpoint != authServer.URL+"/authorize" {
		t.Errorf("authorization_endpoint = %q, want the discovered auth server's", meta.AuthorizationEndpoint)
	}
	if meta.RegistrationEndpoint != authServer.URL+"/register" {
		t.Errorf("registration_endpoint = %q, want the discovered auth server's", meta.RegistrationEndpoint)
	}
}

func TestDiscoverOAuthMetadata_FallsBackToSameOriginAuthServer(t *testing.T) {
	// No /.well-known/oauth-protected-resource at all; the MCP server IS
	// the authorization server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource":
			w.WriteHeader(http.StatusNotFound)
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(AuthServerMetadata{
				AuthorizationEndpoint: "/authorize-here",
				TokenEndpoint:         "/token-here",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	meta, err := DiscoverOAuthMetadata(context.Background(), server.URL+"/mcp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.AuthorizationEndpoint != "/authorize-here" {
		t.Errorf("authorization_endpoint = %q", meta.AuthorizationEndpoint)
	}
}

func TestDiscoverOAuthMetadata_FallsBackToGuessedEndpoints(t *testing.T) {
	// Neither well-known document exists at all.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	meta, err := DiscoverOAuthMetadata(context.Background(), server.URL+"/mcp")
	if err != nil {
		t.Fatalf("unexpected error (should degrade gracefully): %v", err)
	}
	if meta.AuthorizationEndpoint != server.URL+"/oauth/authorize" {
		t.Errorf("authorization_endpoint = %q, want guessed default", meta.AuthorizationEndpoint)
	}
	if meta.TokenEndpoint != server.URL+"/oauth/token" {
		t.Errorf("token_endpoint = %q, want guessed default", meta.TokenEndpoint)
	}
}

func TestRegisterClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var req dcrRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.RedirectURIs) != 1 || req.RedirectURIs[0] != "http://127.0.0.1:9999/callback" {
			t.Errorf("unexpected redirect_uris: %+v", req.RedirectURIs)
		}
		if req.TokenEndpointAuthMethod != "none" {
			t.Errorf("expected public client (token_endpoint_auth_method=none), got %q", req.TokenEndpointAuthMethod)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(dcrResponse{ClientID: "generated-client-id"})
	}))
	defer server.Close()

	clientID, err := RegisterClient(context.Background(), server.URL+"/register", "http://127.0.0.1:9999/callback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clientID != "generated-client-id" {
		t.Errorf("clientID = %q", clientID)
	}
}

func TestGeneratePKCE_ChallengeMatchesVerifier(t *testing.T) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verifier == "" || challenge == "" {
		t.Fatal("expected non-empty verifier and challenge")
	}
	sum := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if challenge != want {
		t.Errorf("challenge = %q, want S256(verifier) = %q", challenge, want)
	}

	verifier2, _, _ := GeneratePKCE()
	if verifier == verifier2 {
		t.Error("expected distinct verifiers across calls")
	}
}

func TestGenerateState_NonEmptyAndDistinct(t *testing.T) {
	s1, err := GenerateState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s2, _ := GenerateState()
	if s1 == "" || s2 == "" {
		t.Fatal("expected non-empty state values")
	}
	if s1 == s2 {
		t.Error("expected distinct state values across calls")
	}
}

func TestBuildAuthorizationURL(t *testing.T) {
	meta := &AuthServerMetadata{AuthorizationEndpoint: "https://auth.example.com/authorize"}
	got := BuildAuthorizationURL(meta, "client-1", "http://127.0.0.1:9999/callback", "state-1", "challenge-1", "https://mcp.example.com")

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("BuildAuthorizationURL produced an unparseable URL: %v", err)
	}
	q := parsed.Query()
	checks := map[string]string{
		"response_type":         "code",
		"client_id":             "client-1",
		"redirect_uri":          "http://127.0.0.1:9999/callback",
		"state":                 "state-1",
		"code_challenge":        "challenge-1",
		"code_challenge_method": "S256",
		"resource":              "https://mcp.example.com",
	}
	for k, want := range checks {
		if got := q.Get(k); got != want {
			t.Errorf("query param %q = %q, want %q", k, got, want)
		}
	}
}

func TestStartLoopbackCallback_ReceivesCode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	redirectURI, results, shutdown, err := StartLoopbackCallback(ctx, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer shutdown()

	// Simulate the browser being redirected here after the user authorizes.
	go func() {
		client := &http.Client{Timeout: 2 * time.Second}
		_, _ = client.Get(redirectURI + "?code=the-code&state=the-state")
	}()

	select {
	case result := <-results:
		if result.Err != nil {
			t.Fatalf("unexpected callback error: %v", result.Err)
		}
		if result.Code != "the-code" || result.State != "the-state" {
			t.Errorf("got code=%q state=%q", result.Code, result.State)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for callback result")
	}
}

func TestStartLoopbackCallback_TimesOut(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, results, shutdown, err := StartLoopbackCallback(ctx, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer shutdown()

	select {
	case result := <-results:
		if result.Err == nil {
			t.Fatal("expected a timeout error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected the timeout to fire the result channel")
	}
}

func TestStartLoopbackCallback_ErrorParam(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	redirectURI, results, shutdown, err := StartLoopbackCallback(ctx, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer shutdown()

	go func() {
		client := &http.Client{Timeout: 2 * time.Second}
		_, _ = client.Get(redirectURI + "?error=access_denied&state=the-state")
	}()

	select {
	case result := <-results:
		if result.Err == nil {
			t.Fatal("expected an error for an error= callback")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for callback result")
	}
}

func TestExchangeCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.FormValue("grant_type") != "authorization_code" {
			t.Errorf("grant_type = %q", r.FormValue("grant_type"))
		}
		if r.FormValue("code") != "auth-code" || r.FormValue("code_verifier") != "verifier-1" {
			t.Errorf("unexpected code/verifier: %q/%q", r.FormValue("code"), r.FormValue("code_verifier"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-1",
			"refresh_token": "refresh-1",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	meta := &AuthServerMetadata{TokenEndpoint: server.URL}
	result, err := ExchangeCode(context.Background(), meta, "client-1", "auth-code", "verifier-1", "http://127.0.0.1:9999/callback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AccessToken != "access-1" || result.RefreshToken != "refresh-1" {
		t.Errorf("unexpected tokens: %+v", result)
	}
	if result.ExpiresAt.Before(time.Now().Add(59 * time.Minute)) {
		t.Errorf("expected ExpiresAt ~1h out, got %v", result.ExpiresAt)
	}
}

func TestExchangeCode_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":             "invalid_grant",
			"error_description": "code expired",
		})
	}))
	defer server.Close()

	meta := &AuthServerMetadata{TokenEndpoint: server.URL}
	_, err := ExchangeCode(context.Background(), meta, "client-1", "auth-code", "verifier-1", "http://127.0.0.1:9999/callback")
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestRefreshOAuthToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.FormValue("grant_type") != "refresh_token" || r.FormValue("refresh_token") != "old-refresh" {
			t.Errorf("unexpected form: %v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "new-access",
			"expires_in":   1800,
		})
	}))
	defer server.Close()

	meta := &AuthServerMetadata{TokenEndpoint: server.URL}
	result, err := RefreshOAuthToken(context.Background(), meta, "client-1", "old-refresh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AccessToken != "new-access" {
		t.Errorf("AccessToken = %q", result.AccessToken)
	}
}

func TestStoredToken_NeedsRefresh(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{name: "zero value never needs refresh", expiresAt: time.Time{}, want: false},
		{name: "far in the future", expiresAt: time.Now().Add(time.Hour), want: false},
		{name: "already expired", expiresAt: time.Now().Add(-time.Minute), want: true},
		{name: "within the 30s buffer", expiresAt: time.Now().Add(10 * time.Second), want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tok := &StoredToken{ExpiresAt: tc.expiresAt}
			if got := tok.NeedsRefresh(); got != tc.want {
				t.Errorf("NeedsRefresh() = %v, want %v", got, tc.want)
			}
		})
	}
}

// fakeTokenBackend is an in-memory tokenBackend for tests — SaveToken/
// LoadToken must never touch the real OS keychain during `go test`.
type fakeTokenBackend struct {
	values map[string]string
}

func (f *fakeTokenBackend) Get(account string) (string, error) {
	return f.values[account], nil
}

func (f *fakeTokenBackend) Set(account, value string) error {
	f.values[account] = value
	return nil
}

func TestSaveAndLoadToken_RoundTrip(t *testing.T) {
	fake := &fakeTokenBackend{values: make(map[string]string)}
	orig := newTokenStorage
	newTokenStorage = func() tokenBackend { return fake }
	t.Cleanup(func() { newTokenStorage = orig })

	tok := &StoredToken{
		AccessToken:   "access-1",
		RefreshToken:  "refresh-1",
		ExpiresAt:     time.Now().Add(time.Hour).Truncate(time.Second),
		ClientID:      "client-1",
		TokenEndpoint: "https://auth.example.com/token",
	}
	if err := SaveToken("my-server", tok); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	loaded, err := LoadToken("my-server")
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if loaded.AccessToken != tok.AccessToken || loaded.RefreshToken != tok.RefreshToken ||
		loaded.ClientID != tok.ClientID || !loaded.ExpiresAt.Equal(tok.ExpiresAt) {
		t.Errorf("round-tripped token mismatch: got %+v, want %+v", loaded, tok)
	}
}

func TestLoadToken_NotFound(t *testing.T) {
	fake := &fakeTokenBackend{values: make(map[string]string)}
	orig := newTokenStorage
	newTokenStorage = func() tokenBackend { return fake }
	t.Cleanup(func() { newTokenStorage = orig })

	_, err := LoadToken("never-saved")
	if err == nil {
		t.Fatal("expected an error for a server with no stored token")
	}
}
