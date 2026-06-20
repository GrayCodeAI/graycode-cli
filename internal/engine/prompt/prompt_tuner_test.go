package prompt

import (
	"testing"

	"github.com/GrayCodeAI/hawk/internal/testutil"
)

func TestPromptTuner_RecordAndBest(t *testing.T) {
	testutil.IsolateStorage(t)

	pt := NewPromptTuner()
	pt.RecordOutcome("tools", "Use tools freely", true)
	pt.RecordOutcome("tools", "Use tools freely", true)
	pt.RecordOutcome("tools", "Ask before using tools", false)

	best, score := pt.BestVariant("tools")
	_, _ = best, score
}

func TestPromptTuner_Report(t *testing.T) {
	testutil.IsolateStorage(t)

	pt := NewPromptTuner()
	pt.RecordOutcome("style", "concise", true)
	pt.RecordOutcome("style", "verbose", false)

	report := pt.Report()
	if report == "" {
		t.Error("Report should be non-empty")
	}
}

func TestPromptTuner_BestVariant_Empty(t *testing.T) {
	testutil.IsolateStorage(t)

	pt := NewPromptTuner()
	best, _ := pt.BestVariant("nonexistent")
	if best != "" {
		t.Errorf("BestVariant on empty = %q, want empty", best)
	}
}
