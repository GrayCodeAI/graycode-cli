package engine

import "github.com/GrayCodeAI/hawk/internal/engine/history"

type (
	CommandRecord          = history.CommandRecord
	CommandFrequency       = history.CommandFrequency
	AliasSuggestion        = history.AliasSuggestion
	CommandHistory         = history.CommandHistory
	SummaryLevel           = history.SummaryLevel
	SumMessage             = history.SumMessage
	Summary                = history.Summary
	ConversationSummarizer = history.ConversationSummarizer
	DistillExample         = history.DistillExample
	DistillStats           = history.DistillStats
	DistillationPipeline   = history.DistillationPipeline
	HeadTailWindow         = history.HeadTailWindow
	WindowResult           = history.WindowResult
	WindowMessage          = history.WindowMessage
	FileMentionDetector    = history.FileMentionDetector
	Annotation             = history.Annotation
	AnnotationManager      = history.AnnotationManager
)

var (
	NewCommandHistory         = history.NewCommandHistory
	NewConversationSummarizer = history.NewConversationSummarizer
	NewDistillationPipeline   = history.NewDistillationPipeline
	NewHeadTailWindow         = history.NewHeadTailWindow
	AdaptiveSizes             = history.AdaptiveSizes
	PreserveToolPairs         = history.PreserveToolPairs
	FormatWindow              = history.FormatWindow
	ShouldApply               = history.ShouldApply
	NewFileMentionDetector    = history.NewFileMentionDetector
	NewAnnotationManager      = history.NewAnnotationManager
	StripAnnotations          = history.StripAnnotations
	DetectAnnotations         = history.DetectAnnotations
	FormatAnnotations         = history.FormatAnnotations
)
