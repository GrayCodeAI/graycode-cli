package cmd

import (
	"context"
	"fmt"
	"strings"

	graphcontracts "github.com/GrayCodeAI/eagle/graph"
	contracts "github.com/GrayCodeAI/eagle/types"
	verifycontracts "github.com/GrayCodeAI/eagle/verify"
	hawkMerlin "github.com/GrayCodeAI/hawk/internal/bridge/merlin"
	merlinLib "github.com/GrayCodeAI/merlin"
)

// MerlinPipelineConfig controls how merlin integrates into the review pipeline.
type MerlinPipelineConfig struct {
	Target      string // URL to merlin
	Depth       int
	Checks      []string // which checks to run
	FailOn      string   // severity threshold
	Concurrency int
	// GraphSessionID enables portable quality-graph journaling for this scan.
	GraphSessionID  string
	GraphToolCallID string
	RepositoryID    string
}

// DefaultMerlinPipelineConfig returns sensible defaults.
func DefaultMerlinPipelineConfig(target string) MerlinPipelineConfig {
	return MerlinPipelineConfig{
		Target:      target,
		Depth:       3,
		Checks:      []string{"links", "security", "forms", "a11y", "perf", "seo"},
		FailOn:      "medium",
		Concurrency: 5,
	}
}

// MerlinToReviewFindings converts an merlin report into ReviewFindings
// so they can be displayed alongside code review findings.
func MerlinToReviewFindings(report *verifycontracts.Report) []ReviewFinding {
	if report == nil {
		return nil
	}

	var findings []ReviewFinding
	for _, f := range report.Findings {
		severity := mapMerlinSeverity(f.Severity)
		findings = append(findings, ReviewFinding{
			Concern:  "merlin:" + f.Check,
			Severity: severity,
			File:     f.URL,
			Line:     0,
			Message:  f.Message,
			Fix:      f.Fix,
		})
	}
	return findings
}

// mapMerlinSeverity converts merlin severity to review severity.
func mapMerlinSeverity(sev contracts.Severity) string {
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

// RunMerlinPipeline runs an merlin scan and returns results formatted
// as review findings that can be merged with code review results.
func RunMerlinPipeline(ctx context.Context, cfg MerlinPipelineConfig) ([]ReviewFinding, string, error) {
	var opts []merlinLib.Option
	if cfg.Depth > 0 {
		opts = append(opts, merlinLib.WithDepth(cfg.Depth))
	}
	if len(cfg.Checks) > 0 {
		opts = append(opts, merlinLib.WithChecks(cfg.Checks...))
	}
	if cfg.FailOn != "" {
		opts = append(opts, merlinLib.WithFailOn(merlinLib.ParseSeverity(cfg.FailOn)))
	}
	if cfg.Concurrency > 0 {
		opts = append(opts, merlinLib.WithConcurrency(cfg.Concurrency))
	}

	bridge := hawkMerlin.NewBridge(opts...)
	if !bridge.Ready() {
		return nil, "", fmt.Errorf("merlin bridge failed to initialize")
	}

	var (
		report *verifycontracts.Report
		err    error
	)
	if strings.TrimSpace(cfg.GraphSessionID) == "" {
		report, err = bridge.RunContracts(ctx, cfg.Target)
	} else {
		report, err = bridge.RunContractsObserved(ctx, cfg.Target, hawkMerlin.GraphObservation{
			SessionID:  cfg.GraphSessionID,
			ToolCallID: cfg.GraphToolCallID,
			Stage:      "merlin-pipeline",
			Scope: graphcontracts.Scope{
				RepositoryID: strings.TrimSpace(cfg.RepositoryID),
			},
		})
	}
	if err != nil {
		return nil, "", fmt.Errorf("merlin scan failed: %w", err)
	}

	findings := MerlinToReviewFindings(report)
	reportStr := formatMerlinReport(report)
	return findings, reportStr, nil
}

// MergeMerlinWithReview merges merlin findings into an existing review
// findings list, deduplicating by file+message.
func MergeMerlinWithReview(reviewFindings, merlinFindings []ReviewFinding) []ReviewFinding {
	seen := map[string]bool{}
	var merged []ReviewFinding

	for _, f := range reviewFindings {
		key := f.File + ":" + f.Message
		seen[key] = true
		merged = append(merged, f)
	}

	for _, f := range merlinFindings {
		key := f.File + ":" + f.Message
		if !seen[key] {
			merged = append(merged, f)
		}
	}

	sortBySeverity(merged)
	return merged
}

// formatMerlinReport creates a concise text summary of an merlin report.
func formatMerlinReport(report *verifycontracts.Report) string {
	if report == nil {
		return "No merlin report."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Merlin: %s — %d pages scanned, %d findings",
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
