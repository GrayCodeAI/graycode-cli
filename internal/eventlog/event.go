// Package eventlog is Hawk's append-only session event spine.
//
// It ports the durable-design idea from DeepSeek Harness: the session log is the
// single source of truth, and the messages the model actually sees are a
// projection of that log (see docs/plans/dsh-harness-port-plan.md). Keeping
// the vocabulary free of Hawk product orchestration lets the log stay a neutral,
// testable record that consumers project through without importing the engine.
//
// Port note: DeepSeek Harness grows its event vocabulary through TypeScript
// declaration merging. Go cannot merge declarations, so the vocabulary is a plain
// const set plus per-kind payloads. New model-visible event kinds are additive
// but must stay reconstructible by DeriveMessages or the record is incomplete.
package eventlog

import "time"

// Type identifies the kind of a durable session event.
type Type string

// The durable event vocabulary. Do not reuse a removed Type; the record refuses
// to load a log containing an event kind the accountable build does not know.
const (
	// SessionMeta is the one-per-log metadata line.
	SessionMeta Type = "session.meta"
	// UserMessage is a user turn appended to the model surface.
	UserMessage Type = "message.user"
	// AssistantMsg is an assistant reply appended to the model surface.
	AssistantMsg Type = "message.assistant"
	// ToolCall is a tool invocation the model requested.
	ToolCall Type = "tool.call"
	// ToolResult is the outcome of one ToolCall.
	ToolResult Type = "tool.result"
	// ContextInjected is ephemeral context literally shown to the model. It must
	// be reconstructible, so it lives on the log even though it is not a
	// conversation turn.
	ContextInjected Type = "context.injected"
	// SessionCompacted marks a compaction pass. Compaction mutates the projected
	// surface, so it is a first-class durable fact.
	SessionCompacted Type = "session.compacted"
)

// Event is one durable fact. Seq is assigned by the Log at append time and is
// what makes the record order reconstructible.
type Event struct {
	Type Type      `json:"type"`
	Seq  uint64    `json:"seq"`
	At   time.Time `json:"at"`
	Data any       `json:"data,omitempty"`
}

// Meta is the SessionMeta payload carried on the first line of the log.
type Meta struct {
	ID       string `json:"id,omitempty"`
	Model    string `json:"model,omitempty"`
	Provider string `json:"provider,omitempty"`
	CWD      string `json:"cwd,omitempty"`
	Agent    string `json:"agent,omitempty"`
	// FormatVersion records the eventlog format revision, independent of the
	// surrounding session JSONL schema so the log can version itself.
	FormatVersion int `json:"format_version"`
}

// Message is the model-surface payload for User and Assistant events. It mirrors
// the wire message shape without importing the product layer.
type Message struct {
	Content     string              `json:"content,omitempty"`
	Thinking    string              `json:"thinking,omitempty"`
	Images      []string            `json:"images,omitempty"`
	ToolUse     []ToolCallPayload   `json:"tool_use,omitempty"`
	ToolResults []ToolResultPayload `json:"tool_results,omitempty"`
}

// ToolCallPayload records the invocation facts needed to reconstruct a ToolCall.
type ToolCallPayload struct {
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ToolResultPayload records one tool result fact.
type ToolResultPayload struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

// Known reports whether t is part of the current vocabulary.
func (t Type) Known() bool {
	switch t {
	case SessionMeta, UserMessage, AssistantMsg, ToolCall, ToolResult,
		ContextInjected, SessionCompacted:
		return true
	default:
		return false
	}
}
