package cmd

import (
	"testing"
	"time"
)

func TestMarkPartialDirty_SchedulesDeferredRender(t *testing.T) {
	m := newTestChatModel()
	m.lastPartialRender = time.Now()

	cmd := m.markPartialDirty()
	if cmd == nil {
		t.Fatal("expected deferred render command when within throttle interval")
	}
	if !m.partialDirty {
		t.Fatal("expected partialDirty to remain set until deferred render fires")
	}
	if !m.partialRenderPending {
		t.Fatal("expected partialRenderPending to be set")
	}
	if m.viewDirty {
		t.Fatal("viewDirty should stay false until the scheduled render fires")
	}
}

func TestStreamRenderTickMsg_FlushesDeferredPartial(t *testing.T) {
	m := newTestChatModel()
	m.partial.WriteString("hello")
	m.partialDirty = true
	m.partialRenderPending = true

	next, _ := m.Update(streamRenderTickMsg{})
	cm := requireChatModel(t, next)
	if cm.partialDirty {
		t.Fatal("expected partialDirty to be cleared after deferred render")
	}
	if cm.partialRenderPending {
		t.Fatal("expected partialRenderPending to be cleared after deferred render")
	}
	if cm.lastPartialRender.IsZero() {
		t.Fatal("expected lastPartialRender to be updated")
	}
}
