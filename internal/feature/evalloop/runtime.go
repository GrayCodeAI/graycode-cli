// Package evalloop evaluates the full hawk agent end-to-end by driving the real
// engine Session and tool loop against a task, rather than invoking an LLM
// directly. It runs in an isolated working directory and snapshots the session
// transcript as an eval artifact.
//
// The LLM backend is injected as an engine.ChatClient, so real evaluations use
// a provider-bound client while CI smoke runs use a deterministic mock client
// with no external credentials.
package evalloop

import (
	"context"
	"time"
)

// Event is a normalized view of one agent-loop step for evaluation reporting.
type Event struct {
	Type      string    `json:"type"` // "content", "error", "tool", ...
	Content   string    `json:"content,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Result is the outcome of one agent-runtime evaluation.
type Result struct {
	// Output is the concatenated assistant output produced by the loop.
	Output string `json:"output"`
	// Events are the normalized loop events in order.
	Events []Event `json:"events"`
	// TokensUsed and CostUSD are reported by the backend, when known.
	TokensUsed int     `json:"tokens_used"`
	CostUSD    float64 `json:"cost_usd"`
	// Transcript is the snapshot of the session's raw messages, for offline
	// replay of a failing run.
	Transcript []byte `json:"-"`
	// Duration is the wall-clock time of the run.
	Duration time.Duration `json:"duration"`
}

// Runtime executes one agent-runtime evaluation.
type Runtime interface {
	// Run drives the agent loop for prompt inside workDir and returns a Result.
	Run(ctx context.Context, workDir, prompt string) (Result, error)
}

// Config configures a SessionRuntime.
type Config struct {
	// SystemPrompt is injected as the session system prompt.
	SystemPrompt string
	// MaxTurns caps the agent loop (0 = engine default).
	MaxTurns int
}

// DefaultConfig returns a sane default configuration.
func DefaultConfig() Config {
	return Config{SystemPrompt: "You are an evaluation agent. Complete the requested task."}
}
