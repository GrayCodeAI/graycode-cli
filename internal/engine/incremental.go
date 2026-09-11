package engine

import (
	"os"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/engine/ctxmgr"
)

// incrementalContextEnabled reports whether the opt-in incremental
// system-context mode is enabled. It is disabled by default so existing
// request assembly is byte-for-byte unchanged; when enabled, dynamic
// system-prompt sections (e.g. memories) are reconciled incrementally and only
// changed sections are re-rendered, preserving a stable system-prompt prefix
// that keeps provider prompt-cache entries valid across turns.
func incrementalContextEnabled() bool {
	return strings.EqualFold(os.Getenv("HAWK_INCREMENTAL_CONTEXT"), "1")
}

// memoryIncremental is a small holder that wires a memory-recall loader into
// an IncrementalContext section. It is recreated when the recall function's
// identity changes.
type memoryIncremental struct {
	ic   *ctxmgr.IncrementalContext
	init bool
}

// newMemoryIncremental builds an incremental context backed by the given
// memory recall loader.
func newMemoryIncremental(recall func() string) (*memoryIncremental, error) {
	ic, err := ctxmgr.NewIncrementalContext([]ctxmgr.Section{
		{Key: "memories", Header: "## Relevant Memories", Load: func() (string, error) {
			if recall == nil {
				return "", nil
			}
			return recall(), nil
		}},
	})
	if err != nil {
		return nil, err
	}
	return &memoryIncremental{ic: ic}, nil
}

// prepare runs the incremental memory-recall path. It returns the section
// content to write for the current turn and whether that content changed:
//
//   - (content, true): write content as the new "## Relevant Memories" section.
//     This covers both first initialization (full) and an incremental change.
//   - ("", false): nothing changed; keep the existing section untouched, which
//     preserves a stable system-prompt prefix (and any provider cache entry)
//     across turns.
func (m *memoryIncremental) prepare() (content string, changed bool) {
	if m == nil || m.ic == nil {
		return "", true
	}
	if !m.init {
		if _, err := m.ic.Initialize(); err != nil {
			return "", true
		}
		m.init = true
		return m.ic.Baseline(), true
	}
	msg, replaced, base, err := m.ic.Reconcile()
	if err != nil || replaced {
		return base, true
	}
	if msg == nil {
		return "", false
	}
	return *msg, true
}
