package planning

import (
	"fmt"
	"testing"
)

func TestClassifyRadius(t *testing.T) {
	tests := []struct {
		count int
		want  BlastRadius
	}{
		{0, RadiusSmall},
		{1, RadiusSmall},
		{3, RadiusSmall},
		{4, RadiusMedium},
		{10, RadiusMedium},
		{11, RadiusLarge},
		{25, RadiusLarge},
		{26, RadiusHuge},
		{100, RadiusHuge},
	}
	for _, tc := range tests {
		got := classifyRadius(tc.count)
		if got != tc.want {
			t.Errorf("classifyRadius(%d) = %v, want %v", tc.count, got, tc.want)
		}
	}
}

func TestBlastRadiusString(t *testing.T) {
	tests := []struct {
		r    BlastRadius
		want string
	}{
		{RadiusSmall, "small"},
		{RadiusMedium, "medium"},
		{RadiusLarge, "large"},
		{RadiusHuge, "huge"},
	}
	for _, tc := range tests {
		if got := tc.r.String(); got != tc.want {
			t.Errorf("BlastRadius(%d).String() = %q, want %q", tc.r, got, tc.want)
		}
	}
}

func TestBlastRadiusEmoji(t *testing.T) {
	if RadiusSmall.Emoji() != "LOW" {
		t.Errorf("expected LOW token for small, got %q", RadiusSmall.Emoji())
	}
	if RadiusHuge.Emoji() != "CRIT" {
		t.Errorf("expected CRIT token for huge, got %q", RadiusHuge.Emoji())
	}
}

func TestBlastRadiusFlags(t *testing.T) {
	if RadiusSmall.NeedsConfirmation() {
		t.Error("small should not need confirmation")
	}
	if !RadiusMedium.NeedsConfirmation() {
		t.Error("medium should need confirmation")
	}
	if RadiusSmall.NeedsValidation() {
		t.Error("small should not need validation")
	}
	if !RadiusLarge.NeedsValidation() {
		t.Error("large should need validation")
	}
	if RadiusLarge.SuggestsDecomposition() {
		t.Error("large should not suggest decomposition")
	}
	if !RadiusHuge.SuggestsDecomposition() {
		t.Error("huge should suggest decomposition")
	}
}

func TestEstimateBlastRadiusEmpty(t *testing.T) {
	report := EstimateBlastRadius(nil)
	if report.FileCount != 0 {
		t.Errorf("expected 0 files, got %d", report.FileCount)
	}
	if report.Radius != RadiusSmall {
		t.Errorf("expected small radius, got %v", report.Radius)
	}
}

func TestEstimateBlastRadiusSmall(t *testing.T) {
	calls := []PlannedCall{
		{ToolName: "Edit", Targets: []string{"cmd/main.go"}},
		{ToolName: "Edit", Targets: []string{"cmd/main.go"}}, // duplicate
		{ToolName: "Read", Targets: []string{"go.mod"}},
	}
	report := EstimateBlastRadius(calls)
	if report.FileCount != 2 {
		t.Errorf("expected 2 unique files, got %d", report.FileCount)
	}
	if report.Radius != RadiusSmall {
		t.Errorf("expected small radius, got %v", report.Radius)
	}
}

func TestEstimateBlastRadiusLarge(t *testing.T) {
	calls := make([]PlannedCall, 15)
	for i := range calls {
		calls[i] = PlannedCall{
			ToolName: "Write",
			Targets:  []string{fmt.Sprintf("pkg/module%d.go", i)},
		}
	}
	report := EstimateBlastRadius(calls)
	if report.FileCount != 15 {
		t.Errorf("expected 15 files, got %d", report.FileCount)
	}
	if report.Radius != RadiusLarge {
		t.Errorf("expected large radius, got %v", report.Radius)
	}
	if !report.Radius.NeedsValidation() {
		t.Error("large radius should need validation")
	}
}

func TestEstimateBlastRadiusFileTypes(t *testing.T) {
	calls := []PlannedCall{
		{ToolName: "Write", Targets: []string{"main.go", "main_test.go", "README.md", "config.yml"}},
	}
	report := EstimateBlastRadius(calls)
	if !report.HasTests {
		t.Error("should detect test files")
	}
	if !report.HasDocs {
		t.Error("should detect doc files")
	}
	if !report.HasConfig {
		t.Error("should detect config files")
	}
	if report.FileTypes[".go"] != 2 {
		t.Errorf("expected 2 .go files, got %d", report.FileTypes[".go"])
	}
}

func TestEstimateBlastRadiusDirs(t *testing.T) {
	calls := []PlannedCall{
		{ToolName: "Write", Targets: []string{"cmd/chat.go", "cmd/main.go", "internal/engine/stream.go"}},
	}
	report := EstimateBlastRadius(calls)
	if len(report.DirsAffected) != 2 {
		t.Errorf("expected 2 directories, got %d", len(report.DirsAffected))
	}
}

func TestBlastRadiusMessage(t *testing.T) {
	report := EstimateBlastRadius([]PlannedCall{
		{ToolName: "Edit", Targets: []string{"a.go"}},
	})
	if report.Message == "" {
		t.Error("message should not be empty")
	}
}
