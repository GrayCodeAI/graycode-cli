package cmd

import (
	"strings"
	"testing"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func TestFormatModelTablePrice(t *testing.T) {
	if got := formatModelTablePrice(0.5, 3); got != "$0.5/$3/M" {
		t.Fatalf("price = %q", got)
	}
	if got := formatModelTablePrice(0, 0); got != "—" {
		t.Fatalf("price = %q", got)
	}
}

func TestFormatModelTablePriceCompact(t *testing.T) {
	if got := formatModelTablePriceCompact(95, 400); got != "$95/$400" {
		t.Fatalf("price = %q", got)
	}
}

func TestFormatModelTableContext(t *testing.T) {
	cases := map[int]string{
		0:       "—",
		32000:   "32k",
		1000000: "1m",
		131072:  "131k",
	}
	for n, want := range cases {
		if got := formatModelTableContext(n); got != want {
			t.Fatalf("context(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestComputeModelTableLayoutFitsModelNames(t *testing.T) {
	rows := []modelTableRow{
		{Model: "anthropic/claude-opus-4.7-fast", Provider: "anthropic", Price: "$30/$150", Context: "1m"},
		{Model: "qwen/qwen3.7-max", Provider: "qwen", Price: "$2.5/$7.5", Context: "1m"},
	}
	layout := computeModelTableLayout(100, rows)
	if layout.Model < len("anthropic/claude-opus-4.7-fast") {
		t.Fatalf("model column too narrow: %+v", layout)
	}
}

func TestComputeModelTableLayoutLeftPacked(t *testing.T) {
	rows := []modelTableRow{{Model: "qwen/qwen3.7-max", Provider: "qwen", Price: "$2.5/$7.5", Context: "1m"}}
	layout := computeModelTableLayout(120, rows)
	total := layout.Model + layout.Owner + layout.Price + layout.Context + modelTableColGap*3
	if total > 92 {
		t.Fatalf("expected compact left-aligned table, got %+v total=%d", layout, total)
	}
}

func TestModelTableRowFromOptionFree(t *testing.T) {
	row := modelTableRowFromOption(configModelOption{
		ID: "baidu/cobuddy:free", DisplayName: "baidu/cobuddy:free", Owner: "baidu", PriceKnown: true,
	})
	if row.Price != "free" || !row.Free {
		t.Fatalf("row = %+v", row)
	}
}

func TestModelTableRowFromOptionUnknownPrice(t *testing.T) {
	row := modelTableRowFromOption(configModelOption{
		ID: "deepseek-v4-flash", DisplayName: "deepseek-v4-flash", Owner: "opencode",
	})
	if row.Price != "—" || row.Free {
		t.Fatalf("row = %+v", row)
	}
}

func TestModelTableOwnerFallback(t *testing.T) {
	option := modelTableRowFromOption(configModelOption{ProviderID: "deployment", GatewayID: "gateway"})
	if option.Provider != "deployment" {
		t.Fatalf("option provider = %q, want deployment", option.Provider)
	}
	entry := modelTableRowFromCatalogEntry(hawkconfig.EngineModel{GatewayID: "gateway"})
	if entry.Provider != "gateway" {
		t.Fatalf("entry provider = %q, want gateway", entry.Provider)
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("anthropic/claude-opus-4.7-fast", 50); strings.Contains(got, "…") {
		t.Fatalf("unexpected truncation: %q", got)
	}
	if got := truncateRunes("anthropic/claude-opus-4.7-fast", 10); !strings.HasSuffix(got, "…") {
		t.Fatalf("expected truncation: %q", got)
	}
}

func TestParseContextWindowLabelRoundTrip(t *testing.T) {
	for _, n := range []int{32000, 131000, 1_000_000} {
		label := formatModelTableContext(n)
		if got := parseContextWindowLabel(label); got != n {
			t.Fatalf("round trip %d -> %q -> %d", n, label, got)
		}
	}
}
