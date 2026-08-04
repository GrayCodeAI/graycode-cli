package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/engine/diff"
	"github.com/GrayCodeAI/hawk/internal/hooks"
	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
	"github.com/GrayCodeAI/hawk/internal/observability/metrics"
	"github.com/GrayCodeAI/hawk/internal/observability/oteltrace"
	"github.com/GrayCodeAI/hawk/internal/prompts"
	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// ToolService is the Session's view of the tool execution surface:
// the tool registry, the post-call pipeline, blast-radius estimation,
// and the per-tool timeout. Extracted from Session in Phase 6 of the
// god-object decomposition (see docs/session-decomposition.md).
type ToolService struct {
	registry          *tool.Registry
	containerExecutor tool.ContainerExecutor
	containerRequired bool
	tracer            *oteltrace.Tracer
	agentSpawn        tool.AgentSpawnFn
	snapshots         SnapshotTracker
	bgMu              sync.Mutex
	workingDirMu      sync.RWMutex
	workingDir        string
	bgManager         *tool.BackgroundAgentManager
	sandbox           *diff.DiffSandbox
	deps              toolExecutionDeps
	metrics           *metrics.Registry
}

func (s *ToolService) SetAgentSpawnFn(fn tool.AgentSpawnFn) {
	if s != nil {
		s.agentSpawn = fn
	}
}

func (s *ToolService) AgentSpawnFn() tool.AgentSpawnFn {
	if s == nil {
		return nil
	}
	return s.agentSpawn
}

// toolExecutionDeps contains the service-owned collaborators needed for one
// raw tool invocation. Keeping these dependencies on ToolService removes the
// permission, approval, tracing, timeout, and retry boundary from Session;
// post-call product hooks remain in Session until the next migration slice.
type toolExecutionDeps struct {
	permissions        *PermissionService
	chat               *ChatService
	memory             *MemoryService
	agentSpawn         tool.AgentSpawnFn
	askUser            func(string) (string, error)
	readOnlyBash       bool
	workingDir         string
	checkApproval      func(context.Context, string, map[string]interface{}) (bool, string)
	recordPolicy       func(types.ToolCall, string, bool, string)
	recordVerification func(types.ToolCall, string, bool)
	lifecycle          *LifecycleService
	appendSystem       func(string)
}

// NewToolService constructs a ToolService with the given registry.
func NewToolService(registry *tool.Registry) *ToolService {
	return &ToolService{registry: registry}
}

// WithExecutionDeps binds the extracted service graph used by ExecuteOne.
func (s *ToolService) WithExecutionDeps(deps toolExecutionDeps) *ToolService {
	s.workingDirMu.Lock()
	defer s.workingDirMu.Unlock()
	s.deps = deps
	s.workingDir = deps.workingDir
	return s
}

// SetWorkingDir configures the preferred working directory for tool execution
// and graph observations.
func (s *ToolService) SetWorkingDir(dir string) {
	if s == nil {
		return
	}
	s.workingDirMu.Lock()
	defer s.workingDirMu.Unlock()
	s.workingDir = dir
	s.deps.workingDir = dir
}

// WorkingDir returns the preferred working directory for tool execution.
func (s *ToolService) WorkingDir() string {
	if s == nil {
		return ""
	}
	s.workingDirMu.RLock()
	defer s.workingDirMu.RUnlock()
	return s.workingDir
}

// WithMetrics attaches the registry used for tool execution counters.
func (s *ToolService) WithMetrics(registry *metrics.Registry) *ToolService {
	s.metrics = registry
	return s
}

// WithContainerExecutor configures container isolation.
func (s *ToolService) WithContainerExecutor(ce tool.ContainerExecutor, required bool) *ToolService {
	s.containerExecutor = ce
	s.containerRequired = required
	return s
}

// WithTracer configures the OTel tracer.
func (s *ToolService) WithTracer(t *oteltrace.Tracer) *ToolService {
	s.tracer = t
	return s
}

// Tracer returns the tool/runtime tracer shared by session loop spans.
func (s *ToolService) Tracer() *oteltrace.Tracer {
	if s == nil {
		return nil
	}
	return s.tracer
}

// WithSnapshots configures the snapshot tracker.
func (s *ToolService) WithSnapshots(snap SnapshotTracker) *ToolService {
	s.snapshots = snap
	return s
}

// WithBackgroundManager configures the background sub-agent manager.
func (s *ToolService) WithBackgroundManager(bm *tool.BackgroundAgentManager) *ToolService {
	s.bgMu.Lock()
	defer s.bgMu.Unlock()
	s.bgManager = bm
	return s
}

// EnsureBackgroundManager returns the configured background manager, creating
// one exactly once when the session has not supplied one. Tool execution may
// initialize this lazily from concurrent read-only calls, so the operation
// must be atomic at the service boundary.
func (s *ToolService) EnsureBackgroundManager() *tool.BackgroundAgentManager {
	if s == nil {
		return nil
	}
	s.bgMu.Lock()
	defer s.bgMu.Unlock()
	if s.bgManager == nil {
		s.bgManager = tool.NewBackgroundAgentManager()
	}
	return s.bgManager
}

// Registry returns the tool registry.
func (s *ToolService) Registry() *tool.Registry { return s.registry }

// Classify splits tool calls into concurrent (read-only) and
// sequential (write) batches.
func (s *ToolService) Classify(calls []types.ToolCall) (concurrent, sequential []types.ToolCall) {
	for _, tc := range calls {
		if tool.IsReadOnly(tc.Name) {
			concurrent = append(concurrent, tc)
		} else {
			sequential = append(sequential, tc)
		}
	}
	return
}

// ExecuteAll runs the complete tool batch pipeline. The service owns the
// public operation and callers no longer need to reach into Session's
// unexported execution method. An unconfigured service produces deterministic
// errors instead of panicking.
func (s *ToolService) ExecuteAll(ctx context.Context, calls []types.ToolCall, ch chan<- StreamEvent, turn int, intent string) []toolExecResult {
	if s == nil || s.deps.permissions == nil {
		results := make([]toolExecResult, len(calls))
		for i, call := range calls {
			msg := "tool execution service is unavailable"
			results[i] = toolExecResult{tc: call, output: msg, isErr: true}
			if ch != nil {
				ch <- StreamEvent{Type: "tool_result", ToolName: call.Name, Content: msg}
			}
		}
		return results
	}
	plannedCalls := make([]PlannedCall, len(calls))
	concurrentCalls := make([]indexedToolCall, 0, len(calls))
	sequentialCalls := make([]indexedToolCall, 0, len(calls))
	for i, call := range calls {
		targets := s.ExtractTargets(call)
		plannedCalls[i] = PlannedCall{ToolName: call.Name, Args: call.Arguments, Targets: targets}
		item := indexedToolCall{index: i, tc: call}
		if tool.IsReadOnly(call.Name) {
			concurrentCalls = append(concurrentCalls, item)
		} else {
			sequentialCalls = append(sequentialCalls, item)
		}
	}
	if report := EstimateBlastRadius(plannedCalls); report.Radius.NeedsConfirmation() && ch != nil {
		ch <- StreamEvent{Type: "blast_radius", Content: report.Message}
	}

	results := make([]toolExecResult, len(calls))
	readOnlySem := make(chan struct{}, maxConcurrentReadOnlyToolCalls)
	networkSem := make(chan struct{}, maxConcurrentNetworkReadOnlyToolCalls)
	var wg sync.WaitGroup
	for _, item := range concurrentCalls {
		wg.Add(1)
		go func(item indexedToolCall) {
			defer wg.Done()
			readOnlySem <- struct{}{}
			defer func() { <-readOnlySem }()
			if isNetworkReadOnlyTool(item.tc.Name) {
				networkSem <- struct{}{}
				defer func() { <-networkSem }()
			}
			results[item.index] = s.ExecuteOne(ctx, item.tc, nil, ch, turn, intent)
		}(item)
	}
	wg.Wait()
	for _, item := range sequentialCalls {
		results[item.index] = s.ExecuteOne(ctx, item.tc, nil, ch, turn, intent)
	}
	return results
}

// ExecuteOne performs the service-owned tool invocation: event
// emission, container readiness, permission/approval, tracing, tool context,
// lookup, timeout, retry, and raw execution. PostProcess and CompleteResult
// own the remaining result lifecycle.
func (s *ToolService) ExecuteOne(ctx context.Context, tc types.ToolCall, override tool.Tool, ch chan<- StreamEvent, turn int, intent string) toolExecResult {
	result := toolExecResult{tc: tc}
	ch <- StreamEvent{Type: "tool_use", ToolName: tc.Name, ToolID: tc.ID}
	if s.containerRequired && (s.containerExecutor == nil || !s.containerExecutor.Running()) {
		msg := "Container not ready — tools are disabled until the sandbox is running."
		ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: msg}
		result.output, result.isErr, result.err = msg, true, fmt.Errorf("%s", msg)
		return result
	}
	var span *oteltrace.Span
	if s.tracer != nil {
		_, span = oteltrace.StartToolSpan(ctx, s.tracer, tc.Name, tc.ID)
	}
	finishDenied := func(tag string, msg string) toolExecResult {
		ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: msg}
		if span != nil {
			span.SetTag(tag, "true")
			span.Finish()
		}
		result.output, result.isErr, result.err, result.span = msg, true, fmt.Errorf("%s", msg), nil
		return result
	}
	if s.deps.permissions == nil {
		return finishDenied("denied", "permission service is unavailable")
	}
	granted, denyMsg := s.deps.permissions.CheckTool(ctx, ToolCallInfo{Name: tc.Name, ID: tc.ID, Args: tc.Arguments})
	if s.deps.recordPolicy != nil {
		s.deps.recordPolicy(tc, "permission", granted, denyMsg)
	}
	if !granted {
		return finishDenied("denied", denyMsg)
	}
	approved, approvalDeny := true, ""
	if s.deps.checkApproval != nil {
		approved, approvalDeny = s.deps.checkApproval(ctx, tc.Name, tc.Arguments)
	}
	if approval := s.deps.permissions.Approval(); approval != nil && approval.Enabled && s.deps.recordPolicy != nil {
		s.deps.recordPolicy(tc, "approval", approved, approvalDeny)
	}
	if !approved {
		return finishDenied("approval_denied", approvalDeny)
	}
	hooks.ExecuteAsync(ctx, hooks.EventPreTool, map[string]interface{}{"tool": tc.Name, "args": tc.Arguments})
	inputJSON, _ := json.Marshal(tc.Arguments)
	var commitChat func(context.Context, string) (string, error)
	if s.deps.chat != nil {
		commitChat = func(chatCtx context.Context, prompt string) (string, error) {
			resp, err := s.deps.chat.Chat(chatCtx, []types.EyrieMessage{{Role: "user", Content: prompt}}, types.ChatOptions{Provider: s.deps.chat.Provider(), Model: s.deps.chat.Model(), MaxTokens: 256})
			if err != nil {
				return "", err
			}
			if resp == nil {
				return "", fmt.Errorf("commit message model returned no response")
			}
			return resp.Content, nil
		}
	}
	var yaad *memory.YaadBridge
	if s.deps.memory != nil {
		yaad = s.deps.memory.Yaad()
	}
	toolCtx := tool.WithToolContext(ctx, &tool.ToolContext{
		AgentSpawnFn:        s.deps.agentSpawn,
		AskUserFn:           s.deps.askUser,
		CommitMessageChatFn: commitChat,
		YaadBridge:          yaad,
		SpecSlugGet:         func() string { return s.deps.permissions.SpecSlug() },
		SpecSlugSet:         func(slug string) { s.deps.permissions.SetSpecSlug(slug) },
		AllowedDirectories:  s.deps.permissions.AllowedDirs(),
		SandboxMode:         s.deps.permissions.SandboxMode(),
		BackgroundManager:   s.EnsureBackgroundManager(),
		ReadOnlyBash:        s.deps.readOnlyBash,
		WorkingDir:          s.WorkingDir(),
	})
	if s.containerExecutor != nil && s.containerExecutor.Running() {
		toolCtx = tool.WithContainerExecutor(toolCtx, s.containerExecutor)
	}
	toolCtx, cancel := context.WithTimeout(toolCtx, toolTimeout(tc.Name))
	t := override
	if t == nil && s.registry != nil {
		var ok bool
		t, ok = s.registry.Get(tc.Name)
		if !ok {
			cancel()
			return finishDenied("error", fmt.Sprintf("Error: unknown tool: %s", tc.Name))
		}
	}
	if t == nil {
		cancel()
		return finishDenied("error", "Error: tool is unavailable")
	}
	var output string
	var execErr error
	if rpp, ok := t.(tool.RetryPolicyProvider); ok {
		output, execErr = tool.RetryExecutor(toolCtx, t, inputJSON, rpp.RetryPolicy())
	} else {
		output, execErr = tool.RetryExecutor(toolCtx, t, inputJSON, tool.DefaultRetryPolicy())
	}
	cancel()
	result.output, result.err, result.isErr, result.span = output, execErr, execErr != nil, span
	if result.isErr {
		result.output = fmt.Sprintf("Error: %s", execErr.Error())
	}
	return result
}

// NormalizeOutput applies the deterministic context-safety policy to a tool
// result before it is persisted or sent to the model. Keeping this in the
// tool service makes output limits consistent for agent-loop and slash-command
// execution paths.
func (s *ToolService) NormalizeOutput(output, canonicalTool, toolID string, contextWindow int) string {
	maxChars := 50000
	if contextWindow > 0 {
		dynamic := contextWindow * 20 / 100 * 4
		if dynamic < 5000 {
			dynamic = 5000
		}
		if dynamic < maxChars {
			maxChars = dynamic
		}
	}
	compressBudget := maxChars / 2
	if len(output) > compressBudget {
		compressed, tokens := CompressForContext(output, compressBudget/4)
		if tokens > 0 && tokens < CountTokensFast(output) {
			output = compressed
		}
	}
	if len(output) > maxChars {
		output = truncateOutputStructurally(output, maxChars)
	}
	return maybeSpillToolOutput(output, canonicalTool, toolID)
}

// truncateOutputStructurally trims oversized tool output at a structural
// boundary instead of a raw byte cut, so JSON-ish results keep whole lines
// (or a valid splice point) rather than being chopped mid-object (Phase 3).
func truncateOutputStructurally(output string, maxChars int) string {
	trimmed := strings.TrimLeft(output, " \t\r\n")
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		// JSON-ish output: prefer the last newline before the cap.
		if cut := strings.LastIndex(output[:maxChars], "\n"); cut >= 0 {
			return output[:cut] + "\n... (truncated)"
		}
		// Single-line JSON: splice at the previous element separator so the
		// visible prefix remains well-formed up to the marker.
		if cut := strings.LastIndex(output[:maxChars], ","); cut >= 0 {
			return output[:cut+1] + "\n... (truncated)"
		}
		// No safe splice: fall back to the byte cap.
		return output[:maxChars] + "\n... (truncated)"
	}
	// Plain text: cut at the last line boundary to keep whole lines.
	if cut := strings.LastIndex(output[:maxChars], "\n"); cut > 0 {
		return output[:cut] + "\n... (truncated)"
	}
	return output[:maxChars] + "\n... (truncated)"
}

// PostProcess applies the domain mutation/validation hooks that follow a raw
// tool invocation. It is intentionally separate from CompleteResult so the
// final event contract remains uniform even when a hook changes the output or
// converts a successful mutation into an error.
func (s *ToolService) PostProcess(ctx context.Context, result toolExecResult, turn int, intent string, contextWindow int) toolExecResult {
	output, isErr := result.output, result.isErr
	canonical := canonicalToolName(result.tc.Name)
	life := s.deps.lifecycle
	if life != nil && life.Limits() != nil {
		life.Limits().RecordToolCall(result.tc.Name)
	}
	if life != nil && life.Beliefs() != nil && (canonical == "Read" || canonical == "Grep" || canonical == "Glob" || canonical == "LS") {
		subject := result.tc.Name
		if p, ok := pathArgument(result.tc.Arguments); ok {
			subject = p
		}
		contentSummary := output
		if len(contentSummary) > 200 {
			contentSummary = contentSummary[:200]
		}
		life.Beliefs().Record("file_purpose", subject, contentSummary, turn)
	}
	if s.deps.memory != nil && s.deps.memory.Enhanced() != nil && (canonical == "Read" || canonical == "Edit" || canonical == "Write") {
		if p, ok := pathArgument(result.tc.Arguments); ok && p != "" {
			if proactiveCtx := s.deps.memory.Enhanced().ProactiveContextForFile(p); proactiveCtx != "" && s.deps.appendSystem != nil {
				s.deps.appendSystem(proactiveCtx)
			}
		}
	}
	if life != nil && life.Beliefs() != nil && (canonical == "Write" || canonical == "Edit") {
		if p, ok := pathArgument(result.tc.Arguments); ok {
			life.Beliefs().Invalidate(p)
		}
	}
	if life != nil && life.AgentsAccum() != nil && !isErr && (canonical == "Write" || canonical == "Edit") {
		if p, ok := pathArgument(result.tc.Arguments); ok && p != "" {
			pattern := prompts.ExtractPattern(result.tc.Name, p, output)
			life.AgentsAccum().Record(intent, pattern, []string{p})
			if err := life.AgentsAccum().Flush(); err != nil {
				slog.Warn("failed to flush agents accumulator", "error", err)
			}
		}
	}
	if life != nil && life.Critic() != nil && !isErr && (canonical == "Write" || canonical == "Edit") {
		if p, ok := pathArgument(result.tc.Arguments); ok {
			origContent := ""
			if data, readErr := readFileContent(p); readErr == nil {
				origContent = data
			}
			verdict := life.Critic().PreScreenPatch(origContent, output, intent)
			if life.Critic().ShouldBlock(verdict) {
				output = fmt.Sprintf("Patch rejected by validator: %s. Try again.", strings.Join(verdict.Issues, "; "))
				isErr = true
			}
		}
	}
	if life != nil && life.Shadow() != nil && !isErr && (canonical == "Write" || canonical == "Edit") {
		if p, ok := pathArgument(result.tc.Arguments); ok {
			validationErrs := life.Shadow().ValidateEdit(p, output)
			if len(validationErrs) > 0 {
				warnings := make([]string, 0, len(validationErrs))
				for _, ve := range validationErrs {
					warnings = append(warnings, ve.Message)
				}
				output += fmt.Sprintf("\n\nValidation warnings: %s", strings.Join(warnings, "; "))
			}
		}
	}
	sandboxIntercepted := false
	if s.sandbox != nil && s.sandbox.IsEnabled() && !isErr && (canonical == "Write" || canonical == "Edit") {
		if p, ok := pathArgument(result.tc.Arguments); ok {
			origContent := ""
			if data, readErr := readFileContent(p); readErr == nil {
				origContent = data
			}
			action := "overwrite"
			if canonical == "Edit" {
				action = "edit"
			}
			s.sandbox.Stage(p, action, origContent, output)
			output = fmt.Sprintf("Change staged for review (%s: %s)", action, p)
			sandboxIntercepted = true
		}
	}
	if life != nil && life.LintLoop() != nil && life.LintLoop().Enabled && !isErr && !sandboxIntercepted && (canonical == "Write" || canonical == "Edit") {
		if p, ok := pathArgument(result.tc.Arguments); ok {
			count := life.LintLoop().ReflectionCount(p)
			if life.LintLoop().ShouldRetry(count) {
				if lintResult, lintErr := life.LintLoop().RunLint(p); lintErr == nil && lintResult != nil {
					if reflected := life.LintLoop().BuildReflectedMessage(lintResult); reflected != "" {
						life.LintLoop().RecordReflection(p)
						output += "\n\n" + reflected
					}
				}
			}
		}
	}
	output = s.NormalizeOutput(output, canonical, result.tc.ID, contextWindow)
	if life != nil && life.Pipeline() != nil {
		var execErr error
		if isErr {
			execErr = fmt.Errorf("%s", output)
		}
		if toolResult := life.Pipeline().PostToolExecution(result.tc.Name, result.tc.Arguments, output, execErr); toolResult != nil {
			if toolResult.StallWarning != "" {
				output += "\n\n" + toolResult.StallWarning
			}
			if toolResult.LintErrors != "" {
				output += "\n\nLint: " + toolResult.LintErrors
			}
			if toolResult.RecoveryAction != "" && toolResult.ShouldRetry {
				output += "\n\nRecovery suggestion: " + toolResult.RecoveryAction
			}
		}
	}
	result.output, result.isErr = output, isErr
	return result
}

// CompleteResult owns the service-level completion contract after Session's
// domain-specific post-processing has finished.
func (s *ToolService) CompleteResult(ctx context.Context, result toolExecResult, ch chan<- StreamEvent) toolExecResult {
	output, isErr := result.output, result.isErr
	if !isErr && s.deps.permissions != nil {
		switch canonicalToolName(result.tc.Name) {
		case "Specify", "Plan", "Tasks", "SpecReset":
			s.deps.permissions.AdvanceSpecStage(result.tc.Name)
		case "ApproveImplementation":
			s.deps.permissions.AdvanceSpecStage(result.tc.Name)
			output = "Spec approved — switched to implementation. You may now make changes."
		}
	}
	if s.metrics != nil {
		s.metrics.Counter("tools.executed").Inc()
		if isErr {
			s.metrics.Counter("tools.errors").Inc()
		}
	}
	if s.deps.memory != nil && s.deps.memory.Enhanced() != nil {
		s.deps.memory.Enhanced().OnToolResult(result.tc.Name, result.tc.Arguments, output, isErr)
	}
	hooks.ExecuteAsync(ctx, hooks.EventPostTool, map[string]interface{}{
		"tool": result.tc.Name, "output": output, "is_err": isErr,
	})
	if s.deps.recordVerification != nil {
		s.deps.recordVerification(result.tc, output, isErr)
	}
	ch <- StreamEvent{Type: "tool_result", ToolName: result.tc.Name, Content: output}
	if result.span != nil {
		if isErr {
			result.span.SetTag("error", "true")
		}
		result.span.Finish()
	}
	result.output = output
	return result
}

// ExtractTargets returns the file targets for a tool call.
func (s *ToolService) ExtractTargets(tc types.ToolCall) []string {
	if s == nil || s.registry == nil {
		return extractTargets(tc)
	}
	if t, ok := s.registry.Get(tc.Name); ok {
		return ExtractTargetsFromSchema(t, tc)
	}
	return extractTargets(tc)
}

// EstimateBlastRadius returns a blast-radius report for a set of
// planned tool calls. Drives the "needs confirmation" prompt.
func (s *ToolService) EstimateBlastRadius(planned []PlannedCall) *BlastRadiusReport {
	return EstimateBlastRadius(planned)
}

// ExecuteRegistered runs a single registered tool call with the configured isolation +
// retry policy. Returns the (output, isErr) pair. The tool_result
// StreamEvent is emitted on ch.
func (s *ToolService) ExecuteRegistered(ctx context.Context, tc types.ToolCall, ch chan<- StreamEvent) (string, bool) {
	if s.containerRequired {
		if s.containerExecutor == nil || !s.containerExecutor.Running() {
			msg := "Container not ready — tools are disabled until the sandbox is running."
			ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: msg}
			return msg, true
		}
	}
	if s.tracer != nil {
		_, _ = oteltrace.StartToolSpan(ctx, s.tracer, tc.Name, tc.ID)
	}
	t, _ := s.registry.Get(tc.Name)
	var output string
	var execErr error
	if rpp, ok := t.(tool.RetryPolicyProvider); ok {
		output, execErr = tool.RetryExecutor(ctx, t, marshalInput(tc), rpp.RetryPolicy())
	} else {
		output, execErr = tool.RetryExecutor(ctx, t, marshalInput(tc), tool.DefaultRetryPolicy())
	}
	isErr := execErr != nil
	if isErr {
		output = fmt.Sprintf("Error: %s", execErr.Error())
	}
	ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: output}
	return output, isErr
}

// BackgroundManager returns the background sub-agent manager, or nil
// if background mode is not available.
func (s *ToolService) BackgroundManager() *tool.BackgroundAgentManager {
	if s == nil {
		return nil
	}
	s.bgMu.Lock()
	defer s.bgMu.Unlock()
	return s.bgManager
}

// ContainerRequired reports whether container-first mode is on.
func (s *ToolService) ContainerRequired() bool { return s.containerRequired }

// ContainerExecutor returns the configured container executor, or nil.
func (s *ToolService) ContainerExecutor() tool.ContainerExecutor { return s.containerExecutor }

// Snapshots returns the configured automatic snapshot tracker.
func (s *ToolService) Snapshots() SnapshotTracker { return s.snapshots }

// Sandbox returns the diff sandbox (staged file changes for
// review before apply). New code should access this through
// s.Tools().Sandbox().
func (s *ToolService) Sandbox() *diff.DiffSandbox { return s.sandbox }

// SetSandbox attaches the diff sandbox.
func (s *ToolService) SetSandbox(sb *diff.DiffSandbox) { s.sandbox = sb }

// marshalInput serializes a tool call's args to JSON.
func marshalInput(tc types.ToolCall) json.RawMessage {
	b, _ := json.Marshal(tc.Arguments)
	return b
}
