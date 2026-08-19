package eventlog

import (
	"encoding/json"
	"testing"
	"time"
)

// TestLogAssignsMonotonicSeq verifies append order is reconstructible from Seq.
func TestLogAssignsMonotonicSeq(t *testing.T) {
	l := New(nil)
	for i := 1; i <= 5; i++ {
		l.Append(UserMessage, Message{Content: "m"})
	}
	events := l.Snapshot()
	if len(events) != 5 {
		t.Fatalf("want 5 events, got %d", len(events))
	}
	for i, ev := range events {
		want := uint64(i + 1)
		if ev.Seq != want {
			t.Errorf("event %d: seq = %d, want %d", i, ev.Seq, want)
		}
		if ev.At.IsZero() {
			t.Errorf("event %d: zero timestamp", i)
		}
	}
	if err := Validate(events); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

// TestLogSnapshotIsImmutable verifies that reading a snapshot is not affected by
// later appends.
func TestLogSnapshotIsImmutable(t *testing.T) {
	l := New(nil)
	l.Append(UserMessage, Message{Content: "first"})
	before := l.Snapshot()
	l.Append(UserMessage, Message{Content: "second"})
	if len(before) != 1 {
		t.Fatalf("snapshot mutated after append: %d events", len(before))
	}
	if got := l.Len(); got != 2 {
		t.Fatalf("Len = %d, want 2", got)
	}
}

// TestLogListenerObservesAppends verifies the observer sees events in order.
func TestLogListenerObservesAppends(t *testing.T) {
	var got []Type
	l := New(func(ev Event) { got = append(got, ev.Type) })
	l.Append(SessionMeta, Meta{FormatVersion: 1})
	l.Append(ToolResult, ToolResultPayload{Content: "ok"})
	want := []Type{SessionMeta, ToolResult}
	if len(got) != len(want) {
		t.Fatalf("listener saw %d events, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("listener event %d = %s, want %s", i, got[i], want[i])
		}
	}
}

// TestLogRejectsUnknownType verifies append fails loudly on an unknown kind.
func TestLogRejectsUnknownType(t *testing.T) {
	l := New(nil)
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on unknown type, got none")
		}
	}()
	l.Append(Type("message.magic"), nil)
}

// TestOfTypeFilters verifies the by-type index.
func TestOfTypeFilters(t *testing.T) {
	l := New(nil)
	l.Append(UserMessage, Message{Content: "u1"})
	l.Append(AssistantMsg, Message{Content: "a1"})
	l.Append(UserMessage, Message{Content: "u2"})
	users := l.OfType(UserMessage)
	if len(users) != 2 {
		t.Fatalf("OfType(UserMessage) = %d, want 2", len(users))
	}
	if users[0].Seq != 1 || users[1].Seq != 3 {
		t.Fatalf("unexpected user seqs: %d, %d", users[0].Seq, users[1].Seq)
	}
}

// TestValidateRejectsUnknownType verifies a wire record with an unknown kind fails.
func TestValidateRejectsUnknownType(t *testing.T) {
	events := []Event{{Type: Type("message.magic"), Seq: 1, At: time.Now()}}
	if err := Validate(events); err == nil {
		t.Fatal("expected ErrUnknownType, got nil")
	}
}

// TestValidateRejectsNonMonotonicSeq verifies sequence ordering is enforced.
func TestValidateRejectsNonMonotonicSeq(t *testing.T) {
	events := []Event{
		{Type: UserMessage, Seq: 2, At: time.Now()},
		{Type: UserMessage, Seq: 1, At: time.Now()},
	}
	if err := Validate(events); err == nil {
		t.Fatal("expected monotonic violation, got nil")
	}
}

// TestWireRoundTrip verifies marshal/decode is byte-stable for the durable shape.
func TestWireRoundTrip(t *testing.T) {
	l := New(nil)
	l.Append(SessionMeta, Meta{ID: "s1", FormatVersion: 1})
	l.Append(UserMessage, Message{Content: "hi"})

	wire, err := MarshalWire(l.Snapshot())
	if err != nil {
		t.Fatalf("MarshalWire: %v", err)
	}
	// SessionMeta data must survive re-marshal as an object.
	var meta Meta
	raw := wire[0].Data
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("decode meta payload: %v", err)
	}
	if meta.ID != "s1" || meta.FormatVersion != 1 {
		t.Fatalf("meta decoded incorrectly: %+v", meta)
	}

	decoded, err := DecodeWire(wire)
	if err != nil {
		t.Fatalf("DecodeWire: %v", err)
	}
	if len(decoded) != 2 {
		t.Fatalf("decoded %d events, want 2", len(decoded))
	}
	if got, ok := decoded[1].Data.(Message); !ok || got.Content != "hi" {
		t.Fatalf("decoded message payload not reconstructed: %#v", decoded[1].Data)
	}
}
