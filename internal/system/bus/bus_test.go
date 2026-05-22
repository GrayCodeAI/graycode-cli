package bus

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestChannelBusPublishListen(t *testing.T) {
	prod, cons := NewChannelBus(8)

	received := make(chan []byte, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = cons.Listen(ctx, func(_ context.Context, data []byte) error {
			received <- data
			return nil
		})
	}()

	if err := prod.Publish(ctx, []byte("hello")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case msg := <-received:
		if string(msg) != "hello" {
			t.Errorf("received %q, want %q", msg, "hello")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestChannelBusCopiesData(t *testing.T) {
	prod, cons := NewChannelBus(8)
	ctx := context.Background()

	received := make(chan []byte, 1)
	go func() {
		_ = cons.Listen(ctx, func(_ context.Context, data []byte) error {
			received <- data
			return nil
		})
	}()

	orig := []byte("original")
	if err := prod.Publish(ctx, orig); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Mutate original after publish.
	orig[0] = 'X'

	select {
	case msg := <-received:
		if msg[0] == 'X' {
			t.Error("Publish should copy data, but original mutation was visible")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestChannelConsumerContextCancel(t *testing.T) {
	_, cons := NewChannelBus(8)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- cons.Listen(ctx, func(_ context.Context, data []byte) error {
			return nil
		})
	}()

	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Listen returned %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Listen to return")
	}
}

func TestChannelProducerClose(t *testing.T) {
	prod, cons := NewChannelBus(8)
	ctx := context.Background()

	done := make(chan error, 1)
	go func() {
		done <- cons.Listen(ctx, func(_ context.Context, data []byte) error {
			return nil
		})
	}()

	if err := prod.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Listen returned %v after Close, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Listen to return after Close")
	}
}

func TestChannelConsumerCloseNoop(t *testing.T) {
	_, cons := NewChannelBus(8)
	if err := cons.Close(); err != nil {
		t.Errorf("Consumer.Close: %v", err)
	}
}

func TestChannelBusHandlerError(t *testing.T) {
	prod, cons := NewChannelBus(8)
	ctx := context.Background()

	var count atomic.Int32
	go func() {
		_ = cons.Listen(ctx, func(_ context.Context, data []byte) error {
			count.Add(1)
			return context.Canceled // non-nil error
		})
	}()

	prod.Publish(ctx, []byte("msg1"))
	prod.Publish(ctx, []byte("msg2"))
	prod.Close()

	// Both messages should be received despite handler errors.
	time.Sleep(50 * time.Millisecond)
	if count.Load() != 2 {
		t.Errorf("handler called %d times, want 2", count.Load())
	}
}

func TestNewChannelBusDefaultSize(t *testing.T) {
	prod, cons := NewChannelBus(0)
	if prod == nil || cons == nil {
		t.Fatal("NewChannelBus(0) returned nil")
	}
	// Should still work.
	ctx := context.Background()
	received := make(chan []byte, 1)
	go func() {
		_ = cons.Listen(ctx, func(_ context.Context, data []byte) error {
			received <- data
			return nil
		})
	}()
	prod.Publish(ctx, []byte("test"))
	prod.Close()
	select {
	case msg := <-received:
		if string(msg) != "test" {
			t.Errorf("received %q, want test", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}
