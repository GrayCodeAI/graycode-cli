package cmd

import (
	"context"
	"strings"
	"sync"
	"time"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/lsp"
	"github.com/GrayCodeAI/hawk/internal/tool"
)

// This file holds the tool-registry construction used by the chat TUI:
// the essential/optional tool sets and the registry builder that wires in
// MCP servers and CLI tool filters. Split out of chat.go for clarity.

const startupMCPToolLoadTimeout = 1500 * time.Millisecond

var (
	defaultRegistryLoadMCPTools       = tool.LoadMCPTools
	defaultRegistryLoadRemoteMCPTools = tool.LoadRemoteMCPTools
)

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
		tool.SessionQueryTool{},
		tool.ScheduleCreateTool{},
		tool.ScheduleListTool{},
		tool.ScheduleDeleteTool{},
		tool.TerminalCreateTool{},
		tool.TerminalSendTool{},
		tool.TerminalReadTool{},
		tool.TerminalListTool{},
		tool.TerminalResizeTool{},
		tool.TerminalKillTool{},
		tool.AgentTool{},
		tool.AskUserQuestionTool{},
		tool.TodoWriteTool{},
		tool.TaskOutputTool{},
		tool.TaskStopTool{},
		tool.WaitTasksTool{},
		tool.KillTaskTool{},
		tool.MonitorTool{},
		tool.MultiEditTool{},
		tool.BrowserTool{},
		tool.ScreenshotTool{},
		tool.ToolHealthTool{},
		tool.RequestCredentialTool{Gateway: func() tool.CredentialGateFn {
			// The actual gateway is wired at session start via SetCredentialGate.
			// This returns nil until then; the tool checks for nil and errors.
			if fn := credentialGate.Load(); fn != nil {
				if gateFn, ok := fn.(tool.CredentialGateFn); ok {
					return gateFn
				}
			}
			return nil
		}},
	}
}

func optionalTools() []tool.Tool {
	// Specialized tools that can be lazy-loaded on demand
	return []tool.Tool{
		// Structured developer workflows. These stay lazy until the intent
		// router or ToolSearch promotes them, keeping the startup schema small.
		tool.GitTool{},
		tool.OutlineTool{},
		&tool.SmartReaderTool{},
		tool.PatchTool{},
		tool.BatchTool{},
		tool.TransactionTool{},
		tool.NewAutoImportTool(),
		tool.ImportOrganizerTool{},
		tool.NewRefactorTool(),
		tool.ConflictResolverTool{},
		tool.DebuggerTool{},
		tool.DevEnvTool{},
		tool.ProjectVerifyTool{},
		tool.AppVerifyTool{},
		tool.GenerateMediaTool{},
		tool.DependencyAuditTool{},
		tool.GitHubTool{},
		&tool.PRGeneratorTool{},
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
		// Advanced specification workflows are lazy-loaded here rather than
		// left as compile-only tools. ToolSearch can now discover the complete
		// spec surface without making these startup-critical.
		tool.SpecAdaptiveTool{},
		tool.SpecAdrTool{},
		tool.AnalyzeTool{},
		tool.SpecBddTool{},
		tool.SpecBlastTool{},
		tool.ChecklistTool{},
		tool.ConstitutionTool{},
		tool.ConvergeTool{},
		tool.SpecDriftTool{},
		tool.SpecGroundTool{},
		tool.SpecLinksTool{},
		tool.SpecMasterTool{},
		tool.SpecParallelTool{},
		tool.SpecPlanVariationsTool{},
		tool.SpecProgressTool{},
		tool.SpecPropertiesTool{},
		tool.SpecProvenanceTool{},
		tool.SpecReviewTool{},
		tool.SpecScaleTool{},
		tool.SpecSuperTool{},
		tool.SpecTestFirstTool{},
		tool.SpecTestGenTool{},
		tool.SpecTraceTool{},
		tool.SpecVersionTool{},
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
		tool.TaskRunTool{},
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
		tool.JobsTool{},
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
	return loadStartupMCPToolSetsWith(defaultRegistryLoadMCPTools, defaultRegistryLoadRemoteMCPTools, servers)
}

// loadStartupMCPToolSetsWith loads MCP tool sets for the given servers using
// explicit loader functions (injected so callers can capture them by value and
// avoid racing tests that swap the package-level loader vars).
func loadStartupMCPToolSetsWith(loadMCP func(context.Context, string, string, ...string) ([]tool.Tool, error), loadRemoteMCP func(context.Context, string, string, string, map[string]string) ([]tool.Tool, error), servers []startupMCPServerSpec) [][]tool.Tool {
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
				mcpTools, err = loadRemoteMCP(ctx, spec.name, spec.serverType, spec.url, spec.headers)
			} else {
				mcpTools, err = loadMCP(ctx, spec.name, spec.command, spec.args...)
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

	filtered, err := filterAvailableTools(
		tools,
		toolsFlagSet,
		parseToolListFromCLI(toolsFlag),
		parseToolListFromCLI(disallowedToolsFlag),
	)
	if err != nil {
		return nil, err
	}
	filtered = append(filtered, tool.LSPTool{Manager: lsp.NewManagerFromProject(".")})
	registry := tool.NewRegistry(filtered...)
	// Lazy model surface: only essential tools are sent to the LLM.
	// Optional tools register for Get/ToolSearch and promote via select:.
	essentialNames := make([]string, 0, len(filtered))
	for _, t := range filtered {
		essentialNames = append(essentialNames, t.Name())
	}
	registry.EnableLazyModelSurface(essentialNames)

	// Register optional tools synchronously. Registration only adds in-memory
	// schemas; keeping it deterministic ensures intent promotion cannot race
	// the first user turn. They remain hidden from the model until promoted.
	for _, t := range optionalTools() {
		_ = registry.Register(t)
	}

	// Load MCP tools in the background so a hung/absent stdio server delays
	// tool availability — not first paint. loadStartupMCPToolSets can block up
	// to 1.5s per configured server. The CLI tool filters still apply.
	// The loader functions are captured by value so tests that override the
	// package vars cannot race with the async goroutine.
	loadMCP := defaultRegistryLoadMCPTools
	loadRemoteMCP := defaultRegistryLoadRemoteMCPTools
	go func() {
		mcpTools := loadStartupMCPToolSetsWith(loadMCP, loadRemoteMCP, configuredStartupMCPServers(settings))
		var all []tool.Tool
		for _, set := range mcpTools {
			all = append(all, set...)
		}
		filteredMCP, err := filterAvailableTools(
			all,
			toolsFlagSet,
			parseToolListFromCLI(toolsFlag),
			parseToolListFromCLI(disallowedToolsFlag),
		)
		if err != nil {
			return
		}
		for _, t := range filteredMCP {
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
