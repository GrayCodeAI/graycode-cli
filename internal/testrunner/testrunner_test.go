package testrunner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectGo(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	checks, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range checks {
		if c.ID == "go.test" {
			found = true
			if c.Kind != KindTest || c.Framework != FrameworkGo {
				t.Fatalf("go.test check wrong: %+v", c)
			}
		}
	}
	if !found {
		t.Fatalf("go.test not detected: %+v", checks)
	}
}

func TestDetectNodeScripts(t *testing.T) {
	root := t.TempDir()
	pkg := `{"scripts":{"test":"vitest run","typecheck":"tsc --noEmit","lint":"eslint ."}}`
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(pkg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package-lock.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	checks, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, c := range checks {
		ids = append(ids, c.ID)
	}
	for _, want := range []string{"npm.test", "npm.typecheck", "npm.lint"} {
		if !contains(ids, want) {
			t.Fatalf("missing %s in %v", want, ids)
		}
	}
}

func TestDetectRejectsNonDir(t *testing.T) {
	if _, err := Detect(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected error for missing root")
	}
}

func TestParseGoSummary(t *testing.T) {
	out := `=== RUN   TestOne
--- PASS: TestOne (0.00s)
=== RUN   TestTwo
--- FAIL: TestTwo (0.00s)
    foo_test.go:12: expected x got y
ok  	example.com/pkg
FAIL`
	check := Check{Framework: FrameworkGo}
	summary := ParseSummary(check, out, "")
	if summary == nil {
		t.Fatal("expected summary")
	}
	if summary.Passed != 1 || summary.Failed != 1 {
		t.Fatalf("passed=%d failed=%d", summary.Passed, summary.Failed)
	}
	if len(summary.Failures) != 1 || summary.Failures[0].Name != "TestTwo" {
		t.Fatalf("failures = %+v", summary.Failures)
	}
	if summary.Failures[0].File != "foo_test.go:12" {
		t.Fatalf("failure file = %q", summary.Failures[0].File)
	}
}

func TestParseCargoSummary(t *testing.T) {
	out := `test add ... ok
test sub ... FAILED
test ignored_test ... ignored
`
	check := Check{Framework: FrameworkCargo}
	summary := ParseSummary(check, out, "")
	if summary == nil {
		t.Fatal("expected summary")
	}
	if summary.Passed != 1 || summary.Failed != 1 || summary.Skipped != 1 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestParseSummaryEmptyIsNil(t *testing.T) {
	if ParseSummary(Check{Framework: FrameworkGo}, "", "") != nil {
		t.Fatal("expected nil for empty output")
	}
}

func TestParseNodeSummaryCounts(t *testing.T) {
	out := `not ok 1 - broke
1..3
# tests 3
# pass 2
# fail 1`
	summary := ParseSummary(Check{Framework: FrameworkNode}, out, "")
	if summary == nil {
		t.Fatal("expected summary")
	}
	if summary.Failed == 0 || len(summary.Failures) == 0 {
		t.Fatalf("summary = %+v", summary)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
