package engine

import (
	"strings"
	"sync"
	"testing"
)

func TestNewTokenPredictor(t *testing.T) {
	tp := NewTokenPredictor()
	if tp == nil {
		t.Fatal("NewTokenPredictor returned nil")
	}
	if len(tp.History) != 0 {
		t.Errorf("expected empty history, got %d records", len(tp.History))
	}
	if len(tp.ModelFactors) == 0 {
		t.Error("expected default model factors, got empty map")
	}
	if tp.ModelFactors["claude-3-opus"] != 1.4 {
		t.Errorf("expected opus factor 1.4, got %f", tp.ModelFactors["claude-3-opus"])
	}
}

func TestClassifyTaskComplexity(t *testing.T) {
	tests := []struct {
		task     string
		expected string
	}{
		{"What is a goroutine?", "trivial"},
		{"Explain how channels work", "trivial"},
		{"Edit the function name in main.go", "simple"},
		{"Fix typo in README", "simple"},
		{"Rename variable across single file", "simple"},
		{"Debug the failing test across multiple files", "moderate"},
		{"Fix bug in authentication handler", "moderate"},
		{"Investigate memory leak across files", "moderate"},
		{"Implement feature for user authentication", "complex"},
		{"Refactor the database layer", "complex"},
		{"Add support for WebSocket connections", "complex"},
		{"Redesign the entire API layer", "extensive"},
		{"Migrate all database calls to new ORM", "extensive"},
		{"Architecture overhaul for microservices", "extensive"},
	}

	for _, tt := range tests {
		t.Run(tt.task, func(t *testing.T) {
			got := ClassifyTaskComplexity(tt.task)
			if got != tt.expected {
				t.Errorf("ClassifyTaskComplexity(%q) = %q, want %q", tt.task, got, tt.expected)
			}
		})
	}
}

func TestPredict(t *testing.T) {
	tp := NewTokenPredictor()

	t.Run("simple task with sonnet", func(t *testing.T) {
		pred := tp.Predict("Edit function in main.go", 400, "claude-sonnet-4")
		if pred == nil {
			t.Fatal("Predict returned nil")
		}
		// Simple: 2000 base input + 400 context = 2400
		if pred.InputTokens != 2400 {
			t.Errorf("expected 2400 input tokens, got %d", pred.InputTokens)
		}
		// Simple: 1000 base output * 1.0 factor = 1000
		if pred.OutputTokens != 1000 {
			t.Errorf("expected 1000 output tokens, got %d", pred.OutputTokens)
		}
		if pred.TotalTokens != 3400 {
			t.Errorf("expected 3400 total tokens, got %d", pred.TotalTokens)
		}
		if pred.EstimatedCost <= 0 {
			t.Error("expected positive cost estimate")
		}
		if pred.Confidence <= 0 || pred.Confidence > 1 {
			t.Errorf("confidence %f out of range (0,1]", pred.Confidence)
		}
		if pred.Reasoning == "" {
			t.Error("expected non-empty reasoning")
		}
	})

	t.Run("complex task with opus", func(t *testing.T) {
		pred := tp.Predict("Implement feature for caching layer", 1000, "claude-3-opus")
		if pred == nil {
			t.Fatal("Predict returned nil")
		}
		// Complex: 8000 base + 1000 context = 9000
		if pred.InputTokens != 9000 {
			t.Errorf("expected 9000 input tokens, got %d", pred.InputTokens)
		}
		// Complex: 4000 * 1.4 (opus factor) = 5600
		if pred.OutputTokens != 5600 {
			t.Errorf("expected 5600 output tokens, got %d", pred.OutputTokens)
		}
		// Opus should be more expensive
		if pred.EstimatedCost < 0.1 {
			t.Errorf("expected opus cost > $0.10, got $%.4f", pred.EstimatedCost)
		}
	})

	t.Run("trivial task with haiku", func(t *testing.T) {
		pred := tp.Predict("What is a pointer?", 100, "claude-3-5-haiku")
		if pred == nil {
			t.Fatal("Predict returned nil")
		}
		// Trivial: 500 + 100 = 600 input
		if pred.InputTokens != 600 {
			t.Errorf("expected 600 input tokens, got %d", pred.InputTokens)
		}
		// Trivial: 200 * 0.7 (haiku factor) = 140
		if pred.OutputTokens != 140 {
			t.Errorf("expected 140 output tokens, got %d", pred.OutputTokens)
		}
		// Trivial should have high confidence
		if pred.Confidence < 0.8 {
			t.Errorf("expected confidence >= 0.8 for trivial task, got %f", pred.Confidence)
		}
	})
}

func TestEstimateCost(t *testing.T) {
	tp := NewTokenPredictor()

	t.Run("sonnet pricing", func(t *testing.T) {
		cost := tp.EstimateCost(10000, "claude-sonnet-4")
		// 6000 input * $3/M + 4000 output * $15/M = $0.018 + $0.060 = $0.078
		if cost < 0.07 || cost > 0.09 {
			t.Errorf("expected cost ~$0.078 for sonnet 10k tokens, got $%.4f", cost)
		}
	})

	t.Run("haiku is cheaper", func(t *testing.T) {
		costHaiku := tp.EstimateCost(10000, "claude-3-5-haiku")
		costSonnet := tp.EstimateCost(10000, "claude-sonnet-4")
		if costHaiku >= costSonnet {
			t.Errorf("haiku ($%.4f) should be cheaper than sonnet ($%.4f)", costHaiku, costSonnet)
		}
	})

	t.Run("zero tokens", func(t *testing.T) {
		cost := tp.EstimateCost(0, "claude-sonnet-4")
		if cost != 0 {
			t.Errorf("expected 0 cost for 0 tokens, got $%.6f", cost)
		}
	})
}

func TestRecordActual(t *testing.T) {
	tp := NewTokenPredictor()

	tp.RecordActual("simple", 3000, 2800, "claude-sonnet-4")
	tp.RecordActual("complex", 12000, 15000, "claude-3-opus")

	if len(tp.History) != 2 {
		t.Fatalf("expected 2 history records, got %d", len(tp.History))
	}

	// First record: predicted 3000, actual 2800 → accuracy = 1 - |2800-3000|/3000 = 0.933
	r := tp.History[0]
	if r.TaskType != "simple" {
		t.Errorf("expected task type 'simple', got %q", r.TaskType)
	}
	if r.Accuracy < 0.93 || r.Accuracy > 0.94 {
		t.Errorf("expected accuracy ~0.933, got %f", r.Accuracy)
	}

	// Second record: predicted 12000, actual 15000 → accuracy = 1 - 3000/12000 = 0.75
	r2 := tp.History[1]
	if r2.Accuracy < 0.74 || r2.Accuracy > 0.76 {
		t.Errorf("expected accuracy ~0.75, got %f", r2.Accuracy)
	}
}

func TestCalibrate(t *testing.T) {
	tp := NewTokenPredictor()

	// Simulate consistent under-prediction for opus
	for i := 0; i < 5; i++ {
		tp.RecordActual("complex", 10000, 15000, "claude-3-opus")
	}

	originalFactor := tp.ModelFactors["claude-3-opus"]
	tp.Calibrate()
	newFactor := tp.ModelFactors["claude-3-opus"]

	if newFactor <= originalFactor {
		t.Errorf("expected factor to increase after under-prediction: original=%f, new=%f",
			originalFactor, newFactor)
	}

	// Simulate consistent over-prediction for haiku
	tp2 := NewTokenPredictor()
	for i := 0; i < 5; i++ {
		tp2.RecordActual("simple", 5000, 2000, "claude-3-5-haiku")
	}

	originalFactor2 := tp2.ModelFactors["claude-3-5-haiku"]
	tp2.Calibrate()
	newFactor2 := tp2.ModelFactors["claude-3-5-haiku"]

	if newFactor2 >= originalFactor2 {
		t.Errorf("expected factor to decrease after over-prediction: original=%f, new=%f",
			originalFactor2, newFactor2)
	}
}

func TestCalibrateNeedsMinimumRecords(t *testing.T) {
	tp := NewTokenPredictor()

	// Only 2 records - not enough to calibrate
	tp.RecordActual("simple", 1000, 2000, "claude-sonnet-4")
	tp.RecordActual("simple", 1000, 2000, "claude-sonnet-4")

	originalFactor := tp.ModelFactors["claude-sonnet-4"]
	tp.Calibrate()
	newFactor := tp.ModelFactors["claude-sonnet-4"]

	if newFactor != originalFactor {
		t.Errorf("expected no change with < 3 records: original=%f, new=%f",
			originalFactor, newFactor)
	}
}

func TestGetAccuracy(t *testing.T) {
	tp := NewTokenPredictor()

	// No history → 0
	acc := tp.GetAccuracy("claude-sonnet-4")
	if acc != 0 {
		t.Errorf("expected 0 accuracy with no history, got %f", acc)
	}

	// Add some records
	tp.RecordActual("simple", 1000, 1000, "claude-sonnet-4") // perfect
	tp.RecordActual("simple", 1000, 1200, "claude-sonnet-4") // 20% error (200/1200)
	tp.RecordActual("simple", 1000, 800, "claude-sonnet-4")  // 25% error (200/800)

	acc = tp.GetAccuracy("claude-sonnet-4")
	// MAPE = (0/1000 + 200/1200 + 200/800) / 3 = (0 + 0.1667 + 0.25) / 3 ≈ 0.1389
	if acc < 0.13 || acc > 0.15 {
		t.Errorf("expected MAPE ~0.139, got %f", acc)
	}

	// Different model should not be affected
	accOpus := tp.GetAccuracy("claude-3-opus")
	if accOpus != 0 {
		t.Errorf("expected 0 for model with no history, got %f", accOpus)
	}
}

func TestFormatPrediction(t *testing.T) {
	pred := &Prediction{
		InputTokens:   2400,
		OutputTokens:  1200,
		TotalTokens:   3600,
		EstimatedCost: 0.018,
		Confidence:    0.75,
		Reasoning:     "Task classified as \"simple\" complexity. Calibrated from 12 similar past tasks.",
	}

	result := FormatPrediction(pred, "claude-sonnet-4")

	checks := []string{
		"Token Estimate:",
		"Input:  ~2,400 tokens",
		"Output: ~1,200 tokens",
		"Total:  ~3,600 tokens",
		"Cost:   ~$0.018",
		"claude-sonnet-4",
		"Confidence: 75%",
		"12 similar past tasks",
	}

	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("FormatPrediction missing %q in output:\n%s", check, result)
		}
	}
}

func TestPredictorFormatNumber(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{15000, "15,000"},
		{1234567, "1,234,567"},
		{100, "100"},
	}

	for _, tt := range tests {
		got := predictorFormatNumber(tt.input)
		if got != tt.expected {
			t.Errorf("predictorFormatNumber(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestWarnIfExpensive(t *testing.T) {
	t.Run("within budget", func(t *testing.T) {
		pred := &Prediction{EstimatedCost: 0.05}
		warn := WarnIfExpensive(pred, 10.0)
		if warn != "" {
			t.Errorf("expected no warning, got %q", warn)
		}
	})

	t.Run("exceeds 10% of budget", func(t *testing.T) {
		pred := &Prediction{EstimatedCost: 1.5}
		warn := WarnIfExpensive(pred, 10.0)
		if warn == "" {
			t.Error("expected warning for expensive prediction")
		}
		if !strings.Contains(warn, "WARNING") {
			t.Error("warning should contain WARNING")
		}
		if !strings.Contains(warn, "15.0%") {
			t.Errorf("warning should mention percentage, got: %s", warn)
		}
	})

	t.Run("zero budget", func(t *testing.T) {
		pred := &Prediction{EstimatedCost: 1.0}
		warn := WarnIfExpensive(pred, 0)
		if warn != "" {
			t.Errorf("expected no warning for zero budget, got %q", warn)
		}
	})

	t.Run("exactly at threshold", func(t *testing.T) {
		pred := &Prediction{EstimatedCost: 1.0}
		warn := WarnIfExpensive(pred, 10.0)
		if warn != "" {
			t.Errorf("expected no warning at exactly 10%%, got %q", warn)
		}
	})

	t.Run("just over threshold", func(t *testing.T) {
		pred := &Prediction{EstimatedCost: 1.01}
		warn := WarnIfExpensive(pred, 10.0)
		if warn == "" {
			t.Error("expected warning just over threshold")
		}
	})
}

func TestTokenPredictorConcurrentAccess(t *testing.T) {
	tp := NewTokenPredictor()
	var wg sync.WaitGroup

	// Concurrent predictions
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tp.Predict("edit a file", 500, "claude-sonnet-4")
		}()
	}

	// Concurrent recording
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tp.RecordActual("simple", 1000, 1100, "claude-sonnet-4")
		}()
	}

	// Concurrent accuracy checks
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tp.GetAccuracy("claude-sonnet-4")
		}()
	}

	wg.Wait()

	if len(tp.History) != 50 {
		t.Errorf("expected 50 history records, got %d", len(tp.History))
	}
}

func TestPredictWithHistory(t *testing.T) {
	tp := NewTokenPredictor()

	// Add history so confidence gets boosted
	for i := 0; i < 10; i++ {
		tp.RecordActual("simple", 3000, 3200, "claude-sonnet-4")
	}

	pred := tp.Predict("edit this file please", 500, "claude-sonnet-4")

	// Should mention calibration in reasoning
	if !strings.Contains(pred.Reasoning, "Calibrated from") {
		t.Error("expected reasoning to mention calibration history")
	}

	// Confidence should be boosted above base
	if pred.Confidence <= 0.75 {
		t.Errorf("expected boosted confidence > 0.75, got %f", pred.Confidence)
	}
}

func TestClassifyDefaultsToSimple(t *testing.T) {
	// A task with no matching keywords defaults to simple
	result := ClassifyTaskComplexity("do the thing with the stuff")
	if result != "simple" {
		t.Errorf("expected default 'simple', got %q", result)
	}
}
