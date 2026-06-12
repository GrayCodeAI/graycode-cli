package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/observability/oteltrace"
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
	snapshots         SnapshotTracker
	bgManager         *tool.BackgroundAgentManager
	mu                sync.Mutex
}

// NewToolService constructs a ToolService with the given registry.
func NewToolService(registry *tool.Registry) *ToolService {
	return &ToolService{registry: registry}
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

// WithSnapshots configures the snapshot tracker.
func (s *ToolService) WithSnapshots(snap SnapshotTracker) *ToolService {
	s.snapshots = snap
	return s
}

// WithBackgroundManager configures the background sub-agent manager.
func (s *ToolService) WithBackgroundManager(bm *tool.BackgroundAgentManager) *ToolService {
	s.bgManager = bm
	return s
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

// ExtractTargets returns the file targets for a tool call.
func (s *ToolService) ExtractTargets(tc types.ToolCall) []string {
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

// ExecuteOne runs a single tool call with the configured isolation +
// retry policy. Returns the (output, isErr) pair. The tool_result
// StreamEvent is emitted on ch.
func (s *ToolService) ExecuteOne(ctx context.Context, tc types.ToolCall, ch chan<- StreamEvent) (string, bool) {
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
func (s *ToolService) BackgroundManager() *tool.BackgroundAgentManager { return s.bgManager }

// ContainerRequired reports whether container-first mode is on.
func (s *ToolService) ContainerRequired() bool { return s.containerRequired }

// ContainerExecutor returns the configured container executor, or nil.
func (s *ToolService) ContainerExecutor() tool.ContainerExecutor { return s.containerExecutor }

// marshalInput serializes a tool call's args to JSON.
func marshalInput(tc types.ToolCall) json.RawMessage {
	b, _ := json.Marshal(tc.Arguments)
	return b
}
