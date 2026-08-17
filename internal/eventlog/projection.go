package eventlog

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
