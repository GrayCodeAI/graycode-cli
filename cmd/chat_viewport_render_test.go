package cmd

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
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

func TestUpdateViewportContent_RewrapsAfterScrollbarGutterAppears(t *testing.T) {
	content, fullWidth, narrowWidth, fullWidthLines, narrowWidthLines := findWidthSensitiveViewportScenario(t)
	viewportHeight := fullWidthLines - 1
	if viewportHeight < 1 {
		viewportHeight = 1
	}

	m := &chatModel{
		messages: []displayMsg{{role: "system", content: content}},
		viewport: viewport.New(fullWidth, viewportHeight),
		width:    fullWidth,
	}
	m.viewDirty = true

	m.updateViewportContent()

	if !m.chatHasOverflow() {
		t.Fatalf(
			"expected overflow after re-wrapping for scrollbar width (viewportHeight=%d fullWidthLines=%d narrowWidthLines=%d contentLines=%d viewportWidth=%d)",
			viewportHeight,
			fullWidthLines,
			narrowWidthLines,
			m.contentLines,
			m.viewport.Width,
		)
	}
	if !m.chatScrollbarVisible() {
		t.Fatal("expected scrollbar to become visible after re-wrap")
	}
	if m.viewport.Width != narrowWidth {
		t.Fatalf("expected viewport width %d with scrollbar gutter, got %d", narrowWidth, m.viewport.Width)
	}
	if m.contentLines != narrowWidthLines {
		t.Fatalf("expected contentLines %d after narrow re-wrap, got %d", narrowWidthLines, m.contentLines)
	}
}

func findWidthSensitiveViewportScenario(t *testing.T) (string, int, int, int, int) {
	t.Helper()

	patterns := []string{
		"alpha ",
		"alpha beta ",
		"alpha beta gamma ",
		"one two three four five ",
		"short mediumlength extralongword ",
		"supercalifragilisticexpialidocious",
		"0123456789012345678901234567890123456789",
	}

	for fullWidth := 21; fullWidth <= 60; fullWidth++ {
		narrowWidth := fullWidth - 1
		for _, pattern := range patterns {
			for n := 4; n < 160; n++ {
				content := strings.TrimSpace(strings.Repeat(pattern, n))

				fullModel := &chatModel{messages: []displayMsg{{role: "system", content: content}}}
				fullLines := renderedLineCount(fullModel.assembleViewportContent(fullWidth))

				narrowModel := &chatModel{messages: []displayMsg{{role: "system", content: content}}}
				narrowLines := renderedLineCount(narrowModel.assembleViewportContent(narrowWidth))

				if narrowLines > fullLines {
					return content, fullWidth, narrowWidth, fullLines, narrowLines
				}
			}
		}
	}

	t.Fatal("failed to find width-sensitive content for viewport render test")
	return "", 0, 0, 0, 0
}
