package hooks

import (
	"context"
	"testing"
)

func TestRegistryRegisterAndExecute(t *testing.T) {
	r := NewRegistry()
	called := false
	r.Register(Hook{
		Name:  "test",
		Event: EventPreQuery,
		Fn: func(ctx context.Context, data map[string]interface{}) error {
			called = true
			return nil
		},
	})

	if err := r.Execute(context.Background(), EventPreQuery, nil); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("hook was not called")
	}
}

func TestRegistryPriority(t *testing.T) {
	r := NewRegistry()
	var order []int
	r.Register(Hook{
		Name:     "second",
		Event:    EventPreQuery,
		Priority: 10,
		Fn: func(ctx context.Context, data map[string]interface{}) error {
			order = append(order, 2)
			return nil
		},
	})
	r.Register(Hook{
		Name:     "first",
		Event:    EventPreQuery,
		Priority: 1,
		Fn: func(ctx context.Context, data map[string]interface{}) error {
			order = append(order, 1)
			return nil
		},
	})

	if err := r.Execute(context.Background(), EventPreQuery, nil); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Fatalf("unexpected order: %v", order)
	}
}

func TestGlobalRegistry(t *testing.T) {
	called := false
	Register(Hook{
		Name:  "global_test",
		Event: EventSessionStart,
		Fn: func(ctx context.Context, data map[string]interface{}) error {
			called = true
			return nil
		},
	})
	if err := Execute(context.Background(), EventSessionStart, nil); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("global hook was not called")
	}
}
