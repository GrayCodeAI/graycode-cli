package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// CouncilConfig controls the Karpathy LLM Council pattern.
type CouncilConfig struct {
	Models   []string // council member model names
	Chairman string   // chairman model (synthesizer)
}

// CouncilResponse holds one model's contribution.
type CouncilResponse struct {
	Model    string
	Response string
}

// CouncilRanking holds one model's ranking of responses.
type CouncilRanking struct {
	Model   string
	Ranking string
}

// CouncilResult holds the full council output.
type CouncilResult struct {
	Responses []CouncilResponse
	Rankings  []CouncilRanking
	Synthesis string
}

// RunCouncil implements Karpathy's 3-stage LLM Council pattern:
//  1. Send query to all models in parallel, collect responses
//  2. Anonymize responses, send ranking prompt to all models in parallel
//  3. Send all responses + rankings to chairman for synthesis
func RunCouncil(ctx context.Context, query string, cfg CouncilConfig, sess *Session) (*CouncilResult, error) {
	if len(cfg.Models) == 0 {
		return nil, fmt.Errorf("council: no models specified")
	}
	if cfg.Chairman == "" {
		cfg.Chairman = sess.Model()
	}

	// Stage 1: parallel query to all models
	responses, err := councilStage1(ctx, sess, query, cfg.Models)
	if err != nil {
		return nil, err
	}

	// Stage 2: parallel ranking by all models
	rankings, err := councilStage2(ctx, sess, query, responses, cfg.Models)
	if err != nil {
		return nil, err
	}

	// Stage 3: chairman synthesis
	synthesis, err := councilStage3(ctx, sess, query, responses, rankings, cfg.Chairman)
	if err != nil {
		return nil, err
	}

	return &CouncilResult{Responses: responses, Rankings: rankings, Synthesis: synthesis}, nil
}

// councilStage1 sends the query to all models in parallel.
func councilStage1(ctx context.Context, sess *Session, query string, models []string) ([]CouncilResponse, error) {
	responses := make([]CouncilResponse, len(models))
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error

	for i, model := range models {
		wg.Add(1)
		go func(idx int, m string) {
			defer wg.Done()
			resp, err := councilQuery(ctx, sess, m, query)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("council: model %s: %w", m, err)
				}
				responses[idx] = CouncilResponse{Model: m, Response: fmt.Sprintf("(error: %v)", err)}
			} else {
				responses[idx] = CouncilResponse{Model: m, Response: resp}
			}
		}(i, model)
	}
	wg.Wait()

	var valid int
	for _, r := range responses {
		if !strings.HasPrefix(r.Response, "(error:") {
			valid++
		}
	}
	if valid == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, fmt.Errorf("council: all models failed")
	}
	return responses, nil
}

// councilStage2 sends anonymized responses to all models for ranking.
func councilStage2(ctx context.Context, sess *Session, query string, responses []CouncilResponse, models []string) ([]CouncilRanking, error) {
	rankPrompt := buildRankingPrompt(query, responses)

	rankings := make([]CouncilRanking, len(models))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, model := range models {
		wg.Add(1)
		go func(idx int, m string) {
			defer wg.Done()
			resp, err := councilQuery(ctx, sess, m, rankPrompt)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				rankings[idx] = CouncilRanking{Model: m, Ranking: "(error)"}
			} else {
				rankings[idx] = CouncilRanking{Model: m, Ranking: resp}
			}
		}(i, model)
	}
	wg.Wait()
	return rankings, nil
}

// councilStage3 sends everything to the chairman for final synthesis.
func councilStage3(ctx context.Context, sess *Session, query string, responses []CouncilResponse, rankings []CouncilRanking, chairman string) (string, error) {
	prompt := buildChairmanPrompt(query, responses, rankings)
	return councilQuery(ctx, sess, chairman, prompt)
}

// buildRankingPrompt creates the Stage 2 ranking prompt (Karpathy's format).
func buildRankingPrompt(query string, responses []CouncilResponse) string {
	var b strings.Builder
	b.WriteString("You are evaluating multiple AI responses to the following question:\n\n")
	b.WriteString("QUESTION: " + query + "\n\n")
	b.WriteString("Here are the responses:\n\n")

	for i, r := range responses {
		label := string(rune('A' + i))
		b.WriteString(fmt.Sprintf("=== Response %s ===\n%s\n\n", label, r.Response))
	}

	b.WriteString("Please rank these responses from best to worst. Consider accuracy, completeness, clarity, and helpfulness.\n\n")
	b.WriteString("Provide a brief justification for your ranking, then end with your final ranking in this exact format:\n\n")
	b.WriteString("FINAL RANKING: [best to worst, e.g. B, A, C]\n")
	return b.String()
}

// buildChairmanPrompt creates the Stage 3 chairman synthesis prompt (Karpathy's format).
func buildChairmanPrompt(query string, responses []CouncilResponse, rankings []CouncilRanking) string {
	var b strings.Builder
	b.WriteString("You are the chairman of an LLM council. Your job is to synthesize the best possible answer.\n\n")
	b.WriteString("ORIGINAL QUESTION: " + query + "\n\n")

	b.WriteString("=== COUNCIL RESPONSES ===\n\n")
	for i, r := range responses {
		label := string(rune('A' + i))
		b.WriteString(fmt.Sprintf("--- Response %s (from %s) ---\n%s\n\n", label, r.Model, r.Response))
	}

	b.WriteString("=== COUNCIL RANKINGS ===\n\n")
	for _, r := range rankings {
		b.WriteString(fmt.Sprintf("--- Ranking by %s ---\n%s\n\n", r.Model, r.Ranking))
	}

	b.WriteString("Based on the responses and rankings above, synthesize the best possible answer to the original question. ")
	b.WriteString("Take the strongest elements from the highest-ranked responses. Be thorough and accurate.\n")
	return b.String()
}

// councilQuery queries a specific model using the session's client infrastructure.
func councilQuery(ctx context.Context, sess *Session, modelName, prompt string) (string, error) {
	sub := NewSession(sess.Provider(), modelName, sess.system, sess.registry)
	sub.SetAPIKeys(sess.apiKeys)
	sub.MaxTurns = 1
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

// DefaultCouncilModels returns diverse models from different providers.
func DefaultCouncilModels() []string {
	return []string{
		"claude-sonnet-4-20250514",
		"gpt-4o",
		"gemini-2.5-flash",
	}
}
