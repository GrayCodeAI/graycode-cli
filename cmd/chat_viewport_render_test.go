package cmd

import (
	"strings"
	"testing"
)

func TestAssembleViewportContent_IncrementalMatchesFullRebuild(t *testing.T) {
	msgs := []displayMsg{
		{role: "user", content: "hello"},
		{role: "assistant", content: "hi **there**"},
		{role: "tool_use", content: "Read"},
		{role: "tool_result", content: "[Read] file contents"},
	}

	m := &chatModel{messages: msgs, width: 80}
	got := m.assembleViewportContent(80)

	fresh := &chatModel{messages: msgs, width: 80}
	want := fresh.assembleViewportContent(80)
	if got != want {
		t.Fatal("initial assemble should be deterministic")
	}

	// Append a message — should match a full rebuild.
	m.messages = append(m.messages, displayMsg{role: "user", content: "follow up"})
	incremental := m.assembleViewportContent(80)

	fresh = &chatModel{messages: m.messages, width: 80}
	full := fresh.assembleViewportContent(80)
	if incremental != full {
		t.Fatal("incremental append should match full rebuild")
	}
	if m.vpRenderedMsgs != len(m.messages) {
		t.Fatalf("expected all messages cached, got %d want %d", m.vpRenderedMsgs, len(m.messages))
	}
}

func TestAssembleViewportContent_ThinkingInPlaceUpdate(t *testing.T) {
	m := &chatModel{
		messages: []displayMsg{{role: "thinking", content: "part"}},
		width:    80,
	}
	first := m.assembleViewportContent(80)

	m.messages[0].content = "part two"
	second := m.assembleViewportContent(80)
	if first == second {
		t.Fatal("thinking growth should change rendered output")
	}

	fresh := &chatModel{messages: m.messages, width: 80}
	full := fresh.assembleViewportContent(80)
	if second != full {
		t.Fatal("in-place thinking update should match full rebuild")
	}
}

func TestAssembleViewportContent_StreamTailNotCached(t *testing.T) {
	m := &chatModel{
		messages: []displayMsg{{role: "user", content: "go"}},
		waiting:  true,
		partial:  &strings.Builder{},
		width:    80,
	}
	m.partial.WriteString("streaming")

	stable := m.assembleViewportContent(80)
	stablePrefix := m.vpStableContent

	m.partial.WriteString(" more")
	withMore := m.assembleViewportContent(80)

	if m.vpStableContent != stablePrefix {
		t.Fatal("stream tail changes should not mutate stable cache")
	}
	if withMore == stable {
		t.Fatal("stream tail growth should change assembled output")
	}
	if !strings.Contains(withMore, "streaming more") {
		t.Fatal("expected updated partial in output")
	}
}

func TestAssembleViewportContent_WidthChangeRebuilds(t *testing.T) {
	m := &chatModel{
		messages: []displayMsg{{role: "user", content: "hello world"}},
		width:    80,
	}
	_ = m.assembleViewportContent(80)
	if m.vpRenderWidth != 80 {
		t.Fatalf("expected width 80 cached, got %d", m.vpRenderWidth)
	}

	_ = m.assembleViewportContent(60)
	if m.vpRenderWidth != 60 {
		t.Fatalf("expected width 60 after resize, got %d", m.vpRenderWidth)
	}
}
