package session

import "testing"

// mockForkStore is a minimal in-memory ForkableStore for tests.
type mockForkStore struct {
	createdName string
	copiedFrom  string
	copiedTo    string
	copiedUpTo  string
}

func (m *mockForkStore) GetForkCheckpoints(string) ([]ForkCheckpoint, error) { return nil, nil }
func (m *mockForkStore) CreateThread(name string) (string, error) {
	m.createdName = name
	return "new-thread-id", nil
}

func (m *mockForkStore) CopyCheckpoints(from, to, upTo string) error {
	m.copiedFrom, m.copiedTo, m.copiedUpTo = from, to, upTo
	return nil
}

// TestForkThread_ShortSourceID guards against a slice-out-of-range panic when
// the source thread ID is shorter than 8 characters and no name is supplied
// (the auto-generated name used to take SourceThreadID[:8] unconditionally).
func TestForkThread_ShortSourceID(t *testing.T) {
	for _, id := range []string{"a", "abc", "1234567", "12345678", "a-longer-thread-id"} {
		store := &mockForkStore{}
		fork, err := ForkThread(store, ForkOptions{SourceThreadID: id})
		if err != nil {
			t.Fatalf("ForkThread(%q) error: %v", id, err)
		}
		if fork.NewThreadID != "new-thread-id" {
			t.Errorf("ForkThread(%q): unexpected new thread id %q", id, fork.NewThreadID)
		}
		if store.createdName == "" {
			t.Errorf("ForkThread(%q): expected an auto-generated thread name", id)
		}
	}
}

func TestForkThread_RequiresSourceID(t *testing.T) {
	if _, err := ForkThread(&mockForkStore{}, ForkOptions{}); err == nil {
		t.Error("expected error for empty source thread ID")
	}
}
