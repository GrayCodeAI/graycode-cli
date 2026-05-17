package analytics

import "testing"

func TestComputeSessionHealth_Perfect(t *testing.T) {
	h := ComputeSessionHealth(10, 0, 0, 0, 0, "success")
	if h.Score != 100 || h.Grade != "A" {
		t.Fatalf("expected 100/A, got %d/%s", h.Score, h.Grade)
	}
	if len(h.Signals) != 0 {
		t.Fatalf("expected no signals, got %v", h.Signals)
	}
}

func TestComputeSessionHealth_WithErrors(t *testing.T) {
	h := ComputeSessionHealth(10, 2, 1, 1, 0, "success")
	// -10 (errors) -5 (retry) -2 (compaction) = 83
	if h.Score != 83 || h.Grade != "B" {
		t.Fatalf("expected 83/B, got %d/%s", h.Score, h.Grade)
	}
}

func TestComputeSessionHealth_Errored(t *testing.T) {
	h := ComputeSessionHealth(5, 3, 2, 1, 1, "errored")
	// -30 -15 -10 -2 -8 = -65 → 35
	expected := 100 - 30 - 15 - 10 - 2 - 8
	if h.Score != expected || h.Grade != "F" {
		t.Fatalf("expected %d/F, got %d/%s", expected, h.Score, h.Grade)
	}
}

func TestDetectMidTaskCompaction_True(t *testing.T) {
	if !DetectMidTaskCompaction([]string{"Edit", "Bash", "Read"}, []string{"Grep", "Edit"}) {
		t.Fatal("expected true for overlapping tools")
	}
}

func TestDetectMidTaskCompaction_False(t *testing.T) {
	if DetectMidTaskCompaction([]string{"Edit", "Bash"}, []string{"Read", "Grep"}) {
		t.Fatal("expected false for non-overlapping tools")
	}
}
