package cmd

import (
	"fmt"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/charmbracelet/lipgloss"
)

const (
	modelTableColModel    = 28
	modelTableColProvider = 12
	modelTableColPrice    = 14
	modelTableColContext  = 8
)

type modelTableRow struct {
	Model    string
	Provider string
	Price    string
	Context  string
}

func modelTableRowFromOption(o configModelOption) modelTableRow {
	name := strings.TrimSpace(o.DisplayName)
	if name == "" {
		name = o.ID
	}
	owner := strings.TrimSpace(o.Owner)
	if owner == "" {
		owner = "—"
	}
	return modelTableRow{
		Model:    name,
		Provider: owner,
		Price:    formatModelTablePrice(o.InputPricePer1M, o.OutputPricePer1M),
		Context:  formatModelTableContext(o.ContextWindow),
	}
}

func formatModelTablePrice(input, output float64) string {
	if input <= 0 && output <= 0 {
		return "—"
	}
	return fmt.Sprintf("$%s/$%s/M", formatPriceComponent(input), formatPriceComponent(output))
}

func formatPriceComponent(v float64) string {
	if v == 0 {
		return "0"
	}
	abs := v
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs < 0.01:
		return fmt.Sprintf("%.3f", v)
	case abs < 1:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
	case abs < 10:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
	default:
		if v == float64(int(v)) {
			return fmt.Sprintf("%.0f", v)
		}
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", v), "0"), ".")
	}
}

func formatModelTableContext(n int) string {
	if n <= 0 {
		return "—"
	}
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fm", float64(n)/1_000_000)
	}
	if n >= 1000 {
		if n%1000 == 0 {
			return fmt.Sprintf("%dk", n/1000)
		}
		return fmt.Sprintf("%.0fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func renderModelTableHeader(headerStyle lipgloss.Style) string {
	return headerStyle.Render(padModelTable(
		"Model", "Owner", "Price", "Context",
		modelTableColModel, modelTableColProvider, modelTableColPrice, modelTableColContext,
	))
}

func renderModelTableRow(row modelTableRow, selected bool, rowStyle, selectedStyle lipgloss.Style) string {
	prefix := "  "
	style := rowStyle
	if selected {
		prefix = "❯ "
		style = selectedStyle
	}
	line := padModelTable(
		row.Model, row.Provider, row.Price, row.Context,
		modelTableColModel, modelTableColProvider, modelTableColPrice, modelTableColContext,
	)
	return style.Render(prefix + line)
}

func padModelTable(c1, c2, c3, c4 string, w1, w2, w3, w4 int) string {
	return fmt.Sprintf("%-*s %-*s %-*s %-*s", w1, truncateRunes(c1, w1), w2, truncateRunes(c2, w2), w3, truncateRunes(c3, w3), w4, truncateRunes(c4, w4))
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

func modelTableRowFromCatalogEntry(m catalog.ModelCatalogEntry) modelTableRow {
	name := strings.TrimSpace(m.DisplayName)
	if name == "" {
		name = m.ID
	}
	owner := catalog.ModelOwner(m)
	if owner == "" {
		owner = "—"
	}
	return modelTableRow{
		Model:    name,
		Provider: owner,
		Price:    formatModelTablePrice(m.InputPricePer1M, m.OutputPricePer1M),
		Context:  formatModelTableContext(m.ContextWindow),
	}
}

func printModelTablePlain(rows []modelTableRow) {
	fmt.Println(padModelTable(
		"Model", "Owner", "Price", "Context",
		modelTableColModel, modelTableColProvider, modelTableColPrice, modelTableColContext,
	))
	for _, row := range rows {
		fmt.Println(padModelTable(
			row.Model, row.Provider, row.Price, row.Context,
			modelTableColModel, modelTableColProvider, modelTableColPrice, modelTableColContext,
		))
	}
}
