# Session God-Object Decomposition — Design and Migration Status

> Status: **IN PROGRESS** — the canonical service graph is active and the
> runtime execution path no longer uses legacy permission or tool fallbacks.
> Remaining work is limited to moving the last non-authoritative Session fields
> into their owning services.
> Author: opencode session
> Date: 2026-06-12
> Scope: `hawk/internal/engine/session.go` (the 35-collaborator `Session` struct)

## Problem

`Session` at `hawk/internal/engine/session.go:42-141` is a god object:

- 35 fields, ~30 of them `*T` pointer collaborators with optional behavior
- Every new feature adds a field (Beliefs, Critic, Trajectory, Steer­ing, etc.)
- Construction requires wiring 30+ fields; unit testing is impractical
- `NewSessionWithClient` (lines 149-187) already has a 35-line constructor body
- The `AgentLoop` 833-line function reads/writes ~25 of these fields, creating a hidden web of dependencies
- New contributors can't tell which field is required vs optional vs diagnostic

## Goal

Break `Session` into ~6 cohesive sub-services, each with:
- a single responsibility
- an interface (so tests can use stubs)
- a clear lifecycle (created once in `NewSessionWithClient`, passed to `agentLoop`)

The `agentLoop` should consume these sub-services as named dependencies — no implicit `s.Beliefs.Size()` reach-throughs.

## Implemented Migration Slice (2026-08)

The refactor branch now enforces these boundaries:

- `SubServices()` is the canonical composition root for the six extracted
  services. The obsolete `SessionServices` bridge has been removed.
- `PersistenceService` owns immutable transcript snapshots, system-context
  mutations, compaction metadata, token accounting, and checkpoint identity.
  Returned messages are deep copies, including nested tool arguments.
- `LifecycleService` owns session-start/end bookkeeping, few-shot learning,
  adaptive feedback, model cascade access, and quality-loop handles.
- `MemoryService` owns recall fallback, Yaad/enhanced-memory finalization, and
  session summaries.
- `PermissionService` owns the approval gate and ask-user callback;
  `Session.CheckApproval` is a thin orchestration facade with no state sync.
- `ToolService.ExecuteAll` owns batching, ordering, blast-radius reporting,
  and read-only concurrency limits. `ToolService.ExecuteOne` now owns raw
  invocation boundaries: permission/approval, tracing, isolation, context
  injection, lookup, timeout, and retry. `PostProcess` now owns
  mutation, validation, sandbox, lint, and pipeline hooks. `CompleteResult` owns
  spec transitions, counters, enhanced-memory notification, post-tool hooks,
  verification observation, span closure, and result emission.
- The agent loop uses these service APIs for transport, persistence, memory,
  lifecycle, permission-stage, and tool-batch operations.

The permission aliases (`Perm`, `Permissions`, `AutoMode`, `Classifier`,
`BypassKill`, `PermissionFn`, `Approval`, and `Autonomy`) have been removed from
`Session`. Remaining lifecycle, memory, and persistence fields are being moved
incrementally; service state is authoritative and no fallback execution path
exists. The first Phase 2 slice also moved token accounting, token-estimate
cache, and checkpoint-manager state fully into `PersistenceService`; the
corresponding duplicate `Session` fields have been removed. `persistID` and
zero-value lazy service materialization remain pending because their call
graphs and compatibility behavior have higher fan-out. Lazy materialization of
the persistence service itself is now synchronized, and `Cost` exposes locked
snapshots while retaining its public fields for source compatibility. WAL
recovery now surfaces non-not-found I/O errors instead of treating them as an
empty session. The next slice moved LLM client/provider/model ownership into
`ChatService` and added synchronization around transport identity and
reattachment; command fixtures now use the explicit constructor rather than
relying on zero-value Session transport state.

This decomposition does not yet make `PersistenceService` the durable storage
authority. It owns live runtime state; `internal/session` still owns the active
JSONL save/load format and external WAL recovery. The implemented
`SQLiteStore`, workspace snapshots, conversation graph, and graph journal are
separate capabilities and must not be treated as interchangeable persistence
backends without an explicit migration and recovery decision.

## Proposed Decomposition

### 1. `ChatService` — owns the LLM transport

**Owns:** `client ChatClient`, `provider string`, `model string`, `apiKeys map[string]string`, `Router *modelPkg.Router`, `DeploymentRouting bool`, `RateLimiter *ratelimit.Limiter`, `OutputSchema string`, `GLMThinkingEnabled *bool`, `ContCfg types.ContinuationConfig`, `ChatOptions{}` builder.

**Methods:**
- `BuildOptions(systemPrompt string, tools []client.EyrieTool, activeModel string, maxTokens int) types.ChatOptions`
- `Chat(ctx, msgs, opts) (*types.EyrieResponse, error)` — non-streaming, used by background goroutines
- `Stream(ctx, msgs, opts) (*types.StreamResult, error)` — with retry, circuit-breaker record, rate-limit wait
- `RecordSuccess(latency time.Duration)` / `RecordFailure(err error)`
- `BuildContinuationConfig() types.ContinuationConfig`

**File:** `hawk/internal/engine/chat_service.go` (~150 LOC)

### 2. `MemoryService` — owns yaad bridge + recall/remember

**Owns:** `Memory MemoryRecaller`, `YaadBridge *memory.YaadBridge`, `EnhancedMemory *memory.EnhancedMemoryManager`, `SkillDistiller *memory.SkillDistiller`, `Sleeptime *memory.SleeptimeAgent`, `Activity *memory.ActivityTracker`, `AgentsAccum *prompts.AgentsAccum`, `FewShotStore *FewShotStore`, `AdaptivePrompt *AdaptivePrompt`.

**Methods:**
- `RecallContext(ctx, lastUserMsg string, budget int) string` — unifies backend recall behind one nil-safe call
- `Remember(ctx, content, category string)` — wraps memory.Remember
- `RunSleeptimeConsolidation(ctx, provider ChatService, messages []types.EyrieMessage)` — background
- `RunSkillDistillation(ctx, provider ChatService, ...)` — background
- `RecordFeedback(ctx, content string, action string)` — wraps EnhancedMemory feedback
- `ShouldRemember(text string) bool` — heuristic for auto-remember

**File:** `hawk/internal/engine/memory_service.go` (~200 LOC)

### 3. `ToolService` — owns the registry + tool execution

**Owns:** `registry *tool.Registry`, `ContainerExecutor tool.ContainerExecutor`, `ContainerRequired bool`, `Snapshots SnapshotTracker`, `BackgroundManager *tool.BackgroundAgentManager`, `AllowedDirs []string`, `Protected PathProtector`.

**Methods:**
- `Classify(calls []types.ToolCall) (concurrent, sequential []types.ToolCall)` — uses `tool.IsReadOnly`
- `ExecuteAll(ctx, calls []types.ToolCall, ch chan<- StreamEvent, turn int, intent string) []toolExecResult` — the 15-stage post-call pipeline moved out of `stream_tool_exec.go`
- `EstimateBlastRadius(calls []types.ToolCall) BlastReport`
- `SpawnBackgroundAgent(ctx, prompt string) (id string, err error)`
- `PollBackgroundAgent(id string) (output string, done bool, err error)`

**File:** `hawk/internal/engine/tool_service.go` (~400 LOC — biggest, owns the 15-stage pipeline)

### 4. `PermissionService` — owns the safety layer

**Owns:** `Perm *PermissionEngine`, `Permissions *PermissionMemory`, `AutoMode *permissions.AutoModeState`, `Classifier *permissions.Classifier`, `BypassKill *permissions.BypassKillswitch`, `Mode PermissionMode`, `Autonomy AutonomyLevel`, `PermissionFn func(PermissionRequest)`, `Approval *ApprovalGate`, `MaxBudgetUSD float64`.

**Methods:**
- `CheckTool(ctx, info ToolCallInfo) (granted bool, denyMsg string)` — wraps Perm.CheckTool + Approval.CheckApproval
- `ExceedsBudget(cost float64) bool`
- `RecordToolCall(name string)`
- `RecordCost(cost float64)`
- `SetMode(mode string) error` — validates + applies
- `SetMaxTurns(n int)`, `SetMaxBudgetUSD(usd float64)`

**File:** `hawk/internal/engine/permission_service.go` (~150 LOC)

### 5. `LifecycleService` — owns the self-improvement loop

**Owns:** `Lifecycle *SessionLifecycle`, `Reflector *Reflector`, `Beliefs *BeliefState`, `Backtrack *BacktrackEngine`, `Limits *LimitTracker`, `Critic *Critic`, `Backtrack`, `Trajectory *TrajectoryDistiller`, `Shadow *branching.ShadowWorkspace`, `Cascade *branching.CascadeRouter`, `Steering *SteeringQueue`, `LoopDet *LoopDetector`, `Snowball *branching.SnowballDetector`.

**Methods:**
- `OnSessionStart(ctx, lastUserMsg string) (learnedCtx string)`
- `OnSessionEnd(ctx, success bool, duration time.Duration, messages []types.EyrieMessage)`
- `SelectModel(taskType, currentModel, hint string) string` — wraps Cascade
- `OnToolFailure(ctx, intent string, history []types.EyrieMessage, errOutput string) (*Reflection, error)`
- `RecordDecision(turn int, toolNames string, msgs []types.EyrieMessage)`
- `CheckLimits(turnCount int) (cont bool, reason string)` — wraps Limits
- `RecordTurn(turn int)`, `RecordToolCall(name string)`
- `DetectDoomLoop(steps []ToolStep) bool`
- `InjectSteering(messages []types.EyrieMessage) []types.EyrieMessage`

**File:** `hawk/internal/engine/lifecycle_service.go` (~250 LOC)

### 6. `PersistenceService` — owns checkpoint, session, yaad snapshot

**Owns:** `persistID string`, `checkpointMgr *session.CheckpointManager`, `lastPromptTokens int`, `lastCompletionTokens int`, `ConvoDAG *storage.DAG`, `OnCompaction OnCompaction`, `PinnedMessages int`, `AutoCompactThresholdPct int`, `ContextWindowCached int`, `AutoCompactor *AutoCompactor`, `CompactSplit`, `CompactProviderNative`, `CompactStrategyEngine`, `Files *FileTracker`.

**Methods:**
- `PersistID() string`, `SetPersistID(id string)`
- `ManageContextBeforeTurn(ctx) (strategy string, compacted bool)`
- `Compact()`, `SmartCompact()`, `ShouldAutoCompact()`, `ShouldCompactByBudget()`
- `AddUser(content string)`, `AddAssistant(content string)`, `AddUserWithImage(...)`, `AddUserWithAttachment(...)`
- `AddUserWithDocumentText(content, label, extracted string)`
- `AppendSystemContext(content string)`, `ReplaceSystemContextSection(header, content string)`
- `Messages() []types.EyrieMessage`, `RawMessages()`, `MessageCount()`, `LoadMessages([]types.EyrieMessage)`
- `RemoveLastExchange()`
- `ConvoHead() string`, `ForkConversation(nodeID string) (string, error)`, `SwitchBranch(nodeID string) error`, `ListBranches(nodeID string) ([]*storage.DAGNode, error)`
- `CheckpointOnCompaction(strategy string, before, after int, manual bool)`
- `TokenTracking() (prompt, completion int)`, `RecordAPIUsage(prompt, completion int)`

**File:** `hawk/internal/engine/persistence_service.go` (~300 LOC)

## What stays on `Session`

After the refactor, `Session` becomes a thin facade:

```go
type Session struct {
    // Identity (still here)
    ID           string
    Provider     string
    Model        string
    SystemPrompt string
    // The 6 sub-services
    Chat        *ChatService
    Memory      *MemoryService
    Tools       *ToolService
    Permission  *PermissionService
    Lifecycle   *LifecycleService
    Persistence *PersistenceService
    // Observability pass-through
    Tracer    *oteltrace.Tracer
    Metrics   *metrics.Registry
    Log       *logger.Logger
    // Loop control (still on Session because the agent loop reads them every turn)
    Sandbox        *DiffSandbox
    Plan           *PlanState
    OutputSchema   string
    Teach          TeachConfig
}
```

~15 fields total, down from 35. The 6 sub-services are constructed once in `NewSessionWithClient` and stay for the session's lifetime.

## `agentLoop` becomes a 250-line orchestrator

```go
func (s *Session) agentLoop(ctx context.Context, ch chan<- StreamEvent) {
    defer close(ch)
    defer s.Lifecycle.OnSessionEnd(ctx, ...)

    hooks.ExecuteAsync(ctx, hooks.EventSessionStart, ...)
    s.Lifecycle.OnSessionStart(ctx, s.lastUserMessage())

    s.Persistence.ManageContextBeforeTurn(ctx)
    msgs := s.Persistence.Messages()
    sysCtx, err := s.Memory.RecallContext(ctx, s.lastUserMessage(), 2000)
    if err == nil && sysCtx != "" {
        s.Persistence.AppendSystemContext(sysCtx)
    }

    activeModel := s.Lifecycle.SelectModel(taskType, s.Model, "")

    for turn := 0; ; turn++ {
        if !s.Lifecycle.CheckLimits(turn) { return }

        opts := s.Chat.BuildOptions(s.system, s.Tools.Registry().EyrieTools(), activeModel, maxTok)
        result, err := s.Chat.Stream(ctx, s.Persistence.Messages(), opts)
        if err != nil { /* error path */ }
        defer result.Close()

        // Consume stream: textContent, toolCalls, sawThinking, stopReason
        ...

        if len(toolCalls) == 0 {
            s.Persistence.AddAssistant(textContent)
            s.Memory.AutoRemember(ctx, textContent)
            ch <- StreamEvent{Type: "done"}
            return
        }
        s.Persistence.AddAssistantWithToolUse(textContent, toolCalls)
        results := s.Tools.ExecuteAll(ctx, toolCalls, ch, turn, textContent)
        s.Persistence.AddToolResults(results)
    }
}
```

The loop body shrinks from 800 lines to ~250 because:
- 15-stage tool pipeline moves to `ToolService.ExecuteAll`
- 20+ post-call side effects (beliefs, memory, critic, shadow, sandbox, snapshot, agents-accum) move to sub-service methods
- Stream retry with ctx-cancel moves to `ChatService.Stream`
- Permission/approval flow moves to `PermissionService.CheckTool`

## Migration Plan

1. **Phase 1** (this PR's 4 fixes): just the time.Sleep, ReadOnlyTools, AGENTS.md, self-review fixes. **No Session refactor yet.**
2. **Phase 2** (separate PR): extract `ChatService` first because it has the fewest field touches. Verify `agentLoop` builds and tests pass.
3. **Phase 3**: extract `MemoryService` (most fields, but only consumed in the preamble/postamble of `agentLoop`).
4. **Phase 4**: extract `PermissionService` (smallest surface).
5. **Phase 5**: extract `LifecycleService` (biggest behavioral surface, but mostly reads `messages` and `tools`).
6. **Phase 6**: extract `PersistenceService` (last because it owns the `messages []types.EyrieMessage` field which is touched everywhere).
7. **Phase 7**: extract `ToolService` (depends on 4, 5; do last).
8. **Phase 8**: delete fields from `Session`, add backwards-compatible shims that delegate to the new services, mark for removal in v0.2.0.

Each phase is a separate PR with a focused test suite.

## Testing Strategy

After the refactor, the following tests should be trivial (impossible to write today):

```go
func TestAgentLoopRespectsContextCancellation(t *testing.T) {
    chat := &mockChatService{streamErr: ctx.Err()}
    session := &Session{Chat: chat, /* no other services wired */}
    stream, _ := session.Stream(ctx)
    // Assert: loop exits on first event
}

func TestToolExecutionRejectsWithoutPermission(t *testing.T) {
    perm := &mockPermissionService{granted: false, denyMsg: "nope"}
    session := &Session{Permission: perm}
    result := session.Tools.ExecuteAll(ctx, calls, ch, 0, "")
    // Assert: all results are isErr=true with "nope" content
}

func TestMemoryRecallReturnsEmptyWhenNoBridge(t *testing.T) {
    session := &Session{Memory: &MemoryService{}} // no YaadBridge
    ctx, _ := session.Memory.RecallContext(ctx, "test", 100)
    assert.Equal(t, "", ctx)
}
```

These tests don't need to construct a `Session` anymore; they can construct just the sub-service under test.

## Open Questions

1. Should `SubSession` (the sub-agent factory at `session.go:207-216`) become a method on `Session` or move to a dedicated `SubAgent` service? Currently it copies the parent's LLM transport — that's a `ChatService` concern, not a `Session` concern. Proposal: move to `SubAgentService`.
2. Should the sub-service interfaces be exported (so external test packages can mock them) or kept internal? The `MockChatService` test above would require either exporting or building test-helper shims.
3. Where do `OutputSchema`, `GLMThinkingEnabled`, `TeachConfig`, `Sandbox`, `Plan` belong? They're all chat/options concerns. Proposal: fold into `ChatService` or extract a `SessionOptions` struct that `ChatService.BuildOptions` reads.

## Estimated Effort

- Phase 1: ✅ done (this PR)
- Phases 2-7: ~6 PRs, ~1500 LOC moved, ~800 LOC of `agentLoop` removed, ~600 LOC of new sub-service files added
- Total: ~5 days of focused refactor work + tests

## Status

**IN PROGRESS.** The implemented migration slice above is live and tested.
Tool execution, compaction, token accounting, lazy persistence initialization,
cost snapshots, and WAL error handling have active coverage. Remaining work is
to move the last non-authoritative lifecycle, memory, and persistence fields,
resolve durable persistence authority, migrate all production call sites, and
then remove compatibility fields in separately reviewed cleanup commits.
