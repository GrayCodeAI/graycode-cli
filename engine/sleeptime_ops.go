package engine

import (
	"encoding/json"
	"strings"

	"github.com/GrayCodeAI/hawk/memory"
)

type memoryOp struct {
	Op      string `json:"op"`
	Type    string `json:"type"`
	Content string `json:"content"`
}

// parseAndApplyMemoryOps parses the LLM's JSON response and applies memory operations via yaad.
func parseAndApplyMemoryOps(bridge *memory.YaadBridge, response string) {
	// Extract JSON array from response (may have surrounding text)
	start := strings.Index(response, "[")
	end := strings.LastIndex(response, "]")
	if start < 0 || end < 0 || end <= start {
		return
	}
	var ops []memoryOp
	if err := json.Unmarshal([]byte(response[start:end+1]), &ops); err != nil {
		return
	}
	for _, op := range ops {
		if op.Content == "" {
			continue
		}
		switch op.Op {
		case "add":
			bridge.Remember(op.Content, op.Type)
		}
	}
}
