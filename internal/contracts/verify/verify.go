// Vendored from github.com/GrayCodeAI/eagle/verify at v0.0.0-20260902153929-5877bed17503 (MIT, Copyright (c) 2026 GrayCode AI).
// The upstream repository no longer exists; this copy is owned by Graycode as its contract surface.
package verify

import (
	"time"

	contracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/types"
)

// Finding is the neutral verification finding contract shared across Graycode and verification engines.
type Finding struct {
	Check    string             `json:"check"`
	Severity contracts.Severity `json:"severity"`
	URL      string             `json:"url"`
	Element  string             `json:"element,omitempty"`
	Message  string             `json:"message"`
	Fix      string             `json:"fix,omitempty"`
	Evidence string             `json:"evidence,omitempty"`
}

// Stats captures verification execution metrics.
type Stats struct {
	PagesScanned     int                        `json:"pages_scanned"`
	FindingsTotal    int                        `json:"findings_total"`
	BySeverity       map[contracts.Severity]int `json:"by_severity"`
	ByCheck          map[string]int             `json:"by_check"`
	DurationPerCheck map[string]time.Duration   `json:"duration_per_check"`
}

// Report is the neutral verification report contract.
type Report struct {
	Target      string             `json:"target"`
	Findings    []Finding          `json:"findings"`
	Stats       Stats              `json:"stats"`
	CrawledURLs int                `json:"crawled_urls"`
	Duration    time.Duration      `json:"duration"`
	FailOn      contracts.Severity `json:"fail_on"`
	// FailOnSet reports whether FailOn was explicitly configured via
	// SetFailOn. When it is false, Failed() treats SeverityCritical as the
	// effective threshold: an unset FailOn must not fail the report on
	// informational findings just because SeverityInfo is the zero value.
	FailOnSet bool `json:"fail_on_set,omitempty"`
}

// SetFailOn sets the fail threshold used by Failed. Set the threshold
// through this method rather than assigning FailOn directly, so that the
// threshold is recorded as explicitly configured.
func (r *Report) SetFailOn(sev contracts.Severity) {
	r.FailOn = sev
	r.FailOnSet = true
}

// Failed reports whether any finding meets or exceeds the configured fail threshold.
// When the threshold was never set — a zero Report, or a Report whose FailOn
// field was assigned directly — SeverityCritical is used as the effective
// threshold, mirroring review.Result.
func (r *Report) Failed() bool {
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

// MaxSeverity returns the highest severity present in the report.
func (r *Report) MaxSeverity() contracts.Severity {
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
