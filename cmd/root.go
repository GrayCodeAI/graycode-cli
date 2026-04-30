package cmd

import (
	"github.com/spf13/cobra"
)

var (
	version    string
	model      string
	provider   string
	promptFlag string
	resumeID   string
	mcpServers []string
)

// SetVersion sets the version string from main.
func SetVersion(v string) { version = v }

var rootCmd = &cobra.Command{
	Use:   "hawk",
	Short: "AI coding agent powered by eyrie",
	Long:  "hawk is an AI coding agent that reads, writes, and runs code in your terminal.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runChat()
	},
}

func init() {
	rootCmd.Flags().StringVarP(&model, "model", "m", "", "model to use (e.g. claude-sonnet-4-20250514)")
	rootCmd.Flags().StringVarP(&promptFlag, "prompt", "p", "", "send a single prompt and exit")
	rootCmd.Flags().StringVar(&provider, "provider", "", "LLM provider (anthropic, openai, gemini, etc.)")
	rootCmd.Flags().StringVarP(&resumeID, "resume", "r", "", "resume a saved session by ID")
	rootCmd.Flags().StringArrayVar(&mcpServers, "mcp", nil, "MCP server command (e.g. --mcp 'npx @modelcontextprotocol/server-filesystem .')")
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print hawk version",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Println("hawk", version)
	},
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
