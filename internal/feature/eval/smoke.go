package eval

// Smoke benchmarking: drive the real headless agent loop (Session.Stream)
// against a stub provider and score the run on steps and tokens. No API key
// is needed, so this doubles as a CI regression gate for the agent pipeline
// itself — the "scorecard" mode of `hawk eval`.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// SmokeSuite returns the headless agent-loop smoke tasks. Each task's
// ValidateFn checks the run's scorecard (steps, tool calls, tokens).
func SmokeSuite() *BenchmarkSuite {
	return &BenchmarkSuite{
		Name: "Agent-Loop Smoke (headless)",
		Tasks: []BenchmarkTask{
			taskSmokeReadFile(),
			taskSmokeNoTools(),
		},
	}
}

// SmokeMode is the duration budget for the smoke tasks.
const SmokeMode = 3 * time.Second

// SmokeScorecard is the JSON shape of a smoke run, passed to ValidateFn via
// the JSON written into the work directory.
type SmokeScorecard struct {
	Steps      int    `json:"steps"`
	ToolCalls  int    `json:"tool_calls"`
	TokensUsed int    `json:"tokens_used"`
	DurationMS int64  `json:"duration_ms"`
	Passed     bool   `json:"passed"`
	Error      string `json:"error,omitempty"`
}

// runSmokeStream drives engine.Session.Stream against a stub provider and
// writes the scorecard JSON into workDir, which the task's ValidateFn reads.
func runSmokeStream(ctx context.Context, workDir string, events []json.RawMessage) {
	start := time.Now()
	steps, toolCalls, tokens, err := streamHeadless(ctx, events)
	card := SmokeScorecard{
		Steps:      steps,
		ToolCalls:  toolCalls,
		TokensUsed: tokens,
		DurationMS: time.Since(start).Milliseconds(),
		Passed:     err == nil,
	}
	if err != nil {
		card.Error = err.Error()
	}
	_ = os.WriteFile(workDir+"/scorecard.json", mustJSON(card), 0o600)
}

// taskSmokeReadFile scores a read+answer task: the loop must terminate and
// emit at least one Read tool call.
func taskSmokeReadFile() BenchmarkTask {
	return BenchmarkTask{
		ID:          "smoke-read-file",
		Description: "Headless agent loop: read a file via the Read tool",
		Prompt:      "Read internal/engine/stream.go and describe the retry timer.",
		TimeLimit:   SmokeMode,
		Tags:        []string{"smoke", "agent-loop"},
		SetupFn: func(workDir string) error {
			// Drive the real agent loop with a stub provider that asks for a
			// Read tool call, then answers.
			runSmokeStream(context.Background(), workDir, []json.RawMessage{
				json.RawMessage(`{"type":"tool_call","tool_call":{"name":"Read","arguments":{"path":"internal/engine/stream.go"}}}`),
				json.RawMessage(`{"type":"content","content":"done"}`),
				json.RawMessage(`{"type":"done","stop_reason":"end_turn"}`),
			})
			return nil
		},
		ValidateFn: func(workDir string) (bool, string) {
			card, err := loadSmokeCard(workDir)
			if err != nil {
				return false, err.Error()
			}
			if !card.Passed {
				return false, "stream did not terminate cleanly: " + card.Error
			}
			if card.ToolCalls < 1 {
				return false, fmt.Sprintf("expected >= 1 tool call, got %d", card.ToolCalls)
			}
			return true, ""
		},
	}
}

// taskSmokeNoTools scores a no-tool answer task: the loop must terminate.
func taskSmokeNoTools() BenchmarkTask {
	return BenchmarkTask{
		ID:          "smoke-no-tools",
		Description: "Headless agent loop: answer without tools",
		Prompt:      "What is the capital of France?",
		TimeLimit:   SmokeMode,
		Tags:        []string{"smoke", "agent-loop"},
		SetupFn: func(workDir string) error {
			// Stub provider answers directly; the loop should end in one turn.
			runSmokeStream(context.Background(), workDir, []json.RawMessage{
				json.RawMessage(`{"type":"content","content":"Paris"}`),
				json.RawMessage(`{"type":"done","stop_reason":"end_turn"}`),
			})
			return nil
		},
		ValidateFn: func(workDir string) (bool, string) {
			card, err := loadSmokeCard(workDir)
			if err != nil {
				return false, err.Error()
			}
			if !card.Passed {
				return false, "stream did not terminate cleanly: " + card.Error
			}
			return true, ""
		},
	}
}

func loadSmokeCard(workDir string) (*SmokeScorecard, error) {
	data, err := os.ReadFile(workDir + "/scorecard.json")
	if err != nil {
		return nil, fmt.Errorf("read scorecard: %w", err)
	}
	var card SmokeScorecard
	if err := json.Unmarshal(data, &card); err != nil {
		return nil, fmt.Errorf("parse scorecard: %w", err)
	}
	return &card, nil
}

func mustJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
