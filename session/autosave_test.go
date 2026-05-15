package session

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestAcquireLock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(fmt.Sprintf("%s/.hawk/sessions", dir), 0o755)

	lock, err := AcquireLock("test-lock")
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	if lock == nil {
		t.Fatal("lock should not be nil")
	}
	lock.Release()
}

func TestAcquireLock_AlreadyLocked(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(fmt.Sprintf("%s/.hawk/sessions", dir), 0o755)

	lock1, err := AcquireLock("locked-session")
	if err != nil {
		t.Fatal(err)
	}
	defer lock1.Release()

	_, err = AcquireLock("locked-session")
	if err == nil {
		t.Error("should fail when session is already locked")
	}
	var lockErr *SessionLockedError
	if err != nil {
		lockErr, _ = err.(*SessionLockedError)
		if lockErr == nil {
			t.Errorf("expected SessionLockedError, got %T", err)
		}
	}
}

func TestAcquireLock_StaleLock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	sessDir := fmt.Sprintf("%s/.hawk/sessions", dir)
	_ = os.MkdirAll(sessDir, 0o755)

	// Create a stale lock (>5 min old)
	lockPath := sessDir + "/stale-session.lock"
	_ = os.WriteFile(lockPath, []byte("old"), 0o644)
	oldTime := time.Now().Add(-10 * time.Minute)
	_ = os.Chtimes(lockPath, oldTime, oldTime)

	lock, err := AcquireLock("stale-session")
	if err != nil {
		t.Fatalf("should acquire stale lock, got: %v", err)
	}
	if lock == nil {
		t.Fatal("lock should not be nil")
	}
	lock.Release()
}

func TestLockFile_Release(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(fmt.Sprintf("%s/.hawk/sessions", dir), 0o755)

	lock, _ := AcquireLock("release-test")
	lock.Release()

	// Should be able to acquire again after release
	lock2, err := AcquireLock("release-test")
	if err != nil {
		t.Fatalf("should acquire after release: %v", err)
	}
	lock2.Release()
}

func TestLockFile_Refresh(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(fmt.Sprintf("%s/.hawk/sessions", dir), 0o755)

	lock, _ := AcquireLock("refresh-test")
	defer lock.Release()

	lock.Refresh()
	// Should not panic
}

func TestLockFile_Release_Nil(t *testing.T) {
	t.Parallel()
	var lock *LockFile
	lock.Release() // should not panic
}

func TestSessionLockedError(t *testing.T) {
	t.Parallel()
	err := &SessionLockedError{ID: "test-123"}
	msg := err.Error()
	if msg == "" {
		t.Error("Error() should not be empty")
	}
}

func TestAddTag(t *testing.T) {
	sess := &Session{Name: ""}
	AddTag(sess, "important")
	if sess.Name == "" {
		t.Error("AddTag should modify Name")
	}
}

func TestRemoveTag(t *testing.T) {
	sess := &Session{Name: "tag:important,tag:urgent"}
	RemoveTag(sess, "important")
	// Just verify no panic — behavior depends on implementation
}

func TestNewAutoSaver(t *testing.T) {
	saved := false
	saver := NewAutoSaver(5*time.Minute, func() { saved = true })
	if saver == nil {
		t.Fatal("NewAutoSaver returned nil")
	}
	saver.Stop()
	_ = saved
}

func TestAutoSaver_Touch(t *testing.T) {
	saver := NewAutoSaver(5*time.Minute, func() {})
	defer saver.Stop()
	saver.Touch()
}

func TestAutoSaver_Reset(t *testing.T) {
	saver := NewAutoSaver(5*time.Minute, func() {})
	defer saver.Stop()
	saver.Reset()
}
