package streaming

import "testing"

func TestSteeringQueue_EnqueueDrain(t *testing.T) {
	t.Parallel()
	sq := NewSteeringQueue()
	sq.Enqueue(SteeringMessage{Content: "fix this", Priority: 1})
	sq.Enqueue(SteeringMessage{Content: "also that", Priority: 2})

	msgs := sq.Drain()
	if len(msgs) != 2 {
		t.Fatalf("Drain() = %d, want 2", len(msgs))
	}
	if msgs[0].Content != "fix this" {
		t.Errorf("first = %q", msgs[0].Content)
	}

	// After drain, queue should be empty
	if sq.HasPending() {
		t.Error("HasPending should be false after drain")
	}
}

func TestSteeringQueue_HasPending(t *testing.T) {
	t.Parallel()
	sq := NewSteeringQueue()
	if sq.HasPending() {
		t.Error("new queue should not have pending")
	}
	sq.Enqueue(SteeringMessage{Content: "x"})
	if !sq.HasPending() {
		t.Error("should have pending after enqueue")
	}
}

func TestSteeringQueue_Clear(t *testing.T) {
	t.Parallel()
	sq := NewSteeringQueue()
	sq.Enqueue(SteeringMessage{Content: "a"})
	sq.Enqueue(SteeringMessage{Content: "b"})
	sq.Clear()
	if sq.HasPending() {
		t.Error("should be empty after Clear")
	}
}

func TestSteeringQueue_Notify(t *testing.T) {
	t.Parallel()
	sq := NewSteeringQueue()
	ch := sq.Notify()
	if ch == nil {
		t.Error("Notify should return non-nil channel")
	}
}

func TestSteeringQueue_DrainEmpty(t *testing.T) {
	t.Parallel()
	sq := NewSteeringQueue()
	msgs := sq.Drain()
	if len(msgs) != 0 {
		t.Errorf("Drain empty = %d, want 0", len(msgs))
	}
}
