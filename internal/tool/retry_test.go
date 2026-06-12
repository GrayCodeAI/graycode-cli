package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

type flakyTool struct {
	failuresLeft int
	calls        int
	delay        time.Duration
}

func (f *flakyTool) Name() string                       { return "flaky" }
func (f *flakyTool) Description() string                { return "flaky tool" }
func (f *flakyTool) Parameters() map[string]interface{} { return nil }
func (f *flakyTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	f.calls++
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.failuresLeft > 0 {
		f.failuresLeft--
		return "", NewTransientError(fmt.Errorf("transient failure #%d", f.failuresLeft))
	}
	return "ok", nil
}

func TestRetryExecutor_RecoversOnTransient(t *testing.T) {
	ft := &flakyTool{failuresLeft: 2}
	policy := RetryPolicy{MaxRetries: 3, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}
	out, err := RetryExecutor(context.Background(), ft, nil, policy)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if out != "ok" {
		t.Fatalf("expected ok, got %q", out)
	}
	if ft.calls != 3 {
		t.Fatalf("expected 3 calls (2 fail + 1 ok), got %d", ft.calls)
	}
}

func TestRetryExecutor_GivesUpAfterMaxRetries(t *testing.T) {
	ft := &flakyTool{failuresLeft: 100}
	policy := RetryPolicy{MaxRetries: 2, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}
	out, err := RetryExecutor(context.Background(), ft, nil, policy)
	if err == nil {
		t.Fatalf("expected error after giving up, got %q", out)
	}
	if ft.calls != 3 {
		t.Fatalf("expected 3 calls (initial + 2 retries), got %d", ft.calls)
	}
	if !IsTransientError(err) {
		t.Fatalf("final error should still be transient: %v", err)
	}
}

func TestRetryExecutor_NonTransientErrorNotRetried(t *testing.T) {
	ft := &nonTransientTool{}
	policy := RetryPolicy{MaxRetries: 5, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}
	_, err := RetryExecutor(context.Background(), ft, nil, policy)
	if err == nil {
		t.Fatal("expected error")
	}
	if ft.calls != 1 {
		t.Fatalf("non-transient should not be retried; got %d calls", ft.calls)
	}
	if !errors.Is(err, errNonTransient) {
		t.Fatalf("expected wrapped non-transient err, got %v", err)
	}
}

type nonTransientTool struct{ calls int }

var errNonTransient = errors.New("permanent failure")

func (f *nonTransientTool) Name() string                       { return "perm" }
func (f *nonTransientTool) Description() string                { return "perm" }
func (f *nonTransientTool) Parameters() map[string]interface{} { return nil }
func (f *nonTransientTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	f.calls++
	return "", errNonTransient
}

func TestRetryExecutor_RespectsContextCancel(t *testing.T) {
	ft := &flakyTool{failuresLeft: 10, delay: 200 * time.Millisecond}
	policy := RetryPolicy{MaxRetries: 10, BaseDelay: 100 * time.Millisecond, MaxDelay: 500 * time.Millisecond}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := RetryExecutor(ctx, ft, nil, policy)
	if err == nil {
		t.Fatal("expected context-cancelled error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if ft.calls > 3 {
		t.Fatalf("expected to bail after a few attempts, got %d calls", ft.calls)
	}
}

func TestIsTransientFileErr(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("resource temporarily unavailable"), true},
		{fmt.Errorf("text file busy"), true},
		{fmt.Errorf("EBUSY: resource busy"), true},
		{fmt.Errorf("connection reset by peer"), true},
		{fmt.Errorf("i/o timeout"), true},
		{fmt.Errorf("no such file or directory"), false},
		{fmt.Errorf("permission denied"), false},
	}
	for _, c := range cases {
		if got := IsTransientFileErr(c.err); got != c.want {
			t.Errorf("IsTransientFileErr(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}
