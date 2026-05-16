// Package history is the Stage-1 namespace for command history, conversation summarisation, distillation, head/tail, annotations.
// See ../REFACTOR_PLAN.md.
package history

import "github.com/GrayCodeAI/hawk/engine"

type CommandRecord = engine.CommandRecord
type CommandFrequency = engine.CommandFrequency
type AliasSuggestion = engine.AliasSuggestion
type CommandHistory = engine.CommandHistory
type SummaryLevel = engine.SummaryLevel
type SumMessage = engine.SumMessage
type Summary = engine.Summary
type ConversationSummarizer = engine.ConversationSummarizer
type DistillExample = engine.DistillExample
type DistillStats = engine.DistillStats
type DistillationPipeline = engine.DistillationPipeline
type HeadTailWindow = engine.HeadTailWindow
type WindowResult = engine.WindowResult
type WindowMessage = engine.WindowMessage
type FileMentionDetector = engine.FileMentionDetector
type Annotation = engine.Annotation
type AnnotationManager = engine.AnnotationManager

var NewCommandHistory = engine.NewCommandHistory
var NewConversationSummarizer = engine.NewConversationSummarizer
var NewDistillationPipeline = engine.NewDistillationPipeline
var NewHeadTailWindow = engine.NewHeadTailWindow
var AdaptiveSizes = engine.AdaptiveSizes
var PreserveToolPairs = engine.PreserveToolPairs
var FormatWindow = engine.FormatWindow
var ShouldApply = engine.ShouldApply
var NewFileMentionDetector = engine.NewFileMentionDetector
var NewAnnotationManager = engine.NewAnnotationManager
var StripAnnotations = engine.StripAnnotations
var DetectAnnotations = engine.DetectAnnotations
var FormatAnnotations = engine.FormatAnnotations
