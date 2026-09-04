package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/engine/token"
	"github.com/GrayCodeAI/graycode-cli/internal/storage"
	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// Tool-catalog shrink, adopted from caveman's toolschema compressor (via
// shrike): large tool catalogs are paid on every request, so when enabled the
// outgoing catalog is compressed with a strict over-keep contract — names,
// types, enums, required, defaults survive byte-for-byte; only annotation
// metadata is dropped and long descriptions reduced to lead+constraint
// sentences. Fail-open throughout: anything that does not round-trip
// cleanly falls back to the original catalog.

// toolShrinkEnabled reports whether opt-in catalog compression is on
// (GRAYCODE_TOOL_SHRINK=1). Default off: existing request bytes unchanged.
func toolShrinkEnabled() bool {
	return strings.EqualFold(os.Getenv("GRAYCODE_TOOL_SHRINK"), "1")
}

// originalsDir stores pre-shrink catalogs keyed by content hash so the exact
// original surface stays recoverable for debugging and diffing.
func originalsDir() string {
	return filepath.Join(storage.StateDir(), "tool-catalog-originals")
}

// shrinkGraycodeRouterTools compresses the graycode tool list via shrike's toolschema
// compressor. The list is converted to the OpenAI function-catalog wire shape,
// shrunk, and converted back; any name-set mismatch fails open to input.
// When compression changed something, the original catalog is persisted under
// the state dir keyed by content hash before the shrunk form is returned.
func shrinkGraycodeRouterTools(tools []types.GraycodeRouterTool) []types.GraycodeRouterTool {
	if len(tools) == 0 || !toolShrinkEnabled() {
		return tools
	}

	type wireTool struct {
		Type     string                   `json:"type"`
		Function types.GraycodeRouterTool `json:"function"`
	}
	wire := make([]wireTool, len(tools))
	for i := range tools {
		wire[i] = wireTool{Type: "function", Function: tools[i]}
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		return tools
	}

	shrunk, changed := token.ShrinkToolCatalog(string(raw))
	if !changed {
		return tools
	}
	var shrunkWire []wireTool
	if err := json.Unmarshal([]byte(shrunk), &shrunkWire); err != nil {
		return tools
	}
	if len(shrunkWire) != len(tools) {
		return tools // structural drift: never risk it
	}
	out := make([]types.GraycodeRouterTool, len(tools))
	for i := range shrunkWire {
		if shrunkWire[i].Function.Name != tools[i].Name {
			slog.Debug("tool shrink name drift, failing open", "position", i)
			return tools
		}
		out[i] = shrunkWire[i].Function
	}

	persistOriginalCatalog(raw)
	slog.Info(
		"tool catalog shrunk",
		"tools", len(tools),
		"bytes_before", len(raw),
		"bytes_after", len(shrunk),
	)
	return out
}

// persistOriginalCatalog writes the pre-shrink catalog once per content hash.
func persistOriginalCatalog(raw []byte) {
	sum := sha256.Sum256(raw)
	name := hex.EncodeToString(sum[:8]) + ".json"
	dir := originalsDir()
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		return
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return
	}
	_ = os.WriteFile(path, raw, 0o600) // #nosec G306 -- session-local recovery copy
}
