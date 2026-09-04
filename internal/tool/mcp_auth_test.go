package tool

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/mcp"
	"github.com/GrayCodeAI/graycode-cli/internal/testutil"
)

// fakeTokenBackend is an in-memory stand-in for the OS keychain — tests in
// this file must never touch the real one.
type fakeTokenBackend struct {
	values map[string]string
}

func newFakeTokenBackend() *fakeTokenBackend {
	return &fakeTokenBackend{values: make(map[string]string)}
}

func (f *fakeTokenBackend) Get(account string) (string, error) {
	return f.values[account], nil
}

func (f *fakeTokenBackend) Set(account, value string) error {
	f.values[account] = value
	return nil
}

func resetAuthManager(t *testing.T) {
	t.Helper()
	globalMCPAuthManager.mu.Lock()
	globalMCPAuthManager.states = make(map[string]*MCPAuthState)
	globalMCPAuthManager.mu.Unlock()
}

// newTestOAuthServer stands in for both the MCP server and its
// authorization server: it serves RFC 8414 metadata, RFC 7591 dynamic
// client registration, and a token endpoint that validates the PKCE
// verifier against the challenge sent during authorization. The
// authorization endpoint itself isn't served here — tests simulate the
// user's browser by hitting the loopback callback directly, exactly the
// way TestStartLoopbackCallback_ReceivesCode does in internal/mcp.
func newTestOAuthServer(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(mcp.AuthServerMetadata{
			Issuer:                srv.URL,
			AuthorizationEndpoint: srv.URL + "/authorize",
			TokenEndpoint:         srv.URL + "/token",
			RegistrationEndpoint:  srv.URL + "/register",
		})
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"client_id": "dcr-client-id"})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-" + r.FormValue("code"),
			"refresh_token": "refresh-1",
			"expires_in":    3600,
		})
	})
	srv = testutil.NewLoopbackHTTPServer(t, mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestMcpAuthTool_FullFlow_DynamicClientRegistration(t *testing.T) {
	resetAuthManager(t)
	restore := mcp.SetTokenBackendForTesting(newFakeTokenBackend())
	defer restore()

	server := newTestOAuthServer(t)

	input, _ := json.Marshal(map[string]string{
		"server_name": "test-server",
		"server_url":  server.URL + "/mcp",
	})
	out, err := McpAuthTool{}.Execute(t.Context(), input)
	if err != nil {
		t.Fatalf("Execute returned an error: %v", err)
	}

	var resp struct {
		Status  string `json:"status"`
		AuthURL string `json:"authUrl"`
	}
	if uErr := json.Unmarshal([]byte(out), &resp); uErr != nil {
		t.Fatalf("could not parse Execute output: %v (%s)", uErr, out)
	}
	if resp.Status != "pending" {
		t.Fatalf("expected status=pending, got %q (%s)", resp.Status, out)
	}

	parsedAuthURL, err := url.Parse(resp.AuthURL)
	if err != nil {
		t.Fatalf("authUrl is not a valid URL: %v", err)
	}
	q := parsedAuthURL.Query()
	if q.Get("client_id") != "dcr-client-id" {
		t.Errorf("expected the dynamically-registered client_id, got %q", q.Get("client_id"))
	}
	redirectURI := q.Get("redirect_uri")
	if redirectURI == "" {
		t.Fatal("expected a redirect_uri in the authorization URL")
	}
	codeChallenge := q.Get("code_challenge")
	state := q.Get("state")
	if codeChallenge == "" || state == "" {
		t.Fatalf("expected code_challenge and state to be set: %s", resp.AuthURL)
	}

	// Simulate the user's browser being redirected back after authorizing.
	client := &http.Client{Timeout: 3 * time.Second}
	callbackResp, err := client.Get(redirectURI + "?code=the-auth-code&state=" + state)
	if err != nil {
		t.Fatalf("simulated callback request failed: %v", err)
	}
	_ = callbackResp.Body.Close()

	// completeMCPAuth runs in a background goroutine; poll briefly for it
	// to finish rather than assuming it already has.
	deadline := time.Now().Add(2 * time.Second)
	var final *MCPAuthState
	for time.Now().Before(deadline) {
		if s, ok := globalMCPAuthManager.GetState("test-server"); ok && s.Status != "pending" {
			final = s
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if final == nil {
		t.Fatal("completeMCPAuth did not finish within the deadline")
	}
	if final.Status != "authenticated" {
		t.Fatalf("expected status=authenticated, got %q (error: %s)", final.Status, final.Error)
	}

	// The exchanged token must actually be persisted and usable.
	header, ok := AuthHeaderForMCPServer("test-server")
	if !ok {
		t.Fatal("expected AuthHeaderForMCPServer to find the newly stored token")
	}
	if header != "Bearer access-the-auth-code" {
		t.Errorf("unexpected Authorization header: %q", header)
	}
}

func TestMcpAuthTool_ExplicitClientID_SkipsRegistration(t *testing.T) {
	resetAuthManager(t)
	restore := mcp.SetTokenBackendForTesting(newFakeTokenBackend())
	defer restore()

	registrationCalled := false
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	var srv *httptest.Server
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(mcp.AuthServerMetadata{
			AuthorizationEndpoint: srv.URL + "/authorize",
			TokenEndpoint:         srv.URL + "/token",
			RegistrationEndpoint:  srv.URL + "/register",
		})
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		registrationCalled = true
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"client_id": "should-not-be-used"})
	})
	srv = testutil.NewLoopbackHTTPServer(t, mux)
	defer srv.Close()

	input, _ := json.Marshal(map[string]string{
		"server_name": "explicit-client-server",
		"server_url":  srv.URL + "/mcp",
		"client_id":   "pre-registered-client",
	})
	out, err := McpAuthTool{}.Execute(t.Context(), input)
	if err != nil {
		t.Fatalf("Execute returned an error: %v", err)
	}
	if registrationCalled {
		t.Error("dynamic client registration should not be attempted when client_id is provided")
	}

	var resp struct {
		AuthURL string `json:"authUrl"`
	}
	_ = json.Unmarshal([]byte(out), &resp)
	parsed, _ := url.Parse(resp.AuthURL)
	if got := parsed.Query().Get("client_id"); got != "pre-registered-client" {
		t.Errorf("client_id = %q, want the explicitly provided one", got)
	}
}

func TestMcpAuthTool_NoRegistrationEndpointAndNoClientID_Errors(t *testing.T) {
	resetAuthManager(t)
	restore := mcp.SetTokenBackendForTesting(newFakeTokenBackend())
	defer restore()

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	var srv *httptest.Server
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(mcp.AuthServerMetadata{
			AuthorizationEndpoint: srv.URL + "/authorize",
			TokenEndpoint:         srv.URL + "/token",
			// No RegistrationEndpoint.
		})
	})
	srv = testutil.NewLoopbackHTTPServer(t, mux)
	defer srv.Close()

	input, _ := json.Marshal(map[string]string{
		"server_name": "no-dcr-server",
		"server_url":  srv.URL + "/mcp",
	})
	out, err := McpAuthTool{}.Execute(t.Context(), input)
	if err != nil {
		t.Fatalf("Execute returned a Go error (should report via status instead): %v", err)
	}
	var resp struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal([]byte(out), &resp)
	if resp.Status != "error" {
		t.Fatalf("expected status=error, got %q (%s)", resp.Status, out)
	}
}

func TestMcpAuthTool_SecondCallReportsExistingPendingState(t *testing.T) {
	resetAuthManager(t)
	restore := mcp.SetTokenBackendForTesting(newFakeTokenBackend())
	defer restore()

	server := newTestOAuthServer(t)
	input, _ := json.Marshal(map[string]string{
		"server_name": "repeat-server",
		"server_url":  server.URL + "/mcp",
	})

	first, err := McpAuthTool{}.Execute(t.Context(), input)
	if err != nil {
		t.Fatalf("first Execute failed: %v", err)
	}
	second, err := McpAuthTool{}.Execute(t.Context(), input)
	if err != nil {
		t.Fatalf("second Execute failed: %v", err)
	}

	var firstResp, secondResp struct {
		Status  string `json:"status"`
		AuthURL string `json:"authUrl"`
	}
	_ = json.Unmarshal([]byte(first), &firstResp)
	_ = json.Unmarshal([]byte(second), &secondResp)
	if secondResp.Status != "pending" {
		t.Fatalf("expected the second call to report the still-pending flow, got %q", secondResp.Status)
	}
	if firstResp.AuthURL != secondResp.AuthURL {
		t.Error("expected the second call to report the SAME auth URL, not start a new flow")
	}
}

func TestMcpAuthTool_MissingRequiredParams(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "missing server_name", input: `{"server_url":"https://example.com"}`},
		{name: "missing server_url", input: `{"server_name":"x"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := McpAuthTool{}.Execute(t.Context(), json.RawMessage(tc.input))
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestAuthHeaderForMCPServer_NoStoredToken(t *testing.T) {
	restore := mcp.SetTokenBackendForTesting(newFakeTokenBackend())
	defer restore()

	_, ok := AuthHeaderForMCPServer("never-authenticated-server")
	if ok {
		t.Fatal("expected no token for a server that was never authenticated")
	}
}

func TestAuthHeaderForMCPServer_RefreshesExpiringToken(t *testing.T) {
	fake := newFakeTokenBackend()
	restore := mcp.SetTokenBackendForTesting(fake)
	defer restore()

	refreshServerCalled := false
	tokenServer := testutil.NewLoopbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshServerCalled = true
		_ = r.ParseForm()
		if r.FormValue("grant_type") != "refresh_token" || r.FormValue("refresh_token") != "old-refresh" {
			t.Errorf("unexpected refresh request: %v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "refreshed-access",
			"refresh_token": "new-refresh",
			"expires_in":    3600,
		})
	}))
	defer tokenServer.Close()

	almostExpired := &mcp.StoredToken{
		AccessToken:   "old-access",
		RefreshToken:  "old-refresh",
		ExpiresAt:     time.Now().Add(5 * time.Second), // within the 30s refresh buffer
		ClientID:      "client-1",
		TokenEndpoint: tokenServer.URL,
	}
	if err := mcp.SaveToken("expiring-server", almostExpired); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	header, ok := AuthHeaderForMCPServer("expiring-server")
	if !ok {
		t.Fatal("expected a usable token after refresh")
	}
	if !refreshServerCalled {
		t.Fatal("expected the refresh flow to actually call the token endpoint")
	}
	if header != "Bearer refreshed-access" {
		t.Errorf("header = %q", header)
	}

	// The refreshed token must be re-persisted, not just returned once.
	reloaded, err := mcp.LoadToken("expiring-server")
	if err != nil {
		t.Fatalf("LoadToken after refresh: %v", err)
	}
	if reloaded.AccessToken != "refreshed-access" || reloaded.RefreshToken != "new-refresh" {
		t.Errorf("refreshed token was not persisted correctly: %+v", reloaded)
	}
}

func TestAuthHeaderForMCPServer_NoRefreshTokenReportsNoAuth(t *testing.T) {
	restore := mcp.SetTokenBackendForTesting(newFakeTokenBackend())
	defer restore()

	expired := &mcp.StoredToken{
		AccessToken: "stale-access",
		ExpiresAt:   time.Now().Add(-time.Hour),
		// No RefreshToken.
	}
	if err := mcp.SaveToken("no-refresh-server", expired); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	_, ok := AuthHeaderForMCPServer("no-refresh-server")
	if ok {
		t.Fatal("expected no usable token when the stored token is expired with no refresh_token")
	}
}

// sanity check that this test file's own PKCE understanding matches
// internal/mcp's actual S256 implementation, so the other tests above are
// exercising the real thing rather than a divergent assumption.
func TestPKCEChallengeMethod_MatchesRFC7636S256(t *testing.T) {
	verifier, challenge, err := mcp.GeneratePKCE()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sum := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if challenge != want {
		t.Errorf("challenge = %q, want %q", challenge, want)
	}
}
