package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// HTTPHook is a remote decision hook invoked via POST.
// The body is {"event":"...","tool":"...","data":{...}}.
// Expected response JSON: {"action":"allow|deny","reason":"...","message":"..."}.
type HTTPHook struct {
	Name     string
	URL      string
	Events   []string // empty = all events
	Timeout  time.Duration
	Priority int
}

// RegisterHTTPDecisionHook registers an HTTP-backed decision hook.
func RegisterHTTPDecisionHook(h HTTPHook) {
	if h.Timeout <= 0 {
		h.Timeout = 3 * time.Second
	}
	if h.Name == "" {
		h.Name = "http:" + h.URL
	}
	client := &http.Client{Timeout: h.Timeout}
	url := h.URL
	events := append([]string{}, h.Events...)
	RegisterDecisionHookWithConfig(DecisionHookConfig{
		Name: h.Name,
		Matcher: DecisionMatcher{
			Events: events,
		},
		Priority: h.Priority,
	}, func(event string, data map[string]interface{}) *HookDecision {
		return invokeHTTPHook(client, url, event, data)
	})
}

func invokeHTTPHook(client *http.Client, url, event string, data map[string]interface{}) *HookDecision {
	payload := map[string]interface{}{
		"event": event,
		"data":  data,
	}
	if tool, ok := data["tool"].(string); ok {
		payload["tool"] = tool
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil // fail-open
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "hawk-hooks/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	var out struct {
		Action  string `json:"action"`
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil
	}
	switch out.Action {
	case ActionDeny:
		return Deny(firstNonEmpty(out.Message, out.Reason, "denied by HTTP hook"))
	case ActionAllow:
		return Allow()
	case ActionInstruct:
		return Instruct(firstNonEmpty(out.Message, out.Reason))
	default:
		return nil
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ValidateHTTPHookURL is a light SSRF guard: only http(s) and no localhost unless
// HAWK_HOOKS_ALLOW_LOCAL=1. Callers may skip this for tests.
func ValidateHTTPHookURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("empty hook URL")
	}
	if !(len(raw) > 8 && (raw[:7] == "http://" || raw[:8] == "https://")) {
		return fmt.Errorf("hook URL must be http(s)")
	}
	return nil
}
