package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

const (
	modelTableColGap   = 4
	modelTableIndent   = 6  // left padding before table rows/header
	modelTableModelPad = 10 // extra room after longest model name
)

type modelTableLayout struct {
	Model   int
	Owner   int
	Price   int
	Context int
}

type modelTableRow struct {
	Model    string
	Provider string
	Price    string
	Context  string
	Free     bool
	Active   bool // configured in-use model — teal row + ●
}

func computeModelTableLayout(viewWidth int, rows []modelTableRow) modelTableLayout {
	if viewWidth <= 0 {
		viewWidth = 80
	}
	usable := viewWidth - modelTableIndent
	if usable < 48 {
		usable = 48
	}

	modelW := runewidth.StringWidth("Model")
	ownerW := runewidth.StringWidth("Owner")
	priceW := runewidth.StringWidth("Price")
	ctxW := runewidth.StringWidth("Ctx")
	for _, row := range rows {
		modelW = maxInt(modelW, runewidth.StringWidth(row.Model))
		ownerW = maxInt(ownerW, runewidth.StringWidth(row.Provider))
		priceW = maxInt(priceW, runewidth.StringWidth(row.Price))
		ctxText := row.Context
		if row.Active {
			ctxText += " ●"
		}
		ctxW = maxInt(ctxW, runewidth.StringWidth(ctxText))
	}

	ownerW += 2
	priceW += 2
	ctxW += 1

	gaps := modelTableColGap * 3
	modelW += modelTableModelPad
	maxModel := usable - ownerW - priceW - ctxW - gaps
	if maxModel < 20 {
		maxModel = 20
	}
	if modelW > maxModel {
		modelW = maxModel
	}

	return modelTableLayout{Model: modelW, Owner: ownerW, Price: priceW, Context: ctxW}
}

func modelTableRowFromOption(o configModelOption) modelTableRow {
	name := catalog.DisplayModelLabel(o.ID, o.DisplayName)
	if name == "" {
		name = shortModelID(o.ID)
	}
	owner := catalog.DisplayModelOwner(o.Owner, o.ID)
	if owner == "" {
		owner = "—"
	}
	free := o.InputPricePer1M <= 0 && o.OutputPricePer1M <= 0
	price := formatModelTablePriceCompact(o.InputPricePer1M, o.OutputPricePer1M)
	if free && price == "—" {
		price = "free"
	}
	return modelTableRow{
		Model:    name,
		Provider: owner,
		Price:    price,
		Context:  formatModelTableContext(o.ContextWindow),
		Free:     free,
	}
}

func formatModelTablePrice(input, output float64) string {
	if input <= 0 && output <= 0 {
		return "—"
	}
	return fmt.Sprintf("$%s/$%s/M", formatPriceComponent(input), formatPriceComponent(output))
}

func formatModelTablePriceCompact(input, output float64) string {
	if input <= 0 && output <= 0 {
		return "—"
	}
	return fmt.Sprintf("$%s/$%s", formatPriceComponent(input), formatPriceComponent(output))
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
		if n%1_000_000 == 0 {
			return fmt.Sprintf("%dm", n/1_000_000)
		}
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

// parseContextWindowLabel reverses formatModelTableContext labels like "131k" or "1.0m".
func parseContextWindowLabel(label string) int {
	label = strings.TrimSpace(label)
	if label == "" || label == "—" {
		return 0
	}
	mult := 1
	switch {
	case strings.HasSuffix(label, "m"):
		mult = 1_000_000
		label = strings.TrimSuffix(label, "m")
	case strings.HasSuffix(label, "k"):
		mult = 1000
		label = strings.TrimSuffix(label, "k")
	}
	f, err := strconv.ParseFloat(label, 64)
	if err != nil || f <= 0 {
		return 0
	}
	return int(f * float64(mult))
}

func renderModelTableHeader(layout modelTableLayout, headerStyle, metaStyle lipgloss.Style) string {
	line := renderModelTableLine(
		[]string{"Model", "Owner", "Price", "Ctx"},
		layout,
		[]lipgloss.Style{headerStyle, headerStyle, headerStyle, headerStyle},
	)
	ruleLen := layout.Model + layout.Owner + layout.Price + layout.Context + modelTableColGap*3
	indent := strings.Repeat(" ", modelTableIndent)
	return indent + line + "\n" + indent + metaStyle.Render(strings.Repeat("─", ruleLen))
}

func renderModelTableRow(row modelTableRow, cursor, active bool, layout modelTableLayout, _, cursorStyle, activeStyle, metaStyle, freeStyle lipgloss.Style) string {
	meta := metaStyle
	priceStyle := metaStyle
	if row.Free {
		priceStyle = freeStyle
	}
	if active && !cursor {
		meta = activeStyle
		priceStyle = activeStyle
	}
	if cursor {
		meta = cursorStyle
		priceStyle = cursorStyle
	}
	prefix := strings.Repeat(" ", modelTableIndent)
	if cursor {
		prefix = strings.Repeat(" ", modelTableIndent-2) + cursorStyle.Render(iconPrompt) + " "
	}

	ctx := truncateRunes(row.Context, layout.Context)
	if active {
		ctx = truncateRunes(row.Context+" ●", layout.Context)
	}

	line := renderModelTableLine(
		[]string{
			truncateRunes(row.Model, layout.Model),
			truncateRunes(row.Provider, layout.Owner),
			truncateRunes(row.Price, layout.Price),
			ctx,
		},
		layout,
		[]lipgloss.Style{meta, meta, priceStyle, meta},
	)
	return prefix + line
}

func renderModelTableLine(values []string, layout modelTableLayout, styles []lipgloss.Style) string {
	widths := []int{layout.Model, layout.Owner, layout.Price, layout.Context}
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = styles[i].Render(padCellLeft(v, widths[i]))
	}
	return strings.Join(parts, strings.Repeat(" ", modelTableColGap))
}

func padCellLeft(value string, width int) string {
	if width <= 0 {
		return ""
	}
	text := truncateRunes(value, width)
	pad := width - runewidth.StringWidth(text)
	if pad < 0 {
		pad = 0
	}
	return text + strings.Repeat(" ", pad)
}

func truncateRunes(s string, maxCols int) string {
	if maxCols <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= maxCols {
		return s
	}
	if maxCols <= 1 {
		return "…"
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		if used+w > maxCols-1 {
			break
		}
		b.WriteRune(r)
		used += w
	}
	return b.String() + "…"
}

func modelTableScrollHint(above, below int, style lipgloss.Style) string {
	prefix := strings.Repeat(" ", modelTableIndent)
	switch {
	case above > 0 && below > 0:
		return style.Render(fmt.Sprintf("%s↑ %d above · ↓ %d below", prefix, above, below))
	case above > 0:
		return style.Render(fmt.Sprintf("%s↑ %d above", prefix, above))
	case below > 0:
		return style.Render(fmt.Sprintf("%s↓ %d below", prefix, below))
	default:
		return ""
	}
}

func modelTableFooter(total, scroll, end, allTotal int, muted lipgloss.Style) string {
	prefix := strings.Repeat(" ", modelTableIndent)
	if total == 0 {
		return muted.Render(prefix + "No models")
	}
	start := scroll + 1
	if end > total {
		end = total
	}
	if start > end {
		start = end
	}
	label := fmt.Sprintf("%d–%d of %d", start, end, total)
	if allTotal > 0 && total < allTotal {
		label += fmt.Sprintf(" (%d total)", allTotal)
	}
	return muted.Render(fmt.Sprintf("%s%s · enter to select", prefix, label))
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
	free := m.InputPricePer1M <= 0 && m.OutputPricePer1M <= 0
	price := formatModelTablePriceCompact(m.InputPricePer1M, m.OutputPricePer1M)
	if free && price == "—" {
		price = "free"
	}
	return modelTableRow{
		Model:    name,
		Provider: owner,
		Price:    price,
		Context:  formatModelTableContext(m.ContextWindow),
		Free:     free,
	}
}

func printModelTablePlain(rows []modelTableRow) {
	layout := computeModelTableLayout(100, rows)
	header := lipgloss.NewStyle().Bold(true)
	meta := lipgloss.NewStyle()
	fmt.Println(renderModelTableHeader(layout, header, meta))
	for _, row := range rows {
		fmt.Println(renderModelTableRow(row, false, false, layout, lipgloss.NewStyle(), lipgloss.NewStyle(), lipgloss.NewStyle(), meta, meta))
	}
}
