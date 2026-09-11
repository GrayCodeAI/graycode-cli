package engine

import (
	"testing"

	"github.com/GrayCodeAI/hawk/internal/conversationarc"
)

func TestSessionArcAccessors(t *testing.T) {
	sess := NewSessionWithClient(NewMockClientForTest(), "test", "test-model", "", nil, false)
	if sess.Arc() != nil {
		t.Fatal("expected nil arc by default")
	}
	a := conversationarc.New()
	a.AddGoal("implement X")
	sess.SetArc(a)
	if got := sess.Arc(); got == nil || len(got.Goals) != 1 {
		t.Fatalf("arc not set: %+v", got)
	}
}
