package diff

import (
	"strings"
	"testing"
)

func TestMerge3_CleanMerge(t *testing.T) {
	// Ours adds line at top, theirs adds line at bottom - no overlap
	base := "line1\nline2\nline3"
	ours := "new-top\nline1\nline2\nline3"
	theirs := "line1\nline2\nline3\nnew-bottom"

	result := Merge3(base, ours, theirs)

	if result.HasConflicts {
		t.Errorf("expected clean merge, got conflicts: %+v", result.Conflicts)
	}
	if result.Stats.ConflictCount != 0 {
		t.Errorf("expected 0 conflicts, got %d", result.Stats.ConflictCount)
	}
}

func TestMerge3_BothAddDifferentThings(t *testing.T) {
	// Ours modifies line1, theirs modifies line3 - different regions
	base := "line1\nline2\nline3"
	ours := "line1-modified\nline2\nline3"
	theirs := "line1\nline2\nline3-modified"

	result := Merge3(base, ours, theirs)

	if result.HasConflicts {
		t.Errorf("expected auto-merge, got conflicts: %+v", result.Conflicts)
	}

	// The merge should contain both modifications
	if !strings.Contains(result.Merged, "line1-modified") {
		t.Errorf("merged result should contain ours change, got: %s", result.Merged)
	}
	if !strings.Contains(result.Merged, "line3-modified") {
		t.Errorf("merged result should contain theirs change, got: %s", result.Merged)
	}
}

func TestMerge3_BothModifySameLine_Conflict(t *testing.T) {
	// Both modify the same line differently - conflict
	base := "line1\nline2\nline3"
	ours := "line1\nline2-ours\nline3"
	theirs := "line1\nline2-theirs\nline3"

	result := Merge3(base, ours, theirs)

	if !result.HasConflicts {
		t.Errorf("expected conflict, got clean merge")
	}
	if result.Stats.ConflictCount < 1 {
		t.Errorf("expected at least 1 conflict, got %d", result.Stats.ConflictCount)
	}

	// Verify conflict markers are present
	if !strings.Contains(result.Merged, "<<<<<<< ours") {
		t.Errorf("merged result should contain conflict markers, got: %s", result.Merged)
	}
	if !strings.Contains(result.Merged, ">>>>>>> theirs") {
		t.Errorf("merged result should contain end marker, got: %s", result.Merged)
	}
}

func TestMerge3_OneDeletesOtherModifies_Conflict(t *testing.T) {
	// Ours deletes line2, theirs modifies line2 - conflict
	base := "line1\nline2\nline3"
	ours := "line1\nline3"
	theirs := "line1\nline2-modified\nline3"

	result := Merge3(base, ours, theirs)

	if !result.HasConflicts {
		t.Errorf("expected conflict when one deletes and other modifies, got clean merge")
	}
}

func TestMerge3_ComplexMultiRegion(t *testing.T) {
	base := "header\nfunc a() {\n  return 1\n}\nfunc b() {\n  return 2\n}\nfooter"
	// Ours modifies func a
	ours := "header\nfunc a() {\n  return 10\n}\nfunc b() {\n  return 2\n}\nfooter"
	// Theirs modifies func b
	theirs := "header\nfunc a() {\n  return 1\n}\nfunc b() {\n  return 20\n}\nfooter"

	result := Merge3(base, ours, theirs)

	if result.HasConflicts {
		t.Errorf("expected clean multi-region merge, got conflicts: %+v", result.Conflicts)
	}

	// Both changes should be present
	if !strings.Contains(result.Merged, "return 10") {
		t.Errorf("merged should contain ours change (return 10), got: %s", result.Merged)
	}
	if !strings.Contains(result.Merged, "return 20") {
		t.Errorf("merged should contain theirs change (return 20), got: %s", result.Merged)
	}
}

func TestMerge3_EmptyInputs(t *testing.T) {
	// All empty
	result := Merge3("", "", "")
	if result.HasConflicts {
		t.Errorf("all empty should not have conflicts")
	}
	if result.Merged != "" {
		t.Errorf("all empty should produce empty merge, got: %q", result.Merged)
	}

	// Base empty, both add same thing
	result = Merge3("", "hello", "hello")
	if result.HasConflicts {
		t.Errorf("both adding same content should not conflict")
	}

	// Base empty, both add different things
	result = Merge3("", "hello", "world")
	if !result.HasConflicts {
		t.Errorf("both adding different content to empty base should conflict")
	}
}

func TestMerge3_IdenticalInputs(t *testing.T) {
	base := "line1\nline2\nline3"

	// All three identical - no-op
	result := Merge3(base, base, base)

	if result.HasConflicts {
		t.Errorf("identical inputs should not have conflicts")
	}
	if result.Merged != base {
		t.Errorf("identical inputs should return base, got: %q", result.Merged)
	}
}

func TestMerge3_BothMakeIdenticalChange(t *testing.T) {
	base := "line1\nline2\nline3"
	// Both make the same change
	modified := "line1\nline2-changed\nline3"

	result := Merge3(base, modified, modified)

	if result.HasConflicts {
		t.Errorf("identical changes should not conflict")
	}
	if result.Merged != modified {
		t.Errorf("identical changes should produce the changed version, got: %q", result.Merged)
	}
}

func TestLCS(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []string
		expected []string
	}{
		{
			name:     "identical",
			a:        []string{"a", "b", "c"},
			b:        []string{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "one empty",
			a:        []string{"a", "b"},
			b:        nil,
			expected: nil,
		},
		{
			name:     "no common",
			a:        []string{"a", "b"},
			b:        []string{"c", "d"},
			expected: nil,
		},
		{
			name:     "partial overlap",
			a:        []string{"a", "b", "c", "d"},
			b:        []string{"a", "c", "d", "e"},
			expected: []string{"a", "c", "d"},
		},
		{
			name:     "interleaved",
			a:        []string{"a", "x", "b", "y", "c"},
			b:        []string{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LCS(tt.a, tt.b)
			if !linesEqual(got, tt.expected) {
				t.Errorf("LCS(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.expected)
			}
		})
	}
}

func TestEditScript(t *testing.T) {
	from := []string{"a", "b", "c"}
	to := []string{"a", "x", "c"}

	edits := EditScript(from, to)

	// Should contain a delete of "b" and insert of "x"
	hasDelete := false
	hasInsert := false
	for _, e := range edits {
		if e.Type == "delete" && e.Line == "b" {
			hasDelete = true
		}
		if e.Type == "insert" && e.Line == "x" {
			hasInsert = true
		}
	}

	if !hasDelete {
		t.Errorf("edit script should delete 'b', got: %+v", edits)
	}
	if !hasInsert {
		t.Errorf("edit script should insert 'x', got: %+v", edits)
	}
}

func TestEditScript_EmptyFrom(t *testing.T) {
	edits := EditScript(nil, []string{"a", "b"})

	insertCount := 0
	for _, e := range edits {
		if e.Type == "insert" {
			insertCount++
		}
	}
	if insertCount != 2 {
		t.Errorf("expected 2 inserts, got %d in %+v", insertCount, edits)
	}
}

func TestEditScript_EmptyTo(t *testing.T) {
	edits := EditScript([]string{"a", "b"}, nil)

	deleteCount := 0
	for _, e := range edits {
		if e.Type == "delete" {
			deleteCount++
		}
	}
	if deleteCount != 2 {
		t.Errorf("expected 2 deletes, got %d in %+v", deleteCount, edits)
	}
}

func TestMergeClean(t *testing.T) {
	base := "a\nb\nc"

	// Clean merge
	merged, clean := MergeClean(base, "a\nb-ours\nc", "a\nb\nc-theirs")
	if !clean {
		t.Errorf("expected clean merge")
	}
	_ = merged

	// Conflict merge
	_, clean = MergeClean(base, "a\nb-ours\nc", "a\nb-theirs\nc")
	if clean {
		t.Errorf("expected conflict (not clean)")
	}
}

func TestFormatConflictMarkers(t *testing.T) {
	conflict := Diff3Conflict{
		BaseLines:   []string{"original line"},
		OursLines:   []string{"our change"},
		TheirsLines: []string{"their change"},
		StartLine:   5,
	}

	formatted := FormatConflictMarkers(conflict)

	if !strings.Contains(formatted, "<<<<<<< ours") {
		t.Errorf("missing ours marker")
	}
	if !strings.Contains(formatted, "||||||| base") {
		t.Errorf("missing base marker")
	}
	if !strings.Contains(formatted, "=======") {
		t.Errorf("missing separator")
	}
	if !strings.Contains(formatted, ">>>>>>> theirs") {
		t.Errorf("missing theirs marker")
	}
	if !strings.Contains(formatted, "our change") {
		t.Errorf("missing our change content")
	}
	if !strings.Contains(formatted, "original line") {
		t.Errorf("missing base content")
	}
	if !strings.Contains(formatted, "their change") {
		t.Errorf("missing their change content")
	}
}

func TestFormatDiff3Result(t *testing.T) {
	result := &Diff3Result{
		Stats: Diff3Stats{
			TotalLines:    50,
			ConflictCount: 1,
			AutoMerged:    45,
			OursOnly:      3,
			TheirsOnly:    2,
		},
		HasConflicts: true,
		Conflicts: []Diff3Conflict{
			{StartLine: 10, OursLines: []string{"a"}, TheirsLines: []string{"b"}, BaseLines: []string{"c"}},
		},
	}

	formatted := FormatDiff3Result(result)

	if !strings.Contains(formatted, "Three-way merge:") {
		t.Errorf("missing header")
	}
	if !strings.Contains(formatted, "Auto-merged: 45 lines") {
		t.Errorf("missing auto-merged stat, got: %s", formatted)
	}
	if !strings.Contains(formatted, "Conflicts: 1") {
		t.Errorf("missing conflict count, got: %s", formatted)
	}
	if !strings.Contains(formatted, "Ours-only changes: 3 lines") {
		t.Errorf("missing ours-only stat, got: %s", formatted)
	}
	if !strings.Contains(formatted, "Theirs-only changes: 2 lines") {
		t.Errorf("missing theirs-only stat, got: %s", formatted)
	}
}

func TestFormatDiff3Result_NoConflicts(t *testing.T) {
	result := &Diff3Result{
		Stats: Diff3Stats{
			TotalLines:    30,
			ConflictCount: 0,
			AutoMerged:    30,
			OursOnly:      5,
			TheirsOnly:    5,
		},
		HasConflicts: false,
	}

	formatted := FormatDiff3Result(result)
	if !strings.Contains(formatted, "Conflicts: 0") {
		t.Errorf("expected 'Conflicts: 0', got: %s", formatted)
	}
}

func TestDiff3Regions(t *testing.T) {
	base := []string{"a", "b", "c", "d"}
	ours := []string{"a", "B", "c", "d"}   // changed b -> B
	theirs := []string{"a", "b", "c", "D"} // changed d -> D

	regions := diff3Regions(base, ours, theirs)

	// Should have at least an unchanged, ours, unchanged, theirs pattern
	hasOurs := false
	hasTheirs := false
	for _, r := range regions {
		if r.Type == "ours" {
			hasOurs = true
		}
		if r.Type == "theirs" {
			hasTheirs = true
		}
	}
	if !hasOurs {
		t.Errorf("expected ours region, got regions: %+v", regions)
	}
	if !hasTheirs {
		t.Errorf("expected theirs region, got regions: %+v", regions)
	}
}
