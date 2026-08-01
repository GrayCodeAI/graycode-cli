package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/GrayCodeAI/hawk/internal/engine/diff"
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
	host              toolExecutionHost
	containerExecutor tool.ContainerExecutor
	containerRequired bool
	tracer            *oteltrace.Tracer
	snapshots         SnapshotTracker
	bgMu              sync.Mutex
	bgManager         *tool.BackgroundAgentManager
	sandbox           *diff.DiffSandbox
}

// toolExecutionHost is the narrow compatibility seam used while the
// historical post-tool pipeline is moved out of Session. It deliberately
// exposes only batch execution; policy, persistence, and lifecycle state are
// still obtained from their dedicated services by the host implementation.
// Keeping this seam small lets callers migrate to ToolService without
// reintroducing direct Session field access.
type toolExecutionHost interface {
	executeSingleTool(context.Context, types.ToolCall, chan<- StreamEvent, int, string) toolExecResult
}

// NewToolService constructs a ToolService with the given registry.
func NewToolService(registry *tool.Registry) *ToolService {
	return &ToolService{registry: registry}
}

// WithExecutionHost attaches the session-independent execution seam. It is
// set once during session construction and is safe to replace in tests.
func (s *ToolService) WithExecutionHost(host toolExecutionHost) *ToolService {
	s.host = host
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
// unexported execution method. A missing host produces deterministic error
// results instead of panicking, which keeps isolated service tests useful.
func (s *ToolService) ExecuteAll(ctx context.Context, calls []types.ToolCall, ch chan<- StreamEvent, turn int, intent string) []toolExecResult {
	if s == nil || s.host == nil {
		results := make([]toolExecResult, len(calls))
		for i, call := range calls {
			msg := "tool execution host is unavailable"
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
			results[item.index] = s.host.executeSingleTool(ctx, item.tc, ch, turn, intent)
		}(item)
	}
	wg.Wait()
	for _, item := range sequentialCalls {
		results[item.index] = s.host.executeSingleTool(ctx, item.tc, ch, turn, intent)
	}
	return results
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
