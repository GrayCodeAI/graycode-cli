package cmd

import (
	"testing"
)

func TestFormatHawkTokenCount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{1234, "1234"},
		{9999, "9999"},
		{10_000, "10k"},
		{150_000, "150k"},
		{999_999, "999k"},
		{1_000_000, "1m"},
		{1_500_000, "1.5m"},
		{12_345_678, "12.3m"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run("", func(t *testing.T) {
			t.Parallel()
			got := formatHawkTokenCount(tc.in)
			if got != tc.want {
				t.Fatalf("formatHawkTokenCount(%d) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
