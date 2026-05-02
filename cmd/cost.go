package cmd

import (
	"fmt"

	"github.com/GrayCodeAI/hawk/analytics"
	"github.com/spf13/cobra"
)

var costCmd = &cobra.Command{
	Use:   "cost",
	Short: "Analyze and optimize LLM API spend",
	Long: `cost provides analysis and optimization recommendations for LLM API
usage. It examines session data to identify wasteful spending patterns and
suggests model routing improvements.

Subcommands:
  analyze   Run a full cost optimization analysis
  summary   Show a quick spend summary`,
}

var costAnalyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Run a full cost optimization analysis",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Wire up to actual session data once the session store
		// exposes CostEntry records. For now, show a placeholder that
		// demonstrates the analytics pipeline works.
		entries := []analytics.CostEntry{}
		report := analytics.Analyze(entries)

		if report.TotalSpend == 0 {
			cmd.Println("No cost data available yet.")
			cmd.Println()
			cmd.Println("Cost analysis will be available once session data is integrated.")
			cmd.Println("The analyzer supports:")
			cmd.Println("  - Spend breakdown by model and task type")
			cmd.Println("  - Wasted spend detection (expensive models for simple tasks)")
			cmd.Println("  - Abandoned output tracking")
			cmd.Println("  - Model routing recommendations")
			cmd.Println("  - Prompt caching suggestions")
			return nil
		}

		cmd.Print(analytics.FormatOptimizationReport(report))
		return nil
	},
}

var costSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show a quick spend summary",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Wire up to actual session data.
		entries := []analytics.CostEntry{}
		report := analytics.Analyze(entries)

		if report.TotalSpend == 0 {
			cmd.Println("No cost data available yet.")
			cmd.Println("Run hawk with LLM interactions to start collecting cost data.")
			return nil
		}

		cmd.Println(fmt.Sprintf("Total spend:      $%.4f", report.TotalSpend))
		cmd.Println(fmt.Sprintf("Productive spend: $%.4f", report.ProductiveSpend))
		cmd.Println(fmt.Sprintf("Wasted spend:     $%.4f", report.WastedSpend))
		cmd.Println(fmt.Sprintf("Yield rate:       %.1f%%", report.YieldRate*100))

		if len(report.Recommendations) > 0 {
			cmd.Println()
			cmd.Println("Top recommendation:")
			rec := report.Recommendations[0]
			cmd.Println(fmt.Sprintf("  [%s] %s (est. savings: $%.4f)", rec.Type, rec.Description, rec.Savings))
		}
		return nil
	},
}

func init() {
	costCmd.AddCommand(costAnalyzeCmd)
	costCmd.AddCommand(costSummaryCmd)
}
