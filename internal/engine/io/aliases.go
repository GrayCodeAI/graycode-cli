// Package io provides clipboard, AI watcher, file watcher, and cron
// scheduler types.
//
// Public types: ClipboardMonitor, AIComment, AIWatcher, FileEvent,
// WatcherConfig, FileWatcher, SingleFileWatcher, CronJob, CronScheduler,
// CronExpr, ClipboardBridge.
//
// Public functions: NewClipboardMonitor, ReadClipboard, WriteClipboard,
// DetectContentType, DetectLanguage, NewAIWatcher, ScanFile, ScanDirectory,
// BuildPrompt, RemoveComment, NewFileWatcher, WatchSingle,
// DefaultIgnorePatterns, MatchesPattern, DedupEvents, FormatEvents,
// NewCronScheduler, ParseCron, IsDue, NextRunTime, SummarizeClipboard,
// FormatForContext.
package io
