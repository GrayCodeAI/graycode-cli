package diff

import (
	"strings"
	"testing"
)

func TestRecordChangeCapturesDiff(t *testing.T) {
	dp := NewDiffPreview()

	old := "line1\nline2\nline3\n"
	new := "line1\nmodified\nline3\n"

	dp.RecordChange("src/file.go", old, new)

	if len(dp.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(dp.Changes))
	}

	change := dp.Changes[0]
	if change.Path != "src/file.go" {
		t.Errorf("expected path src/file.go, got %s", change.Path)
	}
	if change.Type != "modified" {
		t.Errorf("expected type modified, got %s", change.Type)
	}
	if len(change.Hunks) == 0 {
		t.Error("expected at least one hunk")
	}
}

func TestComputeDiffFindsAdditionsAndDeletions(t *testing.T) {
	old := "a\nb\nc\nd\n"
	new := "a\nb\nX\nd\n"

	hunks := ComputeDiff(old, new)
	if len(hunks) == 0 {
		t.Fatal("expected at least one hunk")
	}

	var additions, deletions int
	for _, hunk := range hunks {
		for _, line := range hunk.Lines {
			switch line.Type {
			case "add":
				additions++
			case "remove":
				deletions++
			}
		}
	}

	if additions != 1 {
		t.Errorf("expected 1 addition, got %d", additions)
	}
	if deletions != 1 {
		t.Errorf("expected 1 deletion, got %d", deletions)
	}
}

func TestRenderUnifiedFormat(t *testing.T) {
	dp := NewDiffPreview()
	old := "line1\nline2\nline3\n"
	new := "line1\nchanged\nline3\n"

	dp.RecordChange("src/auth.go", old, new)

	output := RenderUnified(&dp.Changes[0])

	if !strings.Contains(output, "--- a/src/auth.go") {
		t.Error("missing old file header")
	}
	if !strings.Contains(output, "+++ b/src/auth.go") {
		t.Error("missing new file header")
	}
	if !strings.Contains(output, "@@") {
		t.Error("missing hunk header")
	}
	if !strings.Contains(output, "-line2") {
		t.Error("missing removed line")
	}
	if !strings.Contains(output, "+changed") {
		t.Error("missing added line")
	}
}

func TestRenderSummaryMultipleFiles(t *testing.T) {
	dp := NewDiffPreview()

	dp.RecordChange("src/auth.go", "a\nb\nc\n", "a\nb\nc\nd\ne\n")
	dp.RecordChange("src/middleware.go", "", "package middleware\n\nfunc New() {}\n")
	dp.RecordChange("src/old_auth.go", "package old\n\nfunc Old() {}\n", "")

	summary := dp.RenderSummary()

	if !strings.Contains(summary, "Pending Changes:") {
		t.Error("missing header")
	}
	if !strings.Contains(summary, "M src/auth.go") {
		t.Error("missing modified file")
	}
	if !strings.Contains(summary, "A src/middleware.go") {
		t.Error("missing added file")
	}
	if !strings.Contains(summary, "D src/old_auth.go") {
		t.Error("missing deleted file")
	}
	if !strings.Contains(summary, "Total: 3 files") {
		t.Error("missing total line")
	}
}

func TestApproveRejectWorkflow(t *testing.T) {
	dp := NewDiffPreview()
	dp.RecordChange("a.go", "old\n", "new\n")
	dp.RecordChange("b.go", "old\n", "new\n")
	dp.RecordChange("c.go", "old\n", "new\n")

	// Initially all are pending
	pending := dp.GetPending()
	if len(pending) != 3 {
		t.Fatalf("expected 3 pending, got %d", len(pending))
	}

	// Approve one
	dp.Approve("a.go")
	approved := dp.GetApproved()
	if len(approved) != 1 {
		t.Errorf("expected 1 approved, got %d", len(approved))
	}
	if approved[0].Path != "a.go" {
		t.Errorf("expected a.go approved, got %s", approved[0].Path)
	}

	// Reject one
	dp.Reject("b.go", "needs refactoring")
	pending = dp.GetPending()
	if len(pending) != 1 {
		t.Errorf("expected 1 pending, got %d", len(pending))
	}
	if pending[0].Path != "c.go" {
		t.Errorf("expected c.go pending, got %s", pending[0].Path)
	}

	// Check rejected has comment
	for _, change := range dp.Changes {
		if change.Path == "b.go" {
			if !change.Rejected {
				t.Error("b.go should be rejected")
			}
			if change.Comment != "needs refactoring" {
				t.Errorf("expected comment 'needs refactoring', got '%s'", change.Comment)
			}
		}
	}
}

func TestApproveAll(t *testing.T) {
	dp := NewDiffPreview()
	dp.RecordChange("a.go", "x\n", "y\n")
	dp.RecordChange("b.go", "x\n", "y\n")

	dp.ApproveAll()

	approved := dp.GetApproved()
	if len(approved) != 2 {
		t.Errorf("expected 2 approved, got %d", len(approved))
	}
	pending := dp.GetPending()
	if len(pending) != 0 {
		t.Errorf("expected 0 pending, got %d", len(pending))
	}
}

func TestRejectAll(t *testing.T) {
	dp := NewDiffPreview()
	dp.RecordChange("a.go", "x\n", "y\n")
	dp.RecordChange("b.go", "x\n", "y\n")

	dp.RejectAll("not ready")

	pending := dp.GetPending()
	if len(pending) != 0 {
		t.Errorf("expected 0 pending, got %d", len(pending))
	}
	for _, change := range dp.Changes {
		if !change.Rejected {
			t.Errorf("expected %s to be rejected", change.Path)
		}
		if change.Comment != "not ready" {
			t.Errorf("expected comment 'not ready', got '%s'", change.Comment)
		}
	}
}

func TestDiffPreviewStatsCalculation(t *testing.T) {
	dp := NewDiffPreview()
	old := "line1\nline2\nline3\n"
	new := "line1\nmodified\nnewline\nline3\n"

	dp.RecordChange("file.go", old, new)

	change := dp.Changes[0]
	if change.Stats.Additions < 1 {
		t.Errorf("expected at least 1 addition, got %d", change.Stats.Additions)
	}
	if change.Stats.Deletions < 1 {
		t.Errorf("expected at least 1 deletion, got %d", change.Stats.Deletions)
	}
	if change.Stats.NetChange != change.Stats.Additions-change.Stats.Deletions {
		t.Error("net change should be additions minus deletions")
	}
}

func TestEmptyDiffNoChanges(t *testing.T) {
	content := "same content\n"
	hunks := ComputeDiff(content, content)

	if len(hunks) != 0 {
		t.Errorf("expected 0 hunks for identical content, got %d", len(hunks))
	}
}

func TestNewFileAllAdditions(t *testing.T) {
	dp := NewDiffPreview()
	dp.RecordChange("new.go", "", "package new\n\nfunc Hello() {}\n")

	change := dp.Changes[0]
	if change.Type != "created" {
		t.Errorf("expected type created, got %s", change.Type)
	}
	if change.Stats.Additions == 0 {
		t.Error("expected additions for new file")
	}
	if change.Stats.Deletions != 0 {
		t.Errorf("expected 0 deletions for new file, got %d", change.Stats.Deletions)
	}
}

func TestDeletedFileAllDeletions(t *testing.T) {
	dp := NewDiffPreview()
	dp.RecordChange("old.go", "package old\n\nfunc Bye() {}\n", "")

	change := dp.Changes[0]
	if change.Type != "deleted" {
		t.Errorf("expected type deleted, got %s", change.Type)
	}
	if change.Stats.Deletions == 0 {
		t.Error("expected deletions for deleted file")
	}
	if change.Stats.Additions != 0 {
		t.Errorf("expected 0 additions for deleted file, got %d", change.Stats.Additions)
	}
}

func TestLargeDiffMultipleHunks(t *testing.T) {
	// Create content with changes far apart so they form separate hunks
	var oldLines []string
	var newLines []string
	for i := 0; i < 50; i++ {
		oldLines = append(oldLines, "unchanged line")
		newLines = append(newLines, "unchanged line")
	}
	// Change at line 5
	oldLines[4] = "old line 5"
	newLines[4] = "new line 5"
	// Change at line 45 (far enough for separate hunk with context of 3)
	oldLines[44] = "old line 45"
	newLines[44] = "new line 45"

	old := strings.Join(oldLines, "\n") + "\n"
	new := strings.Join(newLines, "\n") + "\n"

	hunks := ComputeDiff(old, new)
	if len(hunks) < 2 {
		t.Errorf("expected at least 2 hunks for distant changes, got %d", len(hunks))
	}
}

func TestMyersAlgorithmKnownInputs(t *testing.T) {
	tests := []struct {
		name    string
		a       []string
		b       []string
		wantAdd int
		wantDel int
	}{
		{
			name:    "single insertion",
			a:       []string{"a", "b"},
			b:       []string{"a", "X", "b"},
			wantAdd: 1,
			wantDel: 0,
		},
		{
			name:    "single deletion",
			a:       []string{"a", "X", "b"},
			b:       []string{"a", "b"},
			wantAdd: 0,
			wantDel: 1,
		},
		{
			name:    "substitution",
			a:       []string{"a", "b", "c"},
			b:       []string{"a", "X", "c"},
			wantAdd: 1,
			wantDel: 1,
		},
		{
			name:    "all different",
			a:       []string{"a", "b"},
			b:       []string{"c", "d"},
			wantAdd: 2,
			wantDel: 2,
		},
		{
			name:    "empty to something",
			a:       []string{},
			b:       []string{"a", "b", "c"},
			wantAdd: 3,
			wantDel: 0,
		},
		{
			name:    "something to empty",
			a:       []string{"a", "b", "c"},
			b:       []string{},
			wantAdd: 0,
			wantDel: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := ComputeMyersDiff(tt.a, tt.b)

			var adds, dels int
			for _, line := range lines {
				switch line.Type {
				case "add":
					adds++
				case "remove":
					dels++
				}
			}

			if adds != tt.wantAdd {
				t.Errorf("expected %d additions, got %d", tt.wantAdd, adds)
			}
			if dels != tt.wantDel {
				t.Errorf("expected %d deletions, got %d", tt.wantDel, dels)
			}
		})
	}
}

func TestContextLinesAroundChanges(t *testing.T) {
	// With 10 lines and a change at line 5, we should see context lines around it
	old := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"
	new := "1\n2\n3\n4\nFIVE\n6\n7\n8\n9\n10\n"

	hunks := ComputeDiff(old, new)
	if len(hunks) == 0 {
		t.Fatal("expected at least one hunk")
	}

	hunk := hunks[0]
	// Should have context lines
	hasContext := false
	for _, line := range hunk.Lines {
		if line.Type == "context" {
			hasContext = true
			break
		}
	}
	if !hasContext {
		t.Error("expected context lines around the change")
	}
}

func TestClearEmptiesState(t *testing.T) {
	dp := NewDiffPreview()
	dp.RecordChange("a.go", "old\n", "new\n")
	dp.RecordChange("b.go", "old\n", "new\n")

	if len(dp.Changes) != 2 {
		t.Fatalf("expected 2 changes before clear, got %d", len(dp.Changes))
	}

	dp.Clear()

	if len(dp.Changes) != 0 {
		t.Errorf("expected 0 changes after clear, got %d", len(dp.Changes))
	}

	pending := dp.GetPending()
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after clear, got %d", len(pending))
	}
}

func TestNewDiffPreviewHasSessionID(t *testing.T) {
	dp := NewDiffPreview()
	if dp.SessionID == "" {
		t.Error("expected non-empty session ID")
	}
	if len(dp.SessionID) != 16 {
		t.Errorf("expected 16 char hex session ID, got %d chars", len(dp.SessionID))
	}
}

func TestRenderAllCombinesDiffs(t *testing.T) {
	dp := NewDiffPreview()
	dp.RecordChange("a.go", "old\n", "new\n")
	dp.RecordChange("b.go", "foo\n", "bar\n")

	output := dp.RenderAll()

	if !strings.Contains(output, "a/a.go") {
		t.Error("missing first file in combined diff")
	}
	if !strings.Contains(output, "a/b.go") {
		t.Error("missing second file in combined diff")
	}
}
