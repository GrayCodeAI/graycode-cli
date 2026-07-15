package mcp

// OAuth support for connecting to auth-gated remote MCP servers.
//
// hawk has zero MCP SDK dependency (see mcp.go/http.go/ws.go's hand-rolled
// JSON-RPC) — this stays consistent with that and hand-rolls OAuth too,
// rather than adopting a third-party client library.
//
// Flow: DiscoverOAuthMetadata -> RegisterClient (if no static client_id) ->
// GeneratePKCE -> StartLoopbackCallback -> BuildAuthorizationURL (user
// visits it in their own browser) -> ExchangeCode once the callback fires.
// RefreshOAuthToken renews an expiring token without repeating the flow.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProtectedResourceMetadata is the RFC 9728 discovery document served by
// the MCP server itself (not its authorization server), pointing at which
// authorization server(s) protect it.
type ProtectedResourceMetadata struct {
	AuthorizationServers []string `json:"authorization_servers"`
}

// AuthServerMetadata is the RFC 8414 discovery document served by an
// authorization server.
type AuthServerMetadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RegistrationEndpoint  string `json:"registration_endpoint,omitempty"`
}

var oauthHTTPClient = &http.Client{Timeout: 10 * time.Second}

func fetchJSON(ctx context.Context, url string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, url)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// DiscoverOAuthMetadata locates the authorization server that protects
// serverURL and returns its metadata. It tries, in order:
//  1. RFC 9728 protected-resource discovery on the MCP server itself
//     ({origin}/.well-known/oauth-protected-resource), then RFC 8414
//     discovery on the first authorization server it names.
//  2. RFC 8414 discovery directly on the MCP server's own origin (the
//     common case where the MCP server and its authorization server are
//     the same host).
//  3. A last-resort guess at conventional endpoint paths, so a server with
//     no discovery documents at all still has *something* to try.
func DiscoverOAuthMetadata(ctx context.Context, serverURL string) (*AuthServerMetadata, error) {
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("invalid server URL: %w", err)
	}
	origin := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)

	if issuer, ok := discoverProtectedResourceIssuer(ctx, origin); ok {
		if meta, err := discoverAuthServerMetadata(ctx, issuer); err == nil {
			return meta, nil
		}
	}

	if meta, err := discoverAuthServerMetadata(ctx, origin); err == nil {
		return meta, nil
	}

	// Last resort: conventional guesses, matching the prior (pre-discovery)
	// behavior so a server with no metadata documents at all still gets an
	// attempt rather than an outright failure.
	return &AuthServerMetadata{
		Issuer:                origin,
		AuthorizationEndpoint: origin + "/oauth/authorize",
		TokenEndpoint:         origin + "/oauth/token",
	}, nil
}

func discoverProtectedResourceIssuer(ctx context.Context, origin string) (string, bool) {
	var doc ProtectedResourceMetadata
	if err := fetchJSON(ctx, origin+"/.well-known/oauth-protected-resource", &doc); err != nil {
		return "", false
	}
	if len(doc.AuthorizationServers) == 0 {
		return "", false
	}
	return doc.AuthorizationServers[0], true
}

func discoverAuthServerMetadata(ctx context.Context, issuer string) (*AuthServerMetadata, error) {
	var meta AuthServerMetadata
	if err := fetchJSON(ctx, strings.TrimSuffix(issuer, "/")+"/.well-known/oauth-authorization-server", &meta); err != nil {
		return nil, err
	}
	if meta.AuthorizationEndpoint == "" || meta.TokenEndpoint == "" {
		return nil, fmt.Errorf("authorization server metadata at %s is missing required endpoints", issuer)
	}
	return &meta, nil
}

// dcrRequest/dcrResponse implement RFC 7591 dynamic client registration for
// a native/public client (no client_secret expected back).
type dcrRequest struct {
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	ClientName              string   `json:"client_name"`
}

type dcrResponse struct {
	ClientID string `json:"client_id"`
}

// RegisterClient performs RFC 7591 dynamic client registration against the
// given registration endpoint and returns the assigned client_id.
func RegisterClient(ctx context.Context, registrationEndpoint, redirectURI string) (string, error) {
	body, err := json.Marshal(dcrRequest{
		RedirectURIs:            []string{redirectURI},
		TokenEndpointAuthMethod: "none",
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		ClientName:              "hawk",
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registrationEndpoint, strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("dynamic client registration failed: HTTP %d", resp.StatusCode)
	}
	var out dcrResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.ClientID == "" {
		return "", fmt.Errorf("registration response did not include a client_id")
	}
	return out.ClientID, nil
}

// GeneratePKCE returns a PKCE code_verifier and its S256 code_challenge.
func GeneratePKCE() (verifier, challenge string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// GenerateState returns a random value for the OAuth state parameter, used
// to bind an authorization request to its callback.
func GenerateState() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// CallbackResult is what StartLoopbackCallback delivers once the OAuth
// redirect arrives (or the wait times out / is cancelled).
type CallbackResult struct {
	Code  string
	State string
	Err   error
}

// StartLoopbackCallback binds an OS-assigned free port on 127.0.0.1,
// returning the redirect_uri to use in the authorization request. It
// serves exactly one request (the OAuth redirect), sends the result on the
// returned channel, and stops accepting further requests. The caller must
// call shutdown() once (after receiving a result, or on timeout) to
// release the listener.
func StartLoopbackCallback(ctx context.Context, timeout time.Duration) (redirectURI string, resultCh <-chan CallbackResult, shutdown func(), err error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, nil, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI = "http://127.0.0.1:" + strconv.Itoa(port) + "/callback"

	results := make(chan CallbackResult, 1)
	var once sync.Once
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		result := CallbackResult{Code: q.Get("code"), State: q.Get("state")}
		if errParam := q.Get("error"); errParam != "" {
			result.Err = fmt.Errorf("authorization server returned error: %s", errParam)
		} else if result.Code == "" {
			result.Err = fmt.Errorf("callback request had no authorization code")
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if result.Err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("<html><body>Authorization failed. You can close this tab.</body></html>"))
		} else {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html><body>Authorized. You can close this tab and return to hawk.</body></html>"))
		}
		once.Do(func() {
			select {
			case results <- result:
			default:
			}
		})
	})
	httpServer := &http.Server{Handler: mux}

	go func() { _ = httpServer.Serve(listener) }()

	timer := time.AfterFunc(timeout, func() {
		select {
		case results <- CallbackResult{Err: fmt.Errorf("timed out waiting for authorization callback")}:
		default:
		}
	})

	shutdownFunc := func() {
		timer.Stop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}

	go func() {
		<-ctx.Done()
		shutdownFunc()
	}()

	return redirectURI, results, shutdownFunc, nil
}

// BuildAuthorizationURL builds the authorization-code+PKCE request URL.
// resourceURL is the MCP server's canonical URL, sent per RFC 8707 so a
// multi-tenant authorization server can scope the issued token correctly.
func BuildAuthorizationURL(meta *AuthServerMetadata, clientID, redirectURI, state, codeChallenge, resourceURL string) string {
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
		"resource":              {resourceURL},
	}
	sep := "?"
	if strings.Contains(meta.AuthorizationEndpoint, "?") {
		sep = "&"
	}
	return meta.AuthorizationEndpoint + sep + q.Encode()
}

// TokenResult is a normalized OAuth token response.
type TokenResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	Error        string `json:"error,omitempty"`
	ErrorDesc    string `json:"error_description,omitempty"`
}

func postTokenRequest(ctx context.Context, tokenEndpoint string, form url.Values) (*TokenResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var out tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}
	if out.Error != "" {
		if out.ErrorDesc != "" {
			return nil, fmt.Errorf("%s: %s", out.Error, out.ErrorDesc)
		}
		return nil, fmt.Errorf("%s", out.Error)
	}
	if out.AccessToken == "" {
		return nil, fmt.Errorf("token endpoint returned no access_token (HTTP %d)", resp.StatusCode)
	}
	result := &TokenResult{AccessToken: out.AccessToken, RefreshToken: out.RefreshToken}
	if out.ExpiresIn > 0 {
		result.ExpiresAt = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	}
	return result, nil
}

// ExchangeCode exchanges an authorization code (+ PKCE verifier) for tokens.
func ExchangeCode(ctx context.Context, meta *AuthServerMetadata, clientID, code, verifier, redirectURI string) (*TokenResult, error) {
	return postTokenRequest(ctx, meta.TokenEndpoint, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	})
}

// RefreshOAuthToken exchanges a refresh token for a new access token.
func RefreshOAuthToken(ctx context.Context, meta *AuthServerMetadata, clientID, refreshToken string) (*TokenResult, error) {
	return postTokenRequest(ctx, meta.TokenEndpoint, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
	})
}
