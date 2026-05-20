package cmd

import (
	"strings"
	"testing"
)

func TestFormatModelTablePrice(t *testing.T) {
	if got := formatModelTablePrice(0.5, 3); got != "$0.5/$3/M" {
		t.Fatalf("price = %q", got)
	}
	if got := formatModelTablePrice(95, 400); got != "$95/$400/M" {
		t.Fatalf("price = %q", got)
	}
	if got := formatModelTablePrice(0, 0); got != "—" {
		t.Fatalf("price = %q", got)
	}
}

func TestFormatModelTableContext(t *testing.T) {
	cases := map[int]string{
		0:       "—",
		32000:   "32k",
		262144:  "262k",
		1000000: "1.0m",
		2048000: "2.0m",
	}
	for n, want := range cases {
		if got := formatModelTableContext(n); got != want {
			t.Fatalf("context(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestPadModelTable(t *testing.T) {
	line := padModelTable("Kimi-K2.6", "moonshotai", "$95/$400/M", "262k", 28, 12, 14, 8)
	for _, part := range []string{"Kimi-K2.6", "moonshotai", "$95/$400/M", "262k"} {
		if !strings.Contains(line, part) {
			t.Fatalf("line = %q, missing %q", line, part)
		}
	}
}
