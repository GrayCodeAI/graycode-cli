package lsp

import (
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestKillProcessTree_Nil(t *testing.T) {
	if err := KillProcessTree(nil); err != nil {
		t.Errorf("expected nil error for nil cmd, got %v", err)
	}

	cmd := &exec.Cmd{}
	if err := KillProcessTree(cmd); err != nil {
		t.Errorf("expected nil error for cmd without process, got %v", err)
	}
}

func TestKillProcessTree_RunningSubprocess(t *testing.T) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-Command", "Start-Sleep -Seconds 10")
	} else {
		cmd = exec.Command("sleep", "10")
	}

	prepareCmdSysProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start test process: %v", err)
	}

	pid := cmd.Process.Pid
	if !isProcessAlive(pid) {
		t.Fatalf("expected process %d to be alive after start", pid)
	}

	err := KillProcessTree(cmd)
	if err != nil {
		t.Errorf("KillProcessTree returned error: %v", err)
	}

	quiescent := WaitForProcessQuiescence(pid, 2*time.Second)
	if !quiescent {
		t.Errorf("process %d was still alive after KillProcessTree", pid)
	}
}

func TestWaitForProcessQuiescence_InvalidPID(t *testing.T) {
	// PID <= 0 returns true immediately
	if !WaitForProcessQuiescence(0, 100*time.Millisecond) {
		t.Error("expected true for PID 0")
	}
	if !WaitForProcessQuiescence(-1, 100*time.Millisecond) {
		t.Error("expected true for PID -1")
	}
}

func TestIsProcessAlive_DeadPID(t *testing.T) {
	// A non-existent high PID should report false on Unix
	if runtime.GOOS != "windows" {
		if isProcessAlive(9999999) {
			t.Error("expected PID 9999999 to not be alive")
		}
	}
}
