package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// CouncilConfig controls multi-model consensus.
type CouncilConfig struct {
	Models     []string // models to consult (e.g., ["claude-sonnet", "gpt-4o", "gemini-pro"])
	Synthesize bool     // have a model synthesize responses (default: true)
	Evaluator  string   // model that evaluates/ranks responses
}

// CouncilResponse holds one model's contribution.
type CouncilResponse struct {
	Model    string
	Response string
	Score    float64 // evaluation score (0-1)
}

// RunCouncil sends a prompt to multiple models in parallel, collects responses,
// optionally evaluates/synthesizes them, and returns the best answer.
func RunCouncil(ctx context.Context, sess *Session, prompt string, cfg CouncilConfig) (string, []CouncilResponse, error) {
	if len(cfg.Models) == 0 {
		return "", nil, fmt.Errorf("council: no models specified")
	}

	// 1. Send prompt to all models in parallel
	responses := make([]CouncilResponse, len(cfg.Models))
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error

	for i, model := range cfg.Models {
		wg.Add(1)
		go func(idx int, modelName string) {
			defer wg.Done()

			resp, err := queryModel(ctx, sess, modelName, prompt)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("council: model %s: %w", modelName, err)
				}
				responses[idx] = CouncilResponse{Model: modelName, Response: fmt.Sprintf("(error: %v)", err)}
				return
			}
			responses[idx] = CouncilResponse{Model: modelName, Response: resp, Score: 0.5}
		}(i, model)
	}
	wg.Wait()

	// Filter out empty responses
	var valid []CouncilResponse
	for _, r := range responses {
		if r.Response != "" && !strings.HasPrefix(r.Response, "(error:") {
			valid = append(valid, r)
		}
	}

	if len(valid) == 0 {
		if firstErr != nil {
			return "", responses, firstErr
		}
		return "", responses, fmt.Errorf("council: all models returned empty responses")
	}

	// If only one valid response, return it directly
	if len(valid) == 1 {
		return valid[0].Response, responses, nil
	}

	// 2. If Synthesize: evaluate and synthesize
	if cfg.Synthesize {
		evaluator := cfg.Evaluator
		if evaluator == "" {
			evaluator = sess.Model()
		}

		synthesisPrompt := buildSynthesisPrompt(prompt, valid)
		synthesized, err := queryModel(ctx, sess, evaluator, synthesisPrompt)
		if err != nil {
			// Fall back to the first valid response
			return valid[0].Response, responses, nil
		}
		return synthesized, responses, nil
	}

	// No synthesis: return the first valid response
	return valid[0].Response, responses, nil
}

// buildSynthesisPrompt creates the prompt for the evaluator model.
func buildSynthesisPrompt(originalPrompt string, responses []CouncilResponse) string {
	var b strings.Builder
	b.WriteString("You received the following prompt:\n\n")
	b.WriteString(originalPrompt)
	b.WriteString("\n\nHere are ")
	b.WriteString(fmt.Sprintf("%d", len(responses)))
	b.WriteString(" responses from different models. Rank them by quality and synthesize the best answer.\n\n")

	for i, r := range responses {
		b.WriteString(fmt.Sprintf("--- Response %d (from %s) ---\n", i+1, r.Model))
		b.WriteString(r.Response)
		b.WriteString("\n\n")
	}

	b.WriteString("Synthesize the best elements of all responses into a single, high-quality answer. Be concise.")
	return b.String()
}

// queryModel queries a specific model using the session's client infrastructure.
// It creates a temporary session with the target model and collects the response.
func queryModel(ctx context.Context, sess *Session, modelName, prompt string) (string, error) {
	sub := NewSession(sess.Provider(), modelName, sess.system, sess.registry)
	sub.SetAPIKeys(sess.apiKeys)
	sub.MaxTurns = 1 // single turn, no tool use
	sub.AddUser(prompt)

	ch, err := sub.Stream(ctx)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for ev := range ch {
		switch ev.Type {
		case "content":
			b.WriteString(ev.Content)
		case "error":
			return b.String(), fmt.Errorf("%s", ev.Content)
		}
	}
	return b.String(), nil
}

// DefaultCouncilModels returns 3 diverse models from different providers.
func DefaultCouncilModels() []string {
	return []string{
		"claude-sonnet-4-20250514",
		"gpt-4o",
		"gemini-2.5-flash",
	}
}
