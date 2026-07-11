package cmd

import (
	"testing"

			tea "charm.land/bubbletea/v2"
	"github.com/GrayCodeAI/hawk/internal/engine"
)

func TestSpecPicker_HasSevenActions(t *testing.T) {
	sp := NewSpecPicker(80)
	if len(sp.entries) != 7 {
		t.Fatalf("expected 7 actions, got %d", len(sp.entries))
	}
	wantOrder := []specPickerAction{specActionStart, specActionStatus, specActionEdit, specActionResume, specActionArchive, specActionConfigure, specActionReset}
	for i, action := range wantOrder {
		if sp.entries[i].Action != action {
			t.Errorf("entry %d = %v, want %v", i, sp.entries[i].Action, action)
		}
	}
}

func TestSpecPicker_OpenRecordsStage(t *testing.T) {
	sp := NewSpecPicker(80)
	sp.Open(engine.SpecStagePlan)
	if !sp.IsOpen() {
		t.Fatal("expected picker to be open")
	}
	if sp.stage != engine.SpecStagePlan {
		t.Fatalf("expected stage SpecStagePlan, got %v", sp.stage)
	}
}

func TestSpecPicker_EnterSelectsAndCloses(t *testing.T) {
	sp := NewSpecPicker(80)
	sp.Open(engine.SpecStageNone)

	chosen, handled := sp.Update(tea.KeyMsg{Type: tea.KeyDown})
	if !handled || chosen != nil {
		t.Fatalf("KeyDown should navigate, not select: chosen=%v handled=%v", chosen, handled)
	}
	if sp.Selected().Action != specActionStatus {
		t.Fatalf("expected selection to move to Status, got %v", sp.Selected().Action)
	}

	chosen, handled = sp.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatal("expected Enter to be handled")
	}
	if chosen == nil || chosen.Action != specActionStatus {
		t.Fatalf("expected chosen Status, got %v", chosen)
	}
	if sp.IsOpen() {
		t.Error("expected picker to close after Enter")
	}
}

func TestSpecPicker_EscClosesWithoutSelecting(t *testing.T) {
	sp := NewSpecPicker(80)
	sp.Open(engine.SpecStageNone)

	chosen, handled := sp.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !handled || chosen != nil {
		t.Fatalf("expected Esc to close without a selection, got chosen=%v", chosen)
	}
	if sp.IsOpen() {
		t.Error("expected picker to be closed after Esc")
	}
}

func TestSpecPicker_FilterByName(t *testing.T) {
	sp := NewSpecPicker(80)
	sp.Open(engine.SpecStageNone)

	for _, r := range "reset" {
		sp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if len(sp.filtered) != 1 || sp.filtered[0].Action != specActionReset {
		t.Fatalf("expected filter 'reset' to match only Reset action, got %d entries: %+v", len(sp.filtered), sp.filtered)
	}
}

func TestChatSpecSubcommand_BareOpensPicker(t *testing.T) {
	m := newTestChatModel()
	result, _ := (&specSubcommand{}).Handle(m, nil, "/spec")
	cm := requireChatModel(t, result)
	if cm.specPicker == nil || !cm.specPicker.IsOpen() {
		t.Fatal("expected bare /spec to open the spec picker")
	}
}

func TestChatSpecSubcommand_WithDescriptionStartsDirectly(t *testing.T) {
	m := newTestChatModel()
	result, _ := (&specSubcommand{}).Handle(m, nil, "/spec add dark mode")
	cm := requireChatModel(t, result)
	if cm.specPicker != nil && cm.specPicker.IsOpen() {
		t.Error("expected /spec with a description to start directly, not open the picker")
	}
	if currentSpecStage(cm.session) != engine.SpecStageSpecify {
		t.Fatalf("expected stage SpecStageSpecify, got %v", currentSpecStage(cm.session))
	}
}
