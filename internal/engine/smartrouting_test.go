package engine

import (
	"testing"

	"github.com/GrayCodeAI/hawk/internal/smartrouting"
)

func TestLifecycleServiceSmartRouting(t *testing.T) {
	sess := NewSessionWithClient(NewMockClientForTest(), "test", "test-model", "", nil, false)
	ls := sess.LifecycleSvc()
	if ls.SmartRouting() != nil {
		t.Fatal("expected nil smart routing by default")
	}
	cfg := &smartrouting.Config{Enabled: true, SimpleModel: "mini", StrongModel: "main"}
	ls.SetSmartRouting(cfg)
	if got := ls.SmartRouting(); got == nil || !got.Enabled {
		t.Fatalf("smart routing not set: %+v", got)
	}
}

func TestSmartRoutingReroutesModel(t *testing.T) {
	cfg := smartrouting.Config{Enabled: true, SimpleModel: "mini", StrongModel: "main"}
	// A trivial turn should route to the simple model.
	d := smartrouting.Route(smartrouting.Input{UserText: "ok", TurnNumber: 2}, cfg)
	if d.Model != "mini" {
		t.Fatalf("expected mini, got %q (%s)", d.Model, d.Complexity)
	}
	// A planning turn should stay on the strong model.
	d2 := smartrouting.Route(smartrouting.Input{UserText: "plan the refactor", TurnNumber: 2}, cfg)
	if d2.Model != "main" {
		t.Fatalf("expected main, got %q (%s)", d2.Model, d2.Complexity)
	}
}
