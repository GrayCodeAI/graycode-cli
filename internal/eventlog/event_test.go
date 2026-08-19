package eventlog

import "testing"

func TestKnownVocabulary(t *testing.T) {
	known := []Type{
		SessionMeta, UserMessage, AssistantMsg, ToolCall, ToolResult,
		ContextInjected, SessionCompacted,
	}
	for _, k := range known {
		if !k.Known() {
			t.Errorf("Type(%q) should be known", k)
		}
	}
	if Type("message.user").Known() == false {
		t.Error("literal UserMessage should be known")
	}
	if Type("nope").Known() {
		t.Error("unknown type reported as known")
	}
}
