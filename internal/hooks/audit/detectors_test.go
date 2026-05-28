package audit

import (
	"testing"
	"time"
)

func newEvent(toolName string, input map[string]interface{}) ToolEvent {
	return ToolEvent{
		ToolName:  toolName,
		ToolInput: input,
		CWD:       "/home/user/project",
		Timestamp: time.Now(),
		SessionID: "test-session",
	}
}

func TestRedundantCdCwd(t *testing.T) {
	tests := []struct {
		name    string
		event   ToolEvent
		wantHit bool
	}{
		{
			name:    "redundant cd to same cwd",
			event:   newEvent("Bash", map[string]interface{}{"command": `cd /home/user/project && git status`}),
			wantHit: true,
		},
		{
			name:    "cd to different directory",
			event:   newEvent("Bash", map[string]interface{}{"command": `cd /tmp && ls`}),
			wantHit: false,
		},
		{
			name:    "no cd prefix",
			event:   newEvent("Bash", map[string]interface{}{"command": `git status`}),
			wantHit: false,
		},
		{
			name:    "non-bash tool",
			event:   newEvent("Read", map[string]interface{}{"command": `cd /home/user/project && cat file`}),
			wantHit: false,
		},
		{
			name:    "cd with quotes",
			event:   newEvent("Bash", map[string]interface{}{"command": `cd "/home/user/project" && make test`}),
			wantHit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hit := RedundantCdCwd.Detect(tt.event, make(DetectorSessionState))
			if (hit != nil) != tt.wantHit {
				t.Errorf("got hit=%v, want hit=%v", hit != nil, tt.wantHit)
			}
		})
	}
}

func TestPreferEditOverReadCat(t *testing.T) {
	tests := []struct {
		name    string
		event   ToolEvent
		wantHit bool
	}{
		{
			name:    "cat source file",
			event:   newEvent("Bash", map[string]interface{}{"command": `cat src/main.go`}),
			wantHit: true,
		},
		{
			name:    "head with flag",
			event:   newEvent("Bash", map[string]interface{}{"command": `head -n 20 file.ts`}),
			wantHit: true,
		},
		{
			name:    "cat with pipeline",
			event:   newEvent("Bash", map[string]interface{}{"command": `cat file.go | grep func`}),
			wantHit: false,
		},
		{
			name:    "cat .env file",
			event:   newEvent("Bash", map[string]interface{}{"command": `cat .env`}),
			wantHit: false,
		},
		{
			name:    "non-source file",
			event:   newEvent("Bash", map[string]interface{}{"command": `cat data.bin`}),
			wantHit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hit := PreferEditOverReadCat.Detect(tt.event, make(DetectorSessionState))
			if (hit != nil) != tt.wantHit {
				t.Errorf("got hit=%v, want hit=%v", hit != nil, tt.wantHit)
			}
		})
	}
}

func TestSleepPollingLoop(t *testing.T) {
	tests := []struct {
		name    string
		event   ToolEvent
		wantHit bool
	}{
		{
			name:    "long sleep",
			event:   newEvent("Bash", map[string]interface{}{"command": `sleep 60`}),
			wantHit: true,
		},
		{
			name:    "short sleep",
			event:   newEvent("Bash", map[string]interface{}{"command": `sleep 5`}),
			wantHit: false,
		},
		{
			name:    "while-sleep loop",
			event:   newEvent("Bash", map[string]interface{}{"command": `while true; do check; sleep 10; done`}),
			wantHit: true,
		},
		{
			name:    "sleep with unit",
			event:   newEvent("Bash", map[string]interface{}{"command": `sleep 1m`}),
			wantHit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hit := SleepPollingLoop.Detect(tt.event, make(DetectorSessionState))
			if (hit != nil) != tt.wantHit {
				t.Errorf("got hit=%v, want hit=%v", hit != nil, tt.wantHit)
			}
		})
	}
}

func TestFindFromRoot(t *testing.T) {
	tests := []struct {
		name    string
		event   ToolEvent
		wantHit bool
	}{
		{
			name:    "find from root",
			event:   newEvent("Bash", map[string]interface{}{"command": `find / -name "*.go"`}),
			wantHit: true,
		},
		{
			name:    "find from /home",
			event:   newEvent("Bash", map[string]interface{}{"command": `find /home -type f`}),
			wantHit: true,
		},
		{
			name:    "find in project",
			event:   newEvent("Bash", map[string]interface{}{"command": `find . -name "*.ts"`}),
			wantHit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hit := FindFromRoot.Detect(tt.event, make(DetectorSessionState))
			if (hit != nil) != tt.wantHit {
				t.Errorf("got hit=%v, want hit=%v", hit != nil, tt.wantHit)
			}
		})
	}
}

func TestGitCommitNoVerify(t *testing.T) {
	tests := []struct {
		name    string
		event   ToolEvent
		wantHit bool
	}{
		{
			name:    "commit with --no-verify",
			event:   newEvent("Bash", map[string]interface{}{"command": `git commit -m "fix" --no-verify`}),
			wantHit: true,
		},
		{
			name:    "commit with -n",
			event:   newEvent("Bash", map[string]interface{}{"command": `git commit -m "fix" -n`}),
			wantHit: true,
		},
		{
			name:    "normal commit",
			event:   newEvent("Bash", map[string]interface{}{"command": `git commit -m "fix"`}),
			wantHit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hit := GitCommitNoVerify.Detect(tt.event, make(DetectorSessionState))
			if (hit != nil) != tt.wantHit {
				t.Errorf("got hit=%v, want hit=%v", hit != nil, tt.wantHit)
			}
		})
	}
}

func TestRereadAfterEdit(t *testing.T) {
	state := make(DetectorSessionState)
	path := "/home/user/project/main.go"

	// Read without prior edit - no hit
	readEvent := newEvent("Read", map[string]interface{}{"file_path": path})
	hit := RereadAfterEdit.Detect(readEvent, state)
	if hit != nil {
		t.Fatal("unexpected hit on first read")
	}

	// Edit - no hit
	editEvent := newEvent("Edit", map[string]interface{}{"file_path": path})
	hit = RereadAfterEdit.Detect(editEvent, state)
	if hit != nil {
		t.Fatal("unexpected hit on edit")
	}

	// Read immediately after edit - hit!
	hit = RereadAfterEdit.Detect(readEvent, state)
	if hit == nil {
		t.Fatal("expected hit on re-read after edit")
	}
	if hit.DetectorName != "reread-after-edit" {
		t.Errorf("wrong detector name: %s", hit.DetectorName)
	}
}

func TestAllDetectorsRegistered(t *testing.T) {
	detectors := AllDetectors()
	if len(detectors) != 8 {
		t.Errorf("expected 8 detectors, got %d", len(detectors))
	}
	names := make(map[string]bool)
	for _, d := range detectors {
		names[d.Name] = true
	}
	expected := []string{
		"redundant-cd-cwd",
		"prefer-edit-over-read-cat",
		"prefer-edit-over-sed-awk",
		"prefer-write-over-heredoc",
		"sleep-polling-loop",
		"find-from-root",
		"git-commit-no-verify",
		"reread-after-edit",
	}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing detector: %s", name)
		}
	}
}
