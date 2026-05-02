package sandbox

import (
	"context"
	"strings"
	"testing"
)

func TestParseMode(t *testing.T) {
	tests := []struct {
		input string
		want  Mode
	}{
		{"strict", ModeStrict},
		{"workspace", ModeWorkspace},
		{"off", ModeOff},
		{"", ModeOff},
		{"unknown", ModeOff},
	}
	for _, tt := range tests {
		got := ParseMode(tt.input)
		if got != tt.want {
			t.Errorf("ParseMode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGenerateProfileStrict(t *testing.T) {
	cfg := SandboxConfig{Mode: ModeStrict, AllowNetwork: true}
	profile := GenerateProfile(cfg)
	if !strings.Contains(profile, "(deny default)") {
		t.Error("strict profile should contain (deny default)")
	}
	if !strings.Contains(profile, "(allow file-read*)") {
		t.Error("strict profile should allow file-read")
	}
	if !strings.Contains(profile, "(allow network*)") {
		t.Error("strict profile with AllowNetwork should allow network")
	}
	if strings.Contains(profile, "(allow file-write") {
		t.Error("strict profile should not allow file-write")
	}
}

func TestGenerateProfileStrictNoNetwork(t *testing.T) {
	cfg := SandboxConfig{Mode: ModeStrict, AllowNetwork: false}
	profile := GenerateProfile(cfg)
	if strings.Contains(profile, "(allow network*)") {
		t.Error("strict profile without AllowNetwork should not allow network")
	}
}

func TestGenerateProfileWorkspace(t *testing.T) {
	cfg := SandboxConfig{
		Mode:         ModeWorkspace,
		WorkspaceDir: "/home/user/project",
		AllowNetwork: true,
	}
	profile := GenerateProfile(cfg)
	if !strings.Contains(profile, "(deny default)") {
		t.Error("workspace profile should contain (deny default)")
	}
	if !strings.Contains(profile, `(allow file-write* (subpath "/home/user/project"))`) {
		t.Error("workspace profile should allow writing to workspace dir")
	}
	if !strings.Contains(profile, `(allow file-write* (subpath "/tmp"))`) {
		t.Error("workspace profile should allow writing to /tmp")
	}
	if !strings.Contains(profile, `(allow file-write* (subpath "/private/tmp"))`) {
		t.Error("workspace profile should allow writing to /private/tmp")
	}
}

func TestGenerateProfileOff(t *testing.T) {
	cfg := SandboxConfig{Mode: ModeOff}
	profile := GenerateProfile(cfg)
	if !strings.Contains(profile, "(allow default)") {
		t.Error("off profile should be permissive")
	}
}

func TestWrapCommandOff(t *testing.T) {
	cfg := SandboxConfig{Mode: ModeOff}
	exe, args := WrapCommand("echo hello", cfg)
	if exe != "bash" {
		t.Errorf("WrapCommand off: exe = %q, want bash", exe)
	}
	if len(args) != 2 || args[0] != "-c" || args[1] != "echo hello" {
		t.Errorf("WrapCommand off: args = %v, unexpected", args)
	}
}

func TestWrapCommandStrict(t *testing.T) {
	cfg := SandboxConfig{Mode: ModeStrict, AllowNetwork: true}
	exe, args := WrapCommand("ls /", cfg)
	if exe != "sandbox-exec" {
		t.Errorf("WrapCommand strict: exe = %q, want sandbox-exec", exe)
	}
	if len(args) < 4 {
		t.Fatalf("WrapCommand strict: too few args: %v", args)
	}
	if args[0] != "-p" {
		t.Errorf("WrapCommand strict: args[0] = %q, want -p", args[0])
	}
	// args[1] is the profile, args[2] is bash, args[3] is -c, args[4] is the command
	if args[2] != "bash" || args[3] != "-c" || args[4] != "ls /" {
		t.Errorf("WrapCommand strict: unexpected args tail: %v", args[2:])
	}
}

func TestWrapCommandWorkspace(t *testing.T) {
	cfg := SandboxConfig{Mode: ModeWorkspace, WorkspaceDir: "/tmp/proj", AllowNetwork: false}
	exe, args := WrapCommand("touch /tmp/proj/file", cfg)
	if exe != "sandbox-exec" {
		t.Errorf("WrapCommand workspace: exe = %q, want sandbox-exec", exe)
	}
	profile := args[1]
	if !strings.Contains(profile, `(subpath "/tmp/proj")`) {
		t.Error("workspace wrap should include workspace subpath in profile")
	}
}

func TestModeFromContextDefault(t *testing.T) {
	ctx := context.Background()
	if m := ModeFromContext(ctx); m != ModeOff {
		t.Errorf("ModeFromContext on empty ctx = %q, want %q", m, ModeOff)
	}
}

func TestContextWithMode(t *testing.T) {
	ctx := ContextWithMode(context.Background(), ModeStrict)
	if m := ModeFromContext(ctx); m != ModeStrict {
		t.Errorf("ModeFromContext = %q, want %q", m, ModeStrict)
	}
}
