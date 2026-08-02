package gateway

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// TestDefaultGatewayRetriesOnFailure verifies the H8 fix: when the gateway
// constructor fails, defaultGateway must return nil but RETRY on the next
// call instead of being permanently nil (the sync.Once footgun that stored a
// discarded error).
func TestDefaultGatewayRetriesOnFailure(t *testing.T) {
	origFn := newGatewayFn
	defer func() { newGatewayFn = origFn }()

	var calls int
	newGatewayFn = func(ctx context.Context) (*Gateway, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("transient init failure")
		}
		return &Gateway{}, nil
	}
	// Reset shared singleton state so this test is isolated.
	defaultGatewayMu.Lock()
	defaultGatewayVal = nil
	defaultGatewayMu.Unlock()

	if g := defaultGateway(context.Background()); g != nil {
		t.Fatal("expected nil gateway after failed init")
	}

	// Second call must retry and succeed.
	g := defaultGateway(context.Background())
	if g == nil {
		t.Fatal("expected retry to succeed after transient failure (previous sync.Once would stay nil forever)")
	}
	if calls != 2 {
		t.Errorf("constructor calls = %d, want 2 (retry)", calls)
	}

	// Successful init is cached: further calls do not re-construct.
	g2 := defaultGateway(context.Background())
	if g2 != g {
		t.Error("expected successful init to be cached")
	}
	if calls != 2 {
		t.Errorf("constructor calls = %d, want still 2 (cached)", calls)
	}
}

// TestDefaultGatewayConcurrentInit verifies concurrent first-call access does
// not construct the gateway more than once.
func TestDefaultGatewayConcurrentInit(t *testing.T) {
	origFn := newGatewayFn
	defer func() { newGatewayFn = origFn }()

	newGatewayFn = func(ctx context.Context) (*Gateway, error) {
		return &Gateway{}, nil
	}
	defaultGatewayMu.Lock()
	defaultGatewayVal = nil
	defaultGatewayMu.Unlock()

	const n = 8
	var wg sync.WaitGroup
	results := make([]*Gateway, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = defaultGateway(context.Background())
		}(i)
	}
	wg.Wait()

	for i := 1; i < n; i++ {
		if results[i] != results[0] {
			t.Fatalf("concurrent init produced different gateway instances: [0]=%p [%d]=%p", results[0], i, results[i])
		}
	}
}
