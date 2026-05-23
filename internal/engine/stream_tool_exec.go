// Package engine holds the extracted tool execution types and functions.
// These are prepared for future integration into the agentLoop.
//
//nolint:unused
package engine

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/tool"
	"github.com/GrayCodeAI/hawk/internal/types"
)

// toolExecResult holds the output of a single tool execution.
// Used by the agentLoop in stream.go for collecting tool results.
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
// Intended to replace the inline tool execution in stream.go's agentLoop.
func (s *Session) executeToolCalls(ctx context.Context, toolCalls []types.ToolCall, ch chan<- StreamEvent) []toolExecResult {
	concurrentCalls, sequentialCalls := classifyToolCalls(toolCalls)

	var results []toolExecResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Execute concurrent (read-only) tools in parallel.
	for _, tc := range concurrentCalls {
		wg.Add(1)
		go func(tc types.ToolCall) {
			defer wg.Done()
			output, isErr := s.runSingleTool(ctx, tc, ch)
			mu.Lock()
			results = append(results, toolExecResult{tc: tc, output: output, isErr: isErr})
			mu.Unlock()
		}(tc)
	}
	wg.Wait()

	// Execute sequential (write) tools one at a time.
	for _, tc := range sequentialCalls {
		output, isErr := s.runSingleTool(ctx, tc, ch)
		results = append(results, toolExecResult{tc: tc, output: output, isErr: isErr})
	}

	return results
}

// runSingleTool executes one tool call with permission checks and sandboxing.
func (s *Session) runSingleTool(ctx context.Context, tc types.ToolCall, ch chan<- StreamEvent) (string, bool) {
	toolCtx := tool.WithToolContext(ctx, &tool.ToolContext{
		AgentSpawnFn: s.AgentSpawnFn,
		AskUserFn:    s.AskUserFn,
		YaadBridge:   s.YaadBridge,
	})
	if s.ContainerExecutor != nil && s.ContainerExecutor.Running() {
		toolCtx = tool.WithContainerExecutor(toolCtx, s.ContainerExecutor)
	}

	toolCtx, toolCancel := context.WithTimeout(toolCtx, toolTimeout(tc.Name))
	defer toolCancel()

	inputJSON, _ := json.Marshal(tc.Arguments)
	output, execErr := s.registry.Execute(toolCtx, tc.Name, inputJSON)
	isErr := execErr != nil

	if isErr {
		ch <- StreamEvent{Type: "tool_result", Content: output, ToolName: tc.Name}
	} else {
		ch <- StreamEvent{Type: "tool_result", Content: output, ToolName: tc.Name, ToolID: tc.ID}
	}

	return output, isErr
}
