package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/GrayCodeAI/hawk/sessioncapture"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var sessionCaptureCmd = &cobra.Command{
	Use:     "trace",
	Aliases: []string{"capture"},
	Short:   "Git-native session capture — rewind, checkpoint, and audit AI sessions",
	Long: `trace integrates with the Trace CLI to record coding sessions into Git.

Sessions are captured alongside commits on a separate branch, giving you
rewind, resume, and full audit capabilities.

Examples:
  hawk trace status
  hawk trace enable
  hawk trace checkpoints
  hawk trace rewind <checkpoint-id>`,
}

var captureEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable session capture in the current repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		bridge := sessioncapture.NewBridge()
		if !bridge.Ready() {
			return fmt.Errorf("trace CLI not found — install from https://github.com/GrayCodeAI/trace")
		}
		dir, _ := os.Getwd()
		if err := bridge.Enable(context.Background(), dir); err != nil {
			return err
		}
		fmt.Println(captureSuccessStyle.Render("Session capture enabled."))
		return nil
	},
}

var captureDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable session capture in the current repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		bridge := sessioncapture.NewBridge()
		if !bridge.Ready() {
			return fmt.Errorf("trace CLI not found")
		}
		dir, _ := os.Getwd()
		if err := bridge.Disable(context.Background(), dir); err != nil {
			return err
		}
		fmt.Println("Session capture disabled.")
		return nil
	},
}

var captureStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current session capture status",
	RunE: func(cmd *cobra.Command, args []string) error {
		bridge := sessioncapture.NewBridge()
		if !bridge.Ready() {
			fmt.Println(captureDimStyle.Render("trace CLI not installed — session capture unavailable"))
			return nil
		}
		dir, _ := os.Getwd()
		status, err := bridge.GetStatus(context.Background(), dir)
		if err != nil {
			return err
		}

		titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
		fmt.Println(titleStyle.Render("Session Capture"))
		fmt.Println()

		if !status.Enabled {
			fmt.Println("  Status: " + captureDimStyle.Render("disabled"))
			fmt.Println("  Run " + lipgloss.NewStyle().Bold(true).Render("hawk capture enable") + " to start")
			return nil
		}

		fmt.Println("  Status:  " + captureSuccessStyle.Render("enabled"))
		if status.SessionID != "" {
			fmt.Println("  Session: " + status.SessionID)
		}
		if status.Phase != "" {
			fmt.Println("  Phase:   " + status.Phase)
		}
		if status.Agent != "" {
			fmt.Println("  Agent:   " + status.Agent)
		}
		return nil
	},
}

var captureCheckpointsCmd = &cobra.Command{
	Use:   "checkpoints",
	Short: "List available checkpoints",
	RunE: func(cmd *cobra.Command, args []string) error {
		bridge := sessioncapture.NewBridge()
		if !bridge.Ready() {
			return fmt.Errorf("trace CLI not found")
		}
		dir, _ := os.Getwd()
		cps, err := bridge.ListCheckpoints(context.Background(), dir)
		if err != nil {
			return err
		}
		if len(cps) == 0 {
			fmt.Println("No checkpoints found.")
			return nil
		}

		titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
		fmt.Println(titleStyle.Render("Checkpoints"))
		fmt.Println()
		for _, cp := range cps {
			line := fmt.Sprintf("  %s  %s", cp.ID, cp.CreatedAt)
			if cp.Prompt != "" {
				prompt := cp.Prompt
				if len(prompt) > 60 {
					prompt = prompt[:57] + "..."
				}
				line += "  " + captureDimStyle.Render(prompt)
			}
			fmt.Println(line)
		}
		return nil
	},
}

var captureRewindCmd = &cobra.Command{
	Use:   "rewind <checkpoint-id>",
	Short: "Rewind to a previous checkpoint",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		bridge := sessioncapture.NewBridge()
		if !bridge.Ready() {
			return fmt.Errorf("trace CLI not found")
		}
		dir, _ := os.Getwd()
		if err := bridge.Rewind(context.Background(), dir, args[0]); err != nil {
			return err
		}
		fmt.Println(captureSuccessStyle.Render(fmt.Sprintf("Rewound to checkpoint %s", args[0])))
		return nil
	},
}

var (
	captureSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	captureDimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

func init() {
	sessionCaptureCmd.AddCommand(captureEnableCmd)
	sessionCaptureCmd.AddCommand(captureDisableCmd)
	sessionCaptureCmd.AddCommand(captureStatusCmd)
	sessionCaptureCmd.AddCommand(captureCheckpointsCmd)
	sessionCaptureCmd.AddCommand(captureRewindCmd)
}

// formatCaptureHelp adds install guidance when trace is missing.
func formatCaptureHelp() string {
	var b strings.Builder
	b.WriteString("Session capture requires the Trace CLI.\n\n")
	b.WriteString("Install:\n")
	b.WriteString("  curl -fsSL https://trace.graycode.ai/install.sh | bash\n")
	b.WriteString("  # or: go install github.com/GrayCodeAI/trace/cmd/trace@latest\n")
	return b.String()
}
