package hunktracker

import (
	"testing"
)

func TestComputeHunksBasic(t *testing.T) {
	base := "a\nb\nc\nd\ne\n"
	cur := "a\nb\nX\nY\nd\ne\n"
	hunks := computeHunks(base, cur)
	if len(hunks) != 1 {
		t.Fatalf("hunks = %+v", hunks)
	}
	h := hunks[0]
	if h.StartLine != 3 || h.Lines != 2 || h.Text != "X\nY" {
		t.Fatalf("hunk = %+v", h)
	}
}

func TestTrackerAttributionAndIdentity(t *testing.T) {
	tr := NewTracker()
	base := "line1\nline2\nline3\n"
	tr.Track("f.go", base)

	hs, ok := tr.Update("f.go", "line1\nAGENT\nline3\n", AuthorAgent)
	if !ok || len(hs) != 1 || hs[0].Author != AuthorAgent {
		t.Fatalf("agent update: %+v ok=%v", hs, ok)
	}

	// External edit adjacent to the agent region: the new change hunk overlaps
	// the previously agent-authored position, so attribution stays "agent".
	hs, _ = tr.Update("f.go", "line1\nTOP\nAGENT\nline3\n", AuthorExternal)
	found := false
	for _, h := range hs {
		if h.StartLine == 2 && h.Lines >= 1 {
			found = true
			if h.Author != AuthorAgent {
				t.Fatalf("agent attribution lost after external edit: %+v", hs)
			}
		}
	}
	if !found {
		t.Fatalf("expected an overlapping hunk near line 2: %+v", hs)
	}
}

func TestTrackerUntrackedIgnored(t *testing.T) {
	tr := NewTracker()
	if hs, ok := tr.Update("nope", "x", AuthorAgent); ok || hs != nil {
		t.Fatal("untracked file must be ignored")
	}
}

func TestAgentTouchedFiles(t *testing.T) {
	tr := NewTracker()
	tr.Track("a.go", "1\n")
	tr.Track("b.go", "2\n")
	if _, ok := tr.Update("a.go", "1\nx\n", AuthorAgent); !ok {
		t.Fatal("update a failed")
	}
	if _, ok := tr.Update("b.go", "2\ny\n", AuthorExternal); !ok {
		t.Fatal("update b failed")
	}
	got := tr.AgentTouchedFiles()
	if len(got) != 1 || got[0] != "a.go" {
		t.Fatalf("AgentTouchedFiles = %v", got)
	}
}

func TestHunkIDDeterministic(t *testing.T) {
	// Identical (baseline, current) pairs must yield identical IDs.
	a := computeHunks("x\ny\n", "x\nNEW\ny\n")
	b := computeHunks("x\ny\n", "x\nNEW\ny\n")
	if len(a) != 1 || len(b) != 1 || a[0].ID != b[0].ID {
		t.Fatalf("IDs differ for identical diffs: %v vs %v", a, b)
	}
}

func TestComputeHunksEmptyBaseline(t *testing.T) {
	hunks := computeHunks("", "one\ntwo\n")
	if len(hunks) != 1 || hunks[0].StartLine != 1 || hunks[0].Lines != 2 {
		t.Fatalf("hunks = %+v", hunks)
	}
}

func TestComputeHunksNoChange(t *testing.T) {
	s := "a\nb\n"
	if hunks := computeHunks(s, s); len(hunks) != 0 {
		t.Fatalf("expected no hunks, got %+v", hunks)
	}
}
