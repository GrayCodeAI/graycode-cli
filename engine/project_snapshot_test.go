package engine

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestSnapshotCacheHit(t *testing.T) {
	// Use a real directory so ls works; use os.TempDir as a safe target.
	dir := t.TempDir()

	// Create a file so ls has output.
	if err := os.WriteFile(dir+"/hello.txt", []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	cache := NewProjectSnapshotCache(dir, 5*time.Second)

	ctx := context.Background()
	snap1 := cache.Get(ctx)
	if snap1 == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if snap1.DirectoryListing == "" {
		t.Fatal("expected non-empty directory listing")
	}

	// Second call within TTL should return the same pointer (cached).
	snap2 := cache.Get(ctx)
	if snap2.GatheredAt != snap1.GatheredAt {
		t.Fatal("expected cache hit: GatheredAt should match")
	}
}

func TestSnapshotCacheExpired(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/file.txt", []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	// Use a very short TTL to force expiry.
	cache := NewProjectSnapshotCache(dir, 1*time.Millisecond)

	ctx := context.Background()
	snap1 := cache.Get(ctx)
	if snap1 == nil {
		t.Fatal("expected non-nil snapshot")
	}

	// Wait for TTL to expire.
	time.Sleep(5 * time.Millisecond)

	snap2 := cache.Get(ctx)
	if snap2 == nil {
		t.Fatal("expected non-nil snapshot after refresh")
	}
	if !snap2.GatheredAt.After(snap1.GatheredAt) {
		t.Fatalf("expected refreshed snapshot with later GatheredAt: snap1=%v snap2=%v",
			snap1.GatheredAt, snap2.GatheredAt)
	}
}

func TestForExploreMode(t *testing.T) {
	snap := &ProjectSnapshot{
		DirectoryListing: "cmd\nengine\ntool",
		RecentCommits:    "abc1234 initial commit",
		GitStatus:        "M engine/agent.go",
		GatheredAt:       time.Now(),
	}

	explore := snap.ForExploreMode()
	if explore == nil {
		t.Fatal("expected non-nil explore snapshot")
	}
	if explore.DirectoryListing != snap.DirectoryListing {
		t.Fatalf("expected DirectoryListing preserved, got %q", explore.DirectoryListing)
	}
	if explore.RecentCommits != snap.RecentCommits {
		t.Fatalf("expected RecentCommits preserved, got %q", explore.RecentCommits)
	}
	if explore.GitStatus != "" {
		t.Fatalf("expected GitStatus stripped for explore mode, got %q", explore.GitStatus)
	}
	if explore.GatheredAt != snap.GatheredAt {
		t.Fatal("expected GatheredAt preserved")
	}
}

func TestForExploreModeNil(t *testing.T) {
	var snap *ProjectSnapshot
	result := snap.ForExploreMode()
	if result != nil {
		t.Fatal("expected nil result from nil snapshot")
	}
}

func TestSnapshotInvalidate(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/a.txt", []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}

	cache := NewProjectSnapshotCache(dir, 1*time.Hour) // Long TTL

	ctx := context.Background()
	snap1 := cache.Get(ctx)
	if snap1 == nil {
		t.Fatal("expected non-nil snapshot")
	}

	// Invalidate and verify next call re-gathers.
	cache.Invalidate()

	// Small sleep so GatheredAt differs.
	time.Sleep(1 * time.Millisecond)

	snap2 := cache.Get(ctx)
	if snap2 == nil {
		t.Fatal("expected non-nil snapshot after invalidate")
	}
	if !snap2.GatheredAt.After(snap1.GatheredAt) {
		t.Fatal("expected fresh snapshot after invalidate")
	}
}

func TestSnapshotDefaultTTL(t *testing.T) {
	cache := NewProjectSnapshotCache("/tmp", 0)
	if cache.ttl != DefaultProjectSnapshotTTL {
		t.Fatalf("expected default TTL %v, got %v", DefaultProjectSnapshotTTL, cache.ttl)
	}
}
