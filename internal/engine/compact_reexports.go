// This file re-exports symbols from the compact sub-package so that existing
// callers of engine.* keep compiling during the Stage 2 migration.
// See docs/plans/engine-refactor-plan.md.
package engine

import (
	"github.com/GrayCodeAI/graycode-cli/internal/types"

	"github.com/GrayCodeAI/graycode-cli/internal/engine/compact"
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
func BuildIncrementalCompactPrompt(priorSummary string) string {
	return compact.BuildIncrementalCompactPrompt(priorSummary)
}

func ExtractPriorSummary(msgs []types.EyrieMessage) string {
	return compact.ExtractPriorSummary(msgs)
}

// PriorSummaryPrefix is the marker prefix graycode prepends to a persisted
// conversation summary message.
const PriorSummaryPrefix = compact.PriorSummaryPrefix

func IsCompactableTool(name string) bool { return compact.IsCompactableTool(name) }
func AdjustIndexToPreserveAPIInvariants(msgs []types.EyrieMessage, startIdx int) int {
	return compact.AdjustIndexToPreserveAPIInvariants(msgs, startIdx)
}
func HasTextContent(m types.EyrieMessage) bool { return compact.HasTextContent(m) }
