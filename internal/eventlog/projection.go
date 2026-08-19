package eventlog

import "time"

// ProjectMessages folds the append-only events into the model-visible message
// surface. This ports DeepSeek Harness's deriveMessages() surface semantics:
// system prompts and injected context become system messages; compaction
// summaries replace pruned content as system messages; user/assistant messages
// and tool facts are projected as-is; empty-content assistant messages are
// skipped (they exist only to host usage data) — matching DSH's
// deriveEventMessage which returns null for content-less assistant/message.
//
// Projection is defined over the in-memory Event.Data values (where Data is
// already a typed Message). Consumers that load a persisted record must decode the
// raw payloads back to Message before projecting; see the owning product package.
func ProjectMessages(events []Event) []Message {
	var out []Message
	inCompaction := false
	for _, ev := range events {
		switch ev.Type {
		case UserMessage:
			if m, ok := ev.Data.(Message); ok {
				out = append(out, m)
			}
		case AssistantMsg:
			if m, ok := ev.Data.(Message); ok {
				// Skip empty-content assistant messages: they exist only
				// to host usage/finish data and must not inject a content-less
				// assistant turn into the provider transcript. (DSH seam:
				// deriveEventMessage returns null for these.)
				if m.Content == "" && m.Thinking == "" && len(m.ToolUse) == 0 && len(m.Images) == 0 && len(m.ContentParts) == 0 {
					continue
				}
				out = append(out, m)
			}
		case ToolResult:
			if m, ok := ev.Data.(Message); ok {
				out = append(out, m)
			}
		case RequestHeader:
			if f, ok := ev.Data.(RequestHeaderFact); ok && f.System != "" {
				out = append(out, Message{
					Role:    "system",
					Content: f.System,
				})
			}
		case ContextInjected:
			if f, ok := ev.Data.(ContextInjectedFact); ok && f.Content != "" {
				out = append(out, Message{
					Role:    "system",
					Content: f.Content,
				})
			}
		case CompactionStart:
			inCompaction = true
		case CompactionPrune:
			if f, ok := ev.Data.(CompactionPruneFact); ok && f.Messages > 0 {
				// Drop the last f.Messages model-visible entries that were pruned.
				drop := f.Messages
				if drop > len(out) {
					drop = len(out)
				}
				out = out[:len(out)-drop]
			}
		case CompactionSummary:
			if f, ok := ev.Data.(CompactionSummaryFact); ok && f.Summary != "" {
				out = append(out, Message{
					Role:    "system",
					Content: f.Summary,
				})
			}
		case CompactionEnd:
			inCompaction = false
		case SessionEndSeed:
			// Seed boundary marker — no surface effect, but consumers can
			// use it to distinguish seeded vs live events.
		default:
			// All other event types (tool calls, turn/step boundaries,
			// approval facts, etc.) are durable record but not model-visible.
			_ = inCompaction
		}
	}
	return out
}

// SessionStatsProjection tracks whole-log conversation figures by folding over
// the event spine, matching DSH's session-stats projection. Counts and wall
// times fold from the complete durable log; every field is 0 until its first
// contributing event lands.
type SessionStatsProjection struct {
	// Turns carrying at least one closed step (step/end).
	Turns int `json:"turns"`
	// Closed steps (step/end events) — all reasons.
	Steps int `json:"steps"`
	// Summed model wall time in milliseconds (step/start → assistant/message).
	LLMMs int64 `json:"llm_ms"`
	// Summed tool wall time in milliseconds (tool/call → tool/result).
	ToolMs int64 `json:"tool_ms"`
	// Summed first-token latency in milliseconds (step/start → first chunk).
	TTFPMs int64 `json:"ttfp_ms"`
	// Steps carrying a recorded first token.
	TTFPSteps int `json:"ttfp_steps"`
	// Summed decode wall time in milliseconds (first chunk → assistant/message).
	DecodeMs int64 `json:"decode_ms"`
	// Summed provider output tokens over decode-timed steps.
	DecodeTokens int `json:"decode_tokens"`
}

// ProjectSessionStats folds the append-only events into a SessionStatsProjection,
// matching DSH's session-stats drive unit. Step start/end pairs bracket tool
// calls and assistant chunks; wall times are derived from event At timestamps.
func ProjectSessionStats(events []Event) SessionStatsProjection {
	var stats SessionStatsProjection
	var stepStart time.Time
	_ = stepStart
	var turnActive bool
	var pendingStepStart bool

	for _, ev := range events {
		switch ev.Type {
		case TurnStart:
			turnActive = true
		case TurnEnd:
			turnActive = false
		case StepStart:
			stepStart = ev.At
			pendingStepStart = true
		case StepEnd:
			if stepStart != (time.Time{}) {
				stats.Steps++
				if turnActive {
					stats.Turns++
					turnActive = false
				}
				stepStart = time.Time{}
			}
			pendingStepStart = false
		case AssistantMsg:
			// End of decode window for a step that produced a message.
			if pendingStepStart && stepStart != (time.Time{}) {
				decodeDur := ev.At.Sub(stepStart)
				if decodeDur > 0 {
					stats.DecodeMs += decodeDur.Milliseconds()
				}
			}
			// End of LLM response window for this step.
			if stepStart != (time.Time{}) {
				llmDur := ev.At.Sub(stepStart)
				if llmDur > 0 {
					stats.LLMMs += llmDur.Milliseconds()
				}
			}
			pendingStepStart = false
		case ToolCall:
			// Start of tool wall time — tracked via ToolResult pairing below.
		case ToolResult:
			// End of tool wall time — best-effort: span from step start.
			if stepStart != (time.Time{}) && ev.At.After(stepStart) {
				toolDur := ev.At.Sub(stepStart)
				if toolDur > 0 {
					stats.ToolMs += toolDur.Milliseconds()
				}
			}
		case AssistantChunk:
			// First non-empty content chunk after step start → TTFT.
			if pendingStepStart {
				if c, ok := ev.Data.(ChunkFact); ok && c.Chunk != "" {
					if stepStart != (time.Time{}) {
						ttft := ev.At.Sub(stepStart)
						if ttft > 0 {
							stats.TTFPMs += ttft.Milliseconds()
						}
						stats.TTFPSteps++
					}
					pendingStepStart = false
				}
			}
		}
	}

	return stats
}

// PresetOption is the select-option shape a presentation layer advertises
// for one permission preset (DSH PresetOption parity).
type PresetOption struct {
	Value       string `json:"value"` // stable option value: table key, or "custom"
	Name        string `json:"name"`  // display label
	Description string `json:"description,omitempty"`
}

// PermissionSelect is the whole permissions projection value: every switchable
// preset in table order plus the effective current value (DSH PermissionSelect).
type PermissionSelect struct {
	Options      []PresetOption `json:"options"`       // switchable presets, plus "custom" when current
	CurrentValue string         `json:"current_value"` // effective: table key, or "custom"
}

// ProjectPermissions folds permission/preset, sandbox/mode, and approval/policy
// events into a PermissionSelect view, matching DSH's permissions projection
// which folds from those three event types over composition defaults.
// Key absence means no permission service is composed — clients hide the control.
func ProjectPermissions(events []Event) PermissionSelect {
	var ps PermissionSelect
	currentPolicy := "ask" // DSH default: APPROVAL_POLICIES[0]

	for _, ev := range events {
		switch ev.Type {
		case PermissionPreset:
			if f, ok := ev.Data.(PermissionPresetFact); ok && f.PresetName != "" {
				// Track the preset name as the current value.
				ps.CurrentValue = f.PresetName
			}
		case SandboxMode:
			// DSH: sandbox/mode influences the effective permission surface.
			if f, ok := ev.Data.(SandboxModeFact); ok {
				if f.Mode != "" {
					ps.CurrentValue = f.Mode
				}
			}
		case ApprovalPolicy:
			if f, ok := ev.Data.(ApprovalPolicyFact); ok {
				if f.Policy != "" {
					currentPolicy = f.Policy
				}
			}
		}
	}

	// If no specific override landed, the effective value is the policy.
	if ps.CurrentValue == "" {
		ps.CurrentValue = currentPolicy
	}

	return ps
}
