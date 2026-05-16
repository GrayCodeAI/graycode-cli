// Package io is the Stage-1 namespace for clipboard, AI watcher, file
// watcher, and cron scheduler types. See ../REFACTOR_PLAN.md.
package io

import "github.com/GrayCodeAI/hawk/engine"

type (
	ClipboardMonitor  = engine.ClipboardMonitor
	AIComment         = engine.AIComment
	AIWatcher         = engine.AIWatcher
	FileEvent         = engine.FileEvent
	WatcherConfig     = engine.WatcherConfig
	FileWatcher       = engine.FileWatcher
	SingleFileWatcher = engine.SingleFileWatcher
	CronJob           = engine.CronJob
	CronScheduler     = engine.CronScheduler
	CronExpr          = engine.CronExpr
)

func NewClipboardMonitor() *ClipboardMonitor  { return engine.NewClipboardMonitor() }
func ReadClipboard() (string, error)          { return engine.ReadClipboard() }
func WriteClipboard(content string) error     { return engine.WriteClipboard(content) }
func DetectContentType(content string) string { return engine.DetectContentType(content) }
func DetectLanguage(code string) string       { return engine.DetectLanguage(code) }
func NewAIWatcher(rootDir string, patterns []string) *AIWatcher {
	return engine.NewAIWatcher(rootDir, patterns)
}

func NewFileWatcher(rootDir string, config WatcherConfig) *FileWatcher {
	return engine.NewFileWatcher(rootDir, config)
}

func WatchSingle(path string, onChange func()) *SingleFileWatcher {
	return engine.WatchSingle(path, onChange)
}
func DefaultIgnorePatterns() []string                { return engine.DefaultIgnorePatterns() }
func NewCronScheduler() *CronScheduler               { return engine.NewCronScheduler() }
func ParseCron(expression string) (*CronExpr, error) { return engine.ParseCron(expression) }
