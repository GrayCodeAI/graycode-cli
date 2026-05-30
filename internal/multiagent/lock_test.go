package mission

import (
	"sync"
	"testing"
	"time"
)

func TestAcquireLock_Success(t *testing.T) {
	mb := NewMessageBus()
	if err := mb.AcquireLock("file.go", "agent-1", time.Minute); err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	if !mb.IsLocked("file.go") {
		t.Error("expected resource to be locked")
	}
}

func TestAcquireLock_Contention(t *testing.T) {
	mb := NewMessageBus()
	if err := mb.AcquireLock("file.go", "agent-1", time.Minute); err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}
	if err := mb.AcquireLock("file.go", "agent-2", time.Minute); err == nil {
		t.Error("expected contention error for second agent")
	}
}

func TestAcquireLock_Reentrant(t *testing.T) {
	mb := NewMessageBus()
	if err := mb.AcquireLock("file.go", "agent-1", time.Minute); err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}
	if err := mb.AcquireLock("file.go", "agent-1", time.Minute); err != nil {
		t.Errorf("same owner should re-acquire, got: %v", err)
	}
}

func TestAcquireLock_Expired(t *testing.T) {
	mb := NewMessageBus()
	if err := mb.AcquireLock("file.go", "agent-1", 10*time.Millisecond); err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if mb.IsLocked("file.go") {
		t.Error("expected expired lock to be unlocked")
	}
	if err := mb.AcquireLock("file.go", "agent-2", time.Minute); err != nil {
		t.Errorf("expected agent-2 to reclaim expired lock, got: %v", err)
	}
}

func TestReleaseLock_Ownership(t *testing.T) {
	mb := NewMessageBus()
	_ = mb.AcquireLock("file.go", "agent-1", time.Minute)
	if err := mb.ReleaseLock("file.go", "agent-2"); err == nil {
		t.Error("expected error releasing lock owned by another agent")
	}
	if err := mb.ReleaseLock("file.go", "agent-1"); err != nil {
		t.Errorf("owner should release, got: %v", err)
	}
	if mb.IsLocked("file.go") {
		t.Error("expected resource to be unlocked after release")
	}
}

func TestReleaseLock_Idempotent(t *testing.T) {
	mb := NewMessageBus()
	if err := mb.ReleaseLock("never-locked", "agent-1"); err != nil {
		t.Errorf("releasing unlocked resource should be idempotent, got: %v", err)
	}
}

func TestWaitForLock_Timeout(t *testing.T) {
	mb := NewMessageBus()
	_ = mb.AcquireLock("file.go", "agent-1", time.Minute)
	err := mb.WaitForLock("file.go", "agent-2", 30*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error waiting for held lock")
	}
}

func TestWaitForLock_Success(t *testing.T) {
	mb := NewMessageBus()
	_ = mb.AcquireLock("file.go", "agent-1", time.Minute)

	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = mb.ReleaseLock("file.go", "agent-1")
	}()

	if err := mb.WaitForLock("file.go", "agent-2", time.Second); err != nil {
		t.Errorf("expected to acquire lock after release, got: %v", err)
	}
}

func TestCleanupExpiredLocks(t *testing.T) {
	mb := NewMessageBus()
	_ = mb.AcquireLock("a", "agent-1", 10*time.Millisecond)
	_ = mb.AcquireLock("b", "agent-1", time.Minute)
	time.Sleep(20 * time.Millisecond)
	mb.CleanupExpiredLocks()
	if mb.IsLocked("a") {
		t.Error("expected expired lock 'a' to be cleaned up")
	}
	if !mb.IsLocked("b") {
		t.Error("expected live lock 'b' to remain")
	}
}

func TestTryLockFiles(t *testing.T) {
	mb := NewMessageBus()
	_ = mb.AcquireLock("b.go", "agent-2", time.Minute)
	failed := mb.TryLockFiles("agent-1", []string{"a.go", "b.go", "c.go"}, time.Minute)
	if len(failed) != 1 || failed[0] != "b.go" {
		t.Errorf("expected only b.go to fail, got %v", failed)
	}
	if !mb.IsLocked("a.go") || !mb.IsLocked("c.go") {
		t.Error("expected a.go and c.go to be locked by agent-1")
	}
}

func TestConcurrentLockAcquire(t *testing.T) {
	mb := NewMessageBus()
	const n = 50
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if err := mb.AcquireLock("shared", string(rune('a'+id)), time.Minute); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if successes != 1 {
		t.Errorf("expected exactly 1 successful acquire under contention, got %d", successes)
	}
}
