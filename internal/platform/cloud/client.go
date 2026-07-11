// Package cloud contains Hawk's optional, fail-open HTTP integration with Hawk Cloud.
package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	Endpoint    string
	DeviceToken string
	HTTPClient  *http.Client
}

type UsageEvent struct {
	EventID             string `json:"eventId"`
	DeviceID            string `json:"deviceId"`
	ProjectID           string `json:"projectId"`
	SessionID           string `json:"sessionId,omitempty"`
	Capability          string `json:"capability"`
	Provider            string `json:"provider,omitempty"`
	Model               string `json:"model,omitempty"`
	InputTokens         int    `json:"inputTokens,omitempty"`
	OutputTokens        int    `json:"outputTokens,omitempty"`
	CachedInputTokens   int    `json:"cachedInputTokens,omitempty"`
	ReasoningTokens     int    `json:"reasoningTokens,omitempty"`
	TokensUsed          int    `json:"tokensUsed"`
	TokensSaved         int    `json:"tokensSaved,omitempty"`
	EstimatedCostMicros int    `json:"estimatedCostMicros,omitempty"`
	DurationMS          int    `json:"durationMs,omitempty"`
	Status              string `json:"status,omitempty"`
	ErrorCode           string `json:"errorCode,omitempty"`
	OccurredAt          string `json:"occurredAt"`
}

// DeliveryContext is bounded repository and delivery metadata Hawk can sync
// with its project-scoped device token.
type DeliveryContext struct {
	ProjectID  string `json:"projectId"`
	Repository struct {
		Provider      string `json:"provider"`
		ExternalID    string `json:"externalId"`
		Name          string `json:"name"`
		URL           string `json:"url,omitempty"`
		DefaultBranch string `json:"defaultBranch,omitempty"`
	} `json:"repository"`
	Branch     string             `json:"branch,omitempty"`
	CommitSHA  string             `json:"commitSha,omitempty"`
	CIRun      *CIRunContext      `json:"ciRun,omitempty"`
	Deployment *DeploymentContext `json:"deployment,omitempty"`
}

type CIRunContext struct {
	Provider   string `json:"provider"`
	ExternalID string `json:"externalId"`
	Workflow   string `json:"workflow,omitempty"`
	Status     string `json:"status"`
}

type DeploymentContext struct {
	Provider    string `json:"provider"`
	ExternalID  string `json:"externalId"`
	Environment string `json:"environment"`
	Status      string `json:"status"`
}

type DeviceLoginStart struct {
	DeviceCode      string `json:"deviceCode"`
	UserCode        string `json:"userCode"`
	VerificationURI string `json:"verificationUri"`
	ExpiresIn       int    `json:"expiresIn"`
	Interval        int    `json:"interval"`
}

type DeviceLoginPoll struct {
	Status      string `json:"status"`
	Token       string `json:"token"`
	DeviceID    string `json:"deviceId"`
	ProjectID   string `json:"projectId"`
	PrincipalID string `json:"principalId"`
}

type Client struct {
	endpoint, token string
	http            *http.Client
}

func New(cfg Config) *Client {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	return &Client{endpoint: strings.TrimRight(cfg.Endpoint, "/"), token: cfg.DeviceToken, http: client}
}

func (c *Client) Enabled() bool { return c.endpoint != "" && c.token != "" }

func (c *Client) startDeviceLogin(ctx context.Context, label, platform, hawkVersion string) (DeviceLoginStart, error) {
	var result DeviceLoginStart
	body, err := json.Marshal(map[string]string{"label": label, "platform": platform, "hawkVersion": hawkVersion})
	if err != nil {
		return result, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v1/auth/device/start", bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return result, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("hawk cloud device login start: %s", resp.Status)
	}
	return result, json.NewDecoder(resp.Body).Decode(&result)
}

func (c *Client) pollDeviceLogin(ctx context.Context, deviceCode string) (DeviceLoginPoll, error) {
	var result DeviceLoginPoll
	body, err := json.Marshal(map[string]string{"deviceCode": deviceCode})
	if err != nil {
		return result, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v1/auth/device/poll", bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return result, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("hawk cloud device login poll: %s", resp.Status)
	}
	return result, json.NewDecoder(resp.Body).Decode(&result)
}

func (c *Client) StartDeviceLogin(ctx context.Context, label, platform, hawkVersion string) (DeviceLoginStart, error) {
	if c.endpoint == "" {
		return DeviceLoginStart{}, fmt.Errorf("hawk cloud endpoint is not configured")
	}
	return c.startDeviceLogin(ctx, label, platform, hawkVersion)
}

func (c *Client) PollDeviceLogin(ctx context.Context, deviceCode string) (DeviceLoginPoll, error) {
	if c.endpoint == "" {
		return DeviceLoginPoll{}, fmt.Errorf("hawk cloud endpoint is not configured")
	}
	return c.pollDeviceLogin(ctx, deviceCode)
}

// RecordUsage intentionally discards transport failures. Cloud sync must not alter local execution.
func (c *Client) RecordUsage(ctx context.Context, event UsageEvent) {
	if !c.Enabled() {
		return
	}
	body, err := json.Marshal(event)
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v1/usage", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err == nil && resp != nil {
		_ = resp.Body.Close()
	}
}

// RecordDeliveryContext is fail-open: delivery metadata must not affect local execution.
func (c *Client) RecordDeliveryContext(ctx context.Context, event DeliveryContext) {
	if !c.Enabled() {
		return
	}
	body, err := json.Marshal(event)
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v1/delivery-context", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err == nil && resp != nil {
		_ = resp.Body.Close()
	}
}
