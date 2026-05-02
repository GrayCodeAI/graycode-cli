package engine

import "fmt"

// FormatCostDisplay returns a compact cost string for the status bar.
func FormatCostDisplay(totalUSD float64) string {
	if totalUSD <= 0 {
		return ""
	}
	if totalUSD < 0.01 {
		return fmt.Sprintf("$%.4f", totalUSD)
	}
	if totalUSD < 1.0 {
		return fmt.Sprintf("$%.3f", totalUSD)
	}
	return fmt.Sprintf("$%.2f", totalUSD)
}
