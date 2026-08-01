package engine

// mockMemoryRecaller is the minimal in-memory backend used by memory-service
// tests. It intentionally lives beside those tests rather than in the removed
// SessionServices compatibility test.
type mockMemoryRecaller struct{}

func (m *mockMemoryRecaller) Recall(query string, tokenBudget int) (string, error) {
	return "recalled: " + query, nil
}

func (m *mockMemoryRecaller) Remember(content, category string) error {
	return nil
}

var _ MemoryRecaller = (*mockMemoryRecaller)(nil)
