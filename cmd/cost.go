package cmd

import (
	"fmt"

	"github.com/GrayCodeAI/hawk/analytics"
	"github.com/spf13/cobra"
)

var costCmd = &cobra.Command{
	Use:   "cost",
	Short: "[Experimental] Analyze and optimize LLM API spend",
	Long: `cost provides analysis and optimization recommendations for LLM API
usage. It examines session data to identify wasteful spending patterns and
suggests model routing improvements.

NOTE: This feature is experimental. Cost tracking is not yet fully available;
session data integration is in progress.

Subcommands:
  analyze   Run a full cost optimization analysis
  summary   Show a quick spend summary`,
}

var costAnalyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Run a full cost optimization analysis",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Println("[Experimental] Cost tracking is not yet fully available.")
		cmd.Println()

		entries := []analytics.CostEntry{}
		report := analytics.Analyze(entries)

		if report.TotalSpend == 0 {
			cmd.Println("No cost data collected in this session.")
			cmd.Println()
			cmd.Println("Once session data integration is complete, the analyzer will support:")
			cmd.Println("  - Spend breakdown by model and task type")
			cmd.Println("  - Wasted spend detection (expensive models for simple tasks)")
			cmd.Println("  - Abandoned output tracking")
			cmd.Println("  - Model routing recommendations")
			cmd.Println("  - Prompt caching suggestions")
			cmd.Println()
			cmd.Println("To track progress: https://github.com/GrayCodeAI/hawk/issues")
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
		cmd.Println("[Experimental] Cost tracking is not yet fully available.")
		cmd.Println()

		entries := []analytics.CostEntry{}
		report := analytics.Analyze(entries)

		if report.TotalSpend == 0 {
			cmd.Println("No cost data collected in this session.")
			cmd.Println("Cost tracking will be available once session data integration is complete.")
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
