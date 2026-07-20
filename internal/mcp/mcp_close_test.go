package mcp

import (
	"os/exec"
	"sync"
	"testing"
)

// newTestServerCat builds a Server backed by a real `cat` subprocess. cat reads
// stdin and exits cleanly (status 0) as soon as stdin is closed, which is
// exactly the shutdown Close() performs. This exercises the real
// stdin.Close + cmd.Wait path without a full MCP handshake.
func newTestServerCat(t *testing.T) *Server {
	t.Helper()
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not available on PATH")
	}
	cmd := exec.Command("cat")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start cat: %v", err)
	}
	return &Server{Name: "test", cmd: cmd, stdin: stdin}
}

// TestServerCloseIdempotent verifies Close() can be called repeatedly: the
// sync.Once guard runs the shutdown exactly once and every subsequent call
// observes the same cached result, without double-closing stdin or calling
// cmd.Wait twice (either of which would race or error).
func TestServerCloseIdempotent(t *testing.T) {
	s := newTestServerCat(t)

	err1 := s.Close()
	if err1 != nil {
		t.Fatalf("first Close() = %v, want nil (cat exits 0 on stdin EOF)", err1)
	}
	for i := 0; i < 3; i++ {
		if err := s.Close(); err != err1 {
			t.Fatalf("Close() call %d = %v, want same as first (%v)", i+2, err, err1)
		}
	}
}

// TestServerCloseConcurrent verifies Close() is safe under concurrent callers:
// Connect's initialize-failure path and a manager's later cleanup can both
// reach Close at once. Run with -race to catch a regression of the sync.Once
// guard.
func TestServerCloseConcurrent(t *testing.T) {
	s := newTestServerCat(t)

	const n = 16
	var wg sync.WaitGroup
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			errs[idx] = s.Close()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("concurrent Close() [%d] = %v, want nil", i, err)
		}
	}
}
