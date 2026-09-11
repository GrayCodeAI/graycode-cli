package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/jobs"
)

// resetJobsRegistry swaps the package-level registry for a fresh one and
// restores the original afterwards, keeping tests isolated.
func resetJobsRegistry(t *testing.T) {
	t.Helper()
	orig := jobsRegistry
	jobsRegistry = jobs.NewRegistry()
	t.Cleanup(func() { jobsRegistry = orig })
}

func TestJobsToolRunListWaitRead(t *testing.T) {
	resetJobsRegistry(t)
	tool := JobsTool{}
	ctx := context.Background()

	// run a short-lived command
	out, err := tool.Execute(ctx, json.RawMessage(`{"action":"run","command":"printf 'hello job world'","session":"sess-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "bash-1") {
		t.Fatalf("run output = %q, want job id bash-1", out)
	}

	// wait for it to settle
	waitOut, err := tool.Execute(ctx, json.RawMessage(`{"action":"wait","id":"bash-1","timeout_sec":10}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(waitOut, `"status": "completed"`) {
		t.Fatalf("wait output = %q, want completed", waitOut)
	}

	// read final output
	readOut, err := tool.Execute(ctx, json.RawMessage(`{"action":"read","id":"bash-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readOut, "hello job world") {
		t.Fatalf("read output = %q, want captured output", readOut)
	}

	// list: the owner session sees its own job
	listOut, err := tool.Execute(ctx, json.RawMessage(`{"action":"list","session":"sess-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listOut, "bash-1") {
		t.Fatalf("list output = %q, want bash-1", listOut)
	}
	// a stranger session sees only unowned jobs (none here)
	stranger, err := tool.Execute(ctx, json.RawMessage(`{"action":"list","session":"other"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stranger, "bash-1") {
		t.Fatalf("stranger list leaked owned job: %q", stranger)
	}
}

func TestJobsToolRunFailureDetail(t *testing.T) {
	resetJobsRegistry(t)
	tool := JobsTool{}
	ctx := context.Background()

	out, err := tool.Execute(ctx, json.RawMessage(`{"action":"run","command":"exit 3"}`))
	if err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSpace(strings.TrimPrefix(out, "job started: "))
	if id == "" {
		t.Fatalf("run output = %q, want job id", out)
	}

	waitOut, err := tool.Execute(ctx, json.RawMessage(`{"action":"wait","id":"`+id+`","timeout_sec":10}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(waitOut, `"status": "failed"`) || !strings.Contains(waitOut, "exit code: 3") {
		t.Fatalf("wait output = %q, want failed with exit code 3", waitOut)
	}
}

func TestJobsToolKill(t *testing.T) {
	resetJobsRegistry(t)
	tool := JobsTool{}
	ctx := context.Background()

	// run a long-lived command
	out, err := tool.Execute(ctx, json.RawMessage(`{"action":"run","command":"sleep 60"}`))
	if err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSpace(strings.TrimPrefix(out, "job started: "))

	killOut, err := tool.Execute(ctx, json.RawMessage(`{"action":"kill","id":"`+id+`","reason":"test teardown"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(killOut, "termination requested") {
		t.Fatalf("kill output = %q", killOut)
	}

	waitOut, err := tool.Execute(ctx, json.RawMessage(`{"action":"wait","id":"`+id+`","timeout_sec":10}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(waitOut, `"status": "killed"`) {
		t.Fatalf("wait output = %q, want killed", waitOut)
	}
}

func TestJobsToolValidation(t *testing.T) {
	resetJobsRegistry(t)
	tool := JobsTool{}
	ctx := context.Background()

	if _, err := tool.Execute(ctx, json.RawMessage(`{"action":"read","id":"missing-1"}`)); err == nil {
		t.Fatal("read of unknown job should error")
	}
	if _, err := tool.Execute(ctx, json.RawMessage(`{"action":"run"}`)); err == nil {
		t.Fatal("run without command should error")
	}
	if _, err := tool.Execute(ctx, json.RawMessage(`{"action":"bogus"}`)); err == nil {
		t.Fatal("unknown action should error")
	}
	if _, err := tool.Execute(ctx, json.RawMessage(`{}`)); err == nil {
		t.Fatal("missing action should error")
	}
}

func TestJobsToolOutputLimit(t *testing.T) {
	resetJobsRegistry(t)
	tool := JobsTool{}
	ctx := context.Background()

	// 32 bytes of output, 16-byte limit.
	out, err := tool.Execute(ctx, json.RawMessage(`{"action":"run","command":"printf '0123456789abcdef0123456789abcdef'","output_limit":16}`))
	if err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSpace(strings.TrimPrefix(out, "job started: "))

	if _, err := tool.Execute(ctx, json.RawMessage(`{"action":"wait","id":"`+id+`","timeout_sec":10}`)); err != nil {
		t.Fatal(err)
	}
	readOut, err := tool.Execute(ctx, json.RawMessage(`{"action":"read","id":"`+id+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readOut, "0123456789abcdef0123456789abcdef") {
		t.Fatalf("read output not capped: %q", readOut)
	}
	if !strings.Contains(readOut, "0123456789abcdef") {
		t.Fatalf("read output missing capped prefix: %q", readOut)
	}
}

func TestJobsToolEngineRegistryShared(t *testing.T) {
	// The engine-facing seam must resolve to the same registry the tool uses.
	resetJobsRegistry(t)
	tool := JobsTool{}
	ctx := context.Background()
	if _, err := tool.Execute(ctx, json.RawMessage(`{"action":"run","command":"printf x"}`)); err != nil {
		t.Fatal(err)
	}
	if got := len(SessionJobsRegistry().List("")); got != 1 {
		t.Fatalf("engine registry sees %d jobs, want 1", got)
	}
	_ = jobs.ErrNotFound // keep the jobs import meaningful for future engine wiring tests
}
