package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/types"

	hooks "github.com/GrayCodeAI/hawk/internal/hooks"
	"github.com/GrayCodeAI/hawk/internal/observability/oteltrace"
	modelPkg "github.com/GrayCodeAI/hawk/internal/provider/routing"
)

// toolExecResult holds the output of a single tool execution.
type toolExecResult struct {
	tc     types.ToolCall
	output string
	isErr  bool
}

// classifyToolCalls splits tool calls into concurrent (read-only) and sequential (write) batches.
func classifyToolCalls(calls []types.ToolCall) (concurrent, sequential []types.ToolCall) {
	safeConcurrent := map[string]bool{"Read": true, "Grep": true, "Glob": true, "LS": true, "WebSearch": true, "WebFetch": true, "ToolSearch": true}
	for _, tc := range calls {
		if safeConcurrent[tc.Name] {
			concurrent = append(concurrent, tc)
		} else {
			sequential = append(sequential, tc)
		}
	}
	return
}

// executeToolCalls runs all tool calls and returns results.
func (s *Session) executeToolCalls(ctx context.Context, toolCalls []types.ToolCall, ch chan<- StreamEvent, turnCount int, intentText string) []toolExecResult {
	concurrentCalls, sequentialCalls := classifyToolCalls(toolCalls)

	var results []toolExecResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, tc := range concurrentCalls {
		wg.Add(1)
		go func(tc types.ToolCall) {
			defer wg.Done()
			r := s.executeSingleTool(ctx, tc, ch, turnCount, intentText)
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}(tc)
	}
	wg.Wait()

	for _, tc := range sequentialCalls {
		r := s.executeSingleTool(ctx, tc, ch, turnCount, intentText)
		results = append(results, r)
	}

	return results
}

// executeSingleTool runs one tool call with permission checks, sandboxing, and all post-processing.
func (s *Session) executeSingleTool(ctx context.Context, tc types.ToolCall, ch chan<- StreamEvent, turnCount int, intentText string) toolExecResult {
	ch <- StreamEvent{Type: "tool_use", ToolName: tc.Name, ToolID: tc.ID}

	var toolSpan *oteltrace.Span
	if s.Tracer != nil {
		_, toolSpan = oteltrace.StartToolSpan(ctx, s.Tracer, tc.Name, tc.ID)
	}

	s.Perm.PromptFn = s.PermissionFn
	s.Perm.Autonomy = s.Autonomy

	granted, denyMsg := s.Perm.CheckTool(ctx, ToolCallInfo{
		Name: tc.Name,
		ID:   tc.ID,
		Args: tc.Arguments,
	})
	if !granted {
		ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: denyMsg}
		if toolSpan != nil {
			toolSpan.SetTag("denied", "true")
			toolSpan.Finish()
		}
		return toolExecResult{tc: tc, output: denyMsg, isErr: true}
	}

	hooks.ExecuteAsync(ctx, hooks.EventPreTool, map[string]interface{}{
		"tool": tc.Name,
		"args": tc.Arguments,
	})

	inputJSON, _ := json.Marshal(tc.Arguments)
	toolCtx := tool.WithToolContext(ctx, &tool.ToolContext{
		AgentSpawnFn: s.AgentSpawnFn,
		AskUserFn:    s.AskUserFn,
		YaadBridge:   s.YaadBridge,
	})
	if s.ContainerExecutor != nil && s.ContainerExecutor.Running() {
		toolCtx = tool.WithContainerExecutor(toolCtx, s.ContainerExecutor)
	}
	toolCtx, toolCancel := context.WithTimeout(toolCtx, toolTimeout(tc.Name))
	output, execErr := s.registry.Execute(toolCtx, tc.Name, inputJSON)
	toolCancel()
	isErr := execErr != nil
	if isErr {
		s.log.Warn("tool execution error", map[string]interface{}{
			"tool":  tc.Name,
			"error": execErr.Error(),
		})
		output = fmt.Sprintf("Error: %s", execErr.Error())
		if s.Backtrack != nil {
			s.Backtrack.MarkOutcome(turnCount, "failure")
		}
	} else {
		s.log.Info("tool executed", map[string]interface{}{
			"tool":   tc.Name,
			"output": len(output),
		})
	}

	if s.Limits != nil {
		s.Limits.RecordToolCall(tc.Name)
	}

	canonical := canonicalToolName(tc.Name)
	if s.Beliefs != nil && (canonical == "Read" || canonical == "Grep" || canonical == "Glob" || canonical == "LS") {
		subject := tc.Name
		if p, ok := pathArgument(tc.Arguments); ok {
			subject = p
		}
		contentSummary := output
		if len(contentSummary) > 200 {
			contentSummary = contentSummary[:200]
		}
		s.Beliefs.Record("file_purpose", subject, contentSummary, turnCount)
	}

	if s.EnhancedMemory != nil && (canonical == "Read" || canonical == "Edit" || canonical == "Write") {
		if p, ok := pathArgument(tc.Arguments); ok && p != "" {
			if proactiveCtx := s.EnhancedMemory.ProactiveContextForFile(p); proactiveCtx != "" {
				s.AppendSystemContext(proactiveCtx)
			}
		}
	}

	if s.Beliefs != nil && (canonical == "Write" || canonical == "Edit") {
		if p, ok := pathArgument(tc.Arguments); ok {
			s.Beliefs.Invalidate(p)
		}
	}

	if s.Critic != nil && !isErr && (canonical == "Write" || canonical == "Edit") {
		if p, ok := pathArgument(tc.Arguments); ok {
			origContent := ""
			if data, readErr := readFileContent(p); readErr == nil {
				origContent = data
			}
			verdict := s.Critic.PreScreenPatch(origContent, output, intentText)
			if s.Critic.ShouldBlock(verdict) {
				issueStr := strings.Join(verdict.Issues, "; ")
				output = fmt.Sprintf("Patch rejected by validator: %s. Try again.", issueStr)
				isErr = true
			}
		}
	}

	if s.Shadow != nil && !isErr && (canonical == "Write" || canonical == "Edit") {
		if p, ok := pathArgument(tc.Arguments); ok {
			validationErrs := s.Shadow.ValidateEdit(p, output)
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
	if s.Sandbox != nil && s.Sandbox.IsEnabled() && !isErr && (canonical == "Write" || canonical == "Edit") {
		if p, ok := pathArgument(tc.Arguments); ok {
			origContent := ""
			if data, readErr := readFileContent(p); readErr == nil {
				origContent = data
			}
			action := "overwrite"
			if canonical == "Edit" {
				action = "edit"
			}
			s.Sandbox.Stage(p, action, origContent, output)
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
	if info, ok := modelPkg.Find(s.model); ok && info.ContextSize > 0 {
		dynamic := info.ContextSize * 20 / 100 * 4
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

	if s.Pipeline != nil {
		var execErr error
		if isErr {
			execErr = fmt.Errorf("%s", output)
		}
		toolResult := s.Pipeline.PostToolExecution(tc.Name, tc.Arguments, output, execErr)
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

	s.metrics.Counter("tools.executed").Inc()
	if isErr {
		s.metrics.Counter("tools.errors").Inc()
	}

	if s.EnhancedMemory != nil {
		s.EnhancedMemory.OnToolResult(tc.Name, tc.Arguments, output, isErr)
	}

	hooks.ExecuteAsync(ctx, hooks.EventPostTool, map[string]interface{}{
		"tool":   tc.Name,
		"output": output,
		"is_err": isErr,
	})

	ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: output}
	if toolSpan != nil {
		if isErr {
			toolSpan.SetTag("error", "true")
		}
		toolSpan.Finish()
	}
	return toolExecResult{tc: tc, output: output, isErr: isErr}
}
