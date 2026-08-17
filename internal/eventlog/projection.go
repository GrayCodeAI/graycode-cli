package eventlog

// ProjectMessages folds the append-only events into the model-visible message
// surface. Only UserMessage and AssistantMsg events contribute a message; the
// tool call/result facts are carried on the Message payload itself, mirroring how
// a single EyrieMessage carries its nested ToolUse/ToolResults.
//
// Projection is defined over the in-memory Event.Data values (where Data is
// already a typed Message). Consumers that load a persisted record must decode the
// raw payloads back to Message before projecting; see the owning product package.
func ProjectMessages(events []Event) []Message {
	var out []Message
	for _, ev := range events {
		switch ev.Type {
		case UserMessage, AssistantMsg:
			if m, ok := ev.Data.(Message); ok {
				out = append(out, m)
			}
		}
	}
	return out
}
