package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

// RemoteTriggerTool manages scheduled remote hawk agents via API.
type RemoteTriggerTool struct {
	BaseURL  string
	TokenFn  func() (string, error)
}

func (RemoteTriggerTool) Name() string      { return "RemoteTrigger" }
func (RemoteTriggerTool) Aliases() []string { return []string{"remote_trigger"} }
func (RemoteTriggerTool) Description() string {
	return "Manage scheduled remote Hawk agents (triggers) via the API. Auth is handled in-process."
}
func (RemoteTriggerTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"list", "get", "create", "update", "run"},
				"description": "The API action to perform",
			},
			"trigger_id": map[string]interface{}{
				"type":        "string",
				"description": "Required for get, update, and run actions",
			},
			"body": map[string]interface{}{
				"type":        "object",
				"description": "JSON body for create and update actions",
			},
		},
		"required": []string{"action"},
	}
}

var triggerIDPattern = regexp.MustCompile(`^[\w-]+$`)

func (t RemoteTriggerTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Action    string         `json:"action"`
		TriggerID string         `json:"trigger_id"`
		Body      map[string]any `json:"body"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}

	if p.Action == "" {
		return "", fmt.Errorf("action is required")
	}
	if (p.Action == "get" || p.Action == "update" || p.Action == "run") && p.TriggerID == "" {
		return "", fmt.Errorf("trigger_id is required for %s", p.Action)
	}
	if p.TriggerID != "" && !triggerIDPattern.MatchString(p.TriggerID) {
		return "", fmt.Errorf("trigger_id must match [\\w-]+")
	}

	baseURL := t.BaseURL
	if baseURL == "" {
		baseURL = "https://api.hawk.ai/v1/code/triggers"
	}

	var (
		method string
		url    string
		body   io.Reader
	)

	switch p.Action {
	case "list":
		method, url = "GET", baseURL
	case "get":
		method, url = "GET", baseURL+"/"+p.TriggerID
	case "create":
		method = "POST"
		url = baseURL
		if p.Body != nil {
			data, _ := json.Marshal(p.Body)
			body = bytes.NewReader(data)
		}
	case "update":
		method = "POST"
		url = baseURL + "/" + p.TriggerID
		if p.Body != nil {
			data, _ := json.Marshal(p.Body)
			body = bytes.NewReader(data)
		}
	case "run":
		method, url = "POST", baseURL+"/"+p.TriggerID+"/run"
	default:
		return "", fmt.Errorf("unknown action %q", p.Action)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Anthropic-Beta", "ccr-triggers-2026-01-30")

	if t.TokenFn != nil {
		token, err := t.TokenFn()
		if err != nil {
			return "", fmt.Errorf("getting auth token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	out, _ := json.Marshal(map[string]any{
		"status": resp.StatusCode,
		"json":   string(respBody),
	})
	return string(out), nil
}
