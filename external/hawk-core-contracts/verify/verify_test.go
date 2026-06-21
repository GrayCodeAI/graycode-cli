package verify

import (
	"testing"

	contracts "github.com/GrayCodeAI/hawk-core-contracts/types"
)

func TestReportFailedAndMaxSeverity(t *testing.T) {
	t.Parallel()

	report := &Report{
		FailOn: contracts.SeverityMedium,
		Findings: []Finding{
			{Severity: contracts.SeverityLow},
			{Severity: contracts.SeverityHigh},
		},
	}

	if !report.Failed() {
		t.Fatal("expected report to fail at medium threshold")
	}
	if got := report.MaxSeverity(); got != contracts.SeverityHigh {
		t.Fatalf("MaxSeverity = %v, want %v", got, contracts.SeverityHigh)
	}
}

func TestNilReportMethods(t *testing.T) {
	t.Parallel()

	var report *Report
	if report.Failed() {
		t.Fatal("nil report should not fail")
	}
	if got := report.MaxSeverity(); got != contracts.SeverityInfo {
		t.Fatalf("MaxSeverity(nil) = %v, want %v", got, contracts.SeverityInfo)
	}
}
