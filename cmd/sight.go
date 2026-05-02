package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/GrayCodeAI/eyrie/client"
	sightLib "github.com/GrayCodeAI/sight"

	hawkconfig "github.com/GrayCodeAI/hawk/config"
	hawkSight "github.com/GrayCodeAI/hawk/sight"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	sightBase       string
	sightPR         int
	sightConcerns   string
	sightModel      string
	sightFailOn     string
	sightFormat     string
	sightReflection bool
	sightMode       string // "review", "describe", "improve"
)

var sightCmd = &cobra.Command{
	Use:   "sight",
	Short: "AI-powered code review on the current branch diff",
	Long: `sight reviews code changes using an LLM, detecting security issues,
bugs, performance problems, and style concerns.

By default it reviews the diff between main and HEAD. Use --base to change
the comparison branch, or --mode to switch between review, describe, and improve.

Examples:
  hawk sight
  hawk sight --base develop
  hawk sight --mode describe
  hawk sight --mode improve --model claude-sonnet-4-20250514
  hawk sight --concerns security,bugs --fail-on high --format json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		hawkconfig.LoadEnvFile()

		diff, err := getDiff()
		if err != nil {
			return fmt.Errorf("failed to get diff: %w", err)
		}
		if strings.TrimSpace(diff) == "" {
			cmd.Println("No changes found.")
			return nil
		}

		// Build eyrie client using hawk's configured provider
		prov := provider
		if prov == "" {
			prov = client.DetectProvider()
		}
		eyrieClient := client.NewEyrieClient(&client.EyrieConfig{Provider: prov})

		var sightOpts []sightLib.Option
		if sightModel != "" {
			sightOpts = append(sightOpts, sightLib.WithModel(sightModel))
		}
		if sightConcerns != "" {
			concerns := strings.Split(sightConcerns, ",")
			for i := range concerns {
				concerns[i] = strings.TrimSpace(concerns[i])
			}
			sightOpts = append(sightOpts, sightLib.WithConcerns(concerns...))
		}
		if sightFailOn != "" {
			sightOpts = append(sightOpts, sightLib.WithFailOn(sightLib.ParseSeverity(sightFailOn)))
		}
		if sightReflection {
			sightOpts = append(sightOpts, sightLib.WithReflection(true))
		}

		bridge := hawkSight.NewBridge(eyrieClient, prov, sightOpts...)
		if !bridge.Ready() {
			return fmt.Errorf("sight bridge failed to initialize")
		}

		ctx := context.Background()

		mode := sightMode
		if mode == "" {
			mode = "review"
		}

		switch mode {
		case "review":
			return runSightReview(ctx, bridge, diff)
		case "describe":
			return runSightDescribe(ctx, bridge, diff)
		case "improve":
			return runSightImprove(ctx, bridge, diff)
		default:
			return fmt.Errorf("unknown mode %q; use review, describe, or improve", mode)
		}
	},
}

func init() {
	sightCmd.Flags().StringVar(&sightBase, "base", "main", "base branch for diff comparison")
	sightCmd.Flags().IntVar(&sightPR, "pr", 0, "pull request number (uses GitHub API to get diff)")
	sightCmd.Flags().StringVar(&sightConcerns, "concerns", "", "comma-separated concerns (security,bugs,performance,correctness,style)")
	sightCmd.Flags().StringVar(&sightModel, "model", "", "LLM model to use for review")
	sightCmd.Flags().StringVar(&sightFailOn, "fail-on", "critical", "minimum severity to cause exit code 1 (info,low,medium,high,critical)")
	sightCmd.Flags().StringVar(&sightFormat, "format", "terminal", "output format: terminal, json, sarif")
	sightCmd.Flags().BoolVar(&sightReflection, "reflection", false, "enable self-reflection pass to validate findings")
	sightCmd.Flags().StringVar(&sightMode, "mode", "review", "operation mode: review, describe, improve")
}

// getDiff obtains the diff to review, either from a PR number or git diff.
func getDiff() (string, error) {
	if sightPR > 0 {
		// Use gh CLI to fetch the PR diff
		out, err := exec.Command("gh", "pr", "diff", fmt.Sprintf("%d", sightPR)).Output()
		if err != nil {
			return "", fmt.Errorf("gh pr diff failed: %w", err)
		}
		return string(out), nil
	}

	base := sightBase
	if base == "" {
		base = "main"
	}
	out, err := exec.Command("git", "diff", base+"...HEAD").Output()
	if err != nil {
		// Fallback to two-dot syntax
		out, err = exec.Command("git", "diff", base, "HEAD").Output()
		if err != nil {
			return "", fmt.Errorf("git diff %s...HEAD failed: %w", base, err)
		}
	}
	return string(out), nil
}

func runSightReview(ctx context.Context, bridge *hawkSight.Bridge, diff string) error {
	result, err := bridge.Review(ctx, diff)
	if err != nil {
		return fmt.Errorf("sight review failed: %w", err)
	}

	switch sightFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			return err
		}
	case "sarif":
		sarif := formatSightSARIF(result)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(sarif); err != nil {
			return err
		}
	default:
		fmt.Print(formatSightTerminal(result))
	}

	if result.Failed() {
		os.Exit(1)
	}
	return nil
}

func runSightDescribe(ctx context.Context, bridge *hawkSight.Bridge, diff string) error {
	desc, err := bridge.Describe(ctx, diff)
	if err != nil {
		return fmt.Errorf("sight describe failed: %w", err)
	}

	switch sightFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(desc); err != nil {
			return err
		}
	default:
		fmt.Print(formatDescriptionTerminal(desc))
	}
	return nil
}

func runSightImprove(ctx context.Context, bridge *hawkSight.Bridge, diff string) error {
	result, err := bridge.Improve(ctx, diff)
	if err != nil {
		return fmt.Errorf("sight improve failed: %w", err)
	}

	switch sightFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			return err
		}
	default:
		fmt.Print(formatImprovementsTerminal(result))
	}
	return nil
}

// formatSightTerminal renders review results using lipgloss styles.
func formatSightTerminal(result *sightLib.Result) string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	critStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))
	highStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	medStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	lowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	b.WriteString(titleStyle.Render("Sight Code Review"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("Reviewed %d files, %d hunks", result.Stats.FilesReviewed, result.Stats.HunksAnalyzed)))
	b.WriteString("\n\n")

	if len(result.Findings) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("No issues found."))
		b.WriteString("\n")
		return b.String()
	}

	for _, f := range result.Findings {
		var sevLabel string
		switch f.Severity {
		case sightLib.SeverityCritical:
			sevLabel = critStyle.Render("CRITICAL")
		case sightLib.SeverityHigh:
			sevLabel = highStyle.Render("HIGH")
		case sightLib.SeverityMedium:
			sevLabel = medStyle.Render("MEDIUM")
		case sightLib.SeverityLow:
			sevLabel = lowStyle.Render("LOW")
		default:
			sevLabel = infoStyle.Render("INFO")
		}

		loc := f.File
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", f.File, f.Line)
		}

		b.WriteString(fmt.Sprintf("  [%s] [%s] %s\n", sevLabel, f.Concern, loc))
		b.WriteString(fmt.Sprintf("    %s\n", f.Message))
		if f.Fix != "" {
			b.WriteString(dimStyle.Render(fmt.Sprintf("    Fix: %s", f.Fix)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(dimStyle.Render("---"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("%d finding(s) total", result.Stats.FindingsTotal))
	for sev, count := range result.Stats.BySeverity {
		b.WriteString(fmt.Sprintf("  %s:%d", sev, count))
	}
	b.WriteString("\n")

	if result.Failed() {
		b.WriteString(critStyle.Render(fmt.Sprintf("FAILED (threshold: %s)", result.FailOn)))
		b.WriteString("\n")
	}

	return b.String()
}

// formatDescriptionTerminal renders a PR description for the terminal.
func formatDescriptionTerminal(desc *sightLib.Description) string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	b.WriteString(titleStyle.Render(desc.Title))
	b.WriteString("\n\n")
	b.WriteString(desc.Summary)
	b.WriteString("\n\n")

	if len(desc.Changes) > 0 {
		b.WriteString("Changes:\n")
		for _, c := range desc.Changes {
			b.WriteString(fmt.Sprintf("  - %s\n", c))
		}
		b.WriteString("\n")
	}

	if desc.ChangeType != "" {
		b.WriteString(dimStyle.Render(fmt.Sprintf("Type: %s  Risk: %s", desc.ChangeType, desc.Risk)))
		b.WriteString("\n")
	}
	if desc.TestPlan != "" {
		b.WriteString(dimStyle.Render(fmt.Sprintf("Test plan: %s", desc.TestPlan)))
		b.WriteString("\n")
	}

	return b.String()
}

// formatImprovementsTerminal renders improvement suggestions for the terminal.
func formatImprovementsTerminal(result *sightLib.ImproveResult) string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	codeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))

	b.WriteString(titleStyle.Render("Sight Improvements"))
	b.WriteString("\n\n")

	if len(result.Improvements) == 0 {
		b.WriteString("Code looks clean. No improvements suggested.\n")
		return b.String()
	}

	for i, imp := range result.Improvements {
		loc := imp.File
		if imp.Line > 0 {
			loc = fmt.Sprintf("%s:%d", imp.File, imp.Line)
		}

		b.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, imp.Category, loc))
		b.WriteString(fmt.Sprintf("   %s\n", imp.Description))
		if imp.Before != "" {
			b.WriteString(dimStyle.Render("   Before: "))
			b.WriteString(imp.Before)
			b.WriteString("\n")
		}
		if imp.After != "" {
			b.WriteString(codeStyle.Render("   After:  "))
			b.WriteString(imp.After)
			b.WriteString("\n")
		}
		if imp.Reasoning != "" {
			b.WriteString(dimStyle.Render(fmt.Sprintf("   Why: %s", imp.Reasoning)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(dimStyle.Render(fmt.Sprintf("%d improvement(s) suggested.", len(result.Improvements))))
	b.WriteString("\n")

	return b.String()
}

// formatSightSARIF produces a minimal SARIF 2.1.0 output for CI integration.
func formatSightSARIF(result *sightLib.Result) map[string]interface{} {
	rules := []map[string]interface{}{}
	ruleIndex := map[string]int{}
	results := []map[string]interface{}{}

	for _, f := range result.Findings {
		ruleID := f.Concern
		if _, ok := ruleIndex[ruleID]; !ok {
			ruleIndex[ruleID] = len(rules)
			rules = append(rules, map[string]interface{}{
				"id":   ruleID,
				"name": f.Concern,
			})
		}

		sarif := map[string]interface{}{
			"ruleId":  ruleID,
			"level":   sarifLevel(f.Severity),
			"message": map[string]string{"text": f.Message},
		}
		if f.File != "" {
			loc := map[string]interface{}{
				"physicalLocation": map[string]interface{}{
					"artifactLocation": map[string]string{"uri": f.File},
				},
			}
			if f.Line > 0 {
				loc["physicalLocation"].(map[string]interface{})["region"] = map[string]int{
					"startLine": f.Line,
				}
			}
			sarif["locations"] = []map[string]interface{}{loc}
		}
		results = append(results, sarif)
	}

	return map[string]interface{}{
		"$schema": "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json",
		"version": "2.1.0",
		"runs": []map[string]interface{}{
			{
				"tool": map[string]interface{}{
					"driver": map[string]interface{}{
						"name":  "hawk-sight",
						"rules": rules,
					},
				},
				"results": results,
			},
		},
	}
}

func sarifLevel(sev sightLib.Severity) string {
	switch sev {
	case sightLib.SeverityCritical, sightLib.SeverityHigh:
		return "error"
	case sightLib.SeverityMedium:
		return "warning"
	default:
		return "note"
	}
}
