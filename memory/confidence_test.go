package memory

import "testing"

func TestConfidenceTracker_Basic(t *testing.T) {
	ct := NewConfidenceTracker(nil)
	if ct == nil {
		t.Fatal("nil")
	}
	ct.RecordAccess("node1", "node2")
	if ct.AccessedCount() != 2 {
		t.Errorf("AccessedCount = %d, want 2", ct.AccessedCount())
	}
	ct.Reset()
	if ct.AccessedCount() != 0 {
		t.Errorf("after Reset, AccessedCount = %d", ct.AccessedCount())
	}
}

func TestSortInjections(t *testing.T) {
	t.Parallel()
	sections := []MemoryInjection{
		{Content: "x", Priority: 1},
		{Content: "y", Priority: 10},
		{Content: "z", Priority: 5},
	}
	sortInjections(sections)
	if sections[0].Priority < sections[1].Priority {
		t.Error("should be sorted by priority descending")
	}
}
