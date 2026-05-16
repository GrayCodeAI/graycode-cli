package engine

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestFormatCostDisplay(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cost float64
		want string
	}{
		{"zero", 0, ""},
		{"negative", -1.0, ""},
		{"sub-cent", 0.005, "$0.0050"},
		{"cents", 0.15, "$0.150"},
		{"dollar", 2.5, "$2.50"},
		{"large", 100.0, "$100.00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatCostDisplay(tt.cost)
			if got != tt.want {
				t.Errorf("FormatCostDisplay(%f) = %q, want %q", tt.cost, got, tt.want)
			}
		})
	}
}

func TestDefaultTimeoutConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultTimeoutConfig()
	if cfg.PerTurn != 60*time.Second {
		t.Errorf("PerTurn = %v, want 60s", cfg.PerTurn)
	}
	if cfg.PerTool != 120*time.Second {
		t.Errorf("PerTool = %v, want 120s", cfg.PerTool)
	}
	if cfg.Total != 0 {
		t.Errorf("Total = %v, want 0 (no default deadline)", cfg.Total)
	}
}

func TestWithTimeout(t *testing.T) {
	t.Parallel()

	t.Run("with total", func(t *testing.T) {
		t.Parallel()
		cfg := TimeoutConfig{Total: 5 * time.Second}
		ctx, cancel := WithTimeout(context.Background(), cfg)
		defer cancel()

		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("context should have deadline")
		}
		if time.Until(deadline) > 6*time.Second {
			t.Error("deadline too far in future")
		}
	})

	t.Run("without total", func(t *testing.T) {
		t.Parallel()
		cfg := TimeoutConfig{Total: 0}
		ctx, cancel := WithTimeout(context.Background(), cfg)
		defer cancel()

		_, ok := ctx.Deadline()
		if ok {
			t.Error("context should not have deadline when Total=0")
		}
	})
}

func TestRemainingTime(t *testing.T) {
	t.Parallel()
	cfg := TimeoutConfig{Total: 10 * time.Second}
	ctx, cancel := WithTimeout(context.Background(), cfg)
	defer cancel()

	remaining := RemainingTime(ctx)
	if remaining == "" {
		t.Error("RemainingTime should return non-empty string")
	}
	if !strings.Contains(remaining, "s") && !strings.Contains(remaining, "m") {
		t.Errorf("RemainingTime() = %q, expected time unit", remaining)
	}
}

func TestRemainingTime_WithoutDeadline(t *testing.T) {
	t.Parallel()
	remaining := RemainingTime(context.Background())
	if remaining != "" {
		t.Errorf("RemainingTime() = %q, want empty for no deadline", remaining)
	}
}
