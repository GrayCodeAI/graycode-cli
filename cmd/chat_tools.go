package cmd

import (
	"context"
	"strings"
	"sync"
	"time"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

// This file holds the tool-registry construction used by the chat TUI:
// the essential/optional tool sets and the registry builder that wires in
// MCP servers and CLI tool filters. Split out of chat.go for clarity.

const startupMCPToolLoadTimeout = 1500 * time.Millisecond

var defaultRegistryLoadMCPTools = tool.LoadMCPTools

type startupMCPServerSpec struct {
	name    string
	command string
	args    []string
}

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

func configuredStartupMCPServers(settings hawkconfig.Settings) []startupMCPServerSpec {
	servers := make([]startupMCPServerSpec, 0, len(settings.MCPServers)+len(mcpServers))
	for _, cfg := range settings.MCPServers {
		if cfg.Name == "" || cfg.Command == "" {
			continue
		}
		servers = append(servers, startupMCPServerSpec{
			name:    cfg.Name,
			command: cfg.Command,
			args:    cfg.Args,
		})
	}
	for _, cmd := range mcpServers {
		parts := strings.Fields(cmd)
		if len(parts) == 0 {
			continue
		}
		servers = append(servers, startupMCPServerSpec{
			name:    parts[0],
			command: parts[0],
			args:    parts[1:],
		})
	}
	return servers
}

func loadStartupMCPToolSets(servers []startupMCPServerSpec) [][]tool.Tool {
	results := make([][]tool.Tool, len(servers))
	var wg sync.WaitGroup
	wg.Add(len(servers))
	for i, spec := range servers {
		go func(i int, spec startupMCPServerSpec) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), startupMCPToolLoadTimeout)
			defer cancel()

			mcpTools, err := defaultRegistryLoadMCPTools(ctx, spec.name, spec.command, spec.args...)
			if err != nil {
				return
			}
			results[i] = mcpTools
		}(i, spec)
	}
	wg.Wait()
	return results
}

func defaultRegistry(settings hawkconfig.Settings) (*tool.Registry, error) {
	// Load essential tools first for fast startup
	tools := essentialTools()
	if tool.IsPowerShellAvailable() {
		tools = append(tools, tool.PowerShellTool{})
	}
	for _, mcpTools := range loadStartupMCPToolSets(configuredStartupMCPServers(settings)) {
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
