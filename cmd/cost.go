package cmd

import (
	"encoding/json"
	"fmt"

	analytics "github.com/GrayCodeAI/graycode-cli/internal/observability"
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

var (
	costAnalyzeJSON bool
	costSummaryJSON bool
)

func init() {
	costAnalyzeCmd.Flags().BoolVar(&costAnalyzeJSON, "json", false, "output the report as JSON")
	costSummaryCmd.Flags().BoolVar(&costSummaryJSON, "json", false, "output the summary as JSON")
	costCmd.AddCommand(costAnalyzeCmd)
	costCmd.AddCommand(costSummaryCmd)
}

var costAnalyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Run a full cost optimization analysis",
	RunE: func(cmd *cobra.Command, args []string) error {
		entries := []analytics.CostEntry{}
		report := analytics.Analyze(entries)

		if costAnalyzeJSON {
			out, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling report: %w", err)
			}
			cmd.Println(string(out))
			return nil
		}

		cmd.Println(auditTint("[Experimental] Cost tracking is not yet fully available.", warnAmber))
		cmd.Println()

		if report.TotalSpend == 0 {
			cmd.Println(auditTint("No cost data collected in this session.", textMuted))
			cmd.Println()
			cmd.Println(auditTint("Once session data integration is complete, the analyzer will support:", textMuted))
			cmd.Println(auditTint("  - Spend breakdown by model and task type", textMuted))
			cmd.Println(auditTint("  - Wasted spend detection (expensive models for simple tasks)", textMuted))
			cmd.Println(auditTint("  - Abandoned output tracking", textMuted))
			cmd.Println(auditTint("  - Model routing recommendations", textMuted))
			cmd.Println(auditTint("  - Prompt caching suggestions", textMuted))
			cmd.Println()
			cmd.Println(auditTint("To track progress: https://github.com/GrayCodeAI/graycode-cli/issues", textMuted))
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
		entries := []analytics.CostEntry{}
		report := analytics.Analyze(entries)

		if costSummaryJSON {
			out, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling report: %w", err)
			}
			cmd.Println(string(out))
			return nil
		}

		cmd.Println("[Experimental] Cost tracking is not yet fully available.")
		cmd.Println()

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
