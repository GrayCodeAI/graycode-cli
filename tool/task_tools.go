package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const maxTaskOutputBytes = 200_000

type backgroundTask struct {
	id      string
	command string
	cmd     *exec.Cmd
	started time.Time
	done    chan struct{}

	mu       sync.RWMutex
	output   bytes.Buffer
	exitText string
	stopped  bool
}

var backgroundTasks = struct {
	sync.RWMutex
	next  int
	tasks map[string]*backgroundTask
}{tasks: make(map[string]*backgroundTask)}

func startBackgroundBash(ctx context.Context, command string) (string, error) {
	backgroundTasks.Lock()
	backgroundTasks.next++
	id := fmt.Sprintf("task_%d", backgroundTasks.next)
	backgroundTasks.Unlock()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	task := &backgroundTask{id: id, command: command, cmd: cmd, started: time.Now(), done: make(chan struct{})}
	backgroundTasks.Lock()
	backgroundTasks.tasks[id] = task
	backgroundTasks.Unlock()

	if err := cmd.Start(); err != nil {
		removeBackgroundTask(id)
		return "", err
	}

	go task.capture(stdout)
	go task.capture(stderr)
	go func() {
		err := cmd.Wait()
		task.mu.Lock()
		if err != nil {
			task.exitText = err.Error()
		} else {
			task.exitText = "exit status 0"
		}
		task.mu.Unlock()
		close(task.done)
	}()

	return id, nil
}

func getBackgroundTask(id string) (*backgroundTask, bool) {
	backgroundTasks.RLock()
	defer backgroundTasks.RUnlock()
	task, ok := backgroundTasks.tasks[id]
	return task, ok
}

func removeBackgroundTask(id string) {
	backgroundTasks.Lock()
	defer backgroundTasks.Unlock()
	delete(backgroundTasks.tasks, id)
}

func (t *backgroundTask) capture(r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			t.appendOutput(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func (t *backgroundTask) appendOutput(data []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.output.Write(data)
	if t.output.Len() <= maxTaskOutputBytes {
		return
	}
	trimmed := t.output.Bytes()[t.output.Len()-maxTaskOutputBytes:]
	t.output.Reset()
	t.output.WriteString("... (output truncated)\n")
	t.output.Write(trimmed)
}

func (t *backgroundTask) snapshot() (status, output string) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	status = "running"
	select {
	case <-t.done:
		status = "completed"
		if t.stopped {
			status = "stopped"
		}
	default:
	}
	output = strings.TrimRight(t.output.String(), "\n")
	if t.exitText != "" {
		if output != "" {
			output += "\n\n"
		}
		output += t.exitText
	}
	return status, output
}

func (t *backgroundTask) stop() error {
	t.mu.Lock()
	t.stopped = true
	t.mu.Unlock()
	if t.cmd.Process == nil {
		return nil
	}
	return t.cmd.Process.Kill()
}

type TaskOutputTool struct{}

func (TaskOutputTool) Name() string      { return "TaskOutput" }
func (TaskOutputTool) Aliases() []string { return []string{"task_output"} }
func (TaskOutputTool) Description() string {
	return "Read output from a background Bash task started with run_in_background."
}
func (TaskOutputTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task_id": map[string]interface{}{"type": "string", "description": "Background task ID"},
		},
		"required": []string{"task_id"},
	}
}
func (TaskOutputTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	task, ok := getBackgroundTask(p.TaskID)
	if !ok {
		return "", fmt.Errorf("background task %q not found", p.TaskID)
	}
	status, output := task.snapshot()
	return fmt.Sprintf("Task: %s\nCommand: %s\nStatus: %s\n\n%s", task.id, task.command, status, output), nil
}

type TaskStopTool struct{}

func (TaskStopTool) Name() string      { return "TaskStop" }
func (TaskStopTool) Aliases() []string { return []string{"task_stop"} }
func (TaskStopTool) Description() string {
	return "Stop a background Bash task."
}
func (TaskStopTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task_id": map[string]interface{}{"type": "string", "description": "Background task ID"},
		},
		"required": []string{"task_id"},
	}
}
func (TaskStopTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	task, ok := getBackgroundTask(p.TaskID)
	if !ok {
		return "", fmt.Errorf("background task %q not found", p.TaskID)
	}
	if err := task.stop(); err != nil {
		return "", err
	}
	return fmt.Sprintf("Stopped background task %s", p.TaskID), nil
}
