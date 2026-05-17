//go:build darwin

package sandbox

import (
	"context"
	"strings"
	"testing"
)

func TestGenerateSeatbeltProfile_DenyDefault(t *testing.T) {
	policy := &SeatbeltPolicy{}
	profile := GenerateSeatbeltProfile(policy)

	if !strings.Contains(profile, "(version 1)") {
		t.Error("profile should start with (version 1)")
	}
	if !strings.Contains(profile, "(deny default)") {
		t.Error("profile should deny by default")
	}
}

func TestGenerateSeatbeltProfile_NetworkToggle(t *testing.T) {
	// Network allowed
	policy := &SeatbeltPolicy{AllowNetwork: true}
	profile := GenerateSeatbeltProfile(policy)
	if !strings.Contains(profile, "(allow network*)") {
		t.Error("profile with AllowNetwork=true should contain (allow network*)")
	}

	// Network denied
	policy = &SeatbeltPolicy{AllowNetwork: false}
	profile = GenerateSeatbeltProfile(policy)
	if strings.Contains(profile, "(allow network*)") {
		t.Error("profile with AllowNetwork=false should not contain (allow network*)")
	}
}

func TestGenerateSeatbeltProfile_ReadPaths(t *testing.T) {
	policy := &SeatbeltPolicy{
		ReadablePaths: []string{"/usr", "/bin", "/tmp"},
	}
	profile := GenerateSeatbeltProfile(policy)

	if !strings.Contains(profile, `(allow file-read* (subpath "/usr"))`) {
		t.Error("profile should allow reading /usr")
	}
	if !strings.Contains(profile, `(allow file-read* (subpath "/bin"))`) {
		t.Error("profile should allow reading /bin")
	}
	if !strings.Contains(profile, `(allow file-read* (subpath "/tmp"))`) {
		t.Error("profile should allow reading /tmp")
	}
}

func TestGenerateSeatbeltProfile_WritePaths(t *testing.T) {
	policy := &SeatbeltPolicy{
		AllowWrite:    true,
		WritablePaths: []string{"/tmp", "/home/user/project"},
	}
	profile := GenerateSeatbeltProfile(policy)

	if !strings.Contains(profile, `(allow file-write* (subpath "/tmp"))`) {
		t.Error("profile should allow writing to /tmp")
	}
	if !strings.Contains(profile, `(allow file-write* (subpath "/home/user/project"))`) {
		t.Error("profile should allow writing to /home/user/project")
	}
}

func TestGenerateSeatbeltProfile_WriteDisabled(t *testing.T) {
	policy := &SeatbeltPolicy{
		AllowWrite:    false,
		WritablePaths: []string{"/tmp"},
	}
	profile := GenerateSeatbeltProfile(policy)

	if strings.Contains(profile, "(allow file-write*") {
		t.Error("profile with AllowWrite=false should not contain any file-write rules")
	}
}

func TestGenerateSeatbeltProfile_ProcessExec(t *testing.T) {
	policy := &SeatbeltPolicy{AllowProcess: true}
	profile := GenerateSeatbeltProfile(policy)
	if !strings.Contains(profile, "(allow process-exec*)") {
		t.Error("profile with AllowProcess=true should allow process-exec*")
	}

	policy = &SeatbeltPolicy{AllowProcess: false}
	profile = GenerateSeatbeltProfile(policy)
	if strings.Contains(profile, "(allow process-exec*)") {
		t.Error("profile with AllowProcess=false should not allow process-exec*")
	}
}

func TestGenerateSeatbeltProfile_Sysctl(t *testing.T) {
	policy := &SeatbeltPolicy{AllowSysctl: true}
	profile := GenerateSeatbeltProfile(policy)
	if !strings.Contains(profile, "(allow sysctl-read)") {
		t.Error("profile with AllowSysctl=true should allow sysctl-read")
	}

	policy = &SeatbeltPolicy{AllowSysctl: false}
	profile = GenerateSeatbeltProfile(policy)
	if strings.Contains(profile, "(allow sysctl-read)") {
		t.Error("profile with AllowSysctl=false should not allow sysctl-read")
	}
}

func TestDefaultHawkPolicy_IncludesWorkDir(t *testing.T) {
	workDir := "/Users/dev/myproject"
	policy := DefaultHawkPolicy(workDir)

	found := false
	for _, p := range policy.ReadablePaths {
		if p == workDir {
			found = true
			break
		}
	}
	if !found {
		t.Error("DefaultHawkPolicy should include workDir in ReadablePaths")
	}

	found = false
	for _, p := range policy.WritablePaths {
		if p == workDir {
			found = true
			break
		}
	}
	if !found {
		t.Error("DefaultHawkPolicy should include workDir in WritablePaths")
	}
}

func TestDefaultHawkPolicy_NetworkAllowed(t *testing.T) {
	policy := DefaultHawkPolicy("/tmp/work")
	if !policy.AllowNetwork {
		t.Error("DefaultHawkPolicy should allow network by default")
	}
}

func TestDefaultHawkPolicy_ProcessAllowed(t *testing.T) {
	policy := DefaultHawkPolicy("/tmp/work")
	if !policy.AllowProcess {
		t.Error("DefaultHawkPolicy should allow process execution by default")
	}
}

func TestDefaultHawkPolicy_ProfileProducesValidSBPL(t *testing.T) {
	workDir := "/tmp/testproject"
	policy := DefaultHawkPolicy(workDir)
	profile := GenerateSeatbeltProfile(policy)

	// Check SBPL structure requirements
	if !strings.HasPrefix(profile, "(version 1)\n") {
		t.Error("generated profile must start with (version 1)")
	}
	if !strings.Contains(profile, "(deny default)") {
		t.Error("generated profile must contain (deny default)")
	}
	// Every opening paren should have a matching close
	opens := strings.Count(profile, "(")
	closes := strings.Count(profile, ")")
	if opens != closes {
		t.Errorf("unbalanced parentheses: %d opens vs %d closes", opens, closes)
	}
}

func TestRunSeatbelted_ReturnsCmd(t *testing.T) {
	policy := &SeatbeltPolicy{
		AllowNetwork:  false,
		AllowWrite:    false,
		AllowProcess:  true,
		AllowSysctl:   true,
		ReadablePaths: []string{"/usr", "/bin"},
	}

	cmd, err := RunSeatbelted(context.Background(), "echo hello", policy)
	if err != nil {
		t.Fatalf("RunSeatbelted returned error: %v", err)
	}
	if cmd == nil {
		t.Fatal("RunSeatbelted returned nil cmd")
	}
	if cmd.Path == "" {
		t.Error("cmd.Path should not be empty")
	}
	// Verify it uses sandbox-exec
	if !strings.Contains(cmd.Path, "sandbox-exec") {
		t.Errorf("expected sandbox-exec in path, got %q", cmd.Path)
	}
}

func TestRunSeatbelted_ProfileFileFlag(t *testing.T) {
	policy := &SeatbeltPolicy{
		AllowProcess:  true,
		ReadablePaths: []string{"/tmp"},
	}

	cmd, err := RunSeatbelted(context.Background(), "true", policy)
	if err != nil {
		t.Fatalf("RunSeatbelted returned error: %v", err)
	}

	// Args should be: sandbox-exec -f <file> bash -c <command>
	args := cmd.Args
	if len(args) < 6 {
		t.Fatalf("expected at least 6 args, got %d: %v", len(args), args)
	}
	if args[1] != "-f" {
		t.Errorf("expected args[1] = -f, got %q", args[1])
	}
	if args[3] != "bash" {
		t.Errorf("expected args[3] = bash, got %q", args[3])
	}
	if args[4] != "-c" {
		t.Errorf("expected args[4] = -c, got %q", args[4])
	}
	if args[5] != "true" {
		t.Errorf("expected args[5] = true, got %q", args[5])
	}
}
