package analytics

import (
	"testing"
	"time"
)

func TestClassifyTurn_Coding(t *testing.T) {
	cat := ClassifyTurn([]string{"Read", "Edit"}, "add a new handler")
	if cat != CategoryCoding {
		t.Errorf("got %s, want coding", cat)
	}
}

func TestClassifyTurn_Debugging(t *testing.T) {
	cat := ClassifyTurn([]string{"Read", "Edit"}, "fix the bug in auth")
	if cat != CategoryDebugging {
		t.Errorf("got %s, want debugging", cat)
	}
}

func TestClassifyTurn_Exploration(t *testing.T) {
	cat := ClassifyTurn([]string{"Read", "Grep", "Glob"}, "find where auth is used")
	if cat != CategoryExploration {
		t.Errorf("got %s, want exploration", cat)
	}
}

func TestClassifyTurn_Planning(t *testing.T) {
	cat := ClassifyTurn([]string{"TodoWrite", "TaskCreate"}, "plan the migration")
	if cat != CategoryPlanning {
		t.Errorf("got %s, want planning", cat)
	}
}

func TestClassifyTurn_Conversation(t *testing.T) {
	cat := ClassifyTurn(nil, "what do you think about this approach?")
	if cat != CategoryConversation {
		t.Errorf("got %s, want conversation", cat)
	}
}

func TestClassifyTurn_Refactoring(t *testing.T) {
	cat := ClassifyTurn([]string{"Edit"}, "refactor the handler")
	if cat != CategoryRefactoring {
		t.Errorf("got %s, want refactoring", cat)
	}
}

func TestOneShotTracker_AllFirstTry(t *testing.T) {
	tr := &OneShotTracker{}
	tr.RecordTurn([]string{"Edit"})   // edit
	tr.RecordTurn([]string{"Bash"})   // non-edit → previous was one-shot
	tr.RecordTurn([]string{"Edit"})   // edit
	tr.RecordTurn([]string{"Read"})   // non-edit → previous was one-shot
	rate := tr.Rate()
	if rate != 100.0 {
		t.Errorf("got %.1f%%, want 100%%", rate)
	}
}

func TestOneShotTracker_WithRetries(t *testing.T) {
	tr := &OneShotTracker{}
	tr.RecordTurn([]string{"Edit"})   // edit 1
	tr.RecordTurn([]string{"Edit"})   // retry → edit 1 was NOT one-shot
	tr.RecordTurn([]string{"Bash"})   // non-edit → edit 2 was one-shot
	total, firstTry := tr.Stats()
	if total != 2 {
		t.Errorf("total: got %d, want 2", total)
	}
	// Only the second edit succeeded first try
	if firstTry != 1 {
		t.Errorf("firstTry: got %d, want 1", firstTry)
	}
}

func TestCommandTracker_Record(t *testing.T) {
	tr := &CommandTracker{}
	tr.Record("go test ./...", 0, 2*time.Second, "/project")
	tr.Record("go build", 1, time.Second, "/project")

	if rate := tr.FailureRate(); rate != 0.5 {
		t.Errorf("failure rate: got %f, want 0.5", rate)
	}
}

func TestCommandTracker_MostUsed(t *testing.T) {
	tr := &CommandTracker{}
	tr.Record("go test", 0, time.Second, "/p")
	tr.Record("go test", 0, time.Second, "/p")
	tr.Record("go build", 0, time.Second, "/p")

	top := tr.MostUsed(1)
	if len(top) != 1 || top[0] != "go test" {
		t.Errorf("most used: got %v, want [go test]", top)
	}
}

func TestCommandTracker_AvgDuration(t *testing.T) {
	tr := &CommandTracker{}
	tr.Record("a", 0, 2*time.Second, "/p")
	tr.Record("b", 0, 4*time.Second, "/p")

	if avg := tr.AvgDuration(); avg != 3*time.Second {
		t.Errorf("avg duration: got %v, want 3s", avg)
	}
}

func TestCommandTracker_Empty(t *testing.T) {
	tr := &CommandTracker{}
	if rate := tr.FailureRate(); rate != 0 {
		t.Errorf("failure rate: got %f, want 0", rate)
	}
	if avg := tr.AvgDuration(); avg != 0 {
		t.Errorf("avg duration: got %v, want 0", avg)
	}
	if top := tr.MostUsed(5); len(top) != 0 {
		t.Errorf("most used: got %v, want empty", top)
	}
}
