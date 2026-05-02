package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	inspectLib "github.com/GrayCodeAI/inspect"

	hawkInspect "github.com/GrayCodeAI/hawk/inspect"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	inspectDepth       int
	inspectChecks      string
	inspectFailOn      string
	inspectFormat      string
	inspectConcurrency int
	inspectTimeout     time.Duration
)

var inspectCmd = &cobra.Command{
	Use:   "inspect <url>",
	Short: "Scan a website for broken links, security issues, accessibility violations, and more",
	Long: `inspect crawls a target URL and runs configurable checks including
broken links, security headers, form problems, accessibility violations,
performance concerns, and SEO issues.

Examples:
  hawk inspect https://example.com
  hawk inspect --depth 3 --checks links,security --format json https://example.com
  hawk inspect --fail-on high --timeout 2m https://example.com`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]

		var opts []inspectLib.Option
		if inspectDepth > 0 {
			opts = append(opts, inspectLib.WithDepth(inspectDepth))
		}
		if inspectChecks != "" {
			checks := strings.Split(inspectChecks, ",")
			for i := range checks {
				checks[i] = strings.TrimSpace(checks[i])
			}
			opts = append(opts, inspectLib.WithChecks(checks...))
		}
		if inspectFailOn != "" {
			opts = append(opts, inspectLib.WithFailOn(inspectLib.ParseSeverity(inspectFailOn)))
		}
		if inspectConcurrency > 0 {
			opts = append(opts, inspectLib.WithConcurrency(inspectConcurrency))
		}
		if inspectTimeout > 0 {
			opts = append(opts, inspectLib.WithTimeout(inspectTimeout))
		}

		bridge := hawkInspect.NewBridge(opts...)
		if !bridge.Ready() {
			return fmt.Errorf("inspect bridge failed to initialize")
		}

		ctx := context.Background()
		if inspectTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, inspectTimeout)
			defer cancel()
		}

		report, err := bridge.Run(ctx, target)
		if err != nil {
			return fmt.Errorf("inspect scan failed: %w", err)
		}

		switch inspectFormat {
		case "json":
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(report); err != nil {
				return err
			}
		case "markdown":
			fmt.Print(formatInspectMarkdown(report))
		default:
			fmt.Print(formatInspectTerminal(report))
		}

		if report.Failed() {
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	inspectCmd.Flags().IntVar(&inspectDepth, "depth", 5, "maximum crawl depth")
	inspectCmd.Flags().StringVar(&inspectChecks, "checks", "", "comma-separated checks to run (links,security,forms,a11y,perf,seo)")
	inspectCmd.Flags().StringVar(&inspectFailOn, "fail-on", "critical", "minimum severity to cause exit code 1 (info,low,medium,high,critical)")
	inspectCmd.Flags().StringVar(&inspectFormat, "format", "terminal", "output format: terminal, json, markdown")
	inspectCmd.Flags().IntVar(&inspectConcurrency, "concurrency", 10, "maximum concurrent requests")
	inspectCmd.Flags().DurationVar(&inspectTimeout, "timeout", 60*time.Second, "scan timeout (e.g. 30s, 2m)")
}

// formatInspectTerminal renders a human-readable report using lipgloss styles.
func formatInspectTerminal(report *inspectLib.Report) string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	critStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))
	highStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	medStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	lowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	b.WriteString(titleStyle.Render(fmt.Sprintf("Inspect Report: %s", report.Target)))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("Scanned %d pages in %s", report.CrawledURLs, report.Duration.Round(time.Millisecond))))
	b.WriteString("\n\n")

	if len(report.Findings) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("No issues found."))
		b.WriteString("\n")
		return b.String()
	}

	for _, f := range report.Findings {
		var sevLabel string
		switch f.Severity {
		case inspectLib.SeverityCritical:
			sevLabel = critStyle.Render("CRITICAL")
		case inspectLib.SeverityHigh:
			sevLabel = highStyle.Render("HIGH")
		case inspectLib.SeverityMedium:
			sevLabel = medStyle.Render("MEDIUM")
		case inspectLib.SeverityLow:
			sevLabel = lowStyle.Render("LOW")
		default:
			sevLabel = infoStyle.Render("INFO")
		}

		b.WriteString(fmt.Sprintf("  [%s] [%s] %s\n", sevLabel, f.Check, f.URL))
		b.WriteString(fmt.Sprintf("    %s\n", f.Message))
		if f.Fix != "" {
			b.WriteString(dimStyle.Render(fmt.Sprintf("    Fix: %s", f.Fix)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(dimStyle.Render("---"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("%d finding(s) total", report.Stats.FindingsTotal))
	for sev, count := range report.Stats.BySeverity {
		b.WriteString(fmt.Sprintf("  %s:%d", sev, count))
	}
	b.WriteString("\n")

	if report.Failed() {
		b.WriteString(critStyle.Render(fmt.Sprintf("FAILED (threshold: %s)", report.FailOn)))
		b.WriteString("\n")
	}

	return b.String()
}

// formatInspectMarkdown renders the report as markdown.
func formatInspectMarkdown(report *inspectLib.Report) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Inspect Report: %s\n\n", report.Target))
	b.WriteString(fmt.Sprintf("Scanned **%d pages** in %s.\n\n", report.CrawledURLs, report.Duration.Round(time.Millisecond)))

	if len(report.Findings) == 0 {
		b.WriteString("**No issues found.**\n")
		return b.String()
	}

	b.WriteString("## Findings\n\n")
	b.WriteString("| Severity | Check | URL | Message |\n")
	b.WriteString("|----------|-------|-----|----------|\n")
	for _, f := range report.Findings {
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", f.Severity, f.Check, f.URL, f.Message))
	}
	b.WriteString("\n")

	b.WriteString(fmt.Sprintf("**%d finding(s) total.**\n", report.Stats.FindingsTotal))
	if report.Failed() {
		b.WriteString(fmt.Sprintf("\n**FAILED** (threshold: %s)\n", report.FailOn))
	}

	return b.String()
}
