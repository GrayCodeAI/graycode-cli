package engine

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/engine/safety"
	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/types"

	"github.com/GrayCodeAI/hawk/internal/observability/oteltrace"
)

// toolExecResult holds the output of a single tool execution.
type toolExecResult struct {
	tc     types.ToolCall
	output string
	isErr  bool
	err    error
	span   *oteltrace.Span
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
	if s.Tools() == nil {
		return nil
	}
	return s.Tools().ExecuteAll(ctx, toolCalls, ch, turnCount, intentText)
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
	if s.Tools() == nil {
		msg := "session tool service is not initialized"
		ch <- StreamEvent{Type: "tool_result", ToolName: tc.Name, Content: msg}
		return toolExecResult{tc: tc, output: msg, isErr: true, err: fmt.Errorf("%s", msg)}
	}
	canonicalPre := canonicalToolName(tc.Name)
	var preEditContent, preEditPath string
	if (canonicalPre == "Write" || canonicalPre == "Edit" || canonicalPre == "MultiEdit") && s.ChatLLM() != nil {
		if p, ok := pathArgument(tc.Arguments); ok && p != "" {
			preEditPath = p
			if data, readErr := readFileContent(p); readErr == nil {
				preEditContent = data
			}
		}
	}
	core := s.Tools().ExecuteOne(ctx, tc, override, ch, turnCount, intentText)
	output, execErr, isErr := core.output, core.err, core.isErr
	if isErr {
		s.Logger().Warn("tool execution error", map[string]interface{}{"tool": tc.Name, "error": execErr.Error()})
		output = fmt.Sprintf("Error: %s", execErr.Error())
		if s.LifecycleSvc().Backtrack() != nil {
			s.LifecycleSvc().Backtrack().MarkOutcome(turnCount, "failure")
		}
		if s.LifecycleSvc().Reflector() != nil && shouldReflect(tc.Name, execErr) {
			reflection, refErr := s.LifecycleSvc().Reflector().Reflect(ctx, intentText, s.Persistence().RawMessages(), output)
			if refErr == nil && reflection != nil {
				output += fmt.Sprintf("\n\n## Self-Reflection\n**What failed:** %s\n**Why:** %s\n**What to do differently:** %s\nTry a different approach based on this analysis.", reflection.WhatFailed, reflection.WhyFailed, reflection.WhatToDo)
			}
		}
	} else {
		s.Logger().Info("tool executed", map[string]interface{}{"tool": tc.Name, "output": len(output)})
		if preEditPath != "" && s.ChatLLM() != nil && shouldSelfReview(tc.Name) {
			if newContent, readErr := readFileContent(preEditPath); readErr == nil && newContent != preEditContent {
				reviewResult, reviewErr := ReviewBeforeWrite(ctx, s.ChatLLM().Client(), s.ChatLLM().Model(), intentText, preEditPath, preEditContent, newContent)
				if reviewErr == nil && reviewResult != nil && !reviewResult.Approved {
					var revertErr error
					if preEditContent == "" {
						revertErr = os.Remove(preEditPath)
					} else {
						revertErr = os.WriteFile(preEditPath, []byte(preEditContent), 0o600)
					}
					if revertErr != nil {
						s.Logger().Error("self-review revert failed; rejecting diff loudly", map[string]interface{}{"path": preEditPath, "error": revertErr.Error()})
						output = fmt.Sprintf("Self-review rejected the change AND the revert failed: %s. Original review issues: %s. Manual intervention required.", revertErr.Error(), strings.Join(reviewResult.Issues, "; "))
					} else {
						issueStr := "Self-review found issues: " + strings.Join(reviewResult.Issues, "; ")
						if len(reviewResult.Suggestions) > 0 {
							issueStr += ". Suggestions: " + strings.Join(reviewResult.Suggestions, "; ")
						}
						output = issueStr + ". Please fix these issues and try again."
					}
					isErr = true
				} else if reviewErr == nil && reviewResult != nil && reviewResult.Approved {
					if diffSummary := generateDiffSummary(preEditContent, newContent, preEditPath); diffSummary != "" {
						output += "\n" + diffSummary
					}
				}
			}
		}
	}
	processed := s.Tools().PostProcess(ctx, toolExecResult{tc: tc, output: output, isErr: isErr, err: execErr, span: core.span}, turnCount, intentText, s.ContextWindowSize())
	return s.Tools().CompleteResult(ctx, processed, ch)
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
