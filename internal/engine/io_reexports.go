package engine

import "github.com/GrayCodeAI/hawk/internal/engine/io"

type (
	ClipboardMonitor  = io.ClipboardMonitor
	AIComment         = io.AIComment
	AIWatcher         = io.AIWatcher
	FileEvent         = io.FileEvent
	WatcherConfig     = io.WatcherConfig
	FileWatcher       = io.FileWatcher
	SingleFileWatcher = io.SingleFileWatcher
	CronJob           = io.CronJob
	CronScheduler     = io.CronScheduler
	CronExpr          = io.CronExpr
	ClipboardBridge   = io.ClipboardBridge
)

var (
	NewClipboardMonitor   = io.NewClipboardMonitor
	ReadClipboard         = io.ReadClipboard
	WriteClipboard        = io.WriteClipboard
	DetectContentType     = io.DetectContentType
	DetectLanguage        = io.DetectLanguage
	NewAIWatcher          = io.NewAIWatcher
	ScanFile              = io.ScanFile
	ScanDirectory         = io.ScanDirectory
	BuildPrompt           = io.BuildPrompt
	RemoveComment         = io.RemoveComment
	NewFileWatcher        = io.NewFileWatcher
	WatchSingle           = io.WatchSingle
	DefaultIgnorePatterns = io.DefaultIgnorePatterns
	MatchesPattern        = io.MatchesPattern
	DedupEvents           = io.DedupEvents
	FormatEvents          = io.FormatEvents
	NewCronScheduler      = io.NewCronScheduler
	ParseCron             = io.ParseCron
	IsDue                 = io.IsDue
	NextRunTime           = io.NextRunTime
	SummarizeClipboard    = io.SummarizeClipboard
	FormatForContext      = io.FormatForContext
)
