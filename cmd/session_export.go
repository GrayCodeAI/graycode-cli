package cmd

import (
	"fmt"
	"os"

	"github.com/GrayCodeAI/graycode-cli/internal/session"
	"github.com/spf13/cobra"
)

var (
	exportFormat string
	exportRedact bool
	exportOutput string
)

var sessionExportCmd = &cobra.Command{
	Use:   "export [session-id]",
	Short: "Export a session as JSON or Markdown",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var s *session.Session
		var err error

		if len(args) > 0 {
			s, err = session.Load(args[0])
		} else {
			s, err = session.LoadLatest()
		}
		if err != nil {
			return fmt.Errorf("load session: %w", err)
		}

		data, err := session.Export(s, exportFormat, exportRedact)
		if err != nil {
			return fmt.Errorf("export session: %w", err)
		}

		if exportOutput == "" {
			cmd.Print(string(data))
			return nil
		}

		if err := os.WriteFile(exportOutput, data, 0o600); err != nil {
			return fmt.Errorf("write output file: %w", err)
		}
		cmd.Printf("Session exported to %s\n", exportOutput)
		return nil
	},
}

func init() {
	sessionExportCmd.Flags().StringVar(&exportFormat, "format", "json", "Output format (json|md)")
	sessionExportCmd.Flags().BoolVar(&exportRedact, "redact", true, "Redact secrets from output")
	sessionExportCmd.Flags().StringVar(&exportOutput, "output", "", "Output file path (default: stdout)")
	sessionsCmd.AddCommand(sessionExportCmd)
}
