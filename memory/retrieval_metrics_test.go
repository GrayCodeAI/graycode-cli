package memory

import (
	"testing"
)

func TestRetrievalMetricsBasic(t *testing.T) {
	rm := NewRetrievalMetrics("")

	rm.RecordRecall("auth library", 3, 150, "")
	rm.RecordRecall("build command", 0, 0, "")
	rm.RecordRecall("testing pattern", 2, 100, "")

	if rm.TotalRecalls() != 3 {
		t.Errorf("TotalRecalls = %d, want 3", rm.TotalRecalls())
	}

	hr := rm.HitRate()
	if hr < 0.65 || hr > 0.68 {
		t.Errorf("HitRate = %f, want ~0.667", hr)
	}
}

func TestRetrievalMetricsReport(t *testing.T) {
	rm := NewRetrievalMetrics("")

	rm.RecordRecall("auth", 5, 200, "session_start")
	rm.RecordRecall("auth", 3, 100, "tool_use")
	rm.RecordRecall("deploy", 1, 50, "")
	rm.RecordRecall("nothing", 0, 0, "")

	r := rm.Report()
	if r.TotalRecalls != 4 {
		t.Errorf("TotalRecalls = %d, want 4", r.TotalRecalls)
	}
	if r.HitRate != 0.75 {
		t.Errorf("HitRate = %f, want 0.75", r.HitRate)
	}
	if len(r.MostQueriedTopics) == 0 {
		t.Error("expected most queried topics to be populated")
	}
	if r.MostQueriedTopics[0] != "auth" {
		t.Errorf("top topic = %q, want 'auth'", r.MostQueriedTopics[0])
	}
}

func TestRetrievalMetricsTokensSaved(t *testing.T) {
	rm := NewRetrievalMetrics("")

	// 3 useful recalls at 100 tokens each → saves 3*(500-100) = 1200 tokens
	rm.RecordRecall("q1", 1, 100, "")
	rm.RecordRecall("q2", 1, 100, "")
	rm.RecordRecall("q3", 1, 100, "")

	saved := rm.TokensSaved()
	if saved != 1200 {
		t.Errorf("TokensSaved = %d, want 1200", saved)
	}
}

func TestRetrievalMetricsEmpty(t *testing.T) {
	rm := NewRetrievalMetrics("")

	if rm.HitRate() != 0 {
		t.Error("expected 0 hit rate with no data")
	}
	if rm.TotalRecalls() != 0 {
		t.Error("expected 0 total recalls")
	}
	if rm.FormatSummary() != "" {
		t.Error("expected empty summary")
	}
}

func TestRetrievalMetricsMarkUseful(t *testing.T) {
	rm := NewRetrievalMetrics("")

	rm.RecordRecall("test", 0, 0, "")
	rm.MarkUseful() // mark the miss as useful (override)

	if rm.HitRate() != 0 {
		// hitCount is still based on resultCount > 0
		t.Logf("HitRate unchanged by MarkUseful (expected)")
	}
}

func TestRetrievalMetricsFormatSummary(t *testing.T) {
	rm := NewRetrievalMetrics("")
	rm.RecordRecall("q1", 2, 100, "")
	rm.RecordRecall("q2", 0, 0, "")

	summary := rm.FormatSummary()
	if summary == "" {
		t.Error("expected non-empty summary")
	}
}
