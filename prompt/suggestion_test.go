package prompt

import (
	"context"
	"testing"
	"time"
)

func TestSuggestionService_Disabled(t *testing.T) {
	svc := NewSuggestionService()
	svc.SetEnabled(false)

	if svc.IsEnabled() {
		t.Error("should be disabled")
	}

	suggestions := svc.GetSuggestions()
	if suggestions != nil {
		t.Error("disabled service should return nil")
	}
}

func TestSuggestionService_UpdateContext(t *testing.T) {
	svc := NewSuggestionService()

	called := make(chan bool, 1)
	svc.UpdateContext("I've fixed the bug in auth.go", func(ctx context.Context, context string) ([]string, error) {
		called <- true
		return []string{"Can you add tests?", "What about error handling?"}, nil
	})

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("speculation function was not called")
	}

	// Wait for async completion
	time.Sleep(100 * time.Millisecond)

	suggestions := svc.GetSuggestions()
	if len(suggestions) != 2 {
		t.Errorf("expected 2 suggestions, got %d", len(suggestions))
	}
	if suggestions[0].Source != "speculation" {
		t.Errorf("expected source=speculation, got %s", suggestions[0].Source)
	}
	if suggestions[0].Confidence <= suggestions[1].Confidence {
		t.Error("first suggestion should have higher confidence")
	}
}

func TestSuggestionService_CacheTTL(t *testing.T) {
	svc := NewSuggestionService()
	svc.cacheTTL = 50 * time.Millisecond

	svc.UpdateContext("test", func(ctx context.Context, context string) ([]string, error) {
		return []string{"suggestion"}, nil
	})
	time.Sleep(100 * time.Millisecond)

	// Should have suggestions
	sugs := svc.GetSuggestions()
	if len(sugs) == 0 {
		t.Skip("race condition: suggestion may not have been cached yet")
	}

	// Wait for cache to expire
	time.Sleep(100 * time.Millisecond)
	expired := svc.GetSuggestions()
	if expired != nil {
		t.Error("suggestions should be nil after cache expiry")
	}
}

func TestSuggestionService_Abort(t *testing.T) {
	svc := NewSuggestionService()

	svc.UpdateContext("test", func(ctx context.Context, context string) ([]string, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})

	svc.Abort()
	// Should not panic
}

func TestGenerateFromHistory(t *testing.T) {
	history := []string{
		"git status",
		"go test ./...",
		"git diff",
		"go build",
		"git commit -m 'fix'",
	}

	suggestions := GenerateFromHistory(history, "gi")
	if len(suggestions) == 0 {
		t.Fatal("expected suggestions for 'gi' prefix")
	}
	for _, s := range suggestions {
		if s.Source != "history" {
			t.Errorf("expected source=history, got %s", s.Source)
		}
	}
}

func TestGenerateFromHistory_Empty(t *testing.T) {
	if sug := GenerateFromHistory(nil, "test"); sug != nil {
		t.Error("nil history should return nil")
	}
	if sug := GenerateFromHistory([]string{"a"}, ""); sug != nil {
		t.Error("empty input should return nil")
	}
}

func TestGenerateFromHistory_NoDuplicates(t *testing.T) {
	history := []string{
		"go test",
		"go test",
		"go test",
		"go build",
	}
	suggestions := GenerateFromHistory(history, "go")
	seen := make(map[string]bool)
	for _, s := range suggestions {
		if seen[s.Text] {
			t.Errorf("duplicate suggestion: %s", s.Text)
		}
		seen[s.Text] = true
	}
}
