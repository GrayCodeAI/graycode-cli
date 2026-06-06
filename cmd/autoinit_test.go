package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/autoinit"
)

// TestAutoInitRunner_WritesContextFileOnce verifies the cmd-layer runner
// produces a recognized context file and that MaybeRun invokes it exactly once
// for a fresh project, then gates on the marker thereafter.
func TestAutoInitRunner_WritesContextFileOnce(t *testing.T) {
	root := t.TempDir()
	// A trivial Go file so BuildHierarchy has something to summarize.
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc Foo() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	calls := 0
	wrapped := func(ctx context.Context, r string) error {
		calls++
		return autoInitRunner(ctx, r)
	}

	dec, err := autoinit.MaybeRun(context.Background(), autoinit.Options{Root: root, Run: wrapped})
	if err != nil {
		t.Fatalf("first MaybeRun: %v", err)
	}
	if !dec.Ran {
		t.Fatalf("expected runner to run on fresh project, got %+v", dec)
	}
	if calls != 1 {
		t.Fatalf("expected 1 runner call, got %d", calls)
	}

	ctxFile := filepath.Join(root, autoInitContextFile)
	if _, err := os.Stat(ctxFile); err != nil {
		t.Fatalf("expected context file %s to be written: %v", autoInitContextFile, err)
	}

	// Second invocation: marker present -> no second call.
	dec, err = autoinit.MaybeRun(context.Background(), autoinit.Options{Root: root, Run: wrapped})
	if err != nil {
		t.Fatalf("second MaybeRun: %v", err)
	}
	if dec.Ran || calls != 1 {
		t.Fatalf("expected gated skip on second run, got %+v calls=%d", dec, calls)
	}
}

// TestAutoInitRunner_NoClobber ensures the runner never overwrites an existing
// context file even if invoked directly.
func TestAutoInitRunner_NoClobber(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, autoInitContextFile)
	if err := os.WriteFile(existing, []byte("# keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := autoInitRunner(context.Background(), root); err != nil {
		t.Fatalf("runner: %v", err)
	}
	got, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# keep me\n" {
		t.Errorf("runner clobbered existing context file: %q", string(got))
	}
}
