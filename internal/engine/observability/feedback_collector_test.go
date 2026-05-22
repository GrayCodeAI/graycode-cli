package observability

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewFeedbackCollector(t *testing.T) {
	fc := NewFeedbackCollector("/tmp/test-feedback")
	if fc == nil {
		t.Fatal("expected non-nil FeedbackCollector")
	}
	if fc.Dir != "/tmp/test-feedback" {
		t.Errorf("expected dir /tmp/test-feedback, got %s", fc.Dir)
	}
	if len(fc.Entries) != 0 {
		t.Errorf("expected empty entries, got %d", len(fc.Entries))
	}
	if len(fc.ImplicitSignals) != 0 {
		t.Errorf("expected empty signals, got %d", len(fc.ImplicitSignals))
	}
}

func TestRecordExplicit(t *testing.T) {
	fc := NewFeedbackCollector("")

	// Valid feedback
	err := fc.RecordExplicit(4, "great response", "quality", "sess-1", "code_generation")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fc.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fc.Entries))
	}
	if fc.Entries[0].Rating != 4 {
		t.Errorf("expected rating 4, got %d", fc.Entries[0].Rating)
	}
	if fc.Entries[0].Comment != "great response" {
		t.Errorf("expected comment 'great response', got %q", fc.Entries[0].Comment)
	}
	if fc.Entries[0].Category != "quality" {
		t.Errorf("expected category 'quality', got %q", fc.Entries[0].Category)
	}
	if fc.Entries[0].SessionID != "sess-1" {
		t.Errorf("expected session 'sess-1', got %q", fc.Entries[0].SessionID)
	}
	if fc.Entries[0].TaskType != "code_generation" {
		t.Errorf("expected task type 'code_generation', got %q", fc.Entries[0].TaskType)
	}
	if fc.Entries[0].ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestRecordExplicit_InvalidRating(t *testing.T) {
	fc := NewFeedbackCollector("")

	err := fc.RecordExplicit(0, "too low", "quality", "s1", "test")
	if err == nil {
		t.Error("expected error for rating 0")
	}

	err = fc.RecordExplicit(6, "too high", "quality", "s1", "test")
	if err == nil {
		t.Error("expected error for rating 6")
	}

	err = fc.RecordExplicit(-1, "negative", "quality", "s1", "test")
	if err == nil {
		t.Error("expected error for negative rating")
	}
}

func TestRecordExplicit_InvalidCategory(t *testing.T) {
	fc := NewFeedbackCollector("")

	err := fc.RecordExplicit(3, "comment", "invalid_category", "s1", "test")
	if err == nil {
		t.Error("expected error for invalid category")
	}
	if !strings.Contains(err.Error(), "invalid category") {
		t.Errorf("error should mention invalid category, got: %v", err)
	}
}

func TestRecordImplicit(t *testing.T) {
	fc := NewFeedbackCollector("")

	signal := ImplicitSignal{
		Type:      "accepted",
		SessionID: "sess-1",
		ToolName:  "code_write",
		Timestamp: time.Now(),
	}
	err := fc.RecordImplicit(signal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fc.ImplicitSignals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(fc.ImplicitSignals))
	}
	if fc.ImplicitSignals[0].Type != "accepted" {
		t.Errorf("expected type 'accepted', got %q", fc.ImplicitSignals[0].Type)
	}
}

func TestRecordImplicit_InvalidType(t *testing.T) {
	fc := NewFeedbackCollector("")

	signal := ImplicitSignal{
		Type:      "invalid_type",
		SessionID: "sess-1",
	}
	err := fc.RecordImplicit(signal)
	if err == nil {
		t.Error("expected error for invalid signal type")
	}
}

func TestRecordImplicit_ZeroTimestamp(t *testing.T) {
	fc := NewFeedbackCollector("")

	signal := ImplicitSignal{
		Type:      "undone",
		SessionID: "sess-1",
	}
	err := fc.RecordImplicit(signal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fc.ImplicitSignals[0].Timestamp.IsZero() {
		t.Error("expected timestamp to be set automatically")
	}
}

func TestGetSatisfactionScore_Empty(t *testing.T) {
	fc := NewFeedbackCollector("")
	score := fc.GetSatisfactionScore()
	if score != 0.0 {
		t.Errorf("expected 0.0 for empty collector, got %f", score)
	}
}

func TestGetSatisfactionScore_ExplicitOnly(t *testing.T) {
	fc := NewFeedbackCollector("")

	_ = fc.RecordExplicit(5, "", "quality", "s1", "test")
	_ = fc.RecordExplicit(3, "", "speed", "s2", "test")

	score := fc.GetSatisfactionScore()
	// Average of 5 and 3 is 4.0 (all explicit, weighted equally among themselves)
	if score != 4.0 {
		t.Errorf("expected score 4.0, got %f", score)
	}
}

func TestGetSatisfactionScore_ImplicitOnly(t *testing.T) {
	fc := NewFeedbackCollector("")

	_ = fc.RecordImplicit(ImplicitSignal{Type: "accepted", SessionID: "s1", Timestamp: time.Now()})
	_ = fc.RecordImplicit(ImplicitSignal{Type: "accepted", SessionID: "s2", Timestamp: time.Now()})

	score := fc.GetSatisfactionScore()
	// Two "accepted" signals, each scoring 4.5, average = 4.5
	if score != 4.5 {
		t.Errorf("expected score 4.5, got %f", score)
	}
}

func TestGetSatisfactionScore_Mixed(t *testing.T) {
	fc := NewFeedbackCollector("")

	// 1 explicit rating of 5, weighted 3x
	_ = fc.RecordExplicit(5, "", "quality", "s1", "test")
	// 1 implicit accepted (4.5), weighted 1x
	_ = fc.RecordImplicit(ImplicitSignal{Type: "accepted", SessionID: "s1", Timestamp: time.Now()})

	score := fc.GetSatisfactionScore()
	// (5*3 + 4.5*1) / (3+1) = 19.5/4 = 4.875 -> rounded to 4.88
	expected := 4.88
	if score != expected {
		t.Errorf("expected score %f, got %f", expected, score)
	}
}

func TestGetTrends_InsufficientData(t *testing.T) {
	fc := NewFeedbackCollector("")
	_ = fc.RecordExplicit(4, "", "quality", "s1", "test")

	trends := fc.GetTrends()
	if !strings.Contains(trends, "Insufficient data") {
		t.Errorf("expected insufficient data message, got %q", trends)
	}
}

func TestGetTrends_Improving(t *testing.T) {
	fc := NewFeedbackCollector("")

	// Older entries with lower ratings
	fc.mu.Lock()
	for i := 0; i < 5; i++ {
		fc.Entries = append(fc.Entries, Feedback{
			Rating:    2,
			Category:  "quality",
			Timestamp: time.Now().Add(-time.Duration(10-i) * time.Hour),
		})
	}
	// Recent entries with higher ratings
	for i := 0; i < 5; i++ {
		fc.Entries = append(fc.Entries, Feedback{
			Rating:    5,
			Category:  "quality",
			Timestamp: time.Now().Add(-time.Duration(5-i) * time.Minute),
		})
	}
	fc.mu.Unlock()

	trends := fc.GetTrends()
	if !strings.Contains(trends, "improving") {
		t.Errorf("expected improving trend, got %q", trends)
	}
}

func TestGetTrends_SpeedComplaints(t *testing.T) {
	fc := NewFeedbackCollector("")

	fc.mu.Lock()
	for i := 0; i < 5; i++ {
		fc.Entries = append(fc.Entries, Feedback{
			Rating:    2,
			Category:  "speed",
			Timestamp: time.Now().Add(-time.Duration(i) * time.Minute),
		})
	}
	fc.mu.Unlock()

	trends := fc.GetTrends()
	if !strings.Contains(trends, "Speed complaints") {
		t.Errorf("expected speed complaints mentioned, got %q", trends)
	}
}

func TestGetByCategory(t *testing.T) {
	fc := NewFeedbackCollector("")

	_ = fc.RecordExplicit(5, "fast", "speed", "s1", "test")
	_ = fc.RecordExplicit(4, "good code", "quality", "s2", "test")
	_ = fc.RecordExplicit(3, "ok speed", "speed", "s3", "test")

	speedFeedback := fc.GetByCategory("speed")
	if len(speedFeedback) != 2 {
		t.Fatalf("expected 2 speed entries, got %d", len(speedFeedback))
	}

	qualityFeedback := fc.GetByCategory("quality")
	if len(qualityFeedback) != 1 {
		t.Fatalf("expected 1 quality entry, got %d", len(qualityFeedback))
	}

	noneFeedback := fc.GetByCategory("accuracy")
	if len(noneFeedback) != 0 {
		t.Errorf("expected 0 accuracy entries, got %d", len(noneFeedback))
	}
}

func TestIdentifyIssues_NoIssues(t *testing.T) {
	fc := NewFeedbackCollector("")

	_ = fc.RecordImplicit(ImplicitSignal{Type: "accepted", SessionID: "s1", Timestamp: time.Now()})

	issues := fc.IdentifyIssues()
	if len(issues) != 1 || issues[0] != "No issues identified" {
		t.Errorf("expected no issues, got %v", issues)
	}
}

func TestIdentifyIssues_MultipleUndos(t *testing.T) {
	fc := NewFeedbackCollector("")

	for i := 0; i < 4; i++ {
		_ = fc.RecordImplicit(ImplicitSignal{Type: "undone", SessionID: "s1", Timestamp: time.Now()})
	}

	issues := fc.IdentifyIssues()
	found := false
	for _, issue := range issues {
		if strings.Contains(issue, "Multiple undos suggest incorrect edits") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected undo issue, got %v", issues)
	}
}

func TestIdentifyIssues_RetriesAfterCodeWrite(t *testing.T) {
	fc := NewFeedbackCollector("")

	for i := 0; i < 3; i++ {
		_ = fc.RecordImplicit(ImplicitSignal{
			Type:      "retried",
			SessionID: "s1",
			ToolName:  "code_write",
			Timestamp: time.Now(),
		})
	}

	issues := fc.IdentifyIssues()
	found := false
	for _, issue := range issues {
		if strings.Contains(issue, "Retries after code_write indicate quality issues") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected code_write retry issue, got %v", issues)
	}
}

func TestIdentifyIssues_HighRejectionRate(t *testing.T) {
	fc := NewFeedbackCollector("")

	for i := 0; i < 4; i++ {
		_ = fc.RecordImplicit(ImplicitSignal{Type: "rejected", SessionID: "s1", Timestamp: time.Now()})
	}

	issues := fc.IdentifyIssues()
	found := false
	for _, issue := range issues {
		if strings.Contains(issue, "High rejection rate") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected rejection issue, got %v", issues)
	}
}

func TestIdentifyIssues_LowCategoryScores(t *testing.T) {
	fc := NewFeedbackCollector("")

	_ = fc.RecordExplicit(1, "bad", "accuracy", "s1", "test")
	_ = fc.RecordExplicit(2, "poor", "accuracy", "s2", "test")

	issues := fc.IdentifyIssues()
	found := false
	for _, issue := range issues {
		if strings.Contains(issue, "Low accuracy scores") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected low accuracy issue, got %v", issues)
	}
}

func TestFeedbackCollectorFormatReport(t *testing.T) {
	fc := NewFeedbackCollector("")

	_ = fc.RecordExplicit(5, "perfect", "quality", "s1", "code")
	_ = fc.RecordExplicit(4, "good", "speed", "s2", "code")
	_ = fc.RecordExplicit(4, "nice", "accuracy", "s3", "code")
	_ = fc.RecordExplicit(5, "great", "helpfulness", "s4", "code")
	_ = fc.RecordImplicit(ImplicitSignal{Type: "accepted", SessionID: "s1", Timestamp: time.Now()})
	_ = fc.RecordImplicit(ImplicitSignal{Type: "accepted", SessionID: "s2", Timestamp: time.Now()})

	report := fc.FormatReport()

	if !strings.Contains(report, "Feedback Report:") {
		t.Error("report should contain header")
	}
	if !strings.Contains(report, "Satisfaction:") {
		t.Error("report should contain satisfaction score")
	}
	if !strings.Contains(report, "acceptance rate") {
		t.Error("report should contain acceptance rate")
	}
	if !strings.Contains(report, "Quality:") {
		t.Error("report should contain Quality category")
	}
	if !strings.Contains(report, "Speed:") {
		t.Error("report should contain Speed category")
	}
	if !strings.Contains(report, "Accuracy:") {
		t.Error("report should contain Accuracy category")
	}
	if !strings.Contains(report, "Helpfulness:") {
		t.Error("report should contain Helpfulness category")
	}
	if !strings.Contains(report, "Trends:") {
		t.Error("report should contain trends")
	}
	if !strings.Contains(report, "Issues:") {
		t.Error("report should contain issues")
	}
}

func TestFeedbackCollectorSaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	fc := NewFeedbackCollector(dir)
	_ = fc.RecordExplicit(5, "excellent", "quality", "sess-1", "code_gen")
	_ = fc.RecordExplicit(3, "ok", "speed", "sess-2", "refactor")
	_ = fc.RecordImplicit(ImplicitSignal{Type: "accepted", SessionID: "sess-1", ToolName: "code_write", Timestamp: time.Now()})
	_ = fc.RecordImplicit(ImplicitSignal{Type: "undone", SessionID: "sess-2", ToolName: "file_write", Timestamp: time.Now()})

	err := fc.Save()
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "feedback.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("feedback.json was not created")
	}

	// Load into a new collector
	fc2 := NewFeedbackCollector(dir)
	err = fc2.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(fc2.Entries) != 2 {
		t.Errorf("expected 2 entries after load, got %d", len(fc2.Entries))
	}
	if len(fc2.ImplicitSignals) != 2 {
		t.Errorf("expected 2 signals after load, got %d", len(fc2.ImplicitSignals))
	}

	if fc2.Entries[0].Rating != 5 {
		t.Errorf("expected first rating 5, got %d", fc2.Entries[0].Rating)
	}
	if fc2.Entries[0].Comment != "excellent" {
		t.Errorf("expected comment 'excellent', got %q", fc2.Entries[0].Comment)
	}
	if fc2.ImplicitSignals[0].Type != "accepted" {
		t.Errorf("expected first signal 'accepted', got %q", fc2.ImplicitSignals[0].Type)
	}
}

func TestFeedbackCollectorLoad_NonexistentFile(t *testing.T) {
	dir := t.TempDir()
	fc := NewFeedbackCollector(dir)

	// Should not error on missing file
	err := fc.Load()
	if err != nil {
		t.Errorf("Load should not error on missing file, got: %v", err)
	}
}

func TestSave_NoDir(t *testing.T) {
	fc := NewFeedbackCollector("")
	err := fc.Save()
	if err == nil {
		t.Error("expected error when dir is empty")
	}
}

func TestLoad_NoDir(t *testing.T) {
	fc := NewFeedbackCollector("")
	err := fc.Load()
	if err == nil {
		t.Error("expected error when dir is empty")
	}
}

func TestFeedbackCollectorConcurrentAccess(t *testing.T) {
	fc := NewFeedbackCollector("")

	var wg sync.WaitGroup
	// Concurrent explicit recordings
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rating := (i % 5) + 1
			categories := []string{"quality", "speed", "accuracy", "helpfulness"}
			_ = fc.RecordExplicit(rating, "test", categories[i%4], "sess", "test")
		}(i)
	}

	// Concurrent implicit recordings
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			types := []string{"accepted", "rejected", "edited", "undone", "retried"}
			_ = fc.RecordImplicit(ImplicitSignal{
				Type:      types[i%5],
				SessionID: "sess",
				Timestamp: time.Now(),
			})
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = fc.GetSatisfactionScore()
			_ = fc.GetTrends()
			_ = fc.GetByCategory("quality")
			_ = fc.IdentifyIssues()
			_ = fc.FormatReport()
		}()
	}

	wg.Wait()

	if len(fc.Entries) != 20 {
		t.Errorf("expected 20 entries, got %d", len(fc.Entries))
	}
	if len(fc.ImplicitSignals) != 20 {
		t.Errorf("expected 20 signals, got %d", len(fc.ImplicitSignals))
	}
}

func TestRenderStars(t *testing.T) {
	tests := []struct {
		rating float64
		want   string
	}{
		{0, ""},
		{1.0, "⭐"},
		{2.5, "⭐⭐½"},
		{3.3, "⭐⭐⭐¼"},
		{4.8, "⭐⭐⭐⭐¾"},
		{5.0, "⭐⭐⭐⭐⭐"},
	}

	for _, tt := range tests {
		got := renderStars(tt.rating)
		if got != tt.want {
			t.Errorf("renderStars(%.1f) = %q, want %q", tt.rating, got, tt.want)
		}
	}
}

func TestImplicitSignalScore(t *testing.T) {
	tests := []struct {
		signalType string
		want       float64
	}{
		{"accepted", 4.5},
		{"edited", 3.0},
		{"rejected", 2.0},
		{"undone", 1.5},
		{"retried", 2.0},
		{"unknown", 3.0},
	}

	for _, tt := range tests {
		got := implicitSignalScore(tt.signalType)
		if got != tt.want {
			t.Errorf("implicitSignalScore(%q) = %f, want %f", tt.signalType, got, tt.want)
		}
	}
}

func TestGetSatisfactionScore_AllSignalTypes(t *testing.T) {
	fc := NewFeedbackCollector("")

	// Record one of each signal type
	_ = fc.RecordImplicit(ImplicitSignal{Type: "accepted", SessionID: "s1", Timestamp: time.Now()})
	_ = fc.RecordImplicit(ImplicitSignal{Type: "rejected", SessionID: "s2", Timestamp: time.Now()})
	_ = fc.RecordImplicit(ImplicitSignal{Type: "edited", SessionID: "s3", Timestamp: time.Now()})
	_ = fc.RecordImplicit(ImplicitSignal{Type: "undone", SessionID: "s4", Timestamp: time.Now()})
	_ = fc.RecordImplicit(ImplicitSignal{Type: "retried", SessionID: "s5", Timestamp: time.Now()})

	score := fc.GetSatisfactionScore()
	// (4.5 + 2.0 + 3.0 + 1.5 + 2.0) / 5 = 13.0/5 = 2.6
	if score != 2.6 {
		t.Errorf("expected score 2.6, got %f", score)
	}
}

func TestFormatReport_Empty(t *testing.T) {
	fc := NewFeedbackCollector("")
	report := fc.FormatReport()
	if !strings.Contains(report, "Feedback Report:") {
		t.Error("empty report should still have header")
	}
	if !strings.Contains(report, "0.0/5.0") {
		t.Error("empty report should show 0.0 satisfaction")
	}
}
