package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/GrayCodeAI/hawk/internal/harness"
	"github.com/spf13/cobra"
)

var (
	harnessOutDir string
	harnessFormat string
	harnessFix    bool
)

var harnessCmd = &cobra.Command{
	Use:   "harness [review|fix]",
	Short: "Audit workspace AI agent harness, work loop dimensions, and generation reports",
	Long: `Evaluate the workspace AI coding agent harness across 5 dimensions:
  1. Feedforward Guidance (AGENTS.md, ZERO.md, specs, skills)
  2. Feedback Sensors (linters, test suites, hooks)
  3. Task Understanding (spec clarity, acceptance criteria)
  4. Step Planning & Execution (execution graphs, step reproducibility)
  5. Verification & Safeguards (safety checks, sandbox policy)

Generates self-contained HTML (report.html), Markdown (report.md), and JSON (findings.json).
Use --fix to automatically repair missing AGENTS.md, skills, or spec directories.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}

		ctx := context.Background()
		opts := harness.EvaluateOptions{
			TargetPath: targetDir,
			OutputDir:  harnessOutDir,
		}

		report, err := harness.EvaluateWorkspace(ctx, targetDir, opts)
		if err != nil {
			return fmt.Errorf("harness evaluation failed: %w", err)
		}

		if harnessFix || (len(args) > 0 && args[0] == "fix") {
			fixResult, fixErr := harness.FixWorkspaceHarness(ctx, targetDir, report)
			if fixErr != nil {
				return fmt.Errorf("harness auto-fix failed: %w", fixErr)
			}
			fmt.Printf("[FIX] Hawk Harness Auto-Repair Results:\n")
			for _, repair := range fixResult.RepairsPerformed {
				fmt.Printf("   + %s\n", repair)
			}
			// Re-evaluate workspace after fix
			report, _ = harness.EvaluateWorkspace(ctx, targetDir, opts)
		}

		outDir := harnessOutDir
		if outDir == "" {
			outDir = filepath.Join(targetDir, ".hawk", "harness")
		}

		if mkdirErr := os.MkdirAll(outDir, 0o750); mkdirErr != nil {
			return fmt.Errorf("failed to create harness output directory: %w", mkdirErr)
		}

		// Write Markdown report
		mdPath := filepath.Join(outDir, "report.md")
		mdContent := harness.RenderMarkdown(report)
		if writeErr := os.WriteFile(mdPath, []byte(mdContent), 0o640); writeErr != nil { // #nosec G306 -- report is intentionally group-readable
			return fmt.Errorf("failed to write report.md: %w", writeErr)
		}

		// Write HTML report
		htmlPath := filepath.Join(outDir, "report.html")
		htmlContent := harness.RenderHTML(report)
		if writeErr := os.WriteFile(htmlPath, []byte(htmlContent), 0o640); writeErr != nil { // #nosec G306 -- report is intentionally group-readable
			return fmt.Errorf("failed to write report.html: %w", writeErr)
		}

		// Write JSON report
		jsonPath := filepath.Join(outDir, "findings.json")
		jsonContent, renderErr := harness.RenderJSON(report)
		if renderErr != nil {
			return fmt.Errorf("failed to serialize findings.json: %w", renderErr)
		}
		if writeErr := os.WriteFile(jsonPath, jsonContent, 0o640); writeErr != nil { // #nosec G306 -- report is intentionally group-readable
			return fmt.Errorf("failed to write findings.json: %w", writeErr)
		}

		// Journal quality observation to Hawk execution graph
		_ = harness.JournalHarnessReport(report, "")

		fmt.Printf("[HAWK] Hawk Harness Evaluation Complete\n")
		fmt.Printf("   Overall Score : %d/100 (%s)\n", report.OverallScore, report.OverallStatus)
		fmt.Printf("   Findings      : %d prioritized issues\n", len(report.Findings))
		fmt.Printf("   HTML Report   : %s\n", htmlPath)
		fmt.Printf("   Markdown      : %s\n", mdPath)
		fmt.Printf("   JSON Findings : %s\n", jsonPath)

		return nil
	},
}

func init() {
	harnessCmd.Flags().StringVar(&harnessOutDir, "out-dir", "", "Directory to save harness reports (default: .hawk/harness)")
	harnessCmd.Flags().StringVar(&harnessFormat, "format", "all", "Report output format (html, markdown, json, all)")
	harnessCmd.Flags().BoolVar(&harnessFix, "fix", false, "Automatically repair missing harness assets (AGENTS.md, skills, specs)")
}
