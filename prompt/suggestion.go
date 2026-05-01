package prompt

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Suggestion represents a contextual prompt suggestion.
type Suggestion struct {
	Text       string `json:"text"`
	Confidence float64 `json:"confidence"`
	Source     string `json:"source"` // "speculation", "history", "context"
}

// SuggestionService generates prompt suggestions based on conversation context.
type SuggestionService struct {
	mu          sync.Mutex
	enabled     bool
	lastContext string
	cache       []Suggestion
	cacheTime   time.Time
	cacheTTL    time.Duration
	cancel      context.CancelFunc
}

// NewSuggestionService creates a new prompt suggestion service.
func NewSuggestionService() *SuggestionService {
	return &SuggestionService{
		enabled:  true,
		cacheTTL: 30 * time.Second,
	}
}

// SetEnabled enables or disables suggestions.
func (s *SuggestionService) SetEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
	if !enabled && s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

// IsEnabled returns whether suggestions are active.
func (s *SuggestionService) IsEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled
}

// GetSuggestions returns cached suggestions or empty if none available.
func (s *SuggestionService) GetSuggestions() []Suggestion {
	s.mu.Lock()
	defer s.mu.Unlock()

	if time.Since(s.cacheTime) > s.cacheTTL {
		return nil
	}
	return s.cache
}

// UpdateContext triggers suggestion generation based on new conversation context.
// speculateFn is called asynchronously to generate suggestions via LLM.
func (s *SuggestionService) UpdateContext(lastAssistant string, speculateFn func(ctx context.Context, context string) ([]string, error)) {
	s.mu.Lock()
	if !s.enabled {
		s.mu.Unlock()
		return
	}

	// Cancel any in-flight speculation
	if s.cancel != nil {
		s.cancel()
	}

	s.lastContext = lastAssistant
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	s.cancel = cancel
	s.mu.Unlock()

	go func() {
		defer cancel()
		suggestions, err := speculateFn(ctx, lastAssistant)
		if err != nil {
			return
		}

		s.mu.Lock()
		defer s.mu.Unlock()

		s.cache = make([]Suggestion, 0, len(suggestions))
		for i, text := range suggestions {
			s.cache = append(s.cache, Suggestion{
				Text:       text,
				Confidence: 1.0 - float64(i)*0.2,
				Source:     "speculation",
			})
		}
		s.cacheTime = time.Now()
	}()
}

// GenerateFromHistory produces suggestions based on command history patterns.
func GenerateFromHistory(history []string, currentInput string) []Suggestion {
	if len(history) == 0 || currentInput == "" {
		return nil
	}

	prefix := strings.ToLower(currentInput)
	var matches []Suggestion

	seen := make(map[string]bool)
	for i := len(history) - 1; i >= 0; i-- {
		h := history[i]
		if seen[h] {
			continue
		}
		if strings.HasPrefix(strings.ToLower(h), prefix) && h != currentInput {
			seen[h] = true
			matches = append(matches, Suggestion{
				Text:       h,
				Confidence: 0.8,
				Source:     "history",
			})
		}
		if len(matches) >= 5 {
			break
		}
	}
	return matches
}

// Abort cancels any in-flight suggestion generation.
func (s *SuggestionService) Abort() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}
