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
var defaultRegistryLoadRemoteMCPTools = tool.LoadRemoteMCPTools

type startupMCPServerSpec struct {
	name    string
	command string
	args    []string

	// Set only for non-stdio (remote) servers: serverType is "http", "sse",
	// or "websocket", url is the server's endpoint, and headers carries any
	// static headers from config plus an auto-injected OAuth bearer token
	// if one is stored for this server (see internal/mcp/oauth.go).
	serverType string
	url        string
	headers    map[string]string
}

func (s startupMCPServerSpec) isRemote() bool { return s.serverType != "" }

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
		tool.SpecifyTool{},
		tool.PlanTool{},
		tool.TasksTool{},
		tool.ApproveImplementationTool{},
		tool.SpecStatusTool{},
		tool.SpecEditTool{},
		tool.SpecListTool{},
		tool.SpecResetTool{},
		tool.SpecConfigTool{},
		tool.ClarifyTool{},
		tool.AnalyzeTool{},
		tool.ChecklistTool{},
		tool.ConstitutionTool{},
		tool.ConvergeTool{},
		tool.TasksToIssuesTool{},
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
		if cfg.Name == "" {
			continue
		}
		switch cfg.Type {
		case "", "stdio":
			if cfg.Command == "" {
				continue
			}
			servers = append(servers, startupMCPServerSpec{
				name:    cfg.Name,
				command: cfg.Command,
				args:    cfg.Args,
			})
		case "http", "sse", "websocket":
			if cfg.URL == "" {
				continue
			}
			servers = append(servers, startupMCPServerSpec{
				name:       cfg.Name,
				serverType: cfg.Type,
				url:        cfg.URL,
				headers:    mergedMCPHeaders(cfg),
			})
		default:
			continue
		}
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

// mergedMCPHeaders combines a remote MCP server's static configured headers
// with an auto-injected "Authorization: Bearer <token>" if a valid,
// non-expired OAuth token is stored for this server (see
// internal/tool/mcp_auth.go). A configured static Authorization header, if
// any, takes precedence over the auto-injected one.
func mergedMCPHeaders(cfg hawkconfig.MCPServerConfig) map[string]string {
	headers := make(map[string]string, len(cfg.Headers)+1)
	for k, v := range cfg.Headers {
		headers[k] = v
	}
	if _, hasAuth := headers["Authorization"]; !hasAuth {
		if bearer, ok := tool.AuthHeaderForMCPServer(cfg.Name); ok {
			headers["Authorization"] = bearer
		}
	}
	return headers
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

			var (
				mcpTools []tool.Tool
				err      error
			)
			if spec.isRemote() {
				mcpTools, err = defaultRegistryLoadRemoteMCPTools(ctx, spec.name, spec.serverType, spec.url, spec.headers)
			} else {
				mcpTools, err = defaultRegistryLoadMCPTools(ctx, spec.name, spec.command, spec.args...)
			}
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
