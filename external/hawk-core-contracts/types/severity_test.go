package types_test

import (
	"testing"

	"github.com/GrayCodeAI/hawk-core-contracts/types"
)

func TestParseSeverity(t *testing.T) {
	tests := []struct {
		in   string
		want types.Severity
	}{
		{in: "critical", want: types.SeverityCritical},
		{in: "HIGH", want: types.SeverityHigh},
		{in: " medium ", want: types.SeverityMedium},
		{in: "low", want: types.SeverityLow},
		{in: "unknown", want: types.SeverityInfo},
	}

	for _, tt := range tests {
		if got := types.ParseSeverity(tt.in); got != tt.want {
			t.Fatalf("ParseSeverity(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
