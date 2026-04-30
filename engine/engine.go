package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hawk/eyrie/client"

	"github.com/GrayCodeAI/hawk/tool"
)

const (
	maxContextMessages = 100 // auto-compact threshold
	maxRecoveryRetries = 3  // max_tokens recovery attempts
)

// Session manages a conversation with an LLM via eyrie.
type Session struct {
	client       *client.EyrieClient
	registry     *tool.Registry
	messages     []client.EyrieMessage
	provider     string
	model        string
	system       string
	Cost         Cost
	Permissions  *PermissionMemory
	PermissionFn func(PermissionRequest)
	AgentSpawnFn func(ctx context.Context, prompt string) (string, error)
	AskUserFn    func(question string) (string, error)
}

// NewSession creates a new conversation session.
func NewSession(provider, model, systemPrompt string, registry *tool.Registry) *Session {
	detected := provider
	if detected == "" {
		detected = client.DetectProvider()
	}
	if model == "" {
		model = client.ResolveDefaultModel(detected)
	}
	s := &Session{
		client:      client.NewEyrieClient(&client.EyrieConfig{Provider: detected}),
		registry:    registry,
		provider:    detected,
		model:       model,
		system:      systemPrompt,
		Permissions: NewPermissionMemory(),
	}
	s.Cost.Model = model
	return s
}

func (s *Session) Model() string    { return s.model }
func (s *Session) Provider() string { return s.provider }

func (s *Session) AddUser(content string) {
	s.messages = append(s.messages, client.EyrieMessage{Role: "user", Content: content})
}

func (s *Session) AddAssistant(content string) {
	s.messages = append(s.messages, client.EyrieMessage{Role: "assistant", Content: content})
}

func (s *Session) LoadMessages(msgs []client.EyrieMessage) {
	s.messages = msgs
}

func (s *Session) MessageCount() int { return len(s.messages) }

// RawMessages returns the conversation messages for persistence.
func (s *Session) RawMessages() []client.EyrieMessage { return s.messages }

// StreamEvent is sent from the engine to the TUI.
type StreamEvent struct {
	Type     string // content, thinking, tool_use, tool_result, done, error
	Content  string
	ToolName string
	ToolID   string
}

// Stream runs the agentic loop: LLM → tool_use → execute → loop.
func (s *Session) Stream(ctx context.Context) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 64)
	go s.agentLoop(ctx, ch)
	return ch, nil
}

func (s *Session) agentLoop(ctx context.Context, ch chan<- StreamEvent) {
	defer close(ch)

	recoveryCount := 0

	for {
		// Auto-compact if conversation is too long
		if len(s.messages) > maxContextMessages {
			s.compact()
		}

		opts := client.ChatOptions{
			Provider:  s.provider,
			Model:     s.model,
			MaxTokens: 16384,
			System:    s.system,
		}
		if s.registry != nil {
			opts.Tools = s.registry.EyrieTools()
		}

		result, err := s.client.StreamChat(ctx, s.messages, opts)
		if err != nil {
			// Handle prompt too long
			if strings.Contains(err.Error(), "too long") || strings.Contains(err.Error(), "too many tokens") {
				s.compact()
				result, err = s.client.StreamChat(ctx, s.messages, opts)
				if err != nil {
					ch <- StreamEvent{Type: "error", Content: err.Error()}
					return
				}
			} else {
				ch <- StreamEvent{Type: "error", Content: err.Error()}
				return
			}
		}

		var textContent string
		var toolCalls []client.ToolCall
		var stopReason string

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
				}
			case "error":
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
			return
		}

		// Append assistant text if any
		if textContent != "" {
			s.messages = append(s.messages, client.EyrieMessage{Role: "assistant", Content: textContent})
		}

		// Execute tools and collect results
		recoveryCount = 0
		type toolExecResult struct {
			tc     client.ToolCall
			output string
			isErr  bool
		}
		var results []toolExecResult
		for _, tc := range toolCalls {
			ch <- StreamEvent{Type: "tool_use", ToolName: tc.Name, ToolID: tc.ID}

			// Check permission for dangerous tools
			if toolNeedsPermission(tc.Name, tc.Arguments) && s.PermissionFn != nil {
				summary := toolSummary(tc.Name, tc.Arguments)
				// Check memory first
				if decision := s.Permissions.Check(tc.Name, summary); decision != nil {
					if !*decision {
						ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: "Permission denied (rule)."}
						results = append(results, toolExecResult{tc: tc, output: "Permission denied (rule).", isErr: true})
						continue
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
							results = append(results, toolExecResult{tc: tc, output: "Permission denied by user.", isErr: true})
							continue
						}
					case <-ctx.Done():
						ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: "Permission prompt cancelled."}
						results = append(results, toolExecResult{tc: tc, output: "Permission prompt cancelled.", isErr: true})
						continue
					case <-time.After(5 * time.Minute):
						ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: "Permission prompt timed out."}
						results = append(results, toolExecResult{tc: tc, output: "Permission prompt timed out.", isErr: true})
						continue
					}
				}
			}

			inputJSON, _ := json.Marshal(tc.Arguments)
			toolCtx := tool.WithToolContext(ctx, &tool.ToolContext{
				AgentSpawnFn: s.AgentSpawnFn,
				AskUserFn:    s.AskUserFn,
			})
			output, execErr := s.registry.Execute(toolCtx, tc.Name, inputJSON)
			isErr := execErr != nil
			if isErr {
				output = fmt.Sprintf("Error: %s", execErr.Error())
			}
			if len(output) > 50000 {
				output = output[:50000] + "\n... (truncated)"
			}

			ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: output}
			results = append(results, toolExecResult{tc: tc, output: output, isErr: isErr})
		}

		// Append assistant message with tool_use blocks
		s.messages = append(s.messages, client.EyrieMessage{
			Role:    "assistant",
			Content: textContent,
			ToolUse: toolCalls,
		})
		// Append tool results as proper tool_result messages
		for _, r := range results {
			s.messages = append(s.messages, client.EyrieMessage{
				Role: "user",
				ToolResult: &client.ToolResult{
					ToolUseID: r.tc.ID,
					Content:   r.output,
					IsError:   r.isErr,
				},
			})
		}
	}
}

// Compact reduces conversation history to save context window.
func (s *Session) Compact() {
	s.compact()
}

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
