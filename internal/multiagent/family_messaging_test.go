package mission

import (
	"strings"
	"testing"
	"time"
)

func TestRoleBetweenAndAllowed(t *testing.T) {
	m := NewFamilyMessenger(FamilyMessengerConfig{})
	m.Register("parent", FamilyLinks{Children: []string{"childA", "childB"}})
	m.Register("childA", FamilyLinks{Parent: "parent", Siblings: []string{"childB"}})
	m.Register("childB", FamilyLinks{Parent: "parent", Siblings: []string{"childA"}})

	if m.RoleBetween("childA", "parent") != FamilyParent {
		t.Fatalf("childA->parent role = %q", m.RoleBetween("childA", "parent"))
	}
	if m.RoleBetween("childA", "childB") != FamilySibling {
		t.Fatalf("childA->childB role = %q", m.RoleBetween("childA", "childB"))
	}
	if m.RoleBetween("parent", "childA") != FamilyChild {
		t.Fatalf("parent->childA role = %q", m.RoleBetween("parent", "childA"))
	}
	// Unrelated: not allowed.
	if m.Allowed("childA", "parentX") || m.Allowed("parent", "outsider") {
		t.Fatal("unrelated agents should not be allowed")
	}
}

func TestSendWithinFamily(t *testing.T) {
	m := NewFamilyMessenger(FamilyMessengerConfig{Capacity: 100, RefillPerSec: 100})
	m.Register("childA", FamilyLinks{Parent: "parent"})
	m.Register("parent", FamilyLinks{Children: []string{"childA"}})

	ok, role := m.Send("childA", "parent", "done")
	if !ok || role != FamilyParent {
		t.Fatalf("send = %v %q", ok, role)
	}
	msgs := m.Receive("parent")
	if len(msgs) != 1 || msgs[0].From != "childA" || msgs[0].Content != "done" {
		t.Fatalf("msgs = %+v", msgs)
	}
}

func TestSendRejectsUnrelated(t *testing.T) {
	m := NewFamilyMessenger(FamilyMessengerConfig{})
	m.Register("a", FamilyLinks{})
	m.Register("b", FamilyLinks{})
	if ok, role := m.Send("a", "b", "hi"); ok || role != "" {
		t.Fatalf("unrelated send should be rejected: %v %q", ok, role)
	}
}

func TestPendingCap(t *testing.T) {
	m := NewFamilyMessenger(FamilyMessengerConfig{MaxPendingPerAgent: 2, Capacity: 100, RefillPerSec: 100})
	m.Register("a", FamilyLinks{Children: []string{"b"}})
	for i := 0; i < 3; i++ {
		ok, _ := m.Send("a", "b", "m")
		if i < 2 && !ok {
			t.Fatalf("send %d should be accepted", i)
		}
		if i == 2 && ok {
			t.Fatal("3rd send should exceed pending cap")
		}
	}
	if m.Pending("b") != 2 {
		t.Fatalf("pending = %d, want 2", m.Pending("b"))
	}
}

func TestRateLimit(t *testing.T) {
	m := NewFamilyMessenger(FamilyMessengerConfig{Capacity: 2, RefillPerSec: 1, MaxPendingPerAgent: 100})
	m.Register("a", FamilyLinks{Children: []string{"b"}})
	// First 2 accepted (capacity 2).
	for i := 0; i < 2; i++ {
		if ok, _ := m.Send("a", "b", "x"); !ok {
			t.Fatalf("send %d should be accepted", i)
		}
	}
	// Third rejected by rate limit.
	if ok, _ := m.Send("a", "b", "x"); ok {
		t.Fatal("third send should be rate-limited")
	}
	// After refill elapses, send is accepted again.
	time.Sleep(1100 * time.Millisecond)
	if ok, _ := m.Send("a", "b", "x"); !ok {
		t.Fatal("send after refill should be accepted")
	}
}

func TestReceiveDrains(t *testing.T) {
	m := NewFamilyMessenger(FamilyMessengerConfig{Capacity: 100, RefillPerSec: 100})
	m.Register("a", FamilyLinks{Children: []string{"b"}})
	m.Send("a", "b", "1")
	m.Send("a", "b", "2")
	if m.Pending("b") != 2 {
		t.Fatalf("pending = %d", m.Pending("b"))
	}
	msgs := m.Receive("b")
	if len(msgs) != 2 || m.Pending("b") != 0 {
		t.Fatalf("receive drained wrong: %d msgs, pending=%d", len(msgs), m.Pending("b"))
	}
}

func TestMessageContentContains(t *testing.T) {
	m := NewFamilyMessenger(FamilyMessengerConfig{Capacity: 100, RefillPerSec: 100})
	m.Register("p", FamilyLinks{Children: []string{"c"}})
	m.Send("p", "c", "please run the tests")
	msgs := m.Receive("c")
	if len(msgs) != 1 || !strings.Contains(msgs[0].Content, "run the tests") {
		t.Fatalf("msgs = %+v", msgs)
	}
}
