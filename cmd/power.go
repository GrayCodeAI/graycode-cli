package cmd

import (
	"fmt"

	"github.com/GrayCodeAI/hawk/engine"
)

// PowerConfig maps a power level (1-10) to all relevant settings.
type PowerConfig struct {
	Level           int
	Model           string
	MaxTokens       int
	ContextWindow   int
	Temperature     float64
	MaxTurns        int
	ToolParallelism int
	ReviewDepth     string // "none", "quick", "thorough"
	AutoApply       bool
	BudgetUSD       float64
}

// PowerPreset returns the configuration for a given power level (1-10).
//
// 1-2: haiku/flash, 4K context, fast, no review, $0.05 budget -- quick questions
// 3-4: sonnet-mini/gpt-4o-mini, 16K context, moderate -- simple tasks
// 5-6: sonnet/gpt-4o, 64K context, standard -- normal coding (DEFAULT)
// 7-8: sonnet/opus, 128K context, thorough review -- complex tasks
// 9-10: opus, 200K context, multi-pass review, council mode -- critical work
func PowerPreset(level int) PowerConfig {
	if level < 1 {
		level = 1
	}
	if level > 10 {
		level = 10
	}

	switch level {
	case 1:
		return PowerConfig{
			Level:           1,
			Model:           "claude-haiku-3",
			MaxTokens:       1024,
			ContextWindow:   4096,
			Temperature:     0.3,
			MaxTurns:        5,
			ToolParallelism: 1,
			ReviewDepth:     "none",
			AutoApply:       false,
			BudgetUSD:       0.05,
		}
	case 2:
		return PowerConfig{
			Level:           2,
			Model:           "claude-haiku-3",
			MaxTokens:       2048,
			ContextWindow:   4096,
			Temperature:     0.3,
			MaxTurns:        10,
			ToolParallelism: 1,
			ReviewDepth:     "none",
			AutoApply:       false,
			BudgetUSD:       0.05,
		}
	case 3:
		return PowerConfig{
			Level:           3,
			Model:           "claude-sonnet-4-20250514",
			MaxTokens:       4096,
			ContextWindow:   16384,
			Temperature:     0.5,
			MaxTurns:        15,
			ToolParallelism: 2,
			ReviewDepth:     "quick",
			AutoApply:       false,
			BudgetUSD:       0.10,
		}
	case 4:
		return PowerConfig{
			Level:           4,
			Model:           "claude-sonnet-4-20250514",
			MaxTokens:       4096,
			ContextWindow:   16384,
			Temperature:     0.5,
			MaxTurns:        20,
			ToolParallelism: 2,
			ReviewDepth:     "quick",
			AutoApply:       false,
			BudgetUSD:       0.20,
		}
	case 5:
		return PowerConfig{
			Level:           5,
			Model:           "claude-sonnet-4-20250514",
			MaxTokens:       8192,
			ContextWindow:   65536,
			Temperature:     0.7,
			MaxTurns:        30,
			ToolParallelism: 4,
			ReviewDepth:     "quick",
			AutoApply:       false,
			BudgetUSD:       0.50,
		}
	case 6:
		return PowerConfig{
			Level:           6,
			Model:           "claude-sonnet-4-20250514",
			MaxTokens:       8192,
			ContextWindow:   65536,
			Temperature:     0.7,
			MaxTurns:        40,
			ToolParallelism: 4,
			ReviewDepth:     "quick",
			AutoApply:       true,
			BudgetUSD:       0.50,
		}
	case 7:
		return PowerConfig{
			Level:           7,
			Model:           "claude-sonnet-4-20250514",
			MaxTokens:       16384,
			ContextWindow:   131072,
			Temperature:     0.7,
			MaxTurns:        50,
			ToolParallelism: 4,
			ReviewDepth:     "thorough",
			AutoApply:       true,
			BudgetUSD:       0.50,
		}
	case 8:
		return PowerConfig{
			Level:           8,
			Model:           "claude-opus-4-20250514",
			MaxTokens:       16384,
			ContextWindow:   131072,
			Temperature:     0.7,
			MaxTurns:        60,
			ToolParallelism: 4,
			ReviewDepth:     "thorough",
			AutoApply:       true,
			BudgetUSD:       1.00,
		}
	case 9:
		return PowerConfig{
			Level:           9,
			Model:           "claude-opus-4-20250514",
			MaxTokens:       16384,
			ContextWindow:   204800,
			Temperature:     0.7,
			MaxTurns:        80,
			ToolParallelism: 8,
			ReviewDepth:     "thorough",
			AutoApply:       true,
			BudgetUSD:       2.00,
		}
	case 10:
		return PowerConfig{
			Level:           10,
			Model:           "claude-opus-4-20250514",
			MaxTokens:       16384,
			ContextWindow:   204800,
			Temperature:     0.7,
			MaxTurns:        100,
			ToolParallelism: 8,
			ReviewDepth:     "thorough",
			AutoApply:       true,
			BudgetUSD:       5.00,
		}
	default:
		// Default to level 5
		return PowerPreset(5)
	}
}

// ApplyPowerLevel configures a session based on power level.
func ApplyPowerLevel(sess *engine.Session, level int) {
	config := PowerPreset(level)

	sess.SetModel(config.Model)
	if err := sess.SetMaxTurns(config.MaxTurns); err == nil {
		// MaxTurns set successfully
	}
	if err := sess.SetMaxBudgetUSD(config.BudgetUSD); err == nil {
		// Budget set successfully
	}

	// Configure autonomy based on power level
	if config.AutoApply {
		sess.Mode = engine.PermissionModeAcceptEdits
	}
}

// DescribePower returns a human-readable description of what a power level does.
func DescribePower(level int) string {
	config := PowerPreset(level)

	reviewDesc := config.ReviewDepth
	if reviewDesc == "" {
		reviewDesc = "none"
	}

	autoApplyStr := "manual"
	if config.AutoApply {
		autoApplyStr = "auto-apply"
	}

	return fmt.Sprintf("Power %d: %s, %dK context, %s review, %s, up to $%.2f/task",
		config.Level,
		config.Model,
		config.ContextWindow/1024,
		reviewDesc,
		autoApplyStr,
		config.BudgetUSD,
	)
}
