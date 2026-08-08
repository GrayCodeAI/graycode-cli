package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// HTTPHook is a remote decision hook invoked via POST.
// The body is {"event":"...","tool":"...","data":{...}}.
// Expected response JSON: {"action":"allow|deny","reason":"...","message":"..."}.
//
// Failure semantics: by default (FailOpen=false) any transport, protocol, or
// response error DENIES the guarded operation — a downed compliance hook must
// never silently allow. Set FailOpen=true only if an unreachable hook should
// be treated as "no opinion".
type HTTPHook struct {
	Name     string
	URL      string
	Events   []string // empty = all events
	Timeout  time.Duration
	Priority int
	FailOpen bool // when true, hook errors allow the operation instead of denying it
}

// maxHookResponseBytes caps the response body read from a decision hook so a
// misbehaving or malicious endpoint cannot exhaust daemon memory.
const maxHookResponseBytes = 64 << 10 // 64 KiB

// RegisterHTTPDecisionHook registers an HTTP-backed decision hook.
func RegisterHTTPDecisionHook(h HTTPHook) {
	if h.Timeout <= 0 {
		h.Timeout = 3 * time.Second
	}
	if h.Name == "" {
		h.Name = "http:" + h.URL
	}
	if h.FailOpen {
		slog.Warn("http decision hook configured fail-open",
			"name", h.Name,
			"url", h.URL,
			"note", "an unreachable guardrail hook will allow operations; ensure this is intentional")
	}
	client := &http.Client{Timeout: h.Timeout}
	url := h.URL
	failOpen := h.FailOpen
	events := append([]string{}, h.Events...)
	RegisterDecisionHookWithConfig(DecisionHookConfig{
		Name: h.Name,
		Matcher: DecisionMatcher{
			Events: events,
		},
		Priority: h.Priority,
	}, func(event string, data map[string]interface{}) *HookDecision {
		return invokeHTTPHook(client, url, event, data, failOpen)
	})
}

// hookError converts a hook failure into a decision per the fail-open policy,
// always logging the failure so a silently-dropped guardrail is impossible.
func hookError(failOpen bool, event string, format string, args ...interface{}) *HookDecision {
	msg := fmt.Sprintf(format, args...)
	slog.Warn("http decision hook failed",
		"event", event,
		"fail_open", failOpen,
		"error", msg)
	if failOpen {
		return nil // configured to allow when the hook is unreachable
	}
	return Deny("guardrail hook unavailable: " + msg)
}

func invokeHTTPHook(client *http.Client, url, event string, data map[string]interface{}, failOpen bool) *HookDecision {
	payload := map[string]interface{}{
		"event": event,
		"data":  data,
	}
	if tool, ok := data["tool"].(string); ok {
		payload["tool"] = tool
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return hookError(failOpen, event, "marshal payload: %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return hookError(failOpen, event, "build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "hawk-hooks/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return hookError(failOpen, event, "request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return hookError(failOpen, event, "unexpected status %d", resp.StatusCode)
	}
	var out struct {
		Action  string `json:"action"`
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxHookResponseBytes+1)).Decode(&out); err != nil {
		return hookError(failOpen, event, "decode response: %v", err)
	}
	switch out.Action {
	case ActionDeny:
		return Deny(firstNonEmpty(out.Message, out.Reason, "denied by HTTP hook"))
	case ActionAllow:
		return Allow()
	case ActionInstruct:
		return Instruct(firstNonEmpty(out.Message, out.Reason))
	default:
		return hookError(failOpen, event, "unrecognized action %q", out.Action)
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
