package consistency

import "testing"

func TestConsensus_Majority(t *testing.T) {
	got, conf := Consensus([]string{"42", "42", "7"})
	if got != "42" || conf != 2.0/3.0 {
		t.Fatalf("Consensus = (%q, %v), want (42, 0.666)", got, conf)
	}
}

func TestConsensus_NormalizesCaseAndSpace(t *testing.T) {
	got, conf := Consensus([]string{"Yes", " yes ", "no"})
	if got != "Yes" || conf != 2.0/3.0 {
		t.Fatalf("Consensus = (%q, %v)", got, conf)
	}
}

func TestConsensus_TieBreaksFirst(t *testing.T) {
	got, _ := Consensus([]string{"a", "b", "a", "b"})
	// Both have count 2; the first-seen wins.
	if got != "a" {
		t.Fatalf("tie should break to first-seen, got %q", got)
	}
}

func TestConsensus_Empty(t *testing.T) {
	got, conf := Consensus(nil)
	if got != "" || conf != 0 {
		t.Fatalf("empty Consensus = (%q, %v), want (\"\", 0)", got, conf)
	}
}

func TestConsensus_Deterministic(t *testing.T) {
	in := []string{"one", "two", "one", "three", "two", "one"}
	a, _ := Consensus(in)
	b, _ := Consensus(in)
	if a != b {
		t.Fatalf("Consensus must be deterministic: %q vs %q", a, b)
	}
}

func TestConsensusBy_FirstLine(t *testing.T) {
	got, conf := ConsensusBy([]string{"PASS\nreason", "PASS\nother", "FAIL\nx"})
	if got != "PASS" || conf != 2.0/3.0 {
		t.Fatalf("ConsensusBy = (%q, %v)", got, conf)
	}
}
