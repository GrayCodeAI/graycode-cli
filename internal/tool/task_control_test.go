package tool

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/GrayCodeAI/hawk/internal/taskruntime"
)

func TestWaitTasksAndKillMonitor(t *testing.T) {
	// Start a short-lived monitor
	input, _ := json.Marshal(map[string]any{
		"command":           "echo mon-line; sleep 0.2; echo done",
		"max_runtime_sec":   10,
		"max_lines_per_sec": 100,
		"description":       "test mon",
	})
	out, err := (MonitorTool{}).Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	// parse task_id from output
	// format: ... task_id=mon_...
	id := ""
	for _, part := range splitFields(out) {
		if len(part) > 8 && part[:8] == "task_id=" {
			id = part[8:]
			break
		}
	}
	if id == "" {
		// fallback: find mon_
		for _, part := range splitFields(out) {
			if len(part) > 4 && part[:4] == "mon_" {
				id = part
				break
			}
		}
	}
	if id == "" {
		t.Fatalf("no task_id in %q", out)
	}

	waitIn, _ := json.Marshal(map[string]any{"task_id": id, "timeout_sec": 5})
	result, err := (WaitTasksTool{}).Execute(context.Background(), waitIn)
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Fatal("empty wait result")
	}

	// kill non-existent
	killIn, _ := json.Marshal(map[string]any{"task_id": "nope"})
	if _, err := (KillTaskTool{}).Execute(context.Background(), killIn); err == nil {
		t.Fatal("expected error for missing task")
	}
}

func TestTaskOutputUnifiedShell(t *testing.T) {
	id, err := startBackgroundBash(context.Background(), "echo hello-unified")
	if err != nil {
		t.Fatal(err)
	}
	// wait for completion
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if tsk, ok := taskruntime.Default.Get(id); ok && tsk.Status != taskruntime.StatusRunning {
			break
		}
		if bt, ok := getBackgroundTask(id); ok {
			select {
			case <-bt.done:
				goto done
			default:
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
done:
	in, _ := json.Marshal(map[string]any{"task_id": id})
	out, err := (TaskOutputTool{}).Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("empty output")
	}
}

func TestCronCreateWithLimits(t *testing.T) {
	globalCronScheduler = &CronScheduler{jobs: make(map[string]*CronJob)}
	in, _ := json.Marshal(map[string]any{
		"schedule":       "*/5 * * * *",
		"prompt":         "check",
		"max_runs":       3,
		"expires_in_sec": 3600,
	})
	out, err := (CronCreateTool{}).Execute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("empty")
	}
	jobs := globalCronScheduler.List()
	if len(jobs) != 1 || jobs[0].MaxRuns != 3 {
		t.Fatalf("jobs=%+v", jobs)
	}
	// tick to max
	id := jobs[0].ID
	if globalCronScheduler.TickRun(id) {
		t.Fatal("should not delete on first run")
	}
	globalCronScheduler.TickRun(id)
	if !globalCronScheduler.TickRun(id) {
		t.Fatal("should delete on max runs")
	}
	if len(globalCronScheduler.List()) != 0 {
		t.Fatal("expected empty after max runs")
	}
}

func splitFields(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' || r == '"' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
