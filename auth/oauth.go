package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OAuthConfig holds OAuth2 provider configuration.
type OAuthConfig struct {
	ClientID     string
	AuthorizeURL string
	TokenURL     string
	ProfileURL   string
	Scopes       []string
}

// OAuthTokens holds the tokens returned from an OAuth2 exchange.
type OAuthTokens struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type,omitempty"`
	Scopes       []string  `json:"scopes,omitempty"`
	Subscription string    `json:"subscription_type,omitempty"`
	RateTier     string    `json:"rate_limit_tier,omitempty"`
}

// OAuthProfile holds user profile information.
type OAuthProfile struct {
	AccountUUID  string `json:"account_uuid"`
	Email        string `json:"email,omitempty"`
	OrgUUID      string `json:"organization_uuid,omitempty"`
	OrgType      string `json:"organization_type,omitempty"`
	Subscription string `json:"subscription_type,omitempty"`
	RateTier     string `json:"rate_limit_tier,omitempty"`
}

// PKCEParams holds PKCE challenge parameters.
type PKCEParams struct {
	CodeVerifier  string
	CodeChallenge string
	State         string
}

const tokenExpiryBuffer = 5 * time.Minute

// GeneratePKCE generates a new PKCE code verifier + challenge pair using S256.
func GeneratePKCE() (*PKCEParams, error) {
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return nil, fmt.Errorf("generating verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])

	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("generating state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	return &PKCEParams{
		CodeVerifier:  verifier,
		CodeChallenge: challenge,
		State:         state,
	}, nil
}

// BuildAuthURL constructs the OAuth authorization URL with PKCE parameters.
func (c *OAuthConfig) BuildAuthURL(pkce *PKCEParams, callbackPort int) string {
	redirectURI := fmt.Sprintf("http://localhost:%d/callback", callbackPort)

	params := url.Values{
		"client_id":             {c.ClientID},
		"response_type":        {"code"},
		"redirect_uri":         {redirectURI},
		"state":                {pkce.State},
		"code_challenge":       {pkce.CodeChallenge},
		"code_challenge_method": {"S256"},
	}
	if len(c.Scopes) > 0 {
		params.Set("scope", strings.Join(c.Scopes, " "))
	}

	return c.AuthorizeURL + "?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for tokens.
func (c *OAuthConfig) ExchangeCode(ctx context.Context, code, codeVerifier string, callbackPort int) (*OAuthTokens, error) {
	redirectURI := fmt.Sprintf("http://localhost:%d/callback", callbackPort)

	data := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {c.ClientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {codeVerifier},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchanging code: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}

	tokens := &OAuthTokens{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		TokenType:    tokenResp.TokenType,
	}
	if tokenResp.Scope != "" {
		tokens.Scopes = strings.Split(tokenResp.Scope, " ")
	}

	return tokens, nil
}

// RefreshAccessToken refreshes an expired access token.
func (c *OAuthConfig) RefreshAccessToken(ctx context.Context, refreshToken string) (*OAuthTokens, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {c.ClientID},
		"refresh_token": {refreshToken},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refreshing token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh failed (%d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decoding refresh response: %w", err)
	}

	rt := refreshToken
	if tokenResp.RefreshToken != "" {
		rt = tokenResp.RefreshToken
	}

	return &OAuthTokens{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: rt,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		TokenType:    tokenResp.TokenType,
	}, nil
}

// FetchProfile retrieves the user profile using the access token.
func (c *OAuthConfig) FetchProfile(ctx context.Context, accessToken string) (*OAuthProfile, error) {
	if c.ProfileURL == "" {
		return nil, fmt.Errorf("profile URL not configured")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.ProfileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating profile request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching profile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("profile fetch failed (%d)", resp.StatusCode)
	}

	var profile OAuthProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, fmt.Errorf("decoding profile: %w", err)
	}

	return &profile, nil
}

// IsExpired returns true if the token is expired or about to expire (within 5 min buffer).
func IsExpired(tokens *OAuthTokens) bool {
	if tokens == nil || tokens.AccessToken == "" {
		return true
	}
	return time.Now().Add(tokenExpiryBuffer).After(tokens.ExpiresAt)
}

// NeedsRefresh returns true if the token should be proactively refreshed.
func NeedsRefresh(tokens *OAuthTokens) bool {
	return IsExpired(tokens) && tokens.RefreshToken != ""
}
