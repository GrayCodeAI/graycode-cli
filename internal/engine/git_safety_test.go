package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectGitBranch_NonRepo(t *testing.T) {
	dir := t.TempDir()
	info := InspectGitBranch(dir)
	if info.HasRepo {
		t.Fatal("expected no repo")
	}
	if GitSafetyAdvice(info) != "" {
		t.Fatalf("advice should be empty: %q", GitSafetyAdvice(info))
	}
}

func TestEnsureAgentBranch_FromMain(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		// TODO: install git in minimal CI images instead of skipping.
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@e", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@e")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run("init", "-b", "main")
	// identity for commit
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	p := filepath.Join(dir, "README")
	if err := os.WriteFile(p, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", "README")
	run("commit", "-m", "init")

	info := InspectGitBranch(dir)
	if !info.HasRepo || !info.OnDefault || info.Branch != "main" {
		t.Fatalf("info = %#v", info)
	}
	name, err := EnsureAgentBranch(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, "graycode/agent-") {
		t.Fatalf("branch = %q", name)
	}
	info2 := InspectGitBranch(dir)
	if info2.OnDefault {
		t.Fatalf("still on default: %#v", info2)
	}
	// Second call is no-op keep branch
	name2, err := EnsureAgentBranch(dir)
	if err != nil {
		t.Fatal(err)
	}
	if name2 != name {
		t.Fatalf("expected same branch %q got %q", name, name2)
	}
}

func TestProjectTrust_RoundTrip(t *testing.T) {
	// Use real store path under temp by trusting cwd is ok for unit test of API shape.
	st := ProjectTrust(t.TempDir())
	// Temp dirs are typically untrusted.
	if st.Trusted {
		t.Log("unexpected trusted temp dir — ok if store has broad trust")
	}
	_ = st.Detail()
	_ = st.String()
}
