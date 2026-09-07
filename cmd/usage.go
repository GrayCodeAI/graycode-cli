package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/GrayCodeAI/graycode-cli/internal/usage"
	"github.com/spf13/cobra"
)

var (
	usagePeriod string
	usageJSON   bool
	usageLedger string // override ledger path for tests; empty uses the default
)

var usageCmd = &cobra.Command{
	Use:   "usage [--period <24h|7d|30d>]",
	Short: "Show local LLM token usage and spend (fx usage parity)",
	Long: `Show per-model token usage and spend from the local usage ledger,
mirroring fx's "usage" command.

Each model generation is recorded to the ledger as it completes. This command
summarizes the ledger over a rolling window:
  --period 24h   the last 24 hours (default)
  --period 7d    the last 7 days
  --period 30d   the last 30 days

Use --json to emit the summary in machine-readable form.`,
	Args: cobra.NoArgs,
	RunE: runUsage,
}

func init() {
	usageCmd.Flags().StringVar(&usagePeriod, "period", "24h", "window: 24h, 7d, or 30d")
	usageCmd.Flags().BoolVar(&usageJSON, "json", false, "output the summary as JSON")
	rootCmd.AddCommand(usageCmd)
}

func runUsage(cmd *cobra.Command, _ []string) error {
	sinceMS, _, err := usage.ParsePeriod(usagePeriod)
	if err != nil {
		return err
	}

	records, err := readUsageLedger()
	if err != nil {
		return err
	}
	sum := usage.Summarize(records, sinceMS)

	if usageJSON {
		raw, err := json.MarshalIndent(sum, "", "  ")
		if err != nil {
			return fmt.Errorf("usage: marshal json: %w", err)
		}
		_, _ = cmd.OutOrStdout().Write(raw)
		_, _ = fmt.Fprintln(cmd.OutOrStdout())
		return nil
	}

	if sum.Generations == 0 {
		cmd.Println(auditTint("No usage recorded in the last "+usagePeriod+".", textMuted))
		cmd.Println(auditTint("The ledger lives at "+usage.LedgerPath(), textMuted))
		return nil
	}

	cmd.Println(auditTint(fmt.Sprintf("Usage (last %s)", usagePeriod), graycodeColor))
	cmd.Println(auditTint(fmt.Sprintf("%-28s %10s %10s %8s %12s", "model", "in", "out", "gen", "cost"), textMuted))
	for _, m := range sum.ByModel {
		// Pad the cost to its column width first, then colorize, so the
		// zero-width ANSI escapes don't break the fixed-width alignment.
		cost := auditTint(fmt.Sprintf("%10.4f$", m.TotalCostUSD), costViolet)
		cmd.Println(fmt.Sprintf("%-28s %10d %10d %8d %s",
			truncateModel(m.Model), m.InputTokens, m.OutputTokens, m.Generations, cost))
	}
	cmd.Println(auditTint("------------------------------------------------------------", textMuted))
	cmd.Println(fmt.Sprintf("%-28s %10d %10s %8d %s",
		auditTint("total", textPrimary), sum.TotalTokens, "", sum.Generations,
		auditTint(fmt.Sprintf("%10.4f$", sum.TotalCostUSD), costViolet)))
	return nil
}

func truncateModel(m string) string {
	if len(m) > 28 {
		return m[:27] + "…"
	}
	return m
}

// readUsageLedger returns ledger records, honoring an optional test override.
func readUsageLedger() ([]usage.Record, error) {
	if usageLedger != "" {
		return usage.ReadFrom(usageLedger)
	}
	return usage.Read()
}
