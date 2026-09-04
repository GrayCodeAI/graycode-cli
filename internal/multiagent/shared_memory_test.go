package mission

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewSharedMemory(t *testing.T) {
	sm := NewSharedMemory()
	if sm == nil {
		t.Fatal("NewSharedMemory returned nil")
	}
	if sm.Entries == nil {
		t.Fatal("Entries map not initialized")
	}
	if sm.Namespaces == nil {
		t.Fatal("Namespaces map not initialized")
	}
}

func TestSetAndGet(t *testing.T) {
	sm := NewSharedMemory()

	sm.Set("auth.token", "abc123", "security-agent")

	val, ok := sm.Get("auth.token")
	if !ok {
		t.Fatal("expected key to exist")
	}
	if val != "abc123" {
		t.Fatalf("expected %q, got %q", "abc123", val)
	}

	// Non-existent key.
	_, ok = sm.Get("nonexistent")
	if ok {
		t.Fatal("expected key to not exist")
	}
}

func TestSetVersionIncrement(t *testing.T) {
	sm := NewSharedMemory()

	sm.Set("counter", 1, "agent-a")
	sm.Set("counter", 2, "agent-a")
	sm.Set("counter", 3, "agent-b")

	sm.mu.RLock()
	entry := sm.Entries["counter"]
	sm.mu.RUnlock()

	if entry.Version != 3 {
		t.Fatalf("expected version 3, got %d", entry.Version)
	}
	if entry.Owner != "agent-b" {
		t.Fatalf("expected owner %q, got %q", "agent-b", entry.Owner)
	}
}

func TestGetString(t *testing.T) {
	sm := NewSharedMemory()

	sm.Set("name", "graycode", "agent-a")
	if got := sm.GetString("name"); got != "graycode" {
		t.Fatalf("expected %q, got %q", "graycode", got)
	}

	// Non-string value should be stringified.
	sm.Set("count", 42, "agent-a")
	if got := sm.GetString("count"); got != "42" {
		t.Fatalf("expected %q, got %q", "42", got)
	}

	// Non-existent key.
	if got := sm.GetString("missing"); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestGetInt(t *testing.T) {
	sm := NewSharedMemory()

	sm.Set("count", 42, "agent-a")
	if got := sm.GetInt("count"); got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}

	sm.Set("float_val", float64(7), "agent-a")
	if got := sm.GetInt("float_val"); got != 7 {
		t.Fatalf("expected 7, got %d", got)
	}

	// Non-existent key.
	if got := sm.GetInt("missing"); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}

	// Non-integer value.
	sm.Set("name", "graycode", "agent-a")
	if got := sm.GetInt("name"); got != 0 {
		t.Fatalf("expected 0 for non-int value, got %d", got)
	}
}

func TestGetBool(t *testing.T) {
	sm := NewSharedMemory()

	sm.Set("enabled", true, "agent-a")
	if got := sm.GetBool("enabled"); !got {
		t.Fatal("expected true, got false")
	}

	sm.Set("disabled", false, "agent-a")
	if got := sm.GetBool("disabled"); got {
		t.Fatal("expected false, got true")
	}

	// Non-existent key.
	if got := sm.GetBool("missing"); got {
		t.Fatal("expected false for missing key")
	}

	// Non-bool value.
	sm.Set("name", "graycode", "agent-a")
	if got := sm.GetBool("name"); got {
		t.Fatal("expected false for non-bool value")
	}
}

func TestDeleteOwnerCheck(t *testing.T) {
	sm := NewSharedMemory()

	sm.Set("secret", "value", "agent-a")

	// Non-owner cannot delete.
	err := sm.Delete("secret", "agent-b")
	if err == nil {
		t.Fatal("expected error when non-owner deletes")
	}
	if !strings.Contains(err.Error(), "cannot delete") {
		t.Fatalf("unexpected error message: %s", err.Error())
	}

	// Owner can delete.
	err = sm.Delete("secret", "agent-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, ok := sm.Get("secret")
	if ok {
		t.Fatal("expected key to be deleted")
	}

	// Delete non-existent key.
	err = sm.Delete("nonexistent", "agent-a")
	if err == nil {
		t.Fatal("expected error for non-existent key")
	}
}

func TestDeleteRemovesFromNamespace(t *testing.T) {
	sm := NewSharedMemory()

	sm.Set("auth.token", "jwt", "agent-a")
	sm.SetNamespace("auth.token", "auth")

	err := sm.Delete("auth.token", "agent-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries := sm.List("auth")
	if len(entries) != 0 {
		t.Fatalf("expected namespace to be empty after delete, got %d entries", len(entries))
	}
}

func TestNamespaces(t *testing.T) {
	sm := NewSharedMemory()

	sm.Set("auth.token", "jwt", "security-agent")
	sm.Set("auth.algo", "RS256", "security-agent")
	sm.Set("progress.auth", 0.75, "impl-agent")

	sm.SetNamespace("auth.token", "auth")
	sm.SetNamespace("auth.algo", "auth")
	sm.SetNamespace("progress.auth", "progress")

	authEntries := sm.List("auth")
	if len(authEntries) != 2 {
		t.Fatalf("expected 2 auth entries, got %d", len(authEntries))
	}

	progressEntries := sm.List("progress")
	if len(progressEntries) != 1 {
		t.Fatalf("expected 1 progress entry, got %d", len(progressEntries))
	}

	// Non-existent namespace.
	empty := sm.List("nonexistent")
	if len(empty) != 0 {
		t.Fatalf("expected 0 entries for nonexistent namespace, got %d", len(empty))
	}
}

func TestSetNamespaceNoDuplicates(t *testing.T) {
	sm := NewSharedMemory()

	sm.Set("key", "value", "agent-a")
	sm.SetNamespace("key", "ns")
	sm.SetNamespace("key", "ns") // duplicate

	sm.mu.RLock()
	keys := sm.Namespaces["ns"]
	sm.mu.RUnlock()

	if len(keys) != 1 {
		t.Fatalf("expected 1 key in namespace, got %d", len(keys))
	}
}

func TestSetNamespaceNonExistentKey(t *testing.T) {
	sm := NewSharedMemory()

	sm.SetNamespace("nonexistent", "ns")

	sm.mu.RLock()
	keys := sm.Namespaces["ns"]
	sm.mu.RUnlock()

	if len(keys) != 0 {
		t.Fatalf("expected 0 keys for non-existent key, got %d", len(keys))
	}
}

func TestWatch(t *testing.T) {
	sm := NewSharedMemory()

	ch := sm.Watch("progress")

	sm.Set("progress", 0.5, "agent-a")
	sm.Set("progress", 0.75, "agent-a")

	// Read first notification.
	select {
	case entry := <-ch:
		if entry.Key != "progress" {
			t.Fatalf("expected key %q, got %q", "progress", entry.Key)
		}
		if entry.Value != 0.5 {
			t.Fatalf("expected value 0.5, got %v", entry.Value)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for first watch notification")
	}

	// Read second notification.
	select {
	case entry := <-ch:
		if entry.Value != 0.75 {
			t.Fatalf("expected value 0.75, got %v", entry.Value)
		}
		if entry.Version != 2 {
			t.Fatalf("expected version 2, got %d", entry.Version)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for second watch notification")
	}
}

func TestSnapshot(t *testing.T) {
	sm := NewSharedMemory()

	sm.Set("a", "alpha", "agent-1")
	sm.Set("b", 42, "agent-2")
	sm.Set("c", true, "agent-3")

	snap := sm.Snapshot()

	if len(snap) != 3 {
		t.Fatalf("expected 3 entries in snapshot, got %d", len(snap))
	}
	if snap["a"] != "alpha" {
		t.Fatalf("expected %q, got %v", "alpha", snap["a"])
	}
	if snap["b"] != 42 {
		t.Fatalf("expected 42, got %v", snap["b"])
	}
	if snap["c"] != true {
		t.Fatalf("expected true, got %v", snap["c"])
	}
}

func TestRestore(t *testing.T) {
	sm := NewSharedMemory()

	sm.Set("existing", "value", "agent-a")

	snap := map[string]interface{}{
		"x": "hello",
		"y": 99,
	}

	sm.Restore(snap)

	// Old entries should be gone.
	_, ok := sm.Get("existing")
	if ok {
		t.Fatal("expected old entry to be cleared after restore")
	}

	// New entries should exist.
	val, ok := sm.Get("x")
	if !ok || val != "hello" {
		t.Fatalf("expected %q, got %v", "hello", val)
	}

	val, ok = sm.Get("y")
	if !ok || val != 99 {
		t.Fatalf("expected 99, got %v", val)
	}

	// Check restored entries have version 1 and owner "snapshot".
	sm.mu.RLock()
	entry := sm.Entries["x"]
	sm.mu.RUnlock()
	if entry.Version != 1 {
		t.Fatalf("expected version 1, got %d", entry.Version)
	}
	if entry.Owner != "snapshot" {
		t.Fatalf("expected owner %q, got %q", "snapshot", entry.Owner)
	}
}

func TestFormatState(t *testing.T) {
	sm := NewSharedMemory()

	output := sm.FormatState()
	if !strings.Contains(output, "0 entries") {
		t.Fatalf("expected empty state format, got: %s", output)
	}

	sm.Set("auth.token_type", "JWT", "security-agent")
	sm.Set("status.tests", true, "test-agent")
	sm.Set("files.modified", []string{"token.go", "middleware.go"}, "impl-agent")

	output = sm.FormatState()

	if !strings.Contains(output, "3 entries") {
		t.Fatalf("expected 3 entries in header, got: %s", output)
	}
	if !strings.Contains(output, `auth.token_type = "JWT"`) {
		t.Fatalf("expected auth.token_type entry, got: %s", output)
	}
	if !strings.Contains(output, "security-agent") {
		t.Fatalf("expected security-agent owner, got: %s", output)
	}
	if !strings.Contains(output, "status.tests = true") {
		t.Fatalf("expected status.tests entry, got: %s", output)
	}
	if !strings.Contains(output, `["token.go", "middleware.go"]`) {
		t.Fatalf("expected files list, got: %s", output)
	}
}

func TestDiff(t *testing.T) {
	sm := NewSharedMemory()

	sm.Set("old", "value", "agent-a")
	time.Sleep(10 * time.Millisecond)

	checkpoint := time.Now()
	time.Sleep(10 * time.Millisecond)

	sm.Set("new1", "v1", "agent-b")
	sm.Set("new2", "v2", "agent-c")

	results := sm.Diff(checkpoint)

	if len(results) != 2 {
		t.Fatalf("expected 2 entries in diff, got %d", len(results))
	}

	keys := make(map[string]bool)
	for _, entry := range results {
		keys[entry.Key] = true
	}
	if !keys["new1"] || !keys["new2"] {
		t.Fatalf("expected new1 and new2 in diff, got keys: %v", keys)
	}
}

func TestClear(t *testing.T) {
	sm := NewSharedMemory()

	sm.Set("a", 1, "agent-a")
	sm.Set("b", 2, "agent-b")
	sm.SetNamespace("a", "ns")

	sm.Clear()

	if len(sm.Entries) != 0 {
		t.Fatalf("expected 0 entries after clear, got %d", len(sm.Entries))
	}
	if len(sm.Namespaces) != 0 {
		t.Fatalf("expected 0 namespaces after clear, got %d", len(sm.Namespaces))
	}
}

func TestTypeDetection(t *testing.T) {
	sm := NewSharedMemory()

	sm.Set("str", "hello", "a")
	sm.Set("num", 42, "a")
	sm.Set("flag", true, "a")
	sm.Set("list", []string{"a", "b"}, "a")
	sm.Set("obj", map[string]interface{}{"key": "val"}, "a")

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	tests := []struct {
		key      string
		expected string
	}{
		{"str", "string"},
		{"num", "int"},
		{"flag", "bool"},
		{"list", "list"},
		{"obj", "json"},
	}

	for _, tt := range tests {
		if sm.Entries[tt.key].Type != tt.expected {
			t.Errorf("key %q: expected type %q, got %q", tt.key, tt.expected, sm.Entries[tt.key].Type)
		}
	}
}

// Concurrent access tests.

func TestConcurrentSetGet(t *testing.T) {
	sm := NewSharedMemory()
	var wg sync.WaitGroup

	// Multiple writers.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", id%10)
			sm.Set(key, id, fmt.Sprintf("agent-%d", id))
		}(i)
	}

	// Multiple readers concurrent with writers.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", id%10)
			sm.Get(key)
			sm.GetString(key)
			sm.GetInt(key)
			sm.GetBool(key)
		}(i)
	}

	wg.Wait()

	// Verify no data corruption - all keys should be accessible.
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key-%d", i)
		_, ok := sm.Get(key)
		if !ok {
			t.Fatalf("expected key %q to exist after concurrent writes", key)
		}
	}
}

func TestConcurrentNamespaces(t *testing.T) {
	sm := NewSharedMemory()
	var wg sync.WaitGroup

	// Populate keys.
	for i := 0; i < 50; i++ {
		sm.Set(fmt.Sprintf("key-%d", i), i, "setup")
	}

	// Concurrent namespace assignments.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ns := fmt.Sprintf("ns-%d", id%5)
			key := fmt.Sprintf("key-%d", id)
			sm.SetNamespace(key, ns)
		}(i)
	}

	// Concurrent list reads.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ns := fmt.Sprintf("ns-%d", id%5)
			sm.List(ns)
		}(i)
	}

	wg.Wait()
}

func TestConcurrentWatchAndSet(t *testing.T) {
	sm := NewSharedMemory()
	var wg sync.WaitGroup

	const numWatchers = 5
	const numWrites = 20

	channels := make([]<-chan *MemEntry, numWatchers)
	for i := 0; i < numWatchers; i++ {
		channels[i] = sm.Watch("counter")
	}

	// Writer goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numWrites; i++ {
			sm.Set("counter", i, "writer")
		}
	}()

	// Reader goroutines drain notifications.
	received := make([]int, numWatchers)
	for i := 0; i < numWatchers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			timeout := time.After(500 * time.Millisecond)
			for {
				select {
				case _, ok := <-channels[idx]:
					if !ok {
						return
					}
					received[idx]++
				case <-timeout:
					return
				}
			}
		}(i)
	}

	wg.Wait()

	// Each watcher should receive at least some notifications.
	// (Channel buffer is 16, so with 20 writes some may be dropped.)
	for i, count := range received {
		if count == 0 {
			t.Errorf("watcher %d received 0 notifications", i)
		}
	}
}

func TestConcurrentSnapshotRestore(t *testing.T) {
	sm := NewSharedMemory()
	var wg sync.WaitGroup

	// Writer populates data.
	for i := 0; i < 20; i++ {
		sm.Set(fmt.Sprintf("k%d", i), i, "writer")
	}

	// Concurrent snapshots and restores.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sm.Snapshot()
		}()
	}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			snap := map[string]interface{}{
				fmt.Sprintf("restored-%d", id): id,
			}
			sm.Restore(snap)
		}(i)
	}

	wg.Wait()

	// Memory should be in a consistent state (no panics).
	sm.Snapshot()
}

func TestConcurrentDiffAndSet(t *testing.T) {
	sm := NewSharedMemory()
	var wg sync.WaitGroup

	checkpoint := time.Now()

	// Concurrent writers.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sm.Set(fmt.Sprintf("item-%d", id), id, "agent")
		}(i)
	}

	// Concurrent diff readers.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sm.Diff(checkpoint)
		}()
	}

	wg.Wait()
}

func TestConcurrentDeleteAndSet(t *testing.T) {
	sm := NewSharedMemory()
	var wg sync.WaitGroup

	// Pre-populate.
	for i := 0; i < 20; i++ {
		sm.Set(fmt.Sprintf("item-%d", i), i, "owner")
	}

	// Concurrent deletes by owner.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sm.Delete(fmt.Sprintf("item-%d", id), "owner")
		}(i)
	}

	// Concurrent new writes.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sm.Set(fmt.Sprintf("new-%d", id), id, "writer")
		}(i)
	}

	wg.Wait()

	// All new entries should exist.
	for i := 0; i < 20; i++ {
		_, ok := sm.Get(fmt.Sprintf("new-%d", i))
		if !ok {
			t.Fatalf("expected new-%d to exist", i)
		}
	}
}
