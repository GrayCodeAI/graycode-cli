// This file re-exports symbols from the compact sub-package so that existing
// callers of engine.* keep compiling during the Stage 2 migration.
// See REFACTOR_PLAN.md.
package engine

import (
	"github.com/GrayCodeAI/eyrie/client"

	"github.com/GrayCodeAI/hawk/internal/engine/compact"
)

type CompactVariant = compact.CompactVariant

const (
	CompactBase    = compact.CompactBase
	CompactPartial = compact.CompactPartial
	CompactUpTo    = compact.CompactUpTo
)

type (
	CompactResult       = compact.CompactResult
	CompactConfig       = compact.CompactConfig
	FileTracker         = compact.FileTracker
	MicroCompactConfig  = compact.MicroCompactConfig
	APICompactConfig    = compact.APICompactConfig
	SessionMemoryConfig = compact.SessionMemoryConfig
	CompactionTrigger   = compact.CompactionTrigger
)

func DefaultCompactConfig() CompactConfig             { return compact.DefaultCompactConfig() }
func DefaultMicroCompactConfig() MicroCompactConfig   { return compact.DefaultMicroCompactConfig() }
func DefaultAPICompactConfig() APICompactConfig       { return compact.DefaultAPICompactConfig() }
func DefaultSessionMemoryConfig() SessionMemoryConfig { return compact.DefaultSessionMemoryConfig() }
func NewFileTracker() *FileTracker                    { return compact.NewFileTracker() }
func NewCompactionTrigger(windowSize int) *CompactionTrigger {
	return compact.NewCompactionTrigger(windowSize)
}

func BuildCompactPrompt(variant CompactVariant) string { return compact.BuildCompactPrompt(variant) }
func FormatCompactSummary(raw string) string           { return compact.FormatCompactSummary(raw) }
func IsCompactableTool(name string) bool               { return compact.IsCompactableTool(name) }
func AdjustIndexToPreserveAPIInvariants(msgs []client.EyrieMessage, startIdx int) int {
	return compact.AdjustIndexToPreserveAPIInvariants(msgs, startIdx)
}
func HasTextContent(m client.EyrieMessage) bool { return compact.HasTextContent(m) }
