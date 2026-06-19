package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

// This file holds the tool-registry construction used by the chat TUI:
// the essential/optional tool sets and the registry builder that wires in
// MCP servers and CLI tool filters. Split out of chat.go for clarity.

func essentialTools() []tool.Tool {
	// Core tools needed for basic agent operation - always loaded at startup
	return []tool.Tool{
		tool.BashTool{},
		tool.FileReadTool{},
		tool.FileWriteTool{},
		tool.FileEditTool{},
		tool.StructuredEditTool{},
		tool.LSTool{},
		tool.GlobTool{},
		tool.GrepTool{},
		tool.WebFetchTool{},
		tool.WebSearchTool{},
		tool.ToolSearchTool{},
		tool.SkillTool{},
		tool.AgentTool{},
		tool.AskUserQuestionTool{},
		tool.TodoWriteTool{},
		tool.TaskOutputTool{},
		tool.TaskStopTool{},
		tool.LSPTool{},
		tool.MultiEditTool{},
	}
}

func optionalTools() []tool.Tool {
	// Specialized tools that can be lazy-loaded on demand
	return []tool.Tool{
		tool.EnterPlanModeTool{},
		tool.ExitPlanModeTool{},
		tool.NotebookEditTool{},
		tool.EnterWorktreeTool{},
		tool.ExitWorktreeTool{},
		tool.ListMcpResourcesTool{},
		tool.ReadMcpResourceTool{},
		tool.ConfigTool{},
		tool.BriefTool{},
		tool.TaskCreateTool{},
		tool.TaskGetTool{},
		tool.TaskListTool{},
		tool.TaskUpdateTool{},
		tool.SleepTool{},
		tool.CronCreateTool{},
		tool.CronDeleteTool{},
		tool.CronListTool{},
		tool.VerifyPlanExecutionTool{},
		tool.WorkflowTool{},
		tool.McpAuthTool{},
		tool.DiagnosticsTool{},
		tool.CodeSearchTool{},
		tool.CoreMemoryAppendTool{},
		tool.CoreMemoryReplaceTool{},
		tool.CoreMemoryRethinkTool{},
		tool.DownloadTool{},
		tool.AgenticFetchTool{},
		tool.ImpactTool{},
		tool.GitHistoryTool{},
		tool.CodeGraphTool{},
		tool.NilAwayTool{},
		tool.ReviveTool{},
		tool.MCPLanguageServerTool{},
		tool.SQLTool{},
	}
}

func defaultRegistry(settings hawkconfig.Settings) (*tool.Registry, error) {
	// Load essential tools first for fast startup
	tools := essentialTools()
	if tool.IsPowerShellAvailable() {
		tools = append(tools, tool.PowerShellTool{})
	}
	// Detect project-level MCP servers (supply chain attack vector).
	// Project .hawk/settings.json can be committed to a repo and define
	// arbitrary commands that execute on clone. Gate behind --allow-project-mcp.
	projectMCPServers := hawkconfig.ProjectMCPServers()
	projectMCPNames := make(map[string]bool, len(projectMCPServers))
	for _, cfg := range projectMCPServers {
		if cfg.Name != "" {
			projectMCPNames[cfg.Name] = true
		}
	}
	for _, cfg := range settings.MCPServers {
		if cfg.Name == "" || cfg.Command == "" {
			continue
		}
		if projectMCPNames[cfg.Name] && !allowProjectMCP {
			fmt.Fprintf(os.Stderr, "hawk: skipping project-level MCP server %q (defined in .hawk/settings.json); use --allow-project-mcp to enable\n", cfg.Name)
			continue
		}
		mcpTools, err := tool.LoadMCPTools(context.Background(), cfg.Name, cfg.Command, cfg.Args...)
		if err != nil {
			continue
		}
		tools = append(tools, mcpTools...)
	}
	// Load MCP server tools
	for _, cmd := range mcpServers {
		parts := strings.Fields(cmd)
		if len(parts) == 0 {
			continue
		}
		name := parts[0]
		mcpTools, err := tool.LoadMCPTools(context.Background(), name, parts[0], parts[1:]...)
		if err != nil {
			// MCP server failed to connect — skip silently, will show in /doctor
			continue
		}
		tools = append(tools, mcpTools...)
	}

	filtered, err := filterAvailableTools(
		tools,
		toolsFlagSet,
		parseToolListFromCLI(toolsFlag),
		parseToolListFromCLI(disallowedToolsFlag),
	)
	if err != nil {
		return nil, err
	}
	registry := tool.NewRegistry(filtered...)

	// Lazy-load optional tools in background
	go func() {
		for _, t := range optionalTools() {
			_ = registry.Register(t)
		}
	}()

	return registry, nil
}

func allTools() []tool.Tool {
	t := essentialTools()
	t = append(t, optionalTools()...)
	return t
}
