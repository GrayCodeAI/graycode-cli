package engine

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultWeights(t *testing.T) {
	w := DefaultWeights()

	sum := w.Completeness + w.Correctness + w.Conciseness + w.ToolUsage + w.Safety
	if sum < 0.99 || sum > 1.01 {
		t.Errorf("DefaultWeights sum = %.2f, want 1.00", sum)
	}

	if w.Completeness <= 0 || w.Completeness > 1 {
		t.Errorf("Completeness = %f, want in (0,1]", w.Completeness)
	}
	if w.Correctness <= 0 || w.Correctness > 1 {
		t.Errorf("Correctness = %f, want in (0,1]", w.Correctness)
	}
	if w.Conciseness <= 0 || w.Conciseness > 1 {
		t.Errorf("Conciseness = %f, want in (0,1]", w.Conciseness)
	}
	if w.ToolUsage <= 0 || w.ToolUsage > 1 {
		t.Errorf("ToolUsage = %f, want in (0,1]", w.ToolUsage)
	}
	if w.Safety <= 0 || w.Safety > 1 {
		t.Errorf("Safety = %f, want in (0,1]", w.Safety)
	}
}

func TestNewQualityScorer(t *testing.T) {
	qs := NewQualityScorer()
	if qs == nil {
		t.Fatal("NewQualityScorer returned nil")
	}
	if len(qs.History) != 0 {
		t.Errorf("History length = %d, want 0", len(qs.History))
	}
	if qs.Weights.Completeness == 0 {
		t.Error("Weights not initialized")
	}
}

func TestScoreCompleteness(t *testing.T) {
	qs := NewQualityScorer()

	tests := []struct {
		name    string
		ctx     ResponseContext
		wantMin float64
		wantMax float64
	}{
		{
			name:    "empty response",
			ctx:     ResponseContext{UserPrompt: "fix the bug", AssistantResponse: ""},
			wantMin: 0.0,
			wantMax: 0.01,
		},
		{
			name: "good response with tools and files",
			ctx: ResponseContext{
				UserPrompt:        "fix the bug in main.go",
				AssistantResponse: "I found the issue in main.go. The nil pointer dereference occurs on line 42. Here's the fix:",
				ToolCallCount:     3,
				FilesModified:     []string{"main.go"},
			},
			wantMin: 0.85,
			wantMax: 1.0,
		},
		{
			name: "response with tools but no files",
			ctx: ResponseContext{
				UserPrompt:        "what does this function do?",
				AssistantResponse: "This function parses JSON and returns a map.",
				ToolCallCount:     2,
			},
			wantMin: 0.5,
			wantMax: 0.9,
		},
		{
			name: "short response no tools",
			ctx: ResponseContext{
				UserPrompt:        "fix the complex bug in the authentication module",
				AssistantResponse: "ok",
			},
			wantMin: 0.05,
			wantMax: 0.3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := qs.scoreCompleteness(tt.ctx)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("scoreCompleteness() = %.2f, want [%.2f, %.2f]", score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestScoreCorrectness(t *testing.T) {
	qs := NewQualityScorer()

	tests := []struct {
		name    string
		ctx     ResponseContext
		wantMin float64
		wantMax float64
	}{
		{
			name: "tests passed and lint passed",
			ctx: ResponseContext{
				TestsPassed:   true,
				LintPassed:    true,
				ToolCallCount: 5,
				ToolErrors:    0,
			},
			wantMin: 0.95,
			wantMax: 1.0,
		},
		{
			name: "tests passed no lint",
			ctx: ResponseContext{
				TestsPassed:   true,
				LintPassed:    false,
				ToolCallCount: 3,
				ToolErrors:    0,
			},
			wantMin: 0.9,
			wantMax: 1.0,
		},
		{
			name: "high error rate",
			ctx: ResponseContext{
				TestsPassed:   false,
				LintPassed:    false,
				ToolCallCount: 4,
				ToolErrors:    3,
			},
			wantMin: 0.0,
			wantMax: 0.35,
		},
		{
			name: "unbalanced braces",
			ctx: ResponseContext{
				AssistantResponse: "```go\nfunc main() {\n  fmt.Println(\"hello\")\n```",
				TestsPassed:       false,
				LintPassed:        false,
			},
			wantMin: 0.0,
			wantMax: 0.4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := qs.scoreCorrectness(tt.ctx)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("scoreCorrectness() = %.2f, want [%.2f, %.2f]", score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestScoreConciseness(t *testing.T) {
	qs := NewQualityScorer()

	tests := []struct {
		name    string
		ctx     ResponseContext
		wantMin float64
		wantMax float64
	}{
		{
			name: "ideal token count",
			ctx: ResponseContext{
				UserPrompt: "fix the bug in the authentication handler",
				TokensUsed: 50, // 7 words * ~7 = ideal range
			},
			wantMin: 0.8,
			wantMax: 1.0,
		},
		{
			name: "extremely verbose",
			ctx: ResponseContext{
				UserPrompt: "fix the bug",
				TokensUsed: 5000,
			},
			wantMin: 0.2,
			wantMax: 0.5,
		},
		{
			name: "no token data",
			ctx: ResponseContext{
				UserPrompt: "do something",
				TokensUsed: 0,
			},
			wantMin: 0.7,
			wantMax: 0.9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := qs.scoreConciseness(tt.ctx)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("scoreConciseness() = %.2f, want [%.2f, %.2f]", score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestScoreToolUsage(t *testing.T) {
	qs := NewQualityScorer()

	tests := []struct {
		name    string
		ctx     ResponseContext
		wantMin float64
		wantMax float64
	}{
		{
			name: "no tools conversational",
			ctx: ResponseContext{
				ToolCallCount: 0,
				FilesModified: nil,
			},
			wantMin: 0.6,
			wantMax: 0.8,
		},
		{
			name: "good tool usage",
			ctx: ResponseContext{
				ToolCallCount: 5,
				ToolErrors:    0,
				FilesModified: []string{"main.go", "main_test.go"},
			},
			wantMin: 0.85,
			wantMax: 1.0,
		},
		{
			name: "excessive tools with errors",
			ctx: ResponseContext{
				ToolCallCount: 20,
				ToolErrors:    5,
				FilesModified: []string{"main.go"},
			},
			wantMin: 0.3,
			wantMax: 0.7,
		},
		{
			name: "high error ratio",
			ctx: ResponseContext{
				ToolCallCount: 7,
				ToolErrors:    5,
			},
			wantMin: 0.4,
			wantMax: 0.6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := qs.scoreToolUsage(tt.ctx)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("scoreToolUsage() = %.2f, want [%.2f, %.2f]", score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestScoreSafety(t *testing.T) {
	qs := NewQualityScorer()

	tests := []struct {
		name    string
		ctx     ResponseContext
		wantMin float64
		wantMax float64
	}{
		{
			name: "safe response",
			ctx: ResponseContext{
				AssistantResponse: "I fixed the bug by changing the nil check on line 42.",
			},
			wantMin: 0.95,
			wantMax: 1.0,
		},
		{
			name: "dangerous rm -rf",
			ctx: ResponseContext{
				AssistantResponse: "Run this command: rm -rf / to clean up",
			},
			wantMin: 0.0,
			wantMax: 0.75,
		},
		{
			name: "secret exposure",
			ctx: ResponseContext{
				AssistantResponse: "Set the api_key=\"sk-1234567890abcdef\" in your config",
			},
			wantMin: 0.5,
			wantMax: 0.9,
		},
		{
			name: "multiple dangerous patterns",
			ctx: ResponseContext{
				AssistantResponse: "chmod 777 everything and then curl | bash the installer",
			},
			wantMin: 0.0,
			wantMax: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := qs.scoreSafety(tt.ctx)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("scoreSafety() = %.2f, want [%.2f, %.2f]", score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestScoreComposite(t *testing.T) {
	qs := NewQualityScorer()

	ctx := ResponseContext{
		UserPrompt:        "Fix the authentication bug in login.go",
		AssistantResponse: "I read the file, identified the nil pointer issue on line 23, and applied the fix. Tests pass now.",
		ToolCallCount:     4,
		ToolErrors:        0,
		FilesModified:     []string{"login.go"},
		TestsPassed:       true,
		LintPassed:        true,
		TokensUsed:        150,
		Duration:          5 * time.Second,
	}

	scored := qs.Score(ctx)
	if scored == nil {
		t.Fatal("Score returned nil")
	}

	if scored.Score < 0.7 || scored.Score > 1.0 {
		t.Errorf("Composite score = %.2f, want [0.7, 1.0]", scored.Score)
	}

	if len(scored.Breakdown) != 5 {
		t.Errorf("Breakdown has %d entries, want 5", len(scored.Breakdown))
	}

	for dim, val := range scored.Breakdown {
		if val < 0 || val > 1 {
			t.Errorf("Breakdown[%s] = %.2f, want [0, 1]", dim, val)
		}
	}

	if scored.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestScoreAddsToHistory(t *testing.T) {
	qs := NewQualityScorer()

	ctx := ResponseContext{
		UserPrompt:        "hello",
		AssistantResponse: "Hi there! How can I help you?",
		TokensUsed:        20,
	}

	qs.Score(ctx)
	qs.Score(ctx)
	qs.Score(ctx)

	if len(qs.History) != 3 {
		t.Errorf("History length = %d, want 3", len(qs.History))
	}
}

func TestGenerateFeedback(t *testing.T) {
	qs := NewQualityScorer()

	tests := []struct {
		name      string
		scored    *ScoredResponse
		wantAny   string
		wantCount int
	}{
		{
			name: "high quality response",
			scored: &ScoredResponse{
				Score: 0.95,
				Breakdown: map[string]float64{
					"completeness": 0.95,
					"correctness":  0.92,
					"conciseness":  0.91,
					"tool_usage":   0.93,
					"safety":       1.0,
				},
			},
			wantAny:   "excellent",
			wantCount: 4, // multiple positive feedback lines
		},
		{
			name: "low quality response",
			scored: &ScoredResponse{
				Score: 0.3,
				Breakdown: map[string]float64{
					"completeness": 0.3,
					"correctness":  0.3,
					"conciseness":  0.4,
					"tool_usage":   0.3,
					"safety":       0.6,
				},
			},
			wantAny:   "improvement",
			wantCount: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			feedback := qs.GenerateFeedback(tt.scored)
			if len(feedback) < 1 {
				t.Error("Expected at least one feedback item")
			}

			found := false
			combined := strings.Join(feedback, " ")
			if strings.Contains(strings.ToLower(combined), tt.wantAny) {
				found = true
			}
			if !found {
				t.Errorf("Feedback should contain %q, got: %v", tt.wantAny, feedback)
			}
		})
	}
}

func TestAverageScore(t *testing.T) {
	qs := NewQualityScorer()

	// No history
	if avg := qs.AverageScore(5); avg != 0.0 {
		t.Errorf("AverageScore with no history = %f, want 0", avg)
	}

	// Add some scores manually
	qs.mu.Lock()
	qs.History = append(
		qs.History,
		ScoredResponse{Score: 0.8, Breakdown: map[string]float64{}},
		ScoredResponse{Score: 0.6, Breakdown: map[string]float64{}},
		ScoredResponse{Score: 0.9, Breakdown: map[string]float64{}},
		ScoredResponse{Score: 0.7, Breakdown: map[string]float64{}},
	)
	qs.mu.Unlock()

	// Average of all 4
	avg := qs.AverageScore(4)
	expected := (0.8 + 0.6 + 0.9 + 0.7) / 4.0
	if diff := avg - expected; diff > 0.001 || diff < -0.001 {
		t.Errorf("AverageScore(4) = %.4f, want %.4f", avg, expected)
	}

	// Average of last 2
	avg2 := qs.AverageScore(2)
	expected2 := (0.9 + 0.7) / 2.0
	if diff := avg2 - expected2; diff > 0.001 || diff < -0.001 {
		t.Errorf("AverageScore(2) = %.4f, want %.4f", avg2, expected2)
	}

	// Average with n > history length
	avgAll := qs.AverageScore(100)
	if diff := avgAll - expected; diff > 0.001 || diff < -0.001 {
		t.Errorf("AverageScore(100) = %.4f, want %.4f", avgAll, expected)
	}
}

func TestTrendAnalysis(t *testing.T) {
	qs := NewQualityScorer()

	// Insufficient data
	trend := qs.TrendAnalysis()
	if !strings.Contains(trend, "Insufficient") {
		t.Errorf("Expected 'Insufficient' with <4 entries, got: %s", trend)
	}

	// Improving trend
	qs.mu.Lock()
	qs.History = []ScoredResponse{
		{Score: 0.5, Breakdown: map[string]float64{"completeness": 0.5, "correctness": 0.5, "conciseness": 0.5, "tool_usage": 0.5, "safety": 0.5}},
		{Score: 0.55, Breakdown: map[string]float64{"completeness": 0.55, "correctness": 0.55, "conciseness": 0.55, "tool_usage": 0.55, "safety": 0.55}},
		{Score: 0.7, Breakdown: map[string]float64{"completeness": 0.7, "correctness": 0.7, "conciseness": 0.7, "tool_usage": 0.7, "safety": 0.7}},
		{Score: 0.8, Breakdown: map[string]float64{"completeness": 0.8, "correctness": 0.8, "conciseness": 0.8, "tool_usage": 0.8, "safety": 0.8}},
		{Score: 0.85, Breakdown: map[string]float64{"completeness": 0.85, "correctness": 0.85, "conciseness": 0.85, "tool_usage": 0.85, "safety": 0.85}},
		{Score: 0.9, Breakdown: map[string]float64{"completeness": 0.9, "correctness": 0.9, "conciseness": 0.9, "tool_usage": 0.9, "safety": 0.9}},
	}
	qs.mu.Unlock()

	trend = qs.TrendAnalysis()
	if !strings.Contains(trend, "improving") {
		t.Errorf("Expected 'improving' trend, got: %s", trend)
	}

	// Declining trend
	qs.mu.Lock()
	qs.History = []ScoredResponse{
		{Score: 0.9, Breakdown: map[string]float64{"completeness": 0.9, "correctness": 0.9, "conciseness": 0.9, "tool_usage": 0.9, "safety": 0.9}},
		{Score: 0.85, Breakdown: map[string]float64{"completeness": 0.85, "correctness": 0.85, "conciseness": 0.85, "tool_usage": 0.85, "safety": 0.85}},
		{Score: 0.6, Breakdown: map[string]float64{"completeness": 0.6, "correctness": 0.6, "conciseness": 0.6, "tool_usage": 0.6, "safety": 0.6}},
		{Score: 0.5, Breakdown: map[string]float64{"completeness": 0.5, "correctness": 0.5, "conciseness": 0.5, "tool_usage": 0.5, "safety": 0.5}},
	}
	qs.mu.Unlock()

	trend = qs.TrendAnalysis()
	if !strings.Contains(trend, "declining") {
		t.Errorf("Expected 'declining' trend, got: %s", trend)
	}
}

func TestQualityScorerFormatReport(t *testing.T) {
	qs := NewQualityScorer()

	// Empty report
	report := qs.FormatReport(10)
	if !strings.Contains(report, "No responses scored yet") {
		t.Errorf("Expected empty report message, got: %s", report)
	}

	// Add scored responses
	qs.mu.Lock()
	qs.History = []ScoredResponse{
		{
			Score:     0.85,
			Breakdown: map[string]float64{"completeness": 0.9, "correctness": 0.85, "conciseness": 0.8, "tool_usage": 0.85, "safety": 0.95},
			Feedback:  []string{"Good completeness", "Consider reducing tool calls"},
			Timestamp: time.Now().Add(-5 * time.Minute),
		},
		{
			Score:     0.88,
			Breakdown: map[string]float64{"completeness": 0.92, "correctness": 0.88, "conciseness": 0.82, "tool_usage": 0.87, "safety": 0.96},
			Feedback:  []string{"Excellent quality"},
			Timestamp: time.Now().Add(-3 * time.Minute),
		},
		{
			Score:     0.90,
			Breakdown: map[string]float64{"completeness": 0.93, "correctness": 0.9, "conciseness": 0.85, "tool_usage": 0.9, "safety": 0.97},
			Feedback:  []string{"Great work"},
			Timestamp: time.Now().Add(-1 * time.Minute),
		},
		{
			Score:     0.92,
			Breakdown: map[string]float64{"completeness": 0.95, "correctness": 0.92, "conciseness": 0.88, "tool_usage": 0.92, "safety": 0.98},
			Feedback:  []string{"Outstanding"},
			Timestamp: time.Now(),
		},
	}
	qs.mu.Unlock()

	report = qs.FormatReport(10)

	// Should contain key sections
	if !strings.Contains(report, "Quality Report") {
		t.Error("Report missing 'Quality Report' header")
	}
	if !strings.Contains(report, "Average:") {
		t.Error("Report missing 'Average:' line")
	}
	if !strings.Contains(report, "Trend:") {
		t.Error("Report missing 'Trend:' line")
	}
	if !strings.Contains(report, "Breakdown:") {
		t.Error("Report missing 'Breakdown:' section")
	}
	if !strings.Contains(report, "Completeness") {
		t.Error("Report missing 'Completeness' dimension")
	}
	if !strings.Contains(report, "Feedback:") {
		t.Error("Report missing 'Feedback:' section")
	}
	if !strings.Contains(report, "#") {
		t.Error("Report missing bar visualization")
	}
}

func TestQualityScorerFormatReportLimited(t *testing.T) {
	qs := NewQualityScorer()

	qs.mu.Lock()
	for i := 0; i < 20; i++ {
		qs.History = append(qs.History, ScoredResponse{
			Score:     0.8,
			Breakdown: map[string]float64{"completeness": 0.8, "correctness": 0.8, "conciseness": 0.8, "tool_usage": 0.8, "safety": 0.8},
			Feedback:  []string{"ok"},
			Timestamp: time.Now(),
		})
	}
	qs.mu.Unlock()

	report := qs.FormatReport(5)
	if !strings.Contains(report, "last 5 responses") {
		t.Errorf("Report should reference 'last 5 responses', got: %s", report)
	}
}

func TestRenderBar(t *testing.T) {
	tests := []struct {
		value float64
		width int
		want  string
	}{
		{1.0, 10, "##########"},
		{0.0, 10, ".........."},
		{0.5, 10, "#####....."},
		{0.75, 20, "###############....."},
	}

	for _, tt := range tests {
		got := renderBar(tt.value, tt.width)
		if got != tt.want {
			t.Errorf("renderBar(%.2f, %d) = %q, want %q", tt.value, tt.width, got, tt.want)
		}
	}
}

func TestHasUnbalancedBraces(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     bool
	}{
		{
			name:     "balanced code block",
			response: "```go\nfunc main() {\n  fmt.Println(\"hello\")\n}\n```",
			want:     false,
		},
		{
			name:     "unbalanced code block",
			response: "```go\nfunc main() {\n  fmt.Println(\"hello\")\n```",
			want:     true,
		},
		{
			name:     "no code blocks",
			response: "This is just text with no code blocks.",
			want:     false,
		},
		{
			name:     "multiple balanced blocks",
			response: "```\n{}\n```\n\n```\n[1, 2, 3]\n```",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasUnbalancedBraces(tt.response)
			if got != tt.want {
				t.Errorf("hasUnbalancedBraces() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBracesBalanced(t *testing.T) {
	tests := []struct {
		code string
		want bool
	}{
		{"func() {}", true},
		{"{[()]}", true},
		{"{", false},
		{"}", false},
		{"{]", false},
		{"", true},
	}

	for _, tt := range tests {
		got := bracesBalanced(tt.code)
		if got != tt.want {
			t.Errorf("bracesBalanced(%q) = %v, want %v", tt.code, got, tt.want)
		}
	}
}

func TestClampFloat(t *testing.T) {
	tests := []struct {
		v, min, max, want float64
	}{
		{0.5, 0, 1, 0.5},
		{-0.5, 0, 1, 0.0},
		{1.5, 0, 1, 1.0},
		{0.0, 0, 1, 0.0},
		{1.0, 0, 1, 1.0},
	}

	for _, tt := range tests {
		got := clampFloat(tt.v, tt.min, tt.max)
		if got != tt.want {
			t.Errorf("clampFloat(%f, %f, %f) = %f, want %f", tt.v, tt.min, tt.max, got, tt.want)
		}
	}
}

func TestScoreConcurrency(t *testing.T) {
	qs := NewQualityScorer()
	done := make(chan struct{})

	// Run concurrent scoring
	for i := 0; i < 10; i++ {
		go func() {
			ctx := ResponseContext{
				UserPrompt:        "test prompt",
				AssistantResponse: "test response with some content",
				ToolCallCount:     2,
				TokensUsed:        50,
			}
			qs.Score(ctx)
			done <- struct{}{}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if len(qs.History) != 10 {
		t.Errorf("Expected 10 history entries after concurrent scoring, got %d", len(qs.History))
	}
}

func TestAverageScoreConcurrency(t *testing.T) {
	qs := NewQualityScorer()

	// Pre-populate
	qs.mu.Lock()
	for i := 0; i < 10; i++ {
		qs.History = append(qs.History, ScoredResponse{Score: 0.8, Breakdown: map[string]float64{}})
	}
	qs.mu.Unlock()

	done := make(chan struct{})

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			_ = qs.AverageScore(5)
			done <- struct{}{}
		}()
	}

	// Concurrent writes
	for i := 0; i < 5; i++ {
		go func() {
			ctx := ResponseContext{
				UserPrompt:        "test",
				AssistantResponse: "response",
				TokensUsed:        10,
			}
			qs.Score(ctx)
			done <- struct{}{}
		}()
	}

	for i := 0; i < 15; i++ {
		<-done
	}
}

func TestScorePoorResponse(t *testing.T) {
	qs := NewQualityScorer()

	ctx := ResponseContext{
		UserPrompt:        "Please refactor the entire authentication module to use JWT tokens instead of session cookies",
		AssistantResponse: "",
		ToolCallCount:     0,
		ToolErrors:        0,
		FilesModified:     nil,
		TestsPassed:       false,
		LintPassed:        false,
		TokensUsed:        0,
		Duration:          100 * time.Millisecond,
	}

	scored := qs.Score(ctx)
	if scored.Score > 0.5 {
		t.Errorf("Poor response scored too high: %.2f", scored.Score)
	}
}

func TestScoreExcellentResponse(t *testing.T) {
	qs := NewQualityScorer()

	ctx := ResponseContext{
		UserPrompt:        "Fix the null pointer in handler.go line 42",
		AssistantResponse: "I read handler.go, found the nil check missing on line 42, and applied a guard clause. All tests pass and lint is clean.",
		ToolCallCount:     4,
		ToolErrors:        0,
		FilesModified:     []string{"handler.go"},
		TestsPassed:       true,
		LintPassed:        true,
		TokensUsed:        80,
		Duration:          3 * time.Second,
	}

	scored := qs.Score(ctx)
	if scored.Score < 0.75 {
		t.Errorf("Excellent response scored too low: %.2f", scored.Score)
	}
}

func TestTrendAnalysisStable(t *testing.T) {
	qs := NewQualityScorer()

	qs.mu.Lock()
	qs.History = []ScoredResponse{
		{Score: 0.80, Breakdown: map[string]float64{"completeness": 0.8, "correctness": 0.8, "conciseness": 0.8, "tool_usage": 0.8, "safety": 0.8}},
		{Score: 0.81, Breakdown: map[string]float64{"completeness": 0.8, "correctness": 0.8, "conciseness": 0.8, "tool_usage": 0.8, "safety": 0.8}},
		{Score: 0.80, Breakdown: map[string]float64{"completeness": 0.8, "correctness": 0.8, "conciseness": 0.8, "tool_usage": 0.8, "safety": 0.8}},
		{Score: 0.81, Breakdown: map[string]float64{"completeness": 0.8, "correctness": 0.8, "conciseness": 0.8, "tool_usage": 0.8, "safety": 0.8}},
	}
	qs.mu.Unlock()

	trend := qs.TrendAnalysis()
	if !strings.Contains(trend, "stable") {
		t.Errorf("Expected 'stable' trend for constant scores, got: %s", trend)
	}
}

func TestScoredResponseModel(t *testing.T) {
	qs := NewQualityScorer()

	ctx := ResponseContext{
		UserPrompt:        "test",
		AssistantResponse: "test response",
		TokensUsed:        10,
	}

	scored := qs.Score(ctx)
	scored.Model = "claude-sonnet"
	scored.TaskType = "code"

	if scored.Model != "claude-sonnet" {
		t.Errorf("Model = %q, want 'claude-sonnet'", scored.Model)
	}
	if scored.TaskType != "code" {
		t.Errorf("TaskType = %q, want 'code'", scored.TaskType)
	}
}
