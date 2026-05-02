package cmd

import (
	"fmt"

	"github.com/GrayCodeAI/hawk/rules"
	"github.com/spf13/cobra"
)

var (
	rulesImportFrom string
	rulesExportTo   string
)

var rulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "Detect, import, and export AI coding rules between tool formats",
	Long: `rules manages AI coding rule files across different tools.
Supported formats: hawk, cursor, claudecode, copilot, gemini.

Subcommands:
  detect                   Show which AI tool rule files exist in the current directory
  import --from <format>   Import rules from another tool's format
  export --to <format>     Export rules to another tool's format`,
}

var rulesDetectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Show which AI tool rule files exist in the current directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		found := rules.Detect(".")

		if len(found) == 0 {
			cmd.Println("No AI tool rule files detected.")
			return nil
		}

		cmd.Println("Detected AI tool rule files:")
		for format, path := range found {
			cmd.Println(fmt.Sprintf("  %-12s %s", format, path))
		}
		return nil
	},
}

var rulesImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import rules from another tool's format into hawk",
	RunE: func(cmd *cobra.Command, args []string) error {
		if rulesImportFrom == "" {
			return fmt.Errorf("--from flag is required (e.g. --from cursor)")
		}

		from := rules.Format(rulesImportFrom)
		imported, err := rules.Import(".", from)
		if err != nil {
			return fmt.Errorf("import from %s failed: %w", rulesImportFrom, err)
		}

		if len(imported) == 0 {
			cmd.Println(fmt.Sprintf("No rules found in %s format.", rulesImportFrom))
			return nil
		}

		// Export to hawk format.
		if err := rules.Export(".", rules.FormatHawk, imported); err != nil {
			return fmt.Errorf("export to hawk format failed: %w", err)
		}

		cmd.Println(fmt.Sprintf("Imported %d rule(s) from %s to .hawk/rules/.", len(imported), rulesImportFrom))
		for _, r := range imported {
			cmd.Println(fmt.Sprintf("  - %s", r.Name))
		}
		return nil
	},
}

var rulesExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export hawk rules to another tool's format",
	RunE: func(cmd *cobra.Command, args []string) error {
		if rulesExportTo == "" {
			return fmt.Errorf("--to flag is required (e.g. --to claudecode)")
		}

		// Read hawk rules.
		hawkRules, err := rules.Import(".", rules.FormatHawk)
		if err != nil {
			return fmt.Errorf("read hawk rules failed: %w", err)
		}

		if len(hawkRules) == 0 {
			cmd.Println("No hawk rules found in .hawk/rules/. Nothing to export.")
			return nil
		}

		to := rules.Format(rulesExportTo)
		if err := rules.Export(".", to, hawkRules); err != nil {
			return fmt.Errorf("export to %s format failed: %w", rulesExportTo, err)
		}

		cmd.Println(fmt.Sprintf("Exported %d rule(s) to %s format.", len(hawkRules), rulesExportTo))
		for _, r := range hawkRules {
			cmd.Println(fmt.Sprintf("  - %s", r.Name))
		}
		return nil
	},
}

func init() {
	rulesImportCmd.Flags().StringVar(&rulesImportFrom, "from", "", "source format to import from (cursor, claudecode, copilot, gemini)")
	rulesExportCmd.Flags().StringVar(&rulesExportTo, "to", "", "target format to export to (cursor, claudecode, copilot, gemini)")

	rulesCmd.AddCommand(rulesDetectCmd)
	rulesCmd.AddCommand(rulesImportCmd)
	rulesCmd.AddCommand(rulesExportCmd)
}
