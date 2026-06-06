package autoinit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMaybeRun_RunsOnceThenSkips(t *testing.T) {
	root := t.TempDir()
	calls := 0
	run := func(ctx context.Context, r string) error {
		if r != root {
			t.Errorf("runner root = %q, want %q", r, root)
		}
		calls++
		return nil
	}

	// First invocation in an empty project: should run.
	dec, err := MaybeRun(context.Background(), Options{Root: root, Run: run})
	if err != nil {
		t.Fatalf("first MaybeRun: %v", err)
	}
	if !dec.Ran {
		t.Fatalf("first run: expected Ran=true, got %+v", dec)
	}
	if calls != 1 {
		t.Fatalf("expected 1 runner call, got %d", calls)
	}
	if !HasRun(root) {
		t.Fatal("expected marker file after first run")
	}

	// Second invocation: marker present -> skip without calling runner again.
	dec, err = MaybeRun(context.Background(), Options{Root: root, Run: run})
	if err != nil {
		t.Fatalf("second MaybeRun: %v", err)
	}
	if dec.Ran {
		t.Errorf("second run: expected Ran=false, got %+v", dec)
	}
	if calls != 1 {
		t.Errorf("runner should not be called again, got %d calls", calls)
	}
	if dec.Skipped == "" {
		t.Error("expected a skip reason on second run")
	}
}

func TestMaybeRun_SkipsWhenContextExists(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# x"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	run := func(ctx context.Context, r string) error { calls++; return nil }

	dec, err := MaybeRun(context.Background(), Options{Root: root, Run: run})
	if err != nil {
		t.Fatalf("MaybeRun: %v", err)
	}
	if dec.Ran || calls != 0 {
		t.Errorf("expected skip when context exists, got %+v calls=%d", dec, calls)
	}
	// Marker should still be written so we never probe again.
	if !HasRun(root) {
		t.Error("expected marker even when skipping due to existing context")
	}
}

func TestMaybeRun_Disabled(t *testing.T) {
	root := t.TempDir()
	calls := 0
	run := func(ctx context.Context, r string) error { calls++; return nil }

	dec, err := MaybeRun(context.Background(), Options{
		Root:            root,
		Run:             run,
		disableEnvValue: "1",
	})
	if err != nil {
		t.Fatalf("MaybeRun: %v", err)
	}
	if dec.Ran || calls != 0 {
		t.Errorf("expected skip when disabled, got %+v calls=%d", dec, calls)
	}
	if HasRun(root) {
		t.Error("disabled auto-init should not write a marker")
	}
}

func TestMaybeRun_ForceIgnoresExistingContext(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "HAWK.md"), []byte("# x"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	run := func(ctx context.Context, r string) error { calls++; return nil }

	dec, err := MaybeRun(context.Background(), Options{Root: root, Run: run, Force: true})
	if err != nil {
		t.Fatalf("MaybeRun: %v", err)
	}
	if !dec.Ran || calls != 1 {
		t.Errorf("force should run despite existing context, got %+v calls=%d", dec, calls)
	}
}

func TestMaybeRun_MarkerWrittenOnRunnerError(t *testing.T) {
	root := t.TempDir()
	run := func(ctx context.Context, r string) error { return context.Canceled }

	dec, err := MaybeRun(context.Background(), Options{Root: root, Run: run})
	if err == nil {
		t.Fatal("expected runner error to propagate")
	}
	if !dec.Ran {
		t.Errorf("expected Ran=true even on error, got %+v", dec)
	}
	if !HasRun(root) {
		t.Error("marker must be written before running so failures are not retried")
	}
}

func TestMaybeRun_NoRunnerStillGates(t *testing.T) {
	root := t.TempDir()
	dec, err := MaybeRun(context.Background(), Options{Root: root})
	if err != nil {
		t.Fatalf("MaybeRun: %v", err)
	}
	if dec.Ran {
		t.Errorf("expected Ran=false with no runner, got %+v", dec)
	}
	if !HasRun(root) {
		t.Error("expected marker written even without a runner")
	}
}

func TestHasContext(t *testing.T) {
	root := t.TempDir()
	if HasContext(root) {
		t.Error("empty dir should not have context")
	}
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !HasContext(root) {
		t.Error("dir with CLAUDE.md should have context")
	}
}
