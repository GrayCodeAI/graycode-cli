package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/types"
)

const (
	anthropicCompactBeta    = "compact-2026-01-12"
	anthropicCompactEdit    = "compact_20260112"
	anthropicMinCompactTrig = 50_000
)

// ProviderNativeCompactStrategy uses Anthropic server-side compaction when available.
type ProviderNativeCompactStrategy struct{}

func (s *ProviderNativeCompactStrategy) Name() string { return "provider_native" }

func (s *ProviderNativeCompactStrategy) ShouldTrigger(msgs []types.EyrieMessage, tokenCount, threshold int) bool {
	if tokenCount < threshold || len(msgs) < 8 {
		return false
	}
	return true
}

func (s *ProviderNativeCompactStrategy) Compact(ctx context.Context, sess *Session) (*CompactResult, error) {
	if sess == nil {
		return nil, fmt.Errorf("no session")
	}
	if !sess.supportsAnthropicNativeCompaction() {
		return nil, fmt.Errorf("provider native compaction not available")
	}
	tokensBefore := EstimateTokens(sess.messages)
	result, err := anthropicNativeCompact(ctx, sess, tokensBefore)
	if err != nil {
		return nil, err
	}
	sess.messages = result.Messages
	return result, nil
}

func (s *Session) supportsAnthropicNativeCompaction() bool {
	if s == nil {
		return false
	}
	key := s.anthropicAPIKey()
	if key == "" {
		return false
	}
	prov := strings.ToLower(strings.TrimSpace(s.provider))
	if prov != "anthropic" && !strings.Contains(prov, "anthropic") {
		// Allow claude models routed via compatible gateways when direct key exists.
		model := strings.ToLower(strings.TrimSpace(s.model))
		if !strings.HasPrefix(model, "claude-") {
			return false
		}
	}
	return anthropicCompactionModel(s.model)
}

func (s *Session) anthropicAPIKey() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if k := strings.TrimSpace(s.apiKeys["anthropic"]); k != "" {
		return k
	}
	return ""
}

func anthropicCompactionModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}
	// Supported families per Anthropic compaction docs.
	prefixes := []string{
		"claude-opus-4", "claude-sonnet-4", "claude-mythos",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(m, p) {
			return true
		}
	}
	return false
}

func anthropicNativeCompact(ctx context.Context, sess *Session, tokensBefore int) (*CompactResult, error) {
	key := sess.anthropicAPIKey()
	trigger := sess.ContextWindowSize() * sess.compactThresholdPct() / 100
	if trigger < anthropicMinCompactTrig {
		trigger = anthropicMinCompactTrig
	}

	msgs, system := buildAnthropicPayloadMessages(sess.messages)
	body := map[string]interface{}{
		"model":      sess.model,
		"max_tokens": 8192,
		"messages":   msgs,
		"context_management": map[string]interface{}{
			"edits": []map[string]interface{}{
				{
					"type": anthropicCompactEdit,
					"trigger": map[string]interface{}{
						"type":  "input_tokens",
						"value": trigger,
					},
					"pause_after_compaction": true,
				},
			},
		},
	}
	if system != "" {
		body["system"] = system
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", key)
	req.Header.Set("Anthropic-Version", "2023-06-01")
	req.Header.Set("Anthropic-Beta", anthropicCompactBeta)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("anthropic compaction HTTP %d: %s", resp.StatusCode, truncateErrBody(respBody))
	}

	var parsed anthropicCompactResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("parse compaction response: %w", err)
	}
	summary := parsed.compactionSummary()
	if summary == "" {
		return nil, fmt.Errorf("anthropic compaction produced no summary (stop=%s)", parsed.StopReason)
	}

	keepEnd := 6
	if keepEnd > len(sess.messages) {
		keepEnd = len(sess.messages)
	}
	tail := append([]types.EyrieMessage(nil), sess.messages[len(sess.messages)-keepEnd:]...)
	compactMsg := types.EyrieMessage{
		Role:    "user",
		Content: FormatCompactSummary(summary),
	}
	newMsgs := append([]types.EyrieMessage{compactMsg}, tail...)
	tokensAfter := EstimateTokens(newMsgs)

	return &CompactResult{
		Messages:     newMsgs,
		TokensBefore: tokensBefore,
		TokensAfter:  tokensAfter,
		Strategy:     "provider_native",
	}, nil
}

type anthropicCompactResponse struct {
	StopReason string `json:"stop_reason"`
	Content    []struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
		// compaction block may nest summary in text or dedicated fields
		Summary string `json:"summary,omitempty"`
	} `json:"content"`
}

func (r *anthropicCompactResponse) compactionSummary() string {
	for _, block := range r.Content {
		switch block.Type {
		case "compaction", "text":
			if s := strings.TrimSpace(block.Summary); s != "" {
				return s
			}
			if s := strings.TrimSpace(block.Text); s != "" {
				return s
			}
		}
	}
	return ""
}

func buildAnthropicPayloadMessages(messages []types.EyrieMessage) ([]map[string]interface{}, string) {
	var system string
	out := make([]map[string]interface{}, 0, len(messages))
	for _, m := range messages {
		if m.Role == "system" {
			system = m.Content
			continue
		}
		if m.Role == "assistant" && len(m.ToolUse) > 0 {
			content := make([]map[string]interface{}, 0)
			if m.Content != "" {
				content = append(content, map[string]interface{}{"type": "text", "text": m.Content})
			}
			for _, tc := range m.ToolUse {
				input := tc.Arguments
				if input == nil {
					input = map[string]interface{}{}
				}
				content = append(content, map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": input,
				})
			}
			out = append(out, map[string]interface{}{"role": "assistant", "content": content})
			continue
		}
		if m.Role == "user" && len(m.ToolResults) > 0 {
			content := make([]map[string]interface{}, 0)
			if m.Content != "" {
				content = append(content, map[string]interface{}{"type": "text", "text": m.Content})
			}
			for _, tr := range m.ToolResults {
				block := map[string]interface{}{
					"type":        "tool_result",
					"tool_use_id": tr.ToolUseID,
					"content":     tr.Content,
				}
				if tr.IsError {
					block["is_error"] = true
				}
				content = append(content, block)
			}
			out = append(out, map[string]interface{}{"role": "user", "content": content})
			continue
		}
		out = append(out, map[string]interface{}{
			"role":    m.Role,
			"content": m.Content,
		})
	}
	return out, system
}

func truncateErrBody(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
