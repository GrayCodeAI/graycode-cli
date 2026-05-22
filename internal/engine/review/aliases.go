// Package review is the Stage-1 namespace for self-review / critique / quality
// scoring types in package engine. See ../REFACTOR_PLAN.md.
package review

// Bot is the rule-driven review bot for diffs.
type Bot = ReviewBot

// Rule is a single check in a Bot's rule set.
type Rule = ReviewRule

// Comment is one finding emitted by a Bot.
type Comment = ReviewComment

// Report aggregates Comments for a single review run.
type Report = ReviewReport

// NewBot returns a fresh review bot with the default rule set.
func NewBot() *Bot { return NewReviewBot() }
