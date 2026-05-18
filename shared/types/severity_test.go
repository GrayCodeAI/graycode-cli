package types_test

import (
	"testing"

	"github.com/GrayCodeAI/hawk/shared/types"
)

func TestParseSeverity_forwarded(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want types.Severity
	}{
		{"critical", types.SeverityCritical},
		{"high", types.SeverityHigh},
		{"medium", types.SeverityMedium},
		{"low", types.SeverityLow},
		{"info", types.SeverityInfo},
		{"unknown", types.SeverityInfo},
	}
	for _, tc := range cases {
		if got := types.ParseSeverity(tc.in); got != tc.want {
			t.Fatalf("ParseSeverity(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
