package container

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// mockCmdWithRecorder returns a mock that records all invocations.
type invocation struct {
	name string
	args []string
}

func mockCmdRecorder(responses []struct {
	stdout   string
	stderr   string
	exitCode int
},
) (func(ctx context.Context, name string, args ...string) *exec.Cmd, *[]invocation) {
	var calls []invocation
	idx := 0
	fn := func(ctx context.Context, name string, args ...string) *exec.Cmd {
		calls = append(calls, invocation{name: name, args: args})
		r := responses[0]
		if idx < len(responses) {
			r = responses[idx]
			idx++
		}
		script := fmt.Sprintf("printf '%%s' %q >&1; printf '%%s' %q >&2; exit %d",
			r.stdout, r.stderr, r.exitCode)
		return exec.CommandContext(ctx, "/bin/sh", "-c", script)
	}
	return fn, &calls
}

func TestContainerErrorCodes(t *testing.T) {
	tests := []struct {
		code string
		msg  string
		want string
	}{
		{CodeNotFound, "container gone", "not_found: container gone"},
		{CodeNotRunning, "stopped", "not_running: stopped"},
		{CodeExecFailed, "bad cmd", "exec_failed: bad cmd"},
		{CodeTimeout, "too slow", "timeout: too slow"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			err := &ContainerError{Code: tt.code, Message: tt.msg}
			if err.Error() != tt.want {
				t.Errorf("got %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestStatusParsing(t *testing.T) {
	tests := []struct {
		name       string
		stdout     string
		stderr     string
		exitCode   int
		wantState  string
		wantErrNil bool
	}{
		{
			name:       "running container",
			stdout:     "running",
			exitCode:   0,
			wantState:  StateRunning,
			wantErrNil: true,
		},
		{
			name:       "exited container",
			stdout:     "exited",
			exitCode:   0,
			wantState:  StateStopped,
			wantErrNil: true,
		},
		{
			name:       "dead container",
			stdout:     "dead",
			exitCode:   0,
			wantState:  StateStopped,
			wantErrNil: true,
		},
		{
			name:       "not found container",
			stderr:     "Error: No such object: abc123",
			exitCode:   1,
			wantState:  StateNotFound,
			wantErrNil: true,
		},
		{
			name:       "empty container ID",
			wantState:  StateNotFound,
			wantErrNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := dockerCmd
			defer func() { dockerCmd = orig }()

			if tt.name == "empty container ID" {
				// No mock needed — Status returns early for empty ID.
				state, err := Status(context.Background(), "")
				if state != tt.wantState {
					t.Errorf("state = %q, want %q", state, tt.wantState)
				}
				if (err == nil) != tt.wantErrNil {
					t.Errorf("err = %v, wantErrNil = %v", err, tt.wantErrNil)
				}
				return
			}

			dockerCmd = func(ctx context.Context, name string, args ...string) *exec.Cmd {
				script := fmt.Sprintf("printf '%%s' %q >&1; printf '%%s' %q >&2; exit %d",
					tt.stdout, tt.stderr, tt.exitCode)
				return exec.CommandContext(ctx, "/bin/sh", "-c", script)
			}

			state, err := Status(context.Background(), "test-container-123")
			if state != tt.wantState {
				t.Errorf("state = %q, want %q", state, tt.wantState)
			}
			if (err == nil) != tt.wantErrNil {
				t.Errorf("err = %v, wantErrNil = %v", err, tt.wantErrNil)
			}
		})
	}
}

func TestExecWithStdin(t *testing.T) {
	orig := dockerCmd
	defer func() { dockerCmd = orig }()

	t.Run("empty container ID", func(t *testing.T) {
		_, err := ExecWithStdin(context.Background(), "", []string{"cat"}, []byte("hello"))
		if err == nil {
			t.Fatal("expected error for empty container ID")
		}
		ce := &ContainerError{}
		ok := errors.As(err, &ce)
		if !ok {
			t.Fatalf("expected *ContainerError, got %T", err)
		}
		if ce.Code != CodeNotFound {
			t.Errorf("code = %q, want %q", ce.Code, CodeNotFound)
		}
	})

	t.Run("successful exec with stdin", func(t *testing.T) {
		dockerCmd = func(ctx context.Context, name string, args ...string) *exec.Cmd {
			// Verify that args contain the expected docker exec structure.
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "exec -i my-container cat") {
				t.Errorf("unexpected args: %v", args)
			}
			// Simulate reading stdin and echoing it.
			cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "cat")
			cmd.Stdin = bytes.NewReader([]byte(`{"key":"value"}`))
			// We can't truly pipe through the mock, so just echo fixed output.
			return exec.CommandContext(ctx, "/bin/sh", "-c", `printf '{"key":"value"}'`)
		}

		result, err := ExecWithStdin(context.Background(), "my-container", []string{"cat"}, []byte(`{"key":"value"}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ExitCode != 0 {
			t.Errorf("exit code = %d, want 0", result.ExitCode)
		}
		if !strings.Contains(result.Stdout, `"key"`) {
			t.Errorf("stdout = %q, expected JSON content", result.Stdout)
		}
	})

	t.Run("non-zero exit code", func(t *testing.T) {
		dockerCmd = func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "/bin/sh", "-c", "printf 'err' >&2; exit 42")
		}

		result, err := ExecWithStdin(context.Background(), "my-container", []string{"false"}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ExitCode != 42 {
			t.Errorf("exit code = %d, want 42", result.ExitCode)
		}
	})
}

func TestEnsureRunning(t *testing.T) {
	orig := dockerCmd
	defer func() { dockerCmd = orig }()

	t.Run("already running", func(t *testing.T) {
		responses := []struct {
			stdout   string
			stderr   string
			exitCode int
		}{
			{stdout: "running", exitCode: 0}, // Status inspect
		}
		fn, calls := mockCmdRecorder(responses)
		dockerCmd = fn

		id, err := EnsureRunning(context.Background(), "existing-id", "alpine:latest", "/workspace")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "existing-id" {
			t.Errorf("id = %q, want %q", id, "existing-id")
		}
		// Only one call: docker inspect.
		if len(*calls) != 1 {
			t.Errorf("expected 1 call, got %d: %v", len(*calls), *calls)
		}
	})

	t.Run("stopped container restarted", func(t *testing.T) {
		responses := []struct {
			stdout   string
			stderr   string
			exitCode int
		}{
			{stdout: "exited", exitCode: 0}, // Status inspect
			{stdout: "", exitCode: 0},       // docker start
		}
		fn, calls := mockCmdRecorder(responses)
		dockerCmd = fn

		id, err := EnsureRunning(context.Background(), "stopped-id", "alpine:latest", "/workspace")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "stopped-id" {
			t.Errorf("id = %q, want %q", id, "stopped-id")
		}
		if len(*calls) != 2 {
			t.Errorf("expected 2 calls, got %d: %v", len(*calls), *calls)
		}
	})

	t.Run("not found creates new", func(t *testing.T) {
		responses := []struct {
			stdout   string
			stderr   string
			exitCode int
		}{
			{stderr: "Error: No such object: gone", exitCode: 1}, // Status inspect
			{stdout: "new-container-id-abc123", exitCode: 0},     // docker run
		}
		fn, calls := mockCmdRecorder(responses)
		dockerCmd = fn

		id, err := EnsureRunning(context.Background(), "gone", "alpine:latest", "/workspace")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "new-container-id-abc123" {
			t.Errorf("id = %q, want %q", id, "new-container-id-abc123")
		}
		if len(*calls) < 2 {
			t.Errorf("expected at least 2 calls, got %d: %v", len(*calls), *calls)
		}
	})

	t.Run("empty ID creates new", func(t *testing.T) {
		responses := []struct {
			stdout   string
			stderr   string
			exitCode int
		}{
			{stdout: "fresh-container-id", exitCode: 0}, // docker run (Rebuild with empty containerID skips rm)
		}
		fn, _ := mockCmdRecorder(responses)
		dockerCmd = fn

		id, err := EnsureRunning(context.Background(), "", "alpine:latest", "/workspace")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "fresh-container-id" {
			t.Errorf("id = %q, want %q", id, "fresh-container-id")
		}
	})
}

func TestRebuild(t *testing.T) {
	orig := dockerCmd
	defer func() { dockerCmd = orig }()

	t.Run("removes old and creates new", func(t *testing.T) {
		responses := []struct {
			stdout   string
			stderr   string
			exitCode int
		}{
			{stdout: "", exitCode: 0},            // docker rm -f
			{stdout: "rebuilt-abc", exitCode: 0}, // docker run
		}
		fn, calls := mockCmdRecorder(responses)
		dockerCmd = fn

		id, err := Rebuild(context.Background(), "old-container", "ubuntu:24.04", "/project")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "rebuilt-abc" {
			t.Errorf("id = %q, want %q", id, "rebuilt-abc")
		}
		// Should have called rm then run.
		if len(*calls) != 2 {
			t.Errorf("expected 2 calls, got %d", len(*calls))
		}
		if !strings.Contains(strings.Join((*calls)[0].args, " "), "rm -f old-container") {
			t.Errorf("first call should be rm -f, got: %v", (*calls)[0].args)
		}
	})

	t.Run("skips rm when no old container", func(t *testing.T) {
		responses := []struct {
			stdout   string
			stderr   string
			exitCode int
		}{
			{stdout: "new-id", exitCode: 0}, // docker run
		}
		fn, calls := mockCmdRecorder(responses)
		dockerCmd = fn

		id, err := Rebuild(context.Background(), "", "alpine:latest", "/work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "new-id" {
			t.Errorf("id = %q, want %q", id, "new-id")
		}
		if len(*calls) != 1 {
			t.Errorf("expected 1 call, got %d", len(*calls))
		}
	})
}
