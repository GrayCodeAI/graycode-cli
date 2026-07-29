package harness

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEvaluateWorkspace(t *testing.T) {
	// Create a temporary workspace directory for testing
	tmpDir := t.TempDir()

	// 1. Evaluate empty workspace
	opts := EvaluateOptions{
		TargetPath: tmpDir,
	}

	report, err := EvaluateWorkspace(context.Background(), tmpDir, opts)
	if err != nil {
		t.Fatalf("EvaluateWorkspace failed: %v", err)
	}

	if report == nil {
		t.Fatal("Expected non-nil report")
	}

	if report.OverallScore >= 80 {
		t.Errorf("Empty directory should not score >= 80, got %d", report.OverallScore)
	}

	if len(report.Findings) == 0 {
		t.Error("Expected findings for empty directory, got 0")
	}

	// 2. Add AGENTS.md, Makefile, .golangci.yml
	agentsPath := filepath.Join(tmpDir, "AGENTS.md")
	agentsContent := `# Project Directives
- Build with make
- Run tests before opening PR
`
	if writeErr := os.WriteFile(agentsPath, []byte(agentsContent), 0o644); writeErr != nil {
		t.Fatalf("Failed to write AGENTS.md: %v", writeErr)
	}

	makefileContent := `test:
	go test ./...
lint:
	golangci-lint run
`
	if writeErr := os.WriteFile(filepath.Join(tmpDir, "Makefile"), []byte(makefileContent), 0o644); writeErr != nil {
		t.Fatalf("Failed to write Makefile: %v", writeErr)
	}

	if writeErr := os.WriteFile(filepath.Join(tmpDir, ".golangci.yml"), []byte("linters:\n  enable:\n    - errcheck\n"), 0o644); writeErr != nil {
		t.Fatalf("Failed to write .golangci.yml: %v", writeErr)
	}

	// 3. Re-evaluate with assets present
	report2, err := EvaluateWorkspace(context.Background(), tmpDir, opts)
	if err != nil {
		t.Fatalf("EvaluateWorkspace with assets failed: %v", err)
	}

	if !report2.Assets.AgentsMD {
		t.Error("Expected AgentsMD asset to be detected")
	}

	if len(report2.Assets.Linters) == 0 {
		t.Error("Expected Linters asset to be detected")
	}

	if report2.OverallScore <= report.OverallScore {
		t.Errorf("Expected score with assets (%d) to be higher than empty score (%d)", report2.OverallScore, report.OverallScore)
	}

	// Test Renderers
	md := RenderMarkdown(report2)
	if len(md) == 0 {
		t.Error("RenderMarkdown returned empty string")
	}

	html := RenderHTML(report2)
	if len(html) == 0 {
		t.Error("RenderHTML returned empty string")
	}

	jsonBytes, err := RenderJSON(report2)
	if err != nil || len(jsonBytes) == 0 {
		t.Errorf("RenderJSON failed: %v", err)
	}

	// Test ToContractReport conversion
	contractRep := report2.ToContractReport()
	if contractRep == nil {
		t.Fatal("ToContractReport returned nil")
	}
	if contractRep.OverallScore != report2.OverallScore {
		t.Errorf("Expected contract overall score %d, got %d", report2.OverallScore, contractRep.OverallScore)
	}

	// Test FixWorkspaceHarness on empty directory
	emptyDir := t.TempDir()
	fixRes, err := FixWorkspaceHarness(context.Background(), emptyDir, nil)
	if err != nil {
		t.Fatalf("FixWorkspaceHarness failed: %v", err)
	}
	if len(fixRes.RepairsPerformed) == 0 {
		t.Error("Expected repairs to be performed on empty directory")
	}
}
