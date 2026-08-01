package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/types"

	hooks "github.com/GrayCodeAI/hawk/internal/hooks"
	"github.com/GrayCodeAI/hawk/internal/observability/oteltrace"
	"github.com/GrayCodeAI/hawk/internal/prompts"
	"github.com/GrayCodeAI/hawk/internal/sandbox"
)

// toolExecResult holds the output of a single tool execution.
type toolExecResult struct {
	tc     types.ToolCall
	output string
	isErr  bool
}

// filePathArgKeys is the list of argument names that are conventionally
// file paths. Tools with non-standard names silently fall through and
// extractTargets returns an empty list. For a more robust extraction, see
// ExtractTargetsFromSchema which walks the tool's JSON Schema.
var filePathArgKeys = []string{"file_path", "path", "file", "destination"}

// extractTargets extracts file paths from a tool call's arguments using a
// hardcoded allowlist of conventional argument names. New tools with
// non-standard names fall through and produce no targets. For
// schema-aware extraction, see ExtractTargetsFromSchema.
func extractTargets(tc types.ToolCall) []string {
	var targets []string
	for _, key := range filePathArgKeys {
		if v, ok := tc.Arguments[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				targets = append(targets, s)
			}
		}
	}
	return targets
}

// filePathLikeKeySubstrings are substrings in JSON Schema property names that
// strongly suggest a file-path argument. Used by ExtractTargetsFromSchema to
// discover non-conventional argument names.
var filePathLikeKeySubstrings = []string{"path", "file", "dir", "destination", "target"}

func filePathPropertyPriority(name string) int {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "src"), strings.Contains(lower, "source"), strings.Contains(lower, "input"):
		return 0
	case strings.Contains(lower, "dst"), strings.Contains(lower, "dest"), strings.Contains(lower, "output"), strings.Contains(lower, "target"), strings.Contains(lower, "backup"):
		return 2
	default:
		return 1
	}
}

// ExtractTargetsFromSchema walks the tool's JSON Schema to discover file-path
// arguments in the tool call. It does this by:
//  1. Reading `parameters` (the JSON Schema map) to enumerate property names.
//  2. Selecting properties whose name contains a filePathLikeKeySubstrings
//     match (case-insensitive), or whose `description` field mentions a path
//     synonym.
//  3. Extracting the value of each selected property from tc.Arguments.
//
// Tools that don't follow the conventional {file_path, path, file, destination}
// naming can now have their file targets correctly extracted.
func ExtractTargetsFromSchema(t tool.Tool, tc types.ToolCall) []string {
	var targets []string
	params := t.Parameters()
	props, _ := params["properties"].(map[string]interface{})
	if props == nil {
		// Fall back to the conventional allowlist if the tool doesn't expose
		// a JSON Schema (e.g. an LLM-emitted tool or a tests-only stub).
		return extractTargets(tc)
	}
	propNames := make([]string, 0, len(props))
	for propName := range props {
		propNames = append(propNames, propName)
	}
	slices.SortStableFunc(propNames, func(a, b string) int {
		pa, pb := filePathPropertyPriority(a), filePathPropertyPriority(b)
		if pa != pb {
			return pa - pb
		}
		return strings.Compare(a, b)
	})
	for _, propName := range propNames {
		propDef := props[propName]
		propNameLower := strings.ToLower(propName)
		// Convention 1: property name contains a file-path substring.
		nameMatches := false
		for _, sub := range filePathLikeKeySubstrings {
			if strings.Contains(propNameLower, sub) {
				nameMatches = true
				break
			}
		}
		// Convention 2: property description mentions "path", "file", or
		// "directory" — strong signal of a file-path argument.
		descMatches := false
		if pd, ok := propDef.(map[string]interface{}); ok {
			if desc, ok := pd["description"].(string); ok {
				dl := strings.ToLower(desc)
				if strings.Contains(dl, "path") || strings.Contains(dl, "file") || strings.Contains(dl, "directory") {
					descMatches = true
				}
			}
		}
		if !nameMatches && !descMatches {
			continue
		}
		// Type must be a string for us to treat it as a file path.
		if pd, ok := propDef.(map[string]interface{}); ok {
			if typ, ok := pd["type"].(string); ok && typ != "string" {
				continue
			}
		}
		v, ok := tc.Arguments[propName]
		if !ok {
			continue
		}
		if s, ok := v.(string); ok && s != "" {
			targets = append(targets, s)
		}
	}
	return targets
}

const (
	maxConcurrentReadOnlyToolCalls        = 8
	maxConcurrentNetworkReadOnlyToolCalls = 3
)

type indexedToolCall struct {
	index int
	tc    types.ToolCall
}

// executeToolCalls runs all tool calls and returns results.
func (s *Session) executeToolCalls(ctx context.Context, toolCalls []types.ToolCall, ch chan<- StreamEvent, turnCount int, intentText string) []toolExecResult {
	// Estimate blast radius before execution. Use the schema-aware target
	// extractor when the tool is registered (so non-conventional argument
	// names like "target_path" or "destFile" are still picked up); fall back
	// to the conventional extractor otherwise.
	plannedCalls := make([]PlannedCall, len(toolCalls))
	concurrentCalls := make([]indexedToolCall, 0, len(toolCalls))
	sequentialCalls := make([]indexedToolCall, 0, len(toolCalls))
	for i, tc := range toolCalls {
		var targets []string
		if s.registry != nil {
			if t, ok := s.registry.Get(tc.Name); ok {
				targets = ExtractTargetsFromSchema(t, tc)
			} else {
				targets = extractTargets(tc)
			}
		} else {
			targets = extractTargets(tc)
		}
		plannedCalls[i] = PlannedCall{
			ToolName: tc.Name,
			Args:     tc.Arguments,
			Targets:  targets,
		}
		item := indexedToolCall{index: i, tc: tc}
		if tool.IsReadOnly(tc.Name) {
			concurrentCalls = append(concurrentCalls, item)
		} else {
			sequentialCalls = append(sequentialCalls, item)
		}
	}
	blastReport := EstimateBlastRadius(plannedCalls)
	if blastReport.Radius.NeedsConfirmation() {
		// Emit blast radius event for TUI display
		ch <- StreamEvent{
			Type:    "blast_radius",
			Content: blastReport.Message,
		}
	}

	results := make([]toolExecResult, len(toolCalls))
	readOnlySem := make(chan struct{}, maxConcurrentReadOnlyToolCalls)
	networkSem := make(chan struct{}, maxConcurrentNetworkReadOnlyToolCalls)
	var wg sync.WaitGroup
	var mu sync.Mutex

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
			mu.Lock()
			results[item.index] = s.executeSingleTool(ctx, item.tc, ch, turnCount, intentText)
			mu.Unlock()
		}(item)
	}
	wg.Wait()

	for _, item := range sequentialCalls {
		mu.Lock()
		results[item.index] = s.executeSingleTool(ctx, item.tc, ch, turnCount, intentText)
		mu.Unlock()
	}

	return results
}

func isNetworkReadOnlyTool(name string) bool {
	switch strings.ToLower(name) {
	case "websearch", "web_search", "webfetch", "web_fetch", "toolsearch", "tool_search":
		return true
	default:
		return false
	}
}

// RunUserShellCommand runs a user-initiated shell command through the same
// permission, approval, sandbox/container, timeout, retry, truncation, hook,
// and post-processing path used by model-initiated Bash tool calls.
func (s *Session) RunUserShellCommand(ctx context.Context, command string, timeoutSeconds int) (string, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	args := map[string]interface{}{"command": command}
	if timeoutSeconds > 0 {
		args["timeout"] = timeoutSeconds
	}
	ch := make(chan StreamEvent, 4)
	result := s.executeSingleToolWithTool(ctx, types.ToolCall{
		ID:        "slash-bash",
		Name:      "Bash",
		Arguments: args,
	}, tool.BashTool{}, ch, 0, command)
	return result.output, result.isErr
}

// executeSingleTool runs one tool call with permission checks, sandboxing, and all post-processing.
func (s *Session) executeSingleTool(ctx context.Context, tc types.ToolCall, ch chan<- StreamEvent, turnCount int, intentText string) toolExecResult {
	return s.executeSingleToolWithTool(ctx, tc, nil, ch, turnCount, intentText)
}

func (s *Session) executeSingleToolWithTool(ctx context.Context, tc types.ToolCall, override tool.Tool, ch chan<- StreamEvent, turnCount int, intentText string) toolExecResult {
	ch <- StreamEvent{Type: "tool_use", ToolName: tc.Name, ToolID: tc.ID}

	if s.Tools().ContainerRequired() {
		if s.Tools().ContainerExecutor() == nil || !s.Tools().ContainerExecutor().Running() {
			msg := "Container not ready — tools are disabled until the sandbox is running."
			ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: msg}
			return toolExecResult{tc: tc, output: msg, isErr: true}
		}
	}

	var toolSpan *oteltrace.Span
	if s.Tracer != nil {
		_, toolSpan = oteltrace.StartToolSpan(ctx, s.Tracer, tc.Name, tc.ID)
	}

	// Delegate to the extracted PermissionService (Phase 7 migration).
	// s.PermSvc() is never nil because NewSessionWithClient always
	// constructs it and aliases it to s.Perm via WithEngine(pe). The
	// legacy s.Perm field is now a thin shim that reads the same
	// engine.
	//
	// We still sync the legacy fields (PermissionFn, Autonomy) to the
	// service before each call because external code (cmd/, daemon/,
	// multiagent/) writes to those fields directly, and the engine
	// only consults the values it holds. The sync is cheap (two
	// pointer assignments) and removes a class of "settings lost"
	// bugs when callers mutate the session after construction.
	if s.PermissionFn != nil {
		s.PermSvc().SetPermissionFn(s.PermissionFn)
	}
	if s.Autonomy != 0 {
		s.PermSvc().SetAutonomy(s.Autonomy)
	}
	granted, denyMsg := s.PermSvc().CheckTool(ctx, ToolCallInfo{
		Name: tc.Name,
		ID:   tc.ID,
		Args: tc.Arguments,
	})
	s.recordPolicyObservation(tc, "permission", granted, denyMsg)
	if !granted {
		ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: denyMsg}
		if toolSpan != nil {
			toolSpan.SetTag("denied", "true")
			toolSpan.Finish()
		}
		return toolExecResult{tc: tc, output: denyMsg, isErr: true}
	}

	// Human-in-the-loop approval gate for high-risk actions (additive; no-op
	// unless s.Approval is configured and enabled). See approval_gate.go.
	approved, approvalDeny := s.CheckApproval(ctx, tc.Name, tc.Arguments)
	if s.Approval != nil && s.Approval.Enabled {
		s.recordPolicyObservation(tc, "approval", approved, approvalDeny)
	}
	if !approved {
		ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: approvalDeny}
		if toolSpan != nil {
			toolSpan.SetTag("approval_denied", "true")
			toolSpan.Finish()
		}
		return toolExecResult{tc: tc, output: approvalDeny, isErr: true}
	}

	hooks.ExecuteAsync(ctx, hooks.EventPreTool, map[string]interface{}{
		"tool": tc.Name,
		"args": tc.Arguments,
	})

	inputJSON, _ := json.Marshal(tc.Arguments)
	sandboxMode := s.PermSvc().SandboxMode()
	toolCtx := tool.WithToolContext(ctx, &tool.ToolContext{
		AgentSpawnFn: s.AgentSpawnFn,
		AskUserFn:    s.AskUserFn,
		CommitMessageChatFn: func(chatCtx context.Context, prompt string) (string, error) {
			if s.ChatLLM() == nil {
				return "", fmt.Errorf("commit message model is unavailable")
			}
			resp, err := s.ChatLLM().Chat(chatCtx, []types.EyrieMessage{{Role: "user", Content: prompt}}, types.ChatOptions{
				Provider:  s.provider,
				Model:     s.model,
				MaxTokens: 256,
			})
			if err != nil {
				return "", err
			}
			if resp == nil {
				return "", fmt.Errorf("commit message model returned no response")
			}
			return resp.Content, nil
		},
		YaadBridge:         s.MemorySvc().Yaad(),
		SpecSlugGet:        func() string { return s.Perm.SpecSlug },
		SpecSlugSet:        func(slug string) { s.Perm.SpecSlug = slug },
		BackgroundManager:  s.ensureBackgroundManager(),
		ReadOnlyBash:       s.readOnlyBash,
		WorkingDir:         s.workingDir,
		AllowedDirectories: append([]string(nil), s.AllowedDirs...),
		SandboxMode:        sandboxMode,
	})
	if sandboxMode != "" {
		toolCtx = sandbox.ContextWithMode(toolCtx, sandboxMode)
	}
	if s.Tools().ContainerExecutor() != nil && s.Tools().ContainerExecutor().Running() {
		toolCtx = tool.WithContainerExecutor(toolCtx, s.Tools().ContainerExecutor())
	}
	toolCtx, toolCancel := context.WithTimeout(toolCtx, toolTimeout(tc.Name))

	// Self-Review Before Apply: capture file state before Write/Edit
	canonicalPre := canonicalToolName(tc.Name)
	var preEditContent string
	var preEditPath string
	if (canonicalPre == "Write" || canonicalPre == "Edit" || canonicalPre == "MultiEdit") && s.client != nil {
		if p, ok := pathArgument(tc.Arguments); ok && p != "" {
			preEditPath = p
			if data, readErr := readFileContent(p); readErr == nil {
				preEditContent = data
			}
		}
	}

	// Apply the per-tool retry policy for transient errors. Tools can opt out
	// by setting a zero-value RetryPolicy on themselves (via the
	// RetryPolicyProvider interface) — Read/Write/Edit etc. don't opt out and
	// get the default policy of 2 retries (3 attempts total) with 200ms→2s
	// exponential backoff.
	t := override
	if t == nil {
		var ok bool
		if s.registry != nil {
			t, ok = s.registry.Get(tc.Name)
		}
		if !ok {
			toolCancel()
			output := fmt.Sprintf("Error: unknown tool: %s", tc.Name)
			ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: output}
			return toolExecResult{tc: tc, output: output, isErr: true}
		}
	}
	var output string
	var execErr error
	if rpp, ok := t.(tool.RetryPolicyProvider); ok {
		output, execErr = tool.RetryExecutor(toolCtx, t, inputJSON, rpp.RetryPolicy())
	} else {
		output, execErr = tool.RetryExecutor(toolCtx, t, inputJSON, tool.DefaultRetryPolicy())
	}
	toolCancel()
	isErr := execErr != nil
	if isErr {
		s.log.Warn("tool execution error", map[string]interface{}{
			"tool":  tc.Name,
			"error": execErr.Error(),
		})
		output = fmt.Sprintf("Error: %s", execErr.Error())
		if s.LifecycleSvc().Backtrack() != nil {
			s.LifecycleSvc().Backtrack().MarkOutcome(turnCount, "failure")
		}

		// LLM Reflection on Failure: ask the model WHY this failed
		if s.LifecycleSvc().Reflector() != nil && shouldReflect(tc.Name, execErr) {
			reflection, refErr := s.LifecycleSvc().Reflector().Reflect(ctx, intentText, s.Persistence().RawMessages(), output)
			if refErr == nil && reflection != nil {
				output += fmt.Sprintf("\n\n## Self-Reflection\n"+
					"**What failed:** %s\n"+
					"**Why:** %s\n"+
					"**What to do differently:** %s\n"+
					"Try a different approach based on this analysis.",
					reflection.WhatFailed, reflection.WhyFailed, reflection.WhatToDo)
			}
		}
	} else {
		s.log.Info("tool executed", map[string]interface{}{
			"tool":   tc.Name,
			"output": len(output),
		})

		// Self-Review Before Apply: for Write/Edit, ask LLM to review changes
		if preEditPath != "" && s.client != nil && shouldSelfReview(tc.Name) {
			if newContent, readErr := readFileContent(preEditPath); readErr == nil && newContent != preEditContent {
				reviewResult, reviewErr := ReviewBeforeWrite(ctx, s.client, s.model, intentText, preEditPath, preEditContent, newContent)
				if reviewErr == nil && reviewResult != nil && !reviewResult.Approved {
					// Revert the file to its original state. If revert fails we
					// MUST surface that as a hard tool error: silently leaving
					// the rejected diff on disk would let a downstream turn
					// build on top of code the LLM just said was wrong.
					var revertErr error
					if preEditContent == "" {
						revertErr = os.Remove(preEditPath)
					} else {
						revertErr = os.WriteFile(preEditPath, []byte(preEditContent), 0o600)
					}
					if revertErr != nil {
						s.log.Error("self-review revert failed; rejecting diff loudly", map[string]interface{}{
							"path":  preEditPath,
							"error": revertErr.Error(),
						})
						output = fmt.Sprintf("Self-review rejected the change AND the revert failed: %s. "+
							"Original review issues: %s. Manual intervention required.",
							revertErr.Error(), strings.Join(reviewResult.Issues, "; "))
						isErr = true
					} else {
						issueStr := "Self-review found issues: " + strings.Join(reviewResult.Issues, "; ")
						if len(reviewResult.Suggestions) > 0 {
							issueStr += ". Suggestions: " + strings.Join(reviewResult.Suggestions, "; ")
						}
						output = issueStr + ". Please fix these issues and try again."
						isErr = true
					}
				} else if reviewErr == nil && reviewResult != nil && reviewResult.Approved {
					// Append diff summary to output for TUI display
					diffSummary := generateDiffSummary(preEditContent, newContent, preEditPath)
					if diffSummary != "" {
						output += "\n" + diffSummary
					}
				}
			}
		}
	}

	if s.LifecycleSvc().Limits() != nil {
		s.LifecycleSvc().Limits().RecordToolCall(tc.Name)
	}

	canonical := canonicalToolName(tc.Name)
	if s.LifecycleSvc().Beliefs() != nil && (canonical == "Read" || canonical == "Grep" || canonical == "Glob" || canonical == "LS") {
		subject := tc.Name
		if p, ok := pathArgument(tc.Arguments); ok {
			subject = p
		}
		contentSummary := output
		if len(contentSummary) > 200 {
			contentSummary = contentSummary[:200]
		}
		s.LifecycleSvc().Beliefs().Record("file_purpose", subject, contentSummary, turnCount)
	}

	if s.MemorySvc().Enhanced() != nil && (canonical == "Read" || canonical == "Edit" || canonical == "Write") {
		if p, ok := pathArgument(tc.Arguments); ok && p != "" {
			if proactiveCtx := s.MemorySvc().Enhanced().ProactiveContextForFile(p); proactiveCtx != "" {
				s.AppendSystemContext(proactiveCtx)
			}
		}
	}

	if s.LifecycleSvc().Beliefs() != nil && (canonical == "Write" || canonical == "Edit") {
		if p, ok := pathArgument(tc.Arguments); ok {
			s.LifecycleSvc().Beliefs().Invalidate(p)
		}
	}

	// Auto-accumulate learnings into Hawk user state.
	if s.AgentsAccum != nil && !isErr && (canonical == "Write" || canonical == "Edit") {
		if p, ok := pathArgument(tc.Arguments); ok && p != "" {
			pattern := prompts.ExtractPattern(tc.Name, p, output)
			s.AgentsAccum.Record(intentText, pattern, []string{p})
			// Flush periodically (every 5 learnings)
			if err := s.AgentsAccum.Flush(); err != nil {
				slog.Warn("failed to flush agents accumulator", "error", err)
			}
		}
	}

	if s.LifecycleSvc().Critic() != nil && !isErr && (canonical == "Write" || canonical == "Edit") {
		if p, ok := pathArgument(tc.Arguments); ok {
			origContent := ""
			if data, readErr := readFileContent(p); readErr == nil {
				origContent = data
			}
			verdict := s.LifecycleSvc().Critic().PreScreenPatch(origContent, output, intentText)
			if s.LifecycleSvc().Critic().ShouldBlock(verdict) {
				issueStr := strings.Join(verdict.Issues, "; ")
				output = fmt.Sprintf("Patch rejected by validator: %s. Try again.", issueStr)
				isErr = true
			}
		}
	}

	if s.LifecycleSvc().Shadow() != nil && !isErr && (canonical == "Write" || canonical == "Edit") {
		if p, ok := pathArgument(tc.Arguments); ok {
			validationErrs := s.LifecycleSvc().Shadow().ValidateEdit(p, output)
			if len(validationErrs) > 0 {
				var warnings []string
				for _, ve := range validationErrs {
					warnings = append(warnings, ve.Message)
				}
				output += fmt.Sprintf("\n\nValidation warnings: %s", strings.Join(warnings, "; "))
			}
		}
	}

	sandboxIntercepted := false
	if s.Tools().Sandbox() != nil && s.Tools().Sandbox().IsEnabled() && !isErr && (canonical == "Write" || canonical == "Edit") {
		if p, ok := pathArgument(tc.Arguments); ok {
			origContent := ""
			if data, readErr := readFileContent(p); readErr == nil {
				origContent = data
			}
			action := "overwrite"
			if canonical == "Edit" {
				action = "edit"
			}
			s.Tools().Sandbox().Stage(p, action, origContent, output)
			output = fmt.Sprintf("Change staged for review (%s: %s)", action, p)
			sandboxIntercepted = true
		}
	}

	if s.LintLoop != nil && s.LintLoop.Enabled && !isErr && !sandboxIntercepted && (canonical == "Write" || canonical == "Edit") {
		if p, ok := pathArgument(tc.Arguments); ok {
			count := s.LintLoop.ReflectionCount(p)
			if s.LintLoop.ShouldRetry(count) {
				if lintResult, lintErr := s.LintLoop.RunLint(p); lintErr == nil && lintResult != nil {
					reflected := s.LintLoop.BuildReflectedMessage(lintResult)
					if reflected != "" {
						s.LintLoop.RecordReflection(p)
						output += "\n\n" + reflected
					}
				}
			}
		}
	}

	maxChars := 50000
	if window := s.ContextWindowSize(); window > 0 {
		dynamic := window * 20 / 100 * 4
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
		output = output[:maxChars] + "\n... (truncated)"
	}
	output = maybeSpillToolOutput(output, canonical, tc.ID)

	if s.LifecycleSvc().Pipeline() != nil {
		var execErr error
		if isErr {
			execErr = fmt.Errorf("%s", output)
		}
		toolResult := s.LifecycleSvc().Pipeline().PostToolExecution(tc.Name, tc.Arguments, output, execErr)
		if toolResult != nil {
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

	// Spec-stage transitions driven by the model's spec workflow tools.
	// Reaching this point means the tool was granted by the permission
	// engine — for ApproveImplementation specifically, that always meant a
	// real user prompt (see PermissionEngine.CheckTool's spec gate), so this
	// is the approval handoff into Implementing.
	if !isErr {
		switch canonicalToolName(tc.Name) {
		case "Specify", "Plan", "Tasks":
			s.Perm.AdvanceSpecStage(tc.Name)
		case "ApproveImplementation":
			s.Perm.AdvanceSpecStage(tc.Name)
			output = "Spec approved — switched to implementation. You may now make changes."
		}
	}

	s.metrics.Counter("tools.executed").Inc()
	if isErr {
		s.metrics.Counter("tools.errors").Inc()
	}

	if s.MemorySvc().Enhanced() != nil {
		s.MemorySvc().Enhanced().OnToolResult(tc.Name, tc.Arguments, output, isErr)
	}

	hooks.ExecuteAsync(ctx, hooks.EventPostTool, map[string]interface{}{
		"tool":   tc.Name,
		"output": output,
		"is_err": isErr,
	})

	s.recordVerificationObservation(tc, output, isErr)
	ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: output}
	if toolSpan != nil {
		if isErr {
			toolSpan.SetTag("error", "true")
		}
		toolSpan.Finish()
	}
	return toolExecResult{tc: tc, output: output, isErr: isErr}
}

// shouldReflect determines if the Reflector should analyze a tool failure.
// Only reflect on meaningful failures, not trivial ones.
func shouldReflect(toolName string, err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Skip reflection for permission denials, timeouts, and cancellations
	if strings.Contains(errStr, "permission denied") || strings.Contains(errStr, "denied") {
		return false
	}
	if strings.Contains(errStr, "context canceled") || strings.Contains(errStr, "deadline exceeded") {
		return false
	}
	// Reflect on code-related tools
	reflectionTools := map[string]bool{
		"Write": true, "Edit": true, "MultiEdit": true, "StructuredEdit": true,
		"Bash": true, "PowerShell": true,
	}
	return reflectionTools[toolName]
}

// shouldSelfReview determines if a tool result should go through LLM self-review.
func shouldSelfReview(toolName string) bool {
	selfReviewTools := map[string]bool{
		"Write": true, "Edit": true, "MultiEdit": true, "StructuredEdit": true,
	}
	return selfReviewTools[toolName]
}

// generateDiffSummary creates a compact diff summary for the TUI to display.
// Returns a string with line-level change stats and a short preview.
func generateDiffSummary(oldContent, newContent, filePath string) string {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	added := 0
	removed := 0

	// Simple line-level diff count
	oldSet := make(map[string]int)
	for _, l := range oldLines {
		oldSet[l]++
	}
	newSet := make(map[string]int)
	for _, l := range newLines {
		newSet[l]++
	}
	for _, l := range newLines {
		if oldSet[l] > 0 {
			oldSet[l]--
		} else {
			added++
		}
	}
	for _, l := range oldLines {
		if newSet[l] > 0 {
			newSet[l]--
		} else {
			removed++
		}
	}

	if added == 0 && removed == 0 {
		return ""
	}

	// Compact summary: +N -N lines
	parts := []string{}
	if added > 0 {
		parts = append(parts, fmt.Sprintf("+%d", added))
	}
	if removed > 0 {
		parts = append(parts, fmt.Sprintf("-%d", removed))
	}
	return fmt.Sprintf("diff %s: %s lines", filePath, strings.Join(parts, " "))
}
