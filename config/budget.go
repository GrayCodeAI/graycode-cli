package config

import (
	"fmt"
)

// BudgetStatus represents the current budget state.
type BudgetStatus int

const (
	BudgetOK       BudgetStatus = iota // within budget
	BudgetWarning                      // approaching limit
	BudgetExceeded                     // over limit
)

// String returns a human-readable budget status.
func (s BudgetStatus) String() string {
	switch s {
	case BudgetOK:
		return "ok"
	case BudgetWarning:
		return "warning"
	case BudgetExceeded:
		return "exceeded"
	default:
		return "unknown"
	}
}

// BudgetConfig holds cost and token budget settings.
type BudgetConfig struct {
	MaxCostUSD          float64 `json:"max_cost_usd"`
	MaxTokensPerSession int     `json:"max_tokens_per_session"`
	WarnAtPercent       float64 `json:"warn_at_percent"` // 0-100, default 80
}

// DefaultBudgetConfig returns a BudgetConfig with sensible defaults.
func DefaultBudgetConfig() BudgetConfig {
	return BudgetConfig{
		MaxCostUSD:          0, // 0 means unlimited
		MaxTokensPerSession: 0, // 0 means unlimited
		WarnAtPercent:       80,
	}
}

// LoadBudget loads budget configuration from settings.
// It reads MaxBudgetUSD from the global settings and applies defaults.
func LoadBudget() BudgetConfig {
	settings := LoadSettings()
	cfg := DefaultBudgetConfig()
	if settings.MaxBudgetUSD > 0 {
		cfg.MaxCostUSD = settings.MaxBudgetUSD
	}
	return cfg
}

// CheckBudget evaluates current spending against the budget config.
// Returns BudgetOK, BudgetWarning, or BudgetExceeded.
func CheckBudget(spent float64, config BudgetConfig) BudgetStatus {
	if config.MaxCostUSD <= 0 {
		return BudgetOK // no budget set
	}
	if spent >= config.MaxCostUSD {
		return BudgetExceeded
	}
	warnAt := config.WarnAtPercent
	if warnAt <= 0 {
		warnAt = 80
	}
	threshold := config.MaxCostUSD * (warnAt / 100.0)
	if spent >= threshold {
		return BudgetWarning
	}
	return BudgetOK
}

// FormatBudgetStatus returns a human-readable string describing the budget status.
func FormatBudgetStatus(status BudgetStatus, spent, max float64) string {
	if max <= 0 {
		return fmt.Sprintf("$%.2f spent (no budget limit)", spent)
	}
	pct := (spent / max) * 100
	switch status {
	case BudgetExceeded:
		return fmt.Sprintf("BUDGET EXCEEDED: $%.2f / $%.2f (%.1f%%)", spent, max, pct)
	case BudgetWarning:
		return fmt.Sprintf("Budget warning: $%.2f / $%.2f (%.1f%%)", spent, max, pct)
	default:
		return fmt.Sprintf("$%.2f / $%.2f (%.1f%%)", spent, max, pct)
	}
}
