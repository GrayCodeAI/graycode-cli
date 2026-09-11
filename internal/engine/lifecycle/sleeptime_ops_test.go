package lifecycle

import (
	"errors"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
)

func TestParseAndApplyMemoryOpsNilBridge(t *testing.T) {
	if err := ParseAndApplyMemoryOps(nil, `[{"op":"add","content":"x"}]`); err == nil {
		t.Fatal("expected error for nil bridge")
	}
}

func TestParseAndApplyMemoryOpsNoArray(t *testing.T) {
	if err := ParseAndApplyMemoryOps(&memory.HarrierBridge{}, "no ops here"); !errors.Is(err, ErrNoMemoryOps) {
		t.Fatalf("expected ErrNoMemoryOps, got %v", err)
	}
}

func TestParseAndApplyMemoryOpsMalformedJSON(t *testing.T) {
	err := ParseAndApplyMemoryOps(&memory.HarrierBridge{}, `prefix [{"op":"add","content":"x"} suffix`)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParseAndApplyMemoryOpsSurroundingText(t *testing.T) {
	bridge := &memory.HarrierBridge{}
	// Bridge is uninitialized, so Remember fails with a BridgeError; the
	// error must be surfaced, proving the ops were parsed and applied.
	err := ParseAndApplyMemoryOps(bridge, "Here are the ops:\n[{\"op\":\"add\",\"content\":\"lesson one\",\"type\":\"convention\"}]\nThat is all.")
	if err == nil {
		t.Fatal("expected Remember error from uninitialized bridge")
	}
	if !strings.Contains(err.Error(), "remember") {
		t.Fatalf("expected remember error to be wrapped, got: %v", err)
	}
}

func TestParseAndApplyMemoryOpsSkipsEmptyContent(t *testing.T) {
	// Empty content ops are skipped; a not-ready bridge still returns nil
	// because nothing was remembered.
	if err := ParseAndApplyMemoryOps(&memory.HarrierBridge{}, `[{"op":"add","content":""}]`); err != nil {
		t.Fatalf("expected no error for empty content, got %v", err)
	}
}

func TestParseAndApplyMemoryOpsUnknownOpIgnored(t *testing.T) {
	if err := ParseAndApplyMemoryOps(&memory.HarrierBridge{}, `[{"op":"delete","content":"x"}]`); err != nil {
		t.Fatalf("expected unknown op to be a no-op, got %v", err)
	}
}

func TestParseAndApplyMemoryOpsEmptyArray(t *testing.T) {
	if err := ParseAndApplyMemoryOps(&memory.HarrierBridge{}, `[]`); err != nil {
		t.Fatalf("expected empty array to succeed, got %v", err)
	}
}
