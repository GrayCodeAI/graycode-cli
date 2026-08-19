package engine

import (
	"errors"
	"testing"
)

func TestEventBusWaterfallOrderAndShortCircuit(t *testing.T) {
	eb := NewEventBus()
	var order []string
	eb.Waterfall(EventPermission, func(ev Event, next func(Event) (Event, error)) (Event, error) {
		order = append(order, "a")
		return next(Event{Type: ev.Type, Payload: "a-set"})
	})
	eb.Waterfall(EventPermission, func(ev Event, next func(Event) (Event, error)) (Event, error) {
		order = append(order, "b")
		// Short-circuit: do not call next.
		return Event{Type: ev.Type, Payload: "stopped"}, nil
	})
	eb.Waterfall(EventPermission, func(ev Event, next func(Event) (Event, error)) (Event, error) {
		order = append(order, "c")
		return next(ev)
	})

	out, err := eb.RunWaterfall(EventPermission, Event{Type: EventPermission})
	if err != nil {
		t.Fatalf("RunWaterfall: %v", err)
	}
	if out.Payload != "stopped" {
		t.Fatalf("out payload = %v, want stopped", out.Payload)
	}
	if len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Fatalf("order = %v, want [a b]", order)
	}
}

func TestEventBusWaterfallErrorStops(t *testing.T) {
	eb := NewEventBus()
	sentinel := errors.New("boom")
	eb.Waterfall(EventToolStarted, func(ev Event, next func(Event) (Event, error)) (Event, error) {
		return ev, sentinel
	})
	called := false
	eb.Waterfall(EventToolStarted, func(ev Event, next func(Event) (Event, error)) (Event, error) {
		called = true
		return next(ev)
	})

	if _, err := eb.RunWaterfall(EventToolStarted, Event{Type: EventToolStarted}); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
	if called {
		t.Fatal("handler after short-circuit should not run")
	}
}

func TestEventBusWaterfallDisposer(t *testing.T) {
	eb := NewEventBus()
	raced := false
	dispose := eb.Waterfall(EventToolCompleted, func(ev Event, next func(Event) (Event, error)) (Event, error) {
		raced = true
		return next(ev)
	})
	dispose()
	out, err := eb.RunWaterfall(EventToolCompleted, Event{Type: EventToolCompleted, Payload: "keep"})
	if err != nil {
		t.Fatalf("RunWaterfall: %v", err)
	}
	if out.Payload != "keep" {
		t.Fatalf("payload = %v, want keep", out.Payload)
	}
	if raced {
		t.Fatal("disposed handler ran")
	}
}

func TestEventBusRunWaterfallNoHandlers(t *testing.T) {
	eb := NewEventBus()
	in := Event{Type: EventError, Payload: "unchanged"}
	out, err := eb.RunWaterfall(EventError, in)
	if err != nil {
		t.Fatalf("RunWaterfall: %v", err)
	}
	if out.Payload != "unchanged" {
		t.Fatalf("payload = %v, want unchanged", out.Payload)
	}
}
