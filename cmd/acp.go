package cmd

import (
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/GrayCodeAI/hawk/internal/acp"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/observability/logger"
	"github.com/spf13/cobra"
)

var acpCmd = &cobra.Command{
	Use:   "acp",
	Short: "Run hawk as an Agent Client Protocol (ACP) server",
	Long: "Run hawk as an ACP server over stdio (JSON-RPC 2.0) so editors such as " +
		"Zed can drive it. Tool-permission prompts are routed back to the client " +
		"via session/request_permission.",
	RunE: runACP,
}

func init() {
	rootCmd.AddCommand(acpCmd)
}

func runACP(cmd *cobra.Command, _ []string) error {
	settings := hawkconfig.LoadSettings()
	newSession := newConfiguredHawkSessionFactory(settings, logger.New(io.Discard, logger.Error))

	factory := func() (*engine.Session, error) {
		systemPrompt, err := buildSystemPrompt()
		if err != nil {
			return nil, err
		}
		// stdout is the JSON-RPC channel; keep logs off it.
		return newSession(systemPrompt, "")
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := acp.NewServer(factory)
	return srv.ServeStdio(ctx)
}
