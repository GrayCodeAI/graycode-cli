package engine

import "testing"

func TestPartySession_GeneratePrompt(t *testing.T) {
	ps := NewPartySession("Should we use microservices or monolith?", []string{"architect", "developer", "devops"})
	if len(ps.Personas) != 3 {
		t.Errorf("expected 3 personas, got %d", len(ps.Personas))
	}
	prompt := ps.GeneratePrompt(1)
	if !hasSubstr(prompt, "microservices") {
		t.Error("expected topic in prompt")
	}
	if !hasSubstr(prompt, "Winston") {
		t.Error("expected architect name in prompt")
	}
}

func TestPartySession_DefaultPersonas(t *testing.T) {
	ps := NewPartySession("test", []string{})
	if len(ps.Personas) != 3 {
		t.Errorf("expected 3 default personas, got %d", len(ps.Personas))
	}
}

func TestListPersonas(t *testing.T) {
	list := ListPersonas()
	if !hasSubstr(list, "architect") {
		t.Error("expected architect in list")
	}
	if !hasSubstr(list, "security") {
		t.Error("expected security in list")
	}
}

func TestBrainstormPrompt_AllPhases(t *testing.T) {
	phases := []BrainstormPhase{BrainstormSetup, BrainstormDiverge, BrainstormOrganize, BrainstormEvaluate, BrainstormConverge}
	for _, phase := range phases {
		p := BrainstormPrompt(phase, "build a CLI tool", "")
		if p == "" {
			t.Errorf("empty prompt for phase %v", phase)
		}
	}
}

func TestBrainstormSession(t *testing.T) {
	bs := NewBrainstormSession("new feature ideas")
	if bs.Topic != "new feature ideas" {
		t.Errorf("topic = %q", bs.Topic)
	}
	if bs.Phase != BrainstormSetup {
		t.Error("expected setup phase")
	}
}
