package engine

import "github.com/GrayCodeAI/hawk/internal/engine/io"

type ClipboardMonitor = io.ClipboardMonitor
type AIComment = io.AIComment
type AIWatcher = io.AIWatcher
type FileEvent = io.FileEvent
type WatcherConfig = io.WatcherConfig
type FileWatcher = io.FileWatcher
type SingleFileWatcher = io.SingleFileWatcher
type CronJob = io.CronJob
type CronScheduler = io.CronScheduler
type CronExpr = io.CronExpr
type ClipboardBridge = io.ClipboardBridge

var NewClipboardMonitor = io.NewClipboardMonitor
var ReadClipboard = io.ReadClipboard
var WriteClipboard = io.WriteClipboard
var DetectContentType = io.DetectContentType
var DetectLanguage = io.DetectLanguage
var NewAIWatcher = io.NewAIWatcher
var ScanFile = io.ScanFile
var ScanDirectory = io.ScanDirectory
var BuildPrompt = io.BuildPrompt
var RemoveComment = io.RemoveComment
var NewFileWatcher = io.NewFileWatcher
var WatchSingle = io.WatchSingle
var DefaultIgnorePatterns = io.DefaultIgnorePatterns
var MatchesPattern = io.MatchesPattern
var DedupEvents = io.DedupEvents
var FormatEvents = io.FormatEvents
var NewCronScheduler = io.NewCronScheduler
var ParseCron = io.ParseCron
var IsDue = io.IsDue
var NextRunTime = io.NextRunTime
var SummarizeClipboard = io.SummarizeClipboard
var FormatForContext = io.FormatForContext
