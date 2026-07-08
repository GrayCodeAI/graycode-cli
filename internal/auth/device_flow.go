package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DeviceFlowConfig holds OAuth device flow configuration.
type DeviceFlowConfig struct {
	ClientID      string
	DeviceAuthURL string // e.g. "https://github.com/login/device/code"
	TokenURL      string // e.g. "https://github.com/login/oauth/access_token"
	Scopes        []string
	PollInterval  time.Duration
	ExpiresIn     time.Duration
}

// DeviceCodeResponse is the initial response from the device auth endpoint.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// TokenResponse is the final token from the OAuth flow.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
}

// DeviceFlow implements the OAuth 2.0 Device Authorization Grant (RFC 8628).
type DeviceFlow struct {
	Config     DeviceFlowConfig
	HTTPClient *http.Client
}

// NewDeviceFlow creates a device flow handler.
func NewDeviceFlow(cfg DeviceFlowConfig) *DeviceFlow {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 5 * time.Second
	}
	return &DeviceFlow{
		Config:     cfg,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// RequestCode initiates the device flow and returns the user code + verification URL.
func (df *DeviceFlow) RequestCode(ctx context.Context) (*DeviceCodeResponse, error) {
	data := url.Values{
		"client_id": {df.Config.ClientID},
		"scope":     {strings.Join(df.Config.Scopes, " ")},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, df.Config.DeviceAuthURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := df.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device auth request failed: HTTP %d", resp.StatusCode)
	}

	var dcr DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&dcr); err != nil {
		return nil, err
	}
	return &dcr, nil
}

// PollForToken polls the token endpoint until the user authorizes or timeout.
func (df *DeviceFlow) PollForToken(ctx context.Context, deviceCode string) (*TokenResponse, error) {
	interval := df.Config.PollInterval
	deadline := time.Now().Add(df.Config.ExpiresIn)
	if df.Config.ExpiresIn == 0 {
		deadline = time.Now().Add(5 * time.Minute)
	}

	timer := time.NewTimer(interval)
	defer timer.Stop()

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}

		token, err := df.exchangeCode(ctx, deviceCode)
		if err != nil {
			if strings.Contains(err.Error(), "authorization_pending") || strings.Contains(err.Error(), "slow_down") {
				if strings.Contains(err.Error(), "slow_down") {
					interval += 5 * time.Second
				}
				timer.Reset(interval)
				continue
			}
			return nil, err
		}
		return token, nil
	}
	return nil, fmt.Errorf("device flow timed out waiting for authorization")
}

func (df *DeviceFlow) exchangeCode(ctx context.Context, deviceCode string) (*TokenResponse, error) {
	data := url.Values{
		"client_id":   {df.Config.ClientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, df.Config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := df.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var raw map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	if errStr, ok := raw["error"].(string); ok {
		return nil, fmt.Errorf("%s", errStr)
	}

	jsonData, _ := json.Marshal(raw)
	var token TokenResponse
	if err := json.Unmarshal(jsonData, &token); err != nil {
		return nil, err
	}
	return &token, nil
}
