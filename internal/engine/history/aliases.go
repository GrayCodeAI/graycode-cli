// Package history is the Stage-1 namespace for command history, conversation summarisation, distillation, head/tail, annotations.
// See ../REFACTOR_PLAN.md.
package history

import "github.com/GrayCodeAI/hawk/engine"

type (
	CommandRecord          = engine.CommandRecord
	CommandFrequency       = engine.CommandFrequency
	AliasSuggestion        = engine.AliasSuggestion
	CommandHistory         = engine.CommandHistory
	SummaryLevel           = engine.SummaryLevel
	SumMessage             = engine.SumMessage
	Summary                = engine.Summary
	ConversationSummarizer = engine.ConversationSummarizer
	DistillExample         = engine.DistillExample
	DistillStats           = engine.DistillStats
	DistillationPipeline   = engine.DistillationPipeline
	HeadTailWindow         = engine.HeadTailWindow
	WindowResult           = engine.WindowResult
	WindowMessage          = engine.WindowMessage
	FileMentionDetector    = engine.FileMentionDetector
	Annotation             = engine.Annotation
	AnnotationManager      = engine.AnnotationManager
)

var (
	NewCommandHistory         = engine.NewCommandHistory
	NewConversationSummarizer = engine.NewConversationSummarizer
	NewDistillationPipeline   = engine.NewDistillationPipeline
	NewHeadTailWindow         = engine.NewHeadTailWindow
	AdaptiveSizes             = engine.AdaptiveSizes
	PreserveToolPairs         = engine.PreserveToolPairs
	FormatWindow              = engine.FormatWindow
	ShouldApply               = engine.ShouldApply
	NewFileMentionDetector    = engine.NewFileMentionDetector
	NewAnnotationManager      = engine.NewAnnotationManager
	StripAnnotations          = engine.StripAnnotations
	DetectAnnotations         = engine.DetectAnnotations
	FormatAnnotations         = engine.FormatAnnotations
)
