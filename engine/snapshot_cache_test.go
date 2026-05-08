package engine

import (
	"errors"
	"testing"
	"time"
)

func TestSnapshotCache_SetAndGet(t *testing.T) {
	cache := NewSnapshotCache(10 * time.Second)

	cache.Set("key1", "value1")

	val, ok := cache.Get("key1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if val != "value1" {
		t.Fatalf("expected 'value1', got %q", val)
	}
}

func TestSnapshotCache_GetMiss(t *testing.T) {
	cache := NewSnapshotCache(10 * time.Second)

	_, ok := cache.Get("nonexistent")
	if ok {
		t.Fatal("expected cache miss for nonexistent key")
	}
}

func TestSnapshotCache_Expiry(t *testing.T) {
	// Use a very short TTL for testing
	cache := NewSnapshotCache(1 * time.Millisecond)

	cache.Set("key1", "value1")

	// Wait for expiry
	time.Sleep(5 * time.Millisecond)

	_, ok := cache.Get("key1")
	if ok {
		t.Fatal("expected cache miss after expiry")
	}
}

func TestSnapshotCache_GetOrCompute_CacheMiss(t *testing.T) {
	cache := NewSnapshotCache(10 * time.Second)

	computeCalls := 0
	val, err := cache.GetOrCompute("key1", func() (string, error) {
		computeCalls++
		return "computed", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "computed" {
		t.Fatalf("expected 'computed', got %q", val)
	}
	if computeCalls != 1 {
		t.Fatalf("expected 1 compute call, got %d", computeCalls)
	}
}

func TestSnapshotCache_GetOrCompute_CacheHit(t *testing.T) {
	cache := NewSnapshotCache(10 * time.Second)

	cache.Set("key1", "cached")

	computeCalls := 0
	val, err := cache.GetOrCompute("key1", func() (string, error) {
		computeCalls++
		return "recomputed", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "cached" {
		t.Fatalf("expected 'cached', got %q", val)
	}
	if computeCalls != 0 {
		t.Fatalf("expected 0 compute calls (cache hit), got %d", computeCalls)
	}
}

func TestSnapshotCache_GetOrCompute_ComputeError(t *testing.T) {
	cache := NewSnapshotCache(10 * time.Second)

	expectedErr := errors.New("compute failed")
	_, err := cache.GetOrCompute("key1", func() (string, error) {
		return "", expectedErr
	})
	if err != expectedErr {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}

	// Verify the error result was NOT cached
	_, ok := cache.Get("key1")
	if ok {
		t.Fatal("error results should not be cached")
	}
}

func TestSnapshotCache_DefaultTTL(t *testing.T) {
	cache := NewSnapshotCache(0)
	if cache.ttl != DefaultSnapshotTTL {
		t.Fatalf("expected default TTL %v, got %v", DefaultSnapshotTTL, cache.ttl)
	}
}

func TestSnapshotCache_Overwrite(t *testing.T) {
	cache := NewSnapshotCache(10 * time.Second)

	cache.Set("key1", "v1")
	cache.Set("key1", "v2")

	val, ok := cache.Get("key1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if val != "v2" {
		t.Fatalf("expected 'v2', got %q", val)
	}
}
