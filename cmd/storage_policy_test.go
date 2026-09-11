package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoragePolicyHelpersDoNotCreateProjectHawk(t *testing.T) {
	project := t.TempDir()
	t.Setenv("HAWK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	t.Setenv("HAWK_CACHE_DIR", filepath.Join(t.TempDir(), "cache"))
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}

	planPath := resolvePlanPath("demo")
	if strings.Contains(planPath, filepath.Join(project, ".hawk")) {
		t.Fatalf("resolvePlanPath leaked project .hawk: %q", planPath)
	}
	saveInputHistory([]string{"hello"})
	recordTipShown("slash-help")

	if _, err := os.Stat(filepath.Join(project, ".hawk")); !os.IsNotExist(err) {
		t.Fatalf("normal storage helpers created project .hawk, stat err=%v", err)
	}
}
