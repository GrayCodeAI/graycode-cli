package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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

		// LLM Reflection on Failure: ask the model WHY this failed
		if s.Reflector != nil && shouldReflect(tc.Name, execErr) {
			reflection, refErr := s.Reflector.Reflect(ctx, intentText, s.messages, output)
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
					// Revert the file to its original state
					if preEditContent == "" {
						_ = os.Remove(preEditPath)
					} else {
						_ = os.WriteFile(preEditPath, []byte(preEditContent), 0o644)
					}
					issueStr := "Self-review found issues: " + strings.Join(reviewResult.Issues, "; ")
					if len(reviewResult.Suggestions) > 0 {
						issueStr += ". Suggestions: " + strings.Join(reviewResult.Suggestions, "; ")
					}
					output = issueStr + ". Please fix these issues and try again."
					isErr = true
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
