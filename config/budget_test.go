package config

import (
	"strings"
	"testing"
)

func TestDefaultBudgetConfig(t *testing.T) {
	cfg := DefaultBudgetConfig()
	if cfg.MaxCostUSD != 0 {
		t.Fatalf("expected 0 default max cost, got %f", cfg.MaxCostUSD)
	}
	if cfg.MaxTokensPerSession != 0 {
		t.Fatalf("expected 0 default max tokens, got %d", cfg.MaxTokensPerSession)
	}
	if cfg.WarnAtPercent != 80 {
		t.Fatalf("expected 80%% warn threshold, got %f", cfg.WarnAtPercent)
	}
}

func TestCheckBudget_NoBudget(t *testing.T) {
	cfg := BudgetConfig{MaxCostUSD: 0}
	status := CheckBudget(10.0, cfg)
	if status != BudgetOK {
		t.Fatalf("expected OK with no budget, got %v", status)
	}
}

func TestCheckBudget_UnderBudget(t *testing.T) {
	cfg := BudgetConfig{MaxCostUSD: 10.0, WarnAtPercent: 80}
	status := CheckBudget(5.0, cfg)
	if status != BudgetOK {
		t.Fatalf("expected OK at 50%%, got %v", status)
	}
}

func TestCheckBudget_Warning(t *testing.T) {
	cfg := BudgetConfig{MaxCostUSD: 10.0, WarnAtPercent: 80}
	status := CheckBudget(8.5, cfg) // 85%
	if status != BudgetWarning {
		t.Fatalf("expected Warning at 85%%, got %v", status)
	}
}

func TestCheckBudget_Exceeded(t *testing.T) {
	cfg := BudgetConfig{MaxCostUSD: 10.0, WarnAtPercent: 80}
	status := CheckBudget(10.0, cfg)
	if status != BudgetExceeded {
		t.Fatalf("expected Exceeded at 100%%, got %v", status)
	}

	status = CheckBudget(12.0, cfg)
	if status != BudgetExceeded {
		t.Fatalf("expected Exceeded at 120%%, got %v", status)
	}
}

func TestCheckBudget_ExactThreshold(t *testing.T) {
	cfg := BudgetConfig{MaxCostUSD: 10.0, WarnAtPercent: 80}
	status := CheckBudget(8.0, cfg) // exactly at 80%
	if status != BudgetWarning {
		t.Fatalf("expected Warning at exactly 80%%, got %v", status)
	}
}

func TestCheckBudget_CustomWarnPercent(t *testing.T) {
	cfg := BudgetConfig{MaxCostUSD: 100.0, WarnAtPercent: 50}
	status := CheckBudget(55.0, cfg) // 55% > 50%
	if status != BudgetWarning {
		t.Fatalf("expected Warning at 55%% with 50%% threshold, got %v", status)
	}
}

func TestCheckBudget_ZeroWarnPercent(t *testing.T) {
	// Zero warn percent should default to 80%
	cfg := BudgetConfig{MaxCostUSD: 10.0, WarnAtPercent: 0}
	status := CheckBudget(7.0, cfg) // 70% < 80%
	if status != BudgetOK {
		t.Fatalf("expected OK at 70%% with defaulted 80%% threshold, got %v", status)
	}
	status = CheckBudget(8.5, cfg) // 85% > 80%
	if status != BudgetWarning {
		t.Fatalf("expected Warning at 85%% with defaulted 80%% threshold, got %v", status)
	}
}

func TestFormatBudgetStatus_NoBudget(t *testing.T) {
	s := FormatBudgetStatus(BudgetOK, 5.0, 0)
	if !strings.Contains(s, "no budget limit") {
		t.Fatalf("expected 'no budget limit', got %q", s)
	}
	if !strings.Contains(s, "$5.00") {
		t.Fatalf("expected '$5.00', got %q", s)
	}
}

func TestFormatBudgetStatus_OK(t *testing.T) {
	s := FormatBudgetStatus(BudgetOK, 3.0, 10.0)
	if !strings.Contains(s, "$3.00") || !strings.Contains(s, "$10.00") {
		t.Fatalf("expected cost and max, got %q", s)
	}
	if !strings.Contains(s, "30.0%") {
		t.Fatalf("expected percentage, got %q", s)
	}
}

func TestFormatBudgetStatus_Warning(t *testing.T) {
	s := FormatBudgetStatus(BudgetWarning, 8.5, 10.0)
	if !strings.Contains(s, "Budget warning") {
		t.Fatalf("expected 'Budget warning', got %q", s)
	}
}

func TestFormatBudgetStatus_Exceeded(t *testing.T) {
	s := FormatBudgetStatus(BudgetExceeded, 12.0, 10.0)
	if !strings.Contains(s, "BUDGET EXCEEDED") {
		t.Fatalf("expected 'BUDGET EXCEEDED', got %q", s)
	}
}

func TestBudgetStatus_String(t *testing.T) {
	tests := []struct {
		status   BudgetStatus
		expected string
	}{
		{BudgetOK, "ok"},
		{BudgetWarning, "warning"},
		{BudgetExceeded, "exceeded"},
		{BudgetStatus(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.expected {
			t.Errorf("BudgetStatus(%d).String() = %q, want %q", tt.status, got, tt.expected)
		}
	}
}

func TestLoadBudget(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := LoadBudget()
	// No settings file → defaults
	if cfg.WarnAtPercent != 80 {
		t.Fatalf("expected 80%% warn threshold from default, got %f", cfg.WarnAtPercent)
	}
}
