package taskruntime

import (
	"context"
	"testing"
	"time"

	agentcontracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/agent"
)

func TestSpawnAndCollect(t *testing.T) {
	r := New()
	id := r.SpawnAgent(context.Background(), "t1", agentcontracts.SpawnRequest{Prompt: "hi"},
		func(_ context.Context, req agentcontracts.SpawnRequest) (agentcontracts.SpawnResult, error) {
			return agentcontracts.SpawnResult{Output: "ok:" + req.Prompt}, nil
		})
	if id != "t1" {
		t.Fatalf("id=%s", id)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !r.IsRunning("t1") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	task, ok := r.Get("t1")
	if !ok || task.Status != StatusCompleted || task.Output != "ok:hi" {
		t.Fatalf("task=%+v ok=%v", task, ok)
	}
	got := r.CollectCompleted()
	if len(got) != 1 {
		t.Fatalf("collect=%d", len(got))
	}
}

func TestKill(t *testing.T) {
	r := New()
	started := make(chan struct{})
	id := r.SpawnAgent(context.Background(), "k1", agentcontracts.SpawnRequest{Prompt: "x"},
		func(ctx context.Context, _ agentcontracts.SpawnRequest) (agentcontracts.SpawnResult, error) {
			close(started)
			<-ctx.Done()
			return agentcontracts.SpawnResult{}, ctx.Err()
		})
	<-started
	if err := r.Kill(id); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !r.IsRunning(id) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	task, ok := r.Get(id)
	if !ok {
		t.Fatal("missing task")
	}
	if task.Status != StatusFailed && task.Status != StatusKilled {
		t.Fatalf("status=%s", task.Status)
	}
}
