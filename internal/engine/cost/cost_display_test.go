package cost

import (
	"testing"
)

func TestFormatCostDisplay(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cost float64
		want string
	}{
		{"zero", 0, ""},
		{"negative", -1.0, ""},
		{"sub-cent", 0.005, "$0.0050"},
		{"cents", 0.15, "$0.150"},
		{"dollar", 2.5, "$2.50"},
		{"large", 100.0, "$100.00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatCostDisplay(tt.cost)
			if got != tt.want {
				t.Errorf("FormatCostDisplay(%f) = %q, want %q", tt.cost, got, tt.want)
			}
		})
	}
}


