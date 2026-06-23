package cmd

import (
	"context"
	"fmt"
	"strings"

	contracts "github.com/GrayCodeAI/hawk-core-contracts/types"
	verifycontracts "github.com/GrayCodeAI/hawk-core-contracts/verify"
	hawkInspect "github.com/GrayCodeAI/hawk/internal/bridge/inspect"
	inspectLib "github.com/GrayCodeAI/inspect"
)

// InspectPipelineConfig controls how inspect integrates into the review pipeline.
type InspectPipelineConfig struct {
	Target      string // URL to inspect
	Depth       int
	Checks      []string // which checks to run
	FailOn      string   // severity threshold
	Concurrency int
}

// DefaultInspectPipelineConfig returns sensible defaults.
func DefaultInspectPipelineConfig(target string) InspectPipelineConfig {
	return InspectPipelineConfig{
		Target:      target,
		Depth:       3,
		Checks:      []string{"links", "security", "forms", "a11y", "perf", "seo"},
		FailOn:      "medium",
		Concurrency: 5,
	}
}

// InspectToReviewFindings converts an inspect report into ReviewFindings
// so they can be displayed alongside code review findings.
func InspectToReviewFindings(report *verifycontracts.Report) []ReviewFinding {
	if report == nil {
		return nil
	}

	var findings []ReviewFinding
	for _, f := range report.Findings {
		severity := mapInspectSeverity(f.Severity)
		findings = append(findings, ReviewFinding{
			Concern:  "inspect:" + f.Check,
			Severity: severity,
			File:     f.URL,
			Line:     0,
			Message:  f.Message,
			Fix:      f.Fix,
		})
	}
	return findings
}

// mapInspectSeverity converts inspect severity to review severity.
func mapInspectSeverity(sev contracts.Severity) string {
	switch sev {
	case contracts.SeverityCritical:
		return "critical"
	case contracts.SeverityHigh:
		return "high"
	case contracts.SeverityMedium:
		return "medium"
	case contracts.SeverityLow:
		return "low"
	default:
		return "low"
	}
}

// RunInspectPipeline runs an inspect scan and returns results formatted
// as review findings that can be merged with code review results.
func RunInspectPipeline(ctx context.Context, cfg InspectPipelineConfig) ([]ReviewFinding, string, error) {
	var opts []inspectLib.Option
	if cfg.Depth > 0 {
		opts = append(opts, inspectLib.WithDepth(cfg.Depth))
	}
	if len(cfg.Checks) > 0 {
		opts = append(opts, inspectLib.WithChecks(cfg.Checks...))
	}
	if cfg.FailOn != "" {
		opts = append(opts, inspectLib.WithFailOn(inspectLib.ParseSeverity(cfg.FailOn)))
	}
	if cfg.Concurrency > 0 {
		opts = append(opts, inspectLib.WithConcurrency(cfg.Concurrency))
	}

	bridge := hawkInspect.NewBridge(opts...)
	if !bridge.Ready() {
		return nil, "", fmt.Errorf("inspect bridge failed to initialize")
	}

	report, err := bridge.RunContracts(ctx, cfg.Target)
	if err != nil {
		return nil, "", fmt.Errorf("inspect scan failed: %w", err)
	}

	findings := InspectToReviewFindings(report)
	reportStr := formatInspectReport(report)
	return findings, reportStr, nil
}

// MergeInspectWithReview merges inspect findings into an existing review
// findings list, deduplicating by file+message.
func MergeInspectWithReview(reviewFindings, inspectFindings []ReviewFinding) []ReviewFinding {
	seen := map[string]bool{}
	var merged []ReviewFinding

	for _, f := range reviewFindings {
		key := f.File + ":" + f.Message
		seen[key] = true
		merged = append(merged, f)
	}

	for _, f := range inspectFindings {
		key := f.File + ":" + f.Message
		if !seen[key] {
			merged = append(merged, f)
		}
	}

	sortBySeverity(merged)
	return merged
}

// formatInspectReport creates a concise text summary of an inspect report.
func formatInspectReport(report *verifycontracts.Report) string {
	if report == nil {
		return "No inspect report."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Inspect: %s — %d pages scanned, %d findings",
		report.Target, report.CrawledURLs, report.Stats.FindingsTotal))

	if report.Stats.BySeverity != nil {
		b.WriteString(" (")
		first := true
		for sev, count := range report.Stats.BySeverity {
			if !first {
				b.WriteString(", ")
			}
			b.WriteString(fmt.Sprintf("%s:%d", sev, count))
			first = false
		}
		b.WriteString(")")
	}

	if report.Failed() {
		b.WriteString(fmt.Sprintf(" FAILED (threshold: %s)", report.FailOn))
	}

	return b.String()
}
