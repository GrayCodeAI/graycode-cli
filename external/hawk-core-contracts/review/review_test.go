package review

import (
	"testing"

	contracts "github.com/GrayCodeAI/hawk-core-contracts/types"
)

func TestResultFailedAndMaxSeverity(t *testing.T) {
	t.Parallel()

	result := &Result{
		FailOn: contracts.SeverityHigh,
		Findings: []Finding{
			{Severity: contracts.SeverityMedium},
			{Severity: contracts.SeverityCritical},
		},
	}

	if !result.Failed() {
		t.Fatal("expected result to fail at high threshold")
	}
	if got := result.MaxSeverity(); got != contracts.SeverityCritical {
		t.Fatalf("MaxSeverity = %v, want %v", got, contracts.SeverityCritical)
	}
}

func TestNilResultMethods(t *testing.T) {
	t.Parallel()

	var result *Result
	if result.Failed() {
		t.Fatal("nil result should not fail")
	}
	if got := result.MaxSeverity(); got != contracts.SeverityInfo {
		t.Fatalf("MaxSeverity(nil) = %v, want %v", got, contracts.SeverityInfo)
	}
}
