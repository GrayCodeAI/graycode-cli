package engine

import (
	"strings"
	"testing"
)

func TestNewRiskAssessor(t *testing.T) {
	ra := NewRiskAssessor()
	if ra == nil {
		t.Fatal("NewRiskAssessor returned nil")
	}
	if len(ra.Factors) != 6 {
		t.Errorf("expected 6 built-in factors, got %d", len(ra.Factors))
	}

	// Verify total weight sums to 1.0
	totalWeight := 0.0
	for _, f := range ra.Factors {
		totalWeight += f.Weight
	}
	if totalWeight < 0.99 || totalWeight > 1.01 {
		t.Errorf("expected total weight ~1.0, got %f", totalWeight)
	}
}

func TestAssess_LowRisk(t *testing.T) {
	ra := NewRiskAssessor()
	ctx := &RiskContext{
		Files:           []string{"main.go"},
		Diff:            "- old\n+ new",
		TestsExist:      true,
		IsExported:      false,
		HasBreakingChange: false,
		LinesChanged:    5,
		FilesAffected:   1,
		Complexity:      2,
	}

	assessment := ra.Assess(ctx)
	if assessment == nil {
		t.Fatal("Assess returned nil")
	}
	if assessment.Level != "low" {
		t.Errorf("expected low risk level, got %q (score: %.2f)", assessment.Level, assessment.Score)
	}
	if assessment.Score < 0 || assessment.Score > 1 {
		t.Errorf("score out of range: %f", assessment.Score)
	}
	if !ShouldProceed(assessment) {
		t.Error("should proceed for low risk")
	}
}

func TestAssess_HighRisk(t *testing.T) {
	ra := NewRiskAssessor()
	ctx := &RiskContext{
		Files:           []string{"auth.go", "config.go", "handler.go", "middleware.go", "router.go"},
		Diff:            strings.Repeat("+line\n", 200),
		TestsExist:      false,
		IsExported:      true,
		HasBreakingChange: true,
		LinesChanged:    250,
		FilesAffected:   5,
		Complexity:      15,
	}

	assessment := ra.Assess(ctx)
	if assessment == nil {
		t.Fatal("Assess returned nil")
	}
	if assessment.Level != "high" && assessment.Level != "critical" {
		t.Errorf("expected high or critical risk level, got %q (score: %.2f)", assessment.Level, assessment.Score)
	}
	if assessment.Score < 0.6 {
		t.Errorf("expected score >= 0.6 for high risk, got %f", assessment.Score)
	}
}

func TestAssess_CriticalRisk(t *testing.T) {
	ra := NewRiskAssessor()
	ctx := &RiskContext{
		Files:           []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go", "g.go", "h.go", "i.go", "j.go", "k.go"},
		Diff:            strings.Repeat("+line\n", 600),
		TestsExist:      false,
		IsExported:      true,
		HasBreakingChange: true,
		LinesChanged:    600,
		FilesAffected:   11,
		Complexity:      25,
	}

	assessment := ra.Assess(ctx)
	if assessment == nil {
		t.Fatal("Assess returned nil")
	}
	if assessment.Level != "critical" {
		t.Errorf("expected critical risk level, got %q (score: %.2f)", assessment.Level, assessment.Score)
	}
	if ShouldProceed(assessment) {
		t.Error("should not proceed for critical risk")
	}
}

func TestAssess_MediumRisk(t *testing.T) {
	ra := NewRiskAssessor()
	ctx := &RiskContext{
		Files:           []string{"handler.go", "handler_test.go"},
		Diff:            strings.Repeat("+line\n", 50),
		TestsExist:      true,
		IsExported:      true,
		HasBreakingChange: false,
		LinesChanged:    60,
		FilesAffected:   2,
		Complexity:      8,
	}

	assessment := ra.Assess(ctx)
	if assessment == nil {
		t.Fatal("Assess returned nil")
	}
	if assessment.Level != "medium" {
		t.Errorf("expected medium risk level, got %q (score: %.2f)", assessment.Level, assessment.Score)
	}
}

func TestGenerateMitigations_NoTests(t *testing.T) {
	assessment := &RiskAssessment{
		Score: 0.7,
		Level: "high",
		Factors: []RiskFactor{
			{Name: "test_coverage", Weight: 0.15, Score: 0.9, Description: "no tests found"},
			{Name: "exported_changes", Weight: 0.20, Score: 0.8, Description: "exported symbols modified"},
			{Name: "breaking_changes", Weight: 0.20, Score: 0.9, Description: "breaking changes detected"},
		},
	}

	mitigations := GenerateMitigations(assessment)
	if len(mitigations) == 0 {
		t.Fatal("expected mitigations to be generated")
	}

	found := map[string]bool{}
	for _, m := range mitigations {
		if strings.Contains(m, "Add tests") {
			found["tests"] = true
		}
		if strings.Contains(m, "exported API") {
			found["exported"] = true
		}
		if strings.Contains(m, "integration tests") {
			found["integration"] = true
		}
	}

	if !found["tests"] {
		t.Error("expected mitigation about adding tests")
	}
	if !found["exported"] {
		t.Error("expected mitigation about exported API review")
	}
	if !found["integration"] {
		t.Error("expected mitigation about integration tests")
	}
}

func TestGenerateMitigations_AllLow(t *testing.T) {
	assessment := &RiskAssessment{
		Score: 0.1,
		Level: "low",
		Factors: []RiskFactor{
			{Name: "test_coverage", Weight: 0.15, Score: 0.2, Description: "tests exist"},
			{Name: "exported_changes", Weight: 0.20, Score: 0.1, Description: "no exported symbols modified"},
			{Name: "breaking_changes", Weight: 0.20, Score: 0.1, Description: "no breaking changes"},
		},
	}

	mitigations := GenerateMitigations(assessment)
	if len(mitigations) != 1 || mitigations[0] != "No specific mitigations needed" {
		t.Errorf("expected 'No specific mitigations needed', got %v", mitigations)
	}
}

func TestFormatAssessment(t *testing.T) {
	assessment := &RiskAssessment{
		Score: 0.62,
		Level: "high",
		Factors: []RiskFactor{
			{Name: "file_count", Weight: 0.15, Score: 0.4, Description: "3 files"},
			{Name: "lines_changed", Weight: 0.15, Score: 0.5, Description: "85 lines"},
			{Name: "exported_changes", Weight: 0.20, Score: 0.8, Description: "exported symbols modified"},
			{Name: "test_coverage", Weight: 0.15, Score: 0.3, Description: "tests exist"},
			{Name: "complexity", Weight: 0.15, Score: 0.6, Description: "complexity score 12"},
			{Name: "breaking_changes", Weight: 0.20, Score: 0.9, Description: "breaking changes detected"},
		},
		Mitigations: []string{
			"Add tests for modified functions",
			"Review exported API changes carefully",
			"Run integration tests before merging",
		},
		Recommendation: "Proceed with caution — add tests first",
	}

	output := FormatAssessment(assessment)

	if !strings.Contains(output, "Risk Assessment: HIGH (0.62)") {
		t.Errorf("missing header in output:\n%s", output)
	}
	if !strings.Contains(output, "═══════════════════════════════") {
		t.Error("missing separator line")
	}
	if !strings.Contains(output, "Factors:") {
		t.Error("missing Factors section")
	}
	if !strings.Contains(output, "File count") {
		t.Error("missing file count factor")
	}
	if !strings.Contains(output, "Mitigations:") {
		t.Error("missing Mitigations section")
	}
	if !strings.Contains(output, "Recommendation:") {
		t.Error("missing Recommendation section")
	}
	if !strings.Contains(output, "█") {
		t.Error("missing bar characters")
	}
	if !strings.Contains(output, "░") {
		t.Error("missing empty bar characters")
	}
}

func TestShouldProceed(t *testing.T) {
	tests := []struct {
		level    string
		expected bool
	}{
		{"low", true},
		{"medium", true},
		{"high", true},
		{"critical", false},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			assessment := &RiskAssessment{Level: tt.level}
			got := ShouldProceed(assessment)
			if got != tt.expected {
				t.Errorf("ShouldProceed for %q: got %v, want %v", tt.level, got, tt.expected)
			}
		})
	}
}

func TestDetermineLevel(t *testing.T) {
	tests := []struct {
		score    float64
		expected string
	}{
		{0.0, "low"},
		{0.1, "low"},
		{0.34, "low"},
		{0.35, "medium"},
		{0.5, "medium"},
		{0.59, "medium"},
		{0.6, "high"},
		{0.7, "high"},
		{0.79, "high"},
		{0.8, "critical"},
		{0.9, "critical"},
		{1.0, "critical"},
	}

	for _, tt := range tests {
		got := determineLevel(tt.score)
		if got != tt.expected {
			t.Errorf("determineLevel(%.2f): got %q, want %q", tt.score, got, tt.expected)
		}
	}
}

func TestRiskAssessorRenderBar(t *testing.T) {
	tests := []struct {
		score    float64
		expected string
	}{
		{0.0, "░░░░░░░░░░"},
		{0.5, "░░░░░█████"},
		{1.0, "██████████"},
	}

	for _, tt := range tests {
		got := riskRenderBar(tt.score)
		if got != tt.expected {
			t.Errorf("riskRenderBar(%.1f): got %q, want %q", tt.score, got, tt.expected)
		}
	}
}

func TestAssess_ScoreClamped(t *testing.T) {
	ra := &RiskAssessor{
		Factors: []RiskFactorDef{
			{
				Name:   "always_max",
				Weight: 1.0,
				EvaluateFn: func(ctx *RiskContext) float64 {
					return 1.5 // exceeds max
				},
			},
		},
	}

	ctx := &RiskContext{}
	assessment := ra.Assess(ctx)
	if assessment.Score > 1.0 {
		t.Errorf("score should be clamped to 1.0, got %f", assessment.Score)
	}
}

func TestAssess_ZeroWeight(t *testing.T) {
	ra := &RiskAssessor{
		Factors: []RiskFactorDef{},
	}

	ctx := &RiskContext{}
	assessment := ra.Assess(ctx)
	if assessment.Score != 0 {
		t.Errorf("expected score 0 with no factors, got %f", assessment.Score)
	}
	if assessment.Level != "low" {
		t.Errorf("expected low level with score 0, got %q", assessment.Level)
	}
}

func TestAssess_ConcurrentAccess(t *testing.T) {
	ra := NewRiskAssessor()
	ctx := &RiskContext{
		Files:         []string{"main.go"},
		LinesChanged:  50,
		FilesAffected: 1,
		Complexity:    5,
	}

	done := make(chan struct{}, 10)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			assessment := ra.Assess(ctx)
			if assessment == nil {
				t.Error("concurrent Assess returned nil")
			}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestFactorDescription(t *testing.T) {
	ctx := &RiskContext{
		FilesAffected:   3,
		LinesChanged:    42,
		IsExported:      true,
		TestsExist:      false,
		HasBreakingChange: true,
		Complexity:      7,
	}

	tests := []struct {
		name     string
		contains string
	}{
		{"file_count", "3 files"},
		{"lines_changed", "42 lines"},
		{"exported_changes", "exported symbols modified"},
		{"test_coverage", "no tests found"},
		{"complexity", "complexity score 7"},
		{"breaking_changes", "breaking changes detected"},
	}

	for _, tt := range tests {
		desc := factorDescription(tt.name, ctx)
		if !strings.Contains(desc, tt.contains) {
			t.Errorf("factorDescription(%q): got %q, want it to contain %q", tt.name, desc, tt.contains)
		}
	}
}

func TestFactorDescription_Negatives(t *testing.T) {
	ctx := &RiskContext{
		FilesAffected:   0,
		LinesChanged:    0,
		IsExported:      false,
		TestsExist:      true,
		HasBreakingChange: false,
		Complexity:      0,
	}

	tests := []struct {
		name     string
		contains string
	}{
		{"exported_changes", "no exported symbols modified"},
		{"test_coverage", "tests exist"},
		{"breaking_changes", "no breaking changes"},
	}

	for _, tt := range tests {
		desc := factorDescription(tt.name, ctx)
		if !strings.Contains(desc, tt.contains) {
			t.Errorf("factorDescription(%q): got %q, want it to contain %q", tt.name, desc, tt.contains)
		}
	}
}

func TestFilesAffectedFallback(t *testing.T) {
	ra := NewRiskAssessor()
	ctx := &RiskContext{
		Files:         []string{"a.go", "b.go", "c.go"},
		FilesAffected: 0, // should fall back to len(Files)
		LinesChanged:  10,
		Complexity:    2,
		TestsExist:    true,
	}

	assessment := ra.Assess(ctx)
	// Verify file_count factor used len(Files) as fallback
	for _, f := range assessment.Factors {
		if f.Name == "file_count" {
			if f.Score < 0.3 {
				t.Errorf("expected file_count score reflecting 3 files, got %f", f.Score)
			}
			break
		}
	}
}
