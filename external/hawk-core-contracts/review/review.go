package review

import (
	"time"

	contracts "github.com/GrayCodeAI/hawk-core-contracts/types"
)

// Finding is the neutral review finding contract shared across Hawk and review engines.
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
}

// ConfidenceBreakdown groups review findings by confidence band.
type ConfidenceBreakdown struct {
	High   []Finding `json:"high"`
	Medium []Finding `json:"medium"`
	Low    []Finding `json:"low"`
}

// Result is the neutral review result contract.
type Result struct {
	Findings            []Finding            `json:"findings"`
	Comments            []InlineComment      `json:"comments"`
	Stats               Stats                `json:"stats"`
	Report              string               `json:"report"`
	FailOn              contracts.Severity   `json:"fail_on"`
	ConfidenceBreakdown *ConfidenceBreakdown `json:"confidence_breakdown,omitempty"`
}

// Failed reports whether any finding meets or exceeds the configured fail threshold.
func (r *Result) Failed() bool {
	if r == nil {
		return false
	}
	for _, f := range r.Findings {
		if f.Severity.AtLeast(r.FailOn) {
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
