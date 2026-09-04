// Vendored from github.com/GrayCodeAI/eagle/review at v0.0.0-20260902153929-5877bed17503 (MIT, Copyright (c) 2026 GrayCode AI).
// The upstream repository no longer exists; this copy is owned by Graycode as its contract surface.
package review

import (
	"fmt"
	"strings"
	"time"

	contracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/types"
)

// Finding is the neutral review finding contract shared across Graycode and review engines.
type Finding struct {
	Concern    string             `json:"concern"`
	Severity   contracts.Severity `json:"severity"`
	File       string             `json:"file"`
	Line       int                `json:"line"`
	EndLine    int                `json:"end_line,omitempty"`
	Message    string             `json:"message"`
	Fix        string             `json:"fix,omitempty"`
	Reasoning  string             `json:"reasoning,omitempty"`
	CWE        string             `json:"cwe,omitempty"`
	Confidence float64            `json:"confidence"`
	SASTSource bool               `json:"sast_source,omitempty"`
}

// Validate reports whether the finding satisfies the minimum contract
// invariants: a non-blank Message, a non-negative Line, and a Confidence
// within [0, 1]. It returns a descriptive error naming the first violated
// field.
func (f Finding) Validate() error {
	if strings.TrimSpace(f.Message) == "" {
		return fmt.Errorf("finding message is empty")
	}
	if f.Line < 0 {
		return fmt.Errorf("finding line %d is negative", f.Line)
	}
	if f.Confidence < 0 || f.Confidence > 1 {
		return fmt.Errorf("finding confidence %v is outside [0, 1]", f.Confidence)
	}
	return nil
}

// InlineComment is a review finding mapped to a concrete diff position.
type InlineComment struct {
	Path       string `json:"path"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line,omitempty"`
	Body       string `json:"body"`
	Suggestion string `json:"suggestion,omitempty"`
}

// Stats captures review execution metrics.
type Stats struct {
	FilesReviewed       int                        `json:"files_reviewed"`
	HunksAnalyzed       int                        `json:"hunks_analyzed"`
	FindingsTotal       int                        `json:"findings_total"`
	BySeverity          map[contracts.Severity]int `json:"by_severity"`
	ByConcern           map[string]int             `json:"by_concern"`
	TokensUsed          int                        `json:"tokens_used"`
	DurationPerConcern  map[string]time.Duration   `json:"duration_per_concern"`
	AverageConfidence   float64                    `json:"average_confidence"`
	HighConfidenceCount int                        `json:"high_confidence_count"`
	LowConfidenceCount  int                        `json:"low_confidence_count"`
	// LLMErrors records non-fatal provider errors encountered during
	// analysis; findings may be partial when it is non-empty.
	LLMErrors []string `json:"llm_errors,omitempty"`
}

// ConfidenceBreakdown groups review findings by confidence band.
type ConfidenceBreakdown struct {
	High   []Finding `json:"high"`
	Medium []Finding `json:"medium"`
	Low    []Finding `json:"low"`
}

// SASTFusionResult tracks how the LLM handled SAST findings during a review.
// Only populated when SAST-LLM fusion is active (preAnalysis enabled).
type SASTFusionResult struct {
	Confirmed   []Finding `json:"confirmed"`
	Dismissed   []Finding `json:"dismissed"`
	Unaddressed []Finding `json:"unaddressed"`
}

// Result is the neutral review result contract.
type Result struct {
	Findings []Finding          `json:"findings"`
	Comments []InlineComment    `json:"comments"`
	Stats    Stats              `json:"stats"`
	Report   string             `json:"report"`
	FailOn   contracts.Severity `json:"fail_on"`
	// FailOnSet reports whether FailOn was explicitly configured via
	// SetFailOn. When it is false, Failed() treats SeverityCritical as the
	// effective threshold: an unset FailOn must not fail the review on
	// informational findings just because SeverityInfo is the zero value.
	FailOnSet           bool                 `json:"fail_on_set,omitempty"`
	SASTFusion          *SASTFusionResult    `json:"sast_fusion,omitempty"`
	ConfidenceBreakdown *ConfidenceBreakdown `json:"confidence_breakdown,omitempty"`
}

// SetFailOn sets the fail threshold used by Failed. Set the threshold
// through this method rather than assigning FailOn directly, so that the
// threshold is recorded as explicitly configured.
func (r *Result) SetFailOn(sev contracts.Severity) {
	r.FailOn = sev
	r.FailOnSet = true
}

// Failed reports whether any finding meets or exceeds the configured fail threshold.
// When the threshold was never set — a zero Result, or a Result whose FailOn
// field was assigned directly — SeverityCritical is used as the effective
// threshold, matching the kestrel and merlin engine defaults. Set the
// threshold via SetFailOn to make an explicit choice (including Info) take
// effect.
func (r *Result) Failed() bool {
	if r == nil {
		return false
	}
	threshold := r.FailOn
	if !r.FailOnSet {
		threshold = contracts.SeverityCritical
	}
	for _, f := range r.Findings {
		if f.Severity.AtLeast(threshold) {
			return true
		}
	}
	return false
}

// MaxSeverity returns the highest severity present in the result.
func (r *Result) MaxSeverity() contracts.Severity {
	if r == nil {
		return contracts.SeverityInfo
	}
	max := contracts.SeverityInfo
	for _, f := range r.Findings {
		if f.Severity > max {
			max = f.Severity
		}
	}
	return max
}
