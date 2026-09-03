package lifecycle

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/intelligence/memory"
)

type memoryOp struct {
	Op      string `json:"op"`
	Type    string `json:"type"`
	Content string `json:"content"`
}

// ErrNoMemoryOps indicates the LLM response contained no JSON array of ops.
var ErrNoMemoryOps = errors.New("no memory ops array in response")

// ParseAndApplyMemoryOps parses the LLM's JSON response and applies memory operations via harrier.
// Every failure mode is surfaced as an error so callers can log and observe the memory loop.
func ParseAndApplyMemoryOps(bridge *memory.HarrierBridge, response string) error {
	if bridge == nil {
		return errors.New("memory ops: harrier bridge is nil")
	}
	// Extract JSON array from response (may have surrounding text)
	start := strings.Index(response, "[")
	end := strings.LastIndex(response, "]")
	if start < 0 || end < 0 || end <= start {
		return ErrNoMemoryOps
	}
	var ops []memoryOp
	if err := json.Unmarshal([]byte(response[start:end+1]), &ops); err != nil {
		return fmt.Errorf("memory ops: parse json: %w", err)
	}
	var errs []error
	for _, op := range ops {
		if op.Content == "" {
			continue
		}
		switch op.Op {
		case "add":
			if err := bridge.Remember(op.Content, op.Type); err != nil {
				errs = append(errs, fmt.Errorf("memory ops: remember: %w", err))
			}
		}
	}
	return errors.Join(errs...)
}
