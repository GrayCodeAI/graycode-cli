package planning

import (
	"context"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// mockProvider returns canned expand/score responses keyed by the user content.
type mockProvider struct {
	expand map[string]string
	scores map[string]string
}

func (m *mockProvider) Chat(ctx context.Context, msgs []types.GraycodeRouterMessage, opts types.ChatOptions) (*types.GraycodeRouterResponse, error) {
	user := ""
	for _, mm := range msgs {
		if mm.Role == "user" {
			user = mm.Content
		}
	}
	if e, ok := m.expand[user]; ok {
		return &types.GraycodeRouterResponse{Content: e}, nil
	}
	if s, ok := m.scores[user]; ok {
		return &types.GraycodeRouterResponse{Content: s}, nil
	}
	return &types.GraycodeRouterResponse{Content: "0.5"}, nil
}

func (m *mockProvider) StreamChat(ctx context.Context, msgs []types.GraycodeRouterMessage, opts types.ChatOptions) (*types.StreamResult, error) {
	return nil, nil
}
func (m *mockProvider) Ping(ctx context.Context) error { return nil }
func (m *mockProvider) Name() string                   { return "mock" }

func TestModelExpander(t *testing.T) {
	p := &mockProvider{expand: map[string]string{
		"plan root": "1. step A\n2. step B\n3. step A\n",
	}}
	e := &ModelExpander{Provider: p, Model: "mock", Prompt: "expand"}
	got := e.Expand("plan root")
	if len(got) != 2 {
		t.Fatalf("expected 2 deduped candidates, got %v", got)
	}
	if got[0] != "step A" || got[1] != "step B" {
		t.Fatalf("unexpected candidates: %v", got)
	}
}

func TestModelScorer(t *testing.T) {
	p := &mockProvider{scores: map[string]string{"good plan": "0.9"}}
	s := &ModelScorer{Provider: p, Model: "mock", Prompt: "score"}
	if got := s.Score("good plan"); got != 0.9 {
		t.Fatalf("Score = %v, want 0.9", got)
	}
}

func TestPlanWithBeamSearch(t *testing.T) {
	p := &mockProvider{
		expand: map[string]string{
			"root": "1. a\n2. b\n",
			"a":    "1. a1\n2. a2\n",
			"b":    "1. b1\n",
			"a1":   "1. a1x\n",
			"b1":   "1. b1x\n",
		},
		scores: map[string]string{
			"a": "0.8", "b": "0.6",
			"a1": "0.9", "a2": "0.4",
			"b1":  "0.7",
			"a1x": "0.95", "b1x": "0.3",
		},
	}
	best, err := PlanWithBeamSearch(p, "mock", "root", "expand", "score", 2, 3)
	if err != nil {
		t.Fatalf("PlanWithBeamSearch: %v", err)
	}
	// The search should prefer the high-scoring a1x branch.
	if !strings.Contains(best, "a1x") {
		t.Fatalf("beam search did not reach best branch, got %q", best)
	}
}

func TestPlanWithBeamSearchNilProvider(t *testing.T) {
	if _, err := PlanWithBeamSearch(nil, "m", "r", "e", "s", 2, 2); err == nil {
		t.Fatal("expected error for nil provider")
	}
}
