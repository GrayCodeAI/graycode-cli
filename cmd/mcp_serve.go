package cmd

import (
	"encoding/json"
	"os"
	"os/signal"
	"syscall"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
	"github.com/GrayCodeAI/graycode-cli/internal/mcp"
	"github.com/spf13/cobra"
)

var mcpConfigWrite bool

func init() {
	mcpConfigCmd.Flags().BoolVar(&mcpConfigWrite, "write", false,
		"also print the well-known client config paths to paste the block into")
	mcpCmd.AddCommand(mcpServeCmd)
	mcpCmd.AddCommand(mcpConfigCmd)
}

// mcpServeCmd runs graycode itself as an MCP server over stdio, exposing graycode's
// capabilities (chat, search, memory, review, scan, compress) to MCP clients
// such as Claude Desktop, Cursor, and Windsurf.
var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run graycode as an MCP server over stdio",
	Long: "Run graycode as a Model Context Protocol server over stdio (JSON-RPC 2.0), " +
		"exposing graycode's tools to MCP clients like Claude Desktop, Cursor, and Windsurf.\n\n" +
		"Use `graycode mcp config` to print the JSON block that registers this command in a client.",
	RunE: runMCPServe,
}

func runMCPServe(cmd *cobra.Command, _ []string) error {
	settings := graycodeconfig.LoadSettings()

	serverVersion := version
	if serverVersion == "" {
		serverVersion = "dev"
	}
	server := mcp.NewMCPServer(mcp.ServerInfo{Name: "graycode", Version: serverVersion})

	// Wire graycode's tool registry in as the executor so delegating tools run for
	// real; a registry build failure degrades to not-configured rather than
	// aborting (the server still answers initialize/tools/list).
	registry, err := defaultRegistry(settings)
	if err == nil {
		mcp.RegisterDefaultTools(server, registry.Execute)
	} else {
		mcp.RegisterDefaultTools(server, nil)
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.ServeStdio(ctx)
}

// mcpConfigCmd emits the JSON block that registers graycode as an MCP server in a
// client's config file, so users don't hand-edit JSON.
var mcpConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Print the MCP-server config block to register graycode in a client",
	Long: "Print the JSON block that registers graycode as an MCP server (pointing at " +
		"`graycode mcp serve`) for clients like Claude Desktop, Cursor, and Windsurf.\n\n" +
		"Pipe it to the client's config file, e.g.:\n" +
		"  graycode mcp config >> ~/Library/Application Support/Claude/claude_desktop_config.json",
	RunE: runMCPConfig,
}

func runMCPConfig(cmd *cobra.Command, _ []string) error {
	exe := graycodeExecutablePath()

	block := map[string]any{
		"mcpServers": map[string]any{
			"graycode": map[string]any{
				"command": exe,
				"args":    []string{"mcp", "serve"},
			},
		},
	}
	out, err := json.MarshalIndent(block, "", "  ")
	if err != nil {
		return err
	}

	if mcpConfigWrite {
		cmd.Println("# Add the \"graycode\" entry below into the \"mcpServers\" object of your client config:")
		cmd.Println("#   Claude Desktop (macOS): ~/Library/Application Support/Claude/claude_desktop_config.json")
		cmd.Println("#   Cursor:                 ~/.cursor/mcp.json")
		cmd.Println("#   Windsurf:               ~/.codeium/windsurf/mcp_config.json")
		cmd.Println()
	}
	cmd.Println(string(out))
	return nil
}

// graycodeExecutablePath returns the absolute path to the running graycode binary, or
// the bare name "graycode" if it cannot be resolved (e.g. during `go run`), so the
// emitted config is still copy-pasteable.
func graycodeExecutablePath() string {
	if exe, err := os.Executable(); err == nil && exe != "" {
		return exe
	}
	return "graycode"
}
