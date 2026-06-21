package verify

import (
	"time"

	contracts "github.com/GrayCodeAI/hawk-core-contracts/types"
)

// Finding is the neutral verification finding contract shared across Hawk and verification engines.
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
}

// Failed reports whether any finding meets or exceeds the configured fail threshold.
func (r *Report) Failed() bool {
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
