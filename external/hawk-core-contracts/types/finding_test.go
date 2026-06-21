package types_test

import (
	"testing"

	"github.com/GrayCodeAI/hawk-core-contracts/types"
)

func TestFindingSliceFilterBySeverity(t *testing.T) {
	findings := types.FindingSlice{
		{ID: "a", Severity: types.SeverityLow, Confidence: 0.2},
		{ID: "b", Severity: types.SeverityHigh, Confidence: 0.8},
		{ID: "c", Severity: types.SeverityCritical, Confidence: 0.9},
	}

	got := findings.FilterBySeverity(types.SeverityHigh)
	if len(got) != 2 {
		t.Fatalf("FilterBySeverity() len = %d, want 2", len(got))
	}
}
