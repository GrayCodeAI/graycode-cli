package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hawk/eyrie/client"

	"github.com/GrayCodeAI/hawk/hooks"
	"github.com/GrayCodeAI/hawk/logger"
	"github.com/GrayCodeAI/hawk/metrics"
	modelPkg "github.com/GrayCodeAI/hawk/model"
	"github.com/GrayCodeAI/hawk/permissions"
	"github.com/GrayCodeAI/hawk/retry"
	"github.com/GrayCodeAI/hawk/tool"
)

const (
	maxContextMessages = 100 // auto-compact threshold
	maxRecoveryRetries = 3   // max_tokens recovery attempts
)

// Session manages a conversation with an LLM via eyrie.
type Session struct {
	client       *client.EyrieClient
	registry     *tool.Registry
	messages     []client.EyrieMessage
	provider     string
	model        string
	apiKeys      map[string]string
	system       string
	log          *logger.Logger
	metrics      *metrics.Registry
	Cost         Cost
	Router       *modelPkg.Router
	Permissions  *PermissionMemory
	AutoMode     *permissions.AutoModeState
	Classifier   *permissions.Classifier
	BypassKill   *permissions.BypassKillswitch
	Mode         PermissionMode
	MaxTurns     int
	MaxBudgetUSD float64
	AllowedDirs  []string
	PermissionFn func(PermissionRequest)
	AgentSpawnFn func(ctx context.Context, prompt string) (string, error)
	AskUserFn    func(question string) (string, error)
}

// NewSession creates a new conversation session.
func NewSession(provider, model, systemPrompt string, registry *tool.Registry) *Session {
	s := &Session{
		client:      client.NewEyrieClient(&client.EyrieConfig{Provider: provider}),
		registry:    registry,
		provider:    provider,
		model:       model,
		apiKeys:     map[string]string{},
		system:      systemPrompt,
		log:         logger.Default(),
		metrics:     metrics.NewRegistry(),
		Permissions: NewPermissionMemory(),
		AutoMode:    permissions.NewAutoModeState(),
		Classifier:  permissions.NewClassifier(),
		BypassKill:  permissions.NewBypassKillswitch(),
	}
	s.Cost.Model = model
	s.Router = modelPkg.NewRouter(modelPkg.StrategyBalanced)
	return s
}

func (s *Session) Model() string              { return s.model }
func (s *Session) Provider() string           { return s.provider }
func (s *Session) Metrics() *metrics.Registry { return s.metrics }

// SetModel updates the active model for subsequent requests.
func (s *Session) SetModel(model string) {
	s.model = strings.TrimSpace(model)
	s.Cost.Model = s.model
}

// SetProvider updates the active provider for subsequent requests.
func (s *Session) SetProvider(provider string) {
	p := strings.TrimSpace(provider)
	s.provider = p
	s.client = client.NewEyrieClient(&client.EyrieConfig{Provider: p})
	for provider, apiKey := range s.apiKeys {
		if strings.TrimSpace(apiKey) != "" {
			s.client.SetAPIKey(provider, apiKey)
		}
	}
}

// SetAPIKey updates a provider API key for subsequent requests.
func (s *Session) SetAPIKey(provider, apiKey string) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	apiKey = strings.TrimSpace(apiKey)
	if provider == "" || apiKey == "" {
		return
	}
	if s.apiKeys == nil {
		s.apiKeys = map[string]string{}
	}
	s.apiKeys[provider] = apiKey
	if s.client != nil {
		s.client.SetAPIKey(provider, apiKey)
	}
}

// SetAPIKeys updates all known provider API keys for subsequent requests.
func (s *Session) SetAPIKeys(apiKeys map[string]string) {
	for provider, apiKey := range apiKeys {
		s.SetAPIKey(provider, apiKey)
	}
}

func (s *Session) AddUser(content string) {
	s.messages = append(s.messages, client.EyrieMessage{Role: "user", Content: content})
}

func (s *Session) AddAssistant(content string) {
	s.messages = append(s.messages, client.EyrieMessage{Role: "assistant", Content: content})
}

// AppendSystemContext adds runtime context, such as /add-dir, to future model calls.
func (s *Session) AppendSystemContext(content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	if strings.TrimSpace(s.system) == "" {
		s.system = content
		return
	}
	s.system += "\n\n" + content
}

// SetLogger replaces the session logger.
func (s *Session) SetLogger(l *logger.Logger) {
	s.log = l
}

// SetAllowedDirs sets directories that file tools are allowed to access.
func (s *Session) SetAllowedDirs(dirs []string) {
	s.AllowedDirs = append([]string(nil), dirs...)
}

func (s *Session) LoadMessages(msgs []client.EyrieMessage) {
	s.messages = msgs
}

func (s *Session) MessageCount() int { return len(s.messages) }

// RawMessages returns the conversation messages for persistence.
func (s *Session) RawMessages() []client.EyrieMessage { return s.messages }

// RemoveLastExchange removes the last user+assistant message pair.
func (s *Session) RemoveLastExchange() {
	if len(s.messages) < 2 {
		return
	}
	// Remove from the end until we've removed one user and one assistant message
	removed := 0
	for i := len(s.messages) - 1; i >= 0 && removed < 2; i-- {
		role := s.messages[i].Role
		if role == "user" || role == "assistant" {
			removed++
		}
		s.messages = s.messages[:i]
	}
}

// StreamEvent is sent from the engine to the TUI.
type StreamEvent struct {
	Type     string // content, thinking, tool_use, tool_result, usage, done, error
	Content  string
	ToolName string
	ToolID   string
	Usage    *StreamUsage // usage data for this event
}

// StreamUsage tracks token usage for a single stream event.
type StreamUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}

// Stream runs the agentic loop: LLM → tool_use → execute → loop.
func (s *Session) Stream(ctx context.Context) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 64)
	go s.agentLoop(ctx, ch)
	return ch, nil
}

func (s *Session) agentLoop(ctx context.Context, ch chan<- StreamEvent) {
	defer close(ch)

	// Session start hook
	hooks.ExecuteAsync(ctx, hooks.EventSessionStart, map[string]interface{}{
		"provider": s.provider,
		"model":    s.model,
	})

	recoveryCount := 0
	turnCount := 0
	snowball := NewSnowballDetector(500000) // 500K token ceiling

	for {
		// Snowball abort check
		if snowball.ShouldAbort() {
			ch <- StreamEvent{Type: "content", Content: "\n\n" + snowball.Summary()}
			ch <- StreamEvent{Type: "done"}
			return
		}

		// Enforce MaxTurns budget
		if s.MaxTurns > 0 && turnCount >= s.MaxTurns {
			ch <- StreamEvent{Type: "content", Content: "Turn limit reached — stopping."}
			ch <- StreamEvent{Type: "done"}
			return
		}
		turnCount++
		// Auto-compact if conversation is too long
		if len(s.messages) > maxContextMessages {
			s.smartCompact()
		}

		// Pre-query hook
		hooks.Execute(ctx, hooks.EventPreQuery, map[string]interface{}{
			"provider": s.provider,
			"model":    s.model,
			"messages": len(s.messages),
		})

		s.log.Info("stream query", map[string]interface{}{
			"provider": s.provider,
			"model":    s.model,
			"messages": len(s.messages),
		})

		opts := client.ChatOptions{
			Provider:  s.provider,
			Model:     s.model,
			MaxTokens: 16384,
			System:    s.system,
		}
		if s.registry != nil {
			opts.Tools = s.registry.EyrieTools()
		}

		// Circuit breaker: select provider with failover
		if s.Router != nil {
			if selectedProvider, err := s.Router.SelectProvider(s.provider); err == nil && selectedProvider != s.provider {
				s.log.Info("provider failover", map[string]interface{}{"from": s.provider, "to": selectedProvider})
				opts.Provider = selectedProvider
			}
		}

		var result *client.StreamResult
		var err error

		// Use retry for transient errors
		retryCfg := retry.DefaultConfig()
		retryCfg.MaxRetries = 2
		retryCfg.BaseDelay = 500 * time.Millisecond

		s.metrics.Counter("api.requests").Inc()
		apiStart := time.Now()

		err = retry.Do(ctx, retryCfg, func() error {
			result, err = s.client.StreamChat(ctx, s.messages, opts)
			if err != nil {
				if strings.Contains(err.Error(), "too long") || strings.Contains(err.Error(), "too many tokens") {
					s.compact()
					result, err = s.client.StreamChat(ctx, s.messages, opts)
				}
			}
			return err
		})

		apiDuration := time.Since(apiStart)
		s.metrics.Timer("api.latency").Record(apiDuration)
		s.metrics.Timer("api.last_latency").Record(apiDuration)

		if err != nil {
			// Record failure for circuit breaker
			if s.Router != nil {
				s.Router.RecordFailure(s.provider, err)
			}
			s.log.Error("stream error", map[string]interface{}{
				"error": err.Error(),
			})
			ch <- StreamEvent{Type: "error", Content: err.Error()}
			return
		}

		// Record success for circuit breaker
		if s.Router != nil {
			s.Router.RecordSuccess(s.provider, apiDuration)
		}

		var textContent string
		var toolCalls []client.ToolCall
		var stopReason string
		var lastUsage *client.EyrieUsage

		// Streaming with retry for transient stream errors
		const maxStreamRetries = 2
		var streamErr error
		for streamAttempt := 0; streamAttempt <= maxStreamRetries; streamAttempt++ {
			streamErr = nil
			for ev := range result.Events {
				select {
				case <-ctx.Done():
					result.Close()
					return
				default:
				}
				switch ev.Type {
				case "content":
					textContent += ev.Content
					ch <- StreamEvent{Type: "content", Content: ev.Content}
				case "thinking":
					ch <- StreamEvent{Type: "thinking", Content: ev.Thinking}
				case "tool_call":
					if ev.ToolCall != nil {
						toolCalls = append(toolCalls, *ev.ToolCall)
					}
				case "usage":
					if ev.Usage != nil {
						s.Cost.Add(ev.Usage.PromptTokens, ev.Usage.CompletionTokens)
						lastUsage = ev.Usage
					}
				case "error":
					streamErr = fmt.Errorf("%s", ev.Error)
					if isRetryableStreamError(streamErr) {
						break // break switch, will check in outer loop
					}
					ch <- StreamEvent{Type: "error", Content: ev.Error}
					result.Close()
					return
				case "done":
					if ev.StopReason != "" {
						stopReason = ev.StopReason
					}
				}
			}
			result.Close()

			if streamErr == nil {
				break
			}
			if !isRetryableStreamError(streamErr) {
				break
			}
			s.log.Warn("stream retry", map[string]interface{}{"attempt": streamAttempt + 1, "error": streamErr.Error()})
			time.Sleep(time.Duration(streamAttempt+1) * time.Second)

			// Re-open the stream for retry
			result, err = s.client.StreamChat(ctx, s.messages, opts)
			if err != nil {
				ch <- StreamEvent{Type: "error", Content: err.Error()}
				return
			}
			// Reset accumulated state for the retry
			textContent = ""
			toolCalls = nil
			stopReason = ""
			lastUsage = nil
		}

		// Snowball detector: record usage after each API response
		if lastUsage != nil {
			progress := 0.5
			if len(toolCalls) > 0 {
				progress = 1.0
			}
			snowball.RecordTurn(lastUsage.PromptTokens+lastUsage.CompletionTokens, progress)
		}

		// Budget enforcement
		if s.MaxBudgetUSD > 0 && s.Cost.TotalUSD() >= s.MaxBudgetUSD {
			ch <- StreamEvent{Type: "content", Content: fmt.Sprintf("\n\nBudget limit reached ($%.2f spent of $%.2f).", s.Cost.TotalUSD(), s.MaxBudgetUSD)}
			ch <- StreamEvent{Type: "done"}
			return
		}

		// Check for inline tool calls in text (some providers embed tool calls in text)
		if len(toolCalls) == 0 && strings.Contains(textContent, "<|tool_calls_section_begin|>") {
			cleanText, inlineCalls := client.ParseInlineToolCalls(textContent)
			if len(inlineCalls) > 0 {
				textContent = cleanText
				for _, ic := range inlineCalls {
					toolCalls = append(toolCalls, ic)
				}
			}
		}

		// Post-query hook
		hooks.ExecuteAsync(ctx, hooks.EventPostQuery, map[string]interface{}{
			"provider": s.provider,
			"model":    s.model,
			"content":  textContent,
			"tools":    len(toolCalls),
		})

		// Handle max_tokens recovery
		if stopReason == "max_tokens" && len(toolCalls) == 0 && recoveryCount < maxRecoveryRetries {
			recoveryCount++
			s.messages = append(s.messages, client.EyrieMessage{Role: "assistant", Content: textContent})
			s.messages = append(s.messages, client.EyrieMessage{Role: "user", Content: "Continue from where you left off."})
			continue
		}

		// No tool calls — done
		if len(toolCalls) == 0 {
			if textContent != "" {
				s.messages = append(s.messages, client.EyrieMessage{Role: "assistant", Content: textContent})
			}
			ch <- StreamEvent{Type: "done"}
			// Session end hook
			hooks.ExecuteAsync(ctx, hooks.EventSessionEnd, map[string]interface{}{
				"provider": s.provider,
				"model":    s.model,
				"messages": len(s.messages),
			})
			return
		}

		// Execute tools and collect results
		recoveryCount = 0
		type toolExecResult struct {
			tc     client.ToolCall
			output string
			isErr  bool
		}

		// Classify tools into concurrent (read-only) and sequential (write) batches
		safeConcurrent := map[string]bool{"Read": true, "Grep": true, "Glob": true, "LS": true, "WebSearch": true, "WebFetch": true, "ToolSearch": true}

		var concurrentCalls []client.ToolCall
		var sequentialCalls []client.ToolCall
		for _, tc := range toolCalls {
			if safeConcurrent[tc.Name] {
				concurrentCalls = append(concurrentCalls, tc)
			} else {
				sequentialCalls = append(sequentialCalls, tc)
			}
		}

		var results []toolExecResult
		var mu sync.Mutex

		// executeSingleTool handles permission checking and execution for one tool call.
		executeSingleTool := func(tc client.ToolCall) toolExecResult {
			ch <- StreamEvent{Type: "tool_use", ToolName: tc.Name, ToolID: tc.ID}

			// Check permission for dangerous tools
			if toolNeedsPermission(tc.Name, tc.Arguments) && s.PermissionFn != nil {
				summary := toolSummary(tc.Name, tc.Arguments)

				// Bypass killswitch check
				if s.BypassKill.IsEnabled() {
					goto executeTool
				}

				// Classifier-based auto-allow for safe commands
				if s.Classifier != nil && tc.Name == "Bash" {
					if classification := s.Classifier.Classify(summary); classification == "safe" {
						goto executeTool
					}
				}

				// Auto-mode check
				if s.AutoMode != nil {
					if allowed, ok := s.AutoMode.ShouldAutoAllow(tc.Name, summary); ok {
						if allowed {
							goto executeTool
						} else {
							ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: "Permission denied (auto-mode)."}
							return toolExecResult{tc: tc, output: "Permission denied (auto-mode).", isErr: true}
						}
					}
				}

				// Permission mode check
				if decision := s.modeDecision(tc.Name); decision != nil {
					if !*decision {
						ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: "Permission denied by permission mode."}
						return toolExecResult{tc: tc, output: "Permission denied by permission mode.", isErr: true}
					}
					goto executeTool
				}

				// Check memory first
				if decision := s.Permissions.Check(tc.Name, summary); decision != nil {
					if !*decision {
						ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: "Permission denied (rule)."}
						return toolExecResult{tc: tc, output: "Permission denied (rule).", isErr: true}
					}
					// allowed by rule, proceed
				} else {
					// Ask user with timeout
					resp := make(chan bool, 1)
					s.PermissionFn(PermissionRequest{
						ToolName: tc.Name,
						ToolID:   tc.ID,
						Summary:  summary,
						Response: resp,
					})
					select {
					case allowed := <-resp:
						if !allowed {
							ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: "Permission denied by user."}
							return toolExecResult{tc: tc, output: "Permission denied by user.", isErr: true}
						}
					case <-ctx.Done():
						ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: "Permission prompt cancelled."}
						return toolExecResult{tc: tc, output: "Permission prompt cancelled.", isErr: true}
					case <-time.After(5 * time.Minute):
						ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: "Permission prompt timed out."}
						return toolExecResult{tc: tc, output: "Permission prompt timed out.", isErr: true}
					}
				}
			}

		executeTool:
			// Pre-tool hook
			hooks.ExecuteAsync(ctx, hooks.EventPreTool, map[string]interface{}{
				"tool": tc.Name,
				"args": tc.Arguments,
			})

			inputJSON, _ := json.Marshal(tc.Arguments)
			toolCtx := tool.WithToolContext(ctx, &tool.ToolContext{
				AgentSpawnFn: s.AgentSpawnFn,
				AskUserFn:    s.AskUserFn,
			})
			output, execErr := s.registry.Execute(toolCtx, tc.Name, inputJSON)
			isErr := execErr != nil
			if isErr {
				s.log.Warn("tool execution error", map[string]interface{}{
					"tool":  tc.Name,
					"error": execErr.Error(),
				})
				output = fmt.Sprintf("Error: %s", execErr.Error())
			} else {
				s.log.Info("tool executed", map[string]interface{}{
					"tool":   tc.Name,
					"output": len(output),
				})
			}
			if len(output) > 50000 {
				output = output[:50000] + "\n... (truncated)"
			}

			// Post-tool hook
			s.metrics.Counter("tools.executed").Inc()
			if isErr {
				s.metrics.Counter("tools.errors").Inc()
			}

			hooks.ExecuteAsync(ctx, hooks.EventPostTool, map[string]interface{}{
				"tool":   tc.Name,
				"output": output,
				"is_err": isErr,
			})

			ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: output}
			return toolExecResult{tc: tc, output: output, isErr: isErr}
		}

		// Execute concurrent batch (read-only tools) in parallel
		if len(concurrentCalls) > 0 {
			var wg sync.WaitGroup
			for _, tc := range concurrentCalls {
				wg.Add(1)
				go func(tc client.ToolCall) {
					defer wg.Done()
					r := executeSingleTool(tc)
					mu.Lock()
					results = append(results, r)
					mu.Unlock()
				}(tc)
			}
			wg.Wait()
		}

		// Execute sequential batch (write tools) one-by-one
		for _, tc := range sequentialCalls {
			r := executeSingleTool(tc)
			results = append(results, r)
		}

		// Append assistant message with tool_use blocks
		assistContent := textContent
		if assistContent == "" && len(toolCalls) > 0 {
			assistContent = " " // non-empty to satisfy APIs that reject empty content
		}
		s.messages = append(s.messages, client.EyrieMessage{
			Role:    "assistant",
			Content: assistContent,
			ToolUse: toolCalls,
		})
		// Append tool results as proper tool_result messages
		for _, r := range results {
			resultContent := r.output
			if resultContent == "" {
				resultContent = "(no output)"
			}
			s.messages = append(s.messages, client.EyrieMessage{
				Role:    "user",
				Content: resultContent,
				ToolResult: &client.ToolResult{
					ToolUseID: r.tc.ID,
					Content:   resultContent,
					IsError:   r.isErr,
				},
			})
		}
	}
}

// isRetryableStreamError checks if a streaming error is transient and worth retrying.
func isRetryableStreamError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "broken pipe")
}

// Compact reduces conversation history (boundary-aware truncation).
func (s *Session) Compact() { s.compact() }

// SmartCompact reduces conversation history using LLM-generated summaries.
func (s *Session) SmartCompact() { s.smartCompact() }

// compact removes older messages while preserving tool_use/tool_result pairing.
func (s *Session) compact() {
	if len(s.messages) <= 20 {
		return
	}
	// Keep first 4 and last 16, but ensure we don't break tool pairs.
	// Walk backwards from the cut point to find a safe boundary.
	cutStart := 4
	cutEnd := len(s.messages) - 16

	// Ensure cutEnd doesn't land in the middle of a tool_use/tool_result pair.
	// A tool_result (user msg with ToolResult) must follow its tool_use (assistant msg with ToolUse).
	// Walk cutEnd forward until we're at a clean boundary.
	for cutEnd < len(s.messages) {
		msg := s.messages[cutEnd]
		if msg.ToolResult != nil {
			// This is a tool_result — we'd orphan it. Include it.
			cutEnd++
			continue
		}
		if msg.Role == "assistant" && len(msg.ToolUse) > 0 {
			// This is a tool_use — we need its results too. Skip past them.
			cutEnd++
			continue
		}
		break
	}

	// Also walk cutStart forward to not orphan pairs at the beginning
	for cutStart < cutEnd {
		msg := s.messages[cutStart]
		if msg.Role == "assistant" && len(msg.ToolUse) > 0 {
			// Include the tool results that follow
			cutStart++
			for cutStart < cutEnd && s.messages[cutStart].ToolResult != nil {
				cutStart++
			}
			continue
		}
		break
	}

	if cutStart >= cutEnd {
		return // nothing to compact
	}

	keep := make([]client.EyrieMessage, 0, len(s.messages)-(cutEnd-cutStart)+1)
	keep = append(keep, s.messages[:cutStart]...)
	keep = append(keep, client.EyrieMessage{
		Role:    "user",
		Content: "[Earlier conversation compacted to save context.]",
	})
	keep = append(keep, s.messages[cutEnd:]...)
	s.messages = keep
}
