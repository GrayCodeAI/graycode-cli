package eventlog

// BoundaryFact is the durable payload for a turn or step boundary event. Step
// boundaries carry both counters so consumers can correlate a step with its turn.
type BoundaryFact struct {
	Turn int `json:"turn,omitempty"`
	Step int `json:"step,omitempty"`
}

// ChunkFact records one partial assistant stream delta. The Seq assigned by Log is
// the replay order, so concordance consumers can rebuild the stream exactly.
// Kind distinguishes the delta variant: "" (text, default for backward compat),
// "thinking" (reasoning/reasoning-delta), "block-start" (response block begins),
// "block-end" (response block ends), "finish" (stream ended; Chunk carries the
// stop reason), and "tool-call" (incremental tool-call; use ToolCallDeltaFact
// via AppendAssistantChunk with Kind "tool-call"). Text and thinking chunks pack
// into consecutive text-chunk rows; structural kinds (block-start, block-end,
// finish, tool-call) always stay standalone.
type ChunkFact struct {
	Turn  int    `json:"turn,omitempty"`
	Step  int    `json:"step,omitempty"`
	Chunk string `json:"chunk,omitempty"`
	Kind  string `json:"kind,omitempty"`
}

// ToolCallDeltaFact records an incremental tool-call delta. It embeds ChunkFact
// for replay context (Turn/Step/Kind) and adds Name (set on the first delta) and
// Arguments (the partial argument string that grows across successive deltas).
// Used by AppendAssistantChunk's "tool-call" variant.
type ToolCallDeltaFact struct {
	ChunkFact
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// CompactionFact records a context-compaction pass. It is durable because compaction
// replaces part of the projected surface; the event re-establishes what was pruned.
type CompactionFact struct {
	Strategy     string `json:"strategy,omitempty"`
	TokensBefore int    `json:"tokens_before,omitempty"`
	TokensAfter  int    `json:"tokens_after,omitempty"`
	Manual       bool   `json:"manual,omitempty"`
}

// ToolWorkflowBounds records one tool-execution run. Tool and Error are informational;
// commands report the observable effect of the run.
type ToolWorkflowBounds struct {
	Tool string `json:"tool,omitempty"`
}

// LLMRetryFact records a stream retry decision.
type LLMRetryFact struct {
	Attempt int    `json:"attempt,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// RequestContextFact records the durable model route metadata at a request
// boundary. Ported from DSH's RequestContext: provider, model, and context
// window — not message/token counts (those are on RequestHeaderFact).
type RequestContextFact struct {
	Provider      string `json:"provider,omitempty"`
	Model         string `json:"model,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`
}

// PermissionFact records a durable approval/permission decision. Allowed reports
// whether the action was permitted.
type PermissionFact struct {
	Tool     string `json:"tool,omitempty"`
	Category string `json:"category,omitempty"`
	Allowed  bool   `json:"allowed"`
	Message  string `json:"message,omitempty"`
}

// ApprovalAskedFact records that a gated action reached the approval gate.
// Hawk fields are the primary model (turn/step/tool/category/question); DSH
// parity fields (ID, CallID, Reason) are optional extensions for cross-platform log reading.
type ApprovalAskedFact struct {
	Turn     int    `json:"turn,omitempty"`
	Step     int    `json:"step,omitempty"`
	Tool     string `json:"tool,omitempty"`
	Category string `json:"category,omitempty"`
	Question string `json:"question,omitempty"`
	// DSH parity fields:
	ID     string `json:"id,omitempty"`      // ApprovalRequestId pairing ask with decide
	CallID string `json:"call_id,omitempty"` // CallId of the exact tool call, when the asker had one
}

// ApprovalDecidedFact records the outcome of an approval gate decision.
// Hawk fields are the primary model (turn/step/tool/category/allowed/message);
// DSH parity fields (ID, Outcome) are optional extensions.
type ApprovalDecidedFact struct {
	Turn     int    `json:"turn,omitempty"`
	Step     int    `json:"step,omitempty"`
	Tool     string `json:"tool,omitempty"`
	Category string `json:"category,omitempty"`
	Allowed  bool   `json:"allowed"`
	Message  string `json:"message,omitempty"`
	// DSH parity fields:
	ID      string `json:"id,omitempty"`      // ApprovalRequestId pairing with approval/asked
	Outcome string `json:"outcome,omitempty"` // "allowed-once" | "rejected" | "cancelled" | "unavailable"
}

// ApprovalPolicyFact records whether a category is covered by the active policy.
// DSH parity: PresetName and Source fields added alongside Hawk's Category/Covered.
type ApprovalPolicyFact struct {
	Category string `json:"category,omitempty"`
	Covered  bool   `json:"covered"`
	// DSH parity fields:
	PresetName string `json:"preset,omitempty"` // the active preset name (DSH: preset)
	Policy     string `json:"policy,omitempty"` // "ask" or "never" (DSH: ApprovalPolicy)
	Source     string `json:"source,omitempty"` // "delegation" for seeded child override
}

// AppendTurnStart appends a turn.start boundary fact.
func (l *Log) AppendTurnStart(turn int) {
	if l == nil {
		return
	}
	l.Append(TurnStart, BoundaryFact{Turn: turn})
}

// AppendTurnEnd appends a turn.end boundary fact with a DSH TurnEndReason.
// Defaults to "completed" when reason is empty (the normal completion path).
func (l *Log) AppendTurnEnd(turn int, reason string) {
	if l == nil {
		return
	}
	if reason == "" {
		reason = "completed"
	}
	l.Append(TurnEnd, TurnEndFact{Turn: turn, Reason: reason})
}

// AppendTurnEndWithError appends a turn.end with reason "error" and a
// structured LlmFailure payload, matching DSH's error turn-end variant.
func (l *Log) AppendTurnEndWithError(turn int, message, code string) {
	if l == nil {
		return
	}
	l.Append(TurnEnd, TurnEndFact{
		Turn:   turn,
		Reason: "error",
		Error:  &LlmFailure{Message: message, Code: code},
	})
}

// AppendTurnEndAborted appends a turn.end with reason "aborted" and a
// structured cause, matching DSH's aborted turn-end variant.
func (l *Log) AppendTurnEndAborted(turn int, cause string) {
	if l == nil {
		return
	}
	l.Append(TurnEnd, TurnEndFact{
		Turn:   turn,
		Reason: "aborted",
		Error:  &LlmFailure{Message: cause, Code: "aborted"},
	})
}

func (l *Log) AppendStepStart(turn, step int) {
	if l == nil {
		return
	}
	l.Append(StepStart, BoundaryFact{Turn: turn, Step: step})
}

// AppendStepEnd appends a step.end boundary fact.
func (l *Log) AppendStepEnd(turn, step int) {
	if l == nil {
		return
	}
	l.Append(StepEnd, BoundaryFact{Turn: turn, Step: step})
}

// AppendAssistantChunk records a partial assistant text stream delta.
func (l *Log) AppendAssistantChunk(turn, step int, chunk string) {
	if l == nil || chunk == "" {
		return
	}
	l.Append(AssistantChunk, ChunkFact{Turn: turn, Step: step, Chunk: chunk})
}

// AppendAssistantThinkingChunk records a partial assistant thinking/reasoning
// stream delta. The Kind field distinguishes it from text deltas so the
// chunk-packer keeps thinking and text chunks in separate runs.
func (l *Log) AppendAssistantThinkingChunk(turn, step int, chunk string) {
	if l == nil || chunk == "" {
		return
	}
	l.Append(AssistantChunk, ChunkFact{Turn: turn, Step: step, Chunk: chunk, Kind: "thinking"})
}

// AppendStreamBlockStart records the start of a response block (DSH block-start).
func (l *Log) AppendStreamBlockStart(turn, step int) {
	if l == nil {
		return
	}
	l.Append(AssistantChunk, ChunkFact{Turn: turn, Step: step, Kind: "block-start"})
}

// AppendStreamBlockEnd records the end of a response block (DSH block-end).
func (l *Log) AppendStreamBlockEnd(turn, step int) {
	if l == nil {
		return
	}
	l.Append(AssistantChunk, ChunkFact{Turn: turn, Step: step, Kind: "block-end"})
}

// AppendStreamFinish records the stream finish with a stop reason (DSH finish).
// The stop reason is carried in Chunk for packer round-trip compatibility.
func (l *Log) AppendStreamFinish(turn, step int, reason string) {
	if l == nil {
		return
	}
	l.Append(AssistantChunk, ChunkFact{Turn: turn, Step: step, Chunk: reason, Kind: "finish"})
}

// AppendToolCallDeltaFact records an incremental tool-call delta (DSH
// tool-call-delta). Name is set on the first delta (when the tool name is
// learned); Arguments carries the partial argument string.
type AppendToolCallDeltaFact struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// AppendToolCallDelta records one incremental tool-call delta.
func (l *Log) AppendToolCallDelta(turn, step int, name, arguments string) {
	if l == nil || (name == "" && arguments == "") {
		return
	}
	l.Append(AssistantChunk, ToolCallDeltaFact{
		ChunkFact: ChunkFact{Turn: turn, Step: step, Kind: "tool-call"},
		Name:      name, Arguments: arguments,
	})
}

// AppendCompaction records a context-compaction pass.
func (l *Log) AppendCompaction(f CompactionFact) {
	if l == nil {
		return
	}
	l.Append(SessionCompacted, f)
}

// AppendToolWorkflowStart records the start of one tool-execution run.
func (l *Log) AppendToolWorkflowStart(tool string) {
	if l == nil {
		return
	}
	l.Append(ToolWorkflowStart, ToolWorkflowBounds{Tool: tool})
}

// AppendToolWorkflowEnd records the end of one tool-execution run.
func (l *Log) AppendToolWorkflowEnd(tool string) {
	if l == nil {
		return
	}
	l.Append(ToolWorkflowEnd, ToolWorkflowBounds{Tool: tool})
}

// AppendLLMRetry records a stream retry decision. Marked ignorable — it is
// trace/observability data that does not affect message reconstruction.
func (l *Log) AppendLLMRetry(attempt int, reason string) {
	if l == nil {
		return
	}
	l.AppendIgnorable(LLMRetry, LLMRetryFact{Attempt: attempt, Reason: reason})
}

// AppendLLMRetryStarted records the start of a retried stream. Marked ignorable.
func (l *Log) AppendLLMRetryStarted(attempt int, reason string) {
	if l == nil {
		return
	}
	l.AppendIgnorable(LLMRetryStarted, LLMRetryFact{Attempt: attempt, Reason: reason})
}

// AppendRequestContext records the durable model route metadata for a request.
func (l *Log) AppendRequestContext(provider, model string, contextWindow int) {
	if l == nil {
		return
	}
	l.Append(RequestContext, RequestContextFact{Provider: provider, Model: model, ContextWindow: contextWindow})
}

// AppendApprovalAsked records that a gated action reached the approval gate.
func (l *Log) AppendApprovalAsked(f ApprovalAskedFact) {
	if l == nil {
		return
	}
	l.Append(ApprovalAsked, f)
}

// AppendApprovalDecided records the outcome of an approval gate decision.
func (l *Log) AppendApprovalDecided(f ApprovalDecidedFact) {
	if l == nil {
		return
	}
	l.Append(ApprovalDecided, f)
}

// AppendApprovalPolicy records whether a category is covered by the active gate policy.
func (l *Log) AppendApprovalPolicy(f ApprovalPolicyFact) {
	if l == nil {
		return
	}
	l.Append(ApprovalPolicy, f)
}

// AppendPermission appends a durable permission/approval decision.
func (l *Log) AppendPermission(f PermissionFact) {
	if l == nil {
		return
	}
	l.Append(PermissionChange, f)
}
