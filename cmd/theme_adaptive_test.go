package cmd

import (
	"fmt"
	"image/color"
	"testing"

	"charm.land/lipgloss/v2/compat"
)

// TestAdaptiveNeutralsPreserveDarkAppearance locks the Dark variant of each
// adaptive neutral color to its historical value, so dark terminals (the
// default) keep rendering exactly as before. The Light variant must differ
// (that is the whole point of the change) and must be non-empty.
func TestAdaptiveNeutralsPreserveDarkAppearance(t *testing.T) {
	cases := []struct {
		name     string
		color    compat.AdaptiveColor
		wantDark string
	}{
		{"textPrimary", textPrimary, "#F0F0F0"},
		{"textMuted", textMuted, "#9E9E9E"},
		{"textDisabled", textDisabled, "#666666"},
		{"borderDim", borderDim, "#555555"},
	}
	for _, c := range cases {
		want := colorHex(c.color.Dark)
		if want != c.wantDark {
			t.Errorf("%s: Dark = %q, want %q (dark-mode appearance must not change)", c.name, c.color.Dark, c.wantDark)
		}
		if c.color.Light == nil {
			t.Errorf("%s: Light variant is empty; light terminals need a legible ink", c.name)
		}
		if fmt.Sprint(c.color.Light) == fmt.Sprint(c.color.Dark) {
			t.Errorf("%s: Light == Dark (%q); adaptive conversion had no effect", c.name, c.color.Light)
		}
	}
}

func colorHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
}
