package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoragePolicyHelpersDoNotCreateProjectGraycode(t *testing.T) {
	project := t.TempDir()
	t.Setenv("GRAYCODE_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	t.Setenv("GRAYCODE_CACHE_DIR", filepath.Join(t.TempDir(), "cache"))
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}

	planPath := resolvePlanPath("demo")
	if strings.Contains(planPath, filepath.Join(project, ".graycode")) {
		t.Fatalf("resolvePlanPath leaked project .graycode: %q", planPath)
	}
	saveInputHistory([]string{"hello"})
	recordTipShown("slash-help")

	if _, err := os.Stat(filepath.Join(project, ".graycode")); !os.IsNotExist(err) {
		t.Fatalf("normal storage helpers created project .graycode, stat err=%v", err)
	}
}
