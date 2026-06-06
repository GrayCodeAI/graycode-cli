package agent

import "testing"

func TestDefaultTurnsForMode(t *testing.T) {
	tests := []struct {
		name string
		mode SubAgentMode
		want int
	}{
		{"explore", SubAgentExplore, DefaultExploreTurns},
		{"general", SubAgentGeneral, DefaultGeneralTurns},
		{"plan", SubAgentPlan, DefaultPlanTurns},
		{"unknown falls back to general", SubAgentMode("bogus"), DefaultGeneralTurns},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DefaultTurnsForMode(tt.mode); got != tt.want {
				t.Errorf("DefaultTurnsForMode(%q) = %d, want %d", tt.mode, got, tt.want)
			}
		})
	}
}

func TestPlanModeBudgetDistinct(t *testing.T) {
	// Plan must resolve to its own budget, distinct from explore and general.
	plan := DefaultTurnsForMode(SubAgentPlan)
	if plan == DefaultExploreTurns {
		t.Errorf("plan budget %d must differ from explore budget %d", plan, DefaultExploreTurns)
	}
	if plan == DefaultGeneralTurns {
		t.Errorf("plan budget %d must differ from general budget %d", plan, DefaultGeneralTurns)
	}
	if plan != DefaultPlanTurns {
		t.Errorf("DefaultTurnsForMode(plan) = %d, want %d", plan, DefaultPlanTurns)
	}
}

func TestExploreThoroughnessTurns(t *testing.T) {
	tests := []struct {
		name string
		th   ExploreThoroughness
		want int
	}{
		{"quick", ThoroughnessQuick, ThoroughnessQuickTurns},
		{"medium", ThoroughnessMedium, ThoroughnessMediumTurns},
		{"very-thorough", ThoroughnessVeryThorough, ThoroughnessVeryThoroughTurns},
		{"empty falls back to default", ExploreThoroughness(""), DefaultExploreTurns},
		{"unknown falls back to default", ExploreThoroughness("ludicrous"), DefaultExploreTurns},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ThoroughnessTurns(tt.th); got != tt.want {
				t.Errorf("ThoroughnessTurns(%q) = %d, want %d", tt.th, got, tt.want)
			}
			if got := (ExploreConfig{Thoroughness: tt.th}).Turns(); got != tt.want {
				t.Errorf("ExploreConfig{%q}.Turns() = %d, want %d", tt.th, got, tt.want)
			}
		})
	}
}

func TestExploreThoroughnessLevelsDistinct(t *testing.T) {
	q := ThoroughnessTurns(ThoroughnessQuick)
	m := ThoroughnessTurns(ThoroughnessMedium)
	v := ThoroughnessTurns(ThoroughnessVeryThorough)
	if q == m || m == v || q == v {
		t.Errorf("thoroughness budgets must be distinct: quick=%d medium=%d very=%d", q, m, v)
	}
	if !(q < m && m < v) {
		t.Errorf("thoroughness budgets must increase: quick=%d medium=%d very=%d", q, m, v)
	}
}

func TestDefaultExploreConfig(t *testing.T) {
	cfg := DefaultExploreConfig()
	if cfg.Thoroughness != ThoroughnessMedium {
		t.Errorf("DefaultExploreConfig().Thoroughness = %q, want %q", cfg.Thoroughness, ThoroughnessMedium)
	}
	if cfg.Turns() != ThoroughnessMediumTurns {
		t.Errorf("DefaultExploreConfig().Turns() = %d, want %d", cfg.Turns(), ThoroughnessMediumTurns)
	}
}

func TestIsReadOnlyMode(t *testing.T) {
	if !IsReadOnlyMode(SubAgentExplore) {
		t.Error("explore should be read-only")
	}
	if !IsReadOnlyMode(SubAgentPlan) {
		t.Error("plan should be read-only")
	}
	if IsReadOnlyMode(SubAgentGeneral) {
		t.Error("general should not be read-only")
	}
}
