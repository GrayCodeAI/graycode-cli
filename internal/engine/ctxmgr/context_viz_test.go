package ctxmgr

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
)

func TestNewContextVisualizer(t *testing.T) {
	cv := NewContextVisualizer(200_000)
	if cv.MaxTokens != 200_000 {
		t.Errorf("expected MaxTokens=200000, got %d", cv.MaxTokens)
	}
	if cv.Sections != nil {
		t.Errorf("expected nil sections initially, got %v", cv.Sections)
	}
}

func TestUpdate_RecalculatesPercentages(t *testing.T) {
	cv := NewContextVisualizer(100_000)

	sections := []ContextSection{
		{Name: "system_prompt", Tokens: 5000},
		{Name: "memory", Tokens: 2000},
		{Name: "conversation", Tokens: 30000},
		{Name: "tool_results", Tokens: 8000},
		{Name: "reserved", Tokens: 5000},
	}

	cv.Update(sections)

	if len(cv.Sections) != 5 {
		t.Fatalf("expected 5 sections, got %d", len(cv.Sections))
	}

	// system_prompt: 5000/100000 = 5.0%
	if cv.Sections[0].Percentage != 5.0 {
		t.Errorf("system_prompt percentage: expected 5.0, got %.1f", cv.Sections[0].Percentage)
	}
	// memory: 2000/100000 = 2.0%
	if cv.Sections[1].Percentage != 2.0 {
		t.Errorf("memory percentage: expected 2.0, got %.1f", cv.Sections[1].Percentage)
	}
	// conversation: 30000/100000 = 30.0%
	if cv.Sections[2].Percentage != 30.0 {
		t.Errorf("conversation percentage: expected 30.0, got %.1f", cv.Sections[2].Percentage)
	}
}

func TestUpdate_ZeroMaxTokens(t *testing.T) {
	cv := NewContextVisualizer(0)
	sections := []ContextSection{
		{Name: "conversation", Tokens: 5000},
	}
	cv.Update(sections)

	if cv.Sections[0].Percentage != 0 {
		t.Errorf("expected 0%% with zero MaxTokens, got %.1f%%", cv.Sections[0].Percentage)
	}
}

func TestRenderBar_Basic(t *testing.T) {
	cv := NewContextVisualizer(200_000)
	cv.Update([]ContextSection{
		{Name: "system_prompt", Tokens: 4200},
		{Name: "memory", Tokens: 2800},
		{Name: "conversation", Tokens: 52000},
		{Name: "tool_results", Tokens: 8000},
		{Name: "reserved", Tokens: 5000},
	})

	result := cv.RenderBar(20)

	if !strings.Contains(result, "Context:") {
		t.Error("bar should contain 'Context:' label")
	}
	if !strings.Contains(result, "used") {
		t.Error("bar should contain 'used' label")
	}
	if !strings.Contains(result, "200,000") {
		t.Errorf("bar should show max tokens formatted, got:\n%s", result)
	}
	// Should have two lines
	lines := strings.Split(result, "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d:\n%s", len(lines), result)
	}
}

func TestRenderBar_SmallWidth(t *testing.T) {
	cv := NewContextVisualizer(100_000)
	cv.Update([]ContextSection{
		{Name: "conversation", Tokens: 50000},
	})

	// width < 10 should be forced to 10
	result := cv.RenderBar(3)
	if !strings.Contains(result, "Context:") {
		t.Error("small width should still produce valid output")
	}
}

func TestRenderDetailed(t *testing.T) {
	cv := NewContextVisualizer(200_000)
	cv.Update([]ContextSection{
		{Name: "system_prompt", Tokens: 4200},
		{Name: "memory", Tokens: 2800},
		{Name: "conversation", Tokens: 52000, Items: []VizContextItem{
			{Label: "User msgs", Tokens: 18000, Role: "user"},
			{Label: "Asst msgs", Tokens: 28000, Role: "assistant"},
			{Label: "Tool results", Tokens: 6000, Role: "tool"},
		}},
		{Name: "repo_map", Tokens: 8000},
		{Name: "readonly_context", Tokens: 12000},
		{Name: "reserved", Tokens: 5000},
	})

	result := cv.RenderDetailed()

	// Check header
	if !strings.Contains(result, "Context Window Usage") {
		t.Error("detailed view should have header")
	}
	if !strings.Contains(result, "200,000") {
		t.Errorf("should show max tokens, got:\n%s", result)
	}

	// Check section names appear
	if !strings.Contains(result, "System Prompt") {
		t.Error("should contain 'System Prompt'")
	}
	if !strings.Contains(result, "Memory (harrier)") {
		t.Error("should contain 'Memory (harrier)'")
	}
	if !strings.Contains(result, "Conversation") {
		t.Error("should contain 'Conversation'")
	}
	if !strings.Contains(result, "Repo Map") {
		t.Error("should contain 'Repo Map'")
	}
	if !strings.Contains(result, "Read-Only Context") {
		t.Error("should contain 'Read-Only Context'")
	}

	// Check sub-items
	if !strings.Contains(result, "User msgs") {
		t.Error("should show conversation sub-items")
	}
	if !strings.Contains(result, "├─") {
		t.Error("should have tree-style sub-item markers")
	}
	if !strings.Contains(result, "└─") {
		t.Error("should have final tree-style marker")
	}

	// Check available
	if !strings.Contains(result, "Available") {
		t.Error("should show available tokens")
	}
}

func TestRenderDetailed_TruncatedItems(t *testing.T) {
	cv := NewContextVisualizer(100_000)
	cv.Update([]ContextSection{
		{Name: "conversation", Tokens: 20000, Items: []VizContextItem{
			{Label: "Big file", Tokens: 15000, Role: "tool", Truncated: true},
			{Label: "Normal msg", Tokens: 5000, Role: "user", Truncated: false},
		}},
	})

	result := cv.RenderDetailed()
	if !strings.Contains(result, "[truncated]") {
		t.Error("should indicate truncated items")
	}
}

func TestRenderCompact(t *testing.T) {
	cv := NewContextVisualizer(200_000)
	cv.Update([]ContextSection{
		{Name: "system_prompt", Tokens: 4000},
		{Name: "memory", Tokens: 2000},
		{Name: "conversation", Tokens: 52000},
		{Name: "repo_map", Tokens: 8000},
		{Name: "readonly_context", Tokens: 12000},
	})

	result := cv.RenderCompact()

	// Should be bracketed
	if !strings.HasPrefix(result, "[") || !strings.HasSuffix(result, "]") {
		t.Errorf("compact format should be bracketed, got: %s", result)
	}

	// Should contain percentage
	if !strings.Contains(result, "39%") {
		t.Errorf("should show total used percentage, got: %s", result)
	}

	// Should contain section short names
	if !strings.Contains(result, "sys:") {
		t.Error("should contain sys: section")
	}
	if !strings.Contains(result, "mem:") {
		t.Error("should contain mem: section")
	}
	if !strings.Contains(result, "conv:") {
		t.Error("should contain conv: section")
	}

	// Should show free space
	if !strings.Contains(result, "free") {
		t.Error("should contain 'free' indicator")
	}
}

func TestRecommend_ConversationHigh(t *testing.T) {
	cv := NewContextVisualizer(100_000)
	cv.Update([]ContextSection{
		{Name: "conversation", Tokens: 30000},
	})

	recs := cv.Recommend()
	if len(recs) == 0 {
		t.Fatal("expected recommendation when conversation > 20%")
	}
	found := false
	for _, r := range recs {
		if strings.Contains(r, "Conversation") && strings.Contains(r, "compacting") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected conversation compaction recommendation, got: %v", recs)
	}
}

func TestRecommend_ToolResultsLarge(t *testing.T) {
	cv := NewContextVisualizer(200_000)
	cv.Update([]ContextSection{
		{Name: "tool_results", Tokens: 8000},
	})

	recs := cv.Recommend()
	found := false
	for _, r := range recs {
		if strings.Contains(r, "Tool results") && strings.Contains(r, "compression") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected tool compression recommendation, got: %v", recs)
	}
}

func TestRecommend_ReadOnlyLarge(t *testing.T) {
	cv := NewContextVisualizer(200_000)
	cv.Update([]ContextSection{
		{Name: "readonly_context", Tokens: 12000},
	})

	recs := cv.Recommend()
	found := false
	for _, r := range recs {
		if strings.Contains(r, "read-only context") && strings.Contains(r, "review") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected read-only context recommendation, got: %v", recs)
	}
}

func TestRecommend_NoRecommendations(t *testing.T) {
	cv := NewContextVisualizer(200_000)
	cv.Update([]ContextSection{
		{Name: "system_prompt", Tokens: 3000},
		{Name: "memory", Tokens: 1500},
		{Name: "conversation", Tokens: 10000},
	})

	recs := cv.Recommend()
	if len(recs) != 0 {
		t.Errorf("expected no recommendations for small usage, got: %v", recs)
	}
}

func TestRecommend_Compressible(t *testing.T) {
	cv := NewContextVisualizer(100_000)
	cv.Update([]ContextSection{
		{Name: "repo_map", Tokens: 20000, Compressible: true},
	})

	recs := cv.Recommend()
	found := false
	for _, r := range recs {
		if strings.Contains(r, "compressible") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected compressible recommendation, got: %v", recs)
	}
}

func TestHistoryChart(t *testing.T) {
	cv := NewContextVisualizer(200_000)
	snapshots := []ContextSnapshot{
		{Turn: 1, TotalTokens: 16000, Percentage: 8},
		{Turn: 5, TotalTokens: 70000, Percentage: 35},
		{Turn: 10, TotalTokens: 156000, Percentage: 78, Compacted: true},
		{Turn: 11, TotalTokens: 56000, Percentage: 28},
	}

	result := cv.HistoryChart(snapshots, 20)

	lines := strings.Split(result, "\n")
	if len(lines) != 4 {
		t.Errorf("expected 4 lines, got %d:\n%s", len(lines), result)
	}

	// Check turn numbers
	if !strings.Contains(lines[0], "Turn  1:") {
		t.Errorf("first line should show Turn  1, got: %s", lines[0])
	}
	if !strings.Contains(lines[2], "Turn 10:") {
		t.Errorf("third line should show Turn 10, got: %s", lines[2])
	}

	// Check compaction marker
	if !strings.Contains(lines[2], "← compacted") {
		t.Errorf("compacted turn should be marked, got: %s", lines[2])
	}
	if strings.Contains(lines[3], "← compacted") {
		t.Errorf("non-compacted turn should not be marked, got: %s", lines[3])
	}

	// Check percentages
	if !strings.Contains(lines[0], "8%") {
		t.Errorf("first line should show 8%%, got: %s", lines[0])
	}
	if !strings.Contains(lines[1], "35%") {
		t.Errorf("second line should show 35%%, got: %s", lines[1])
	}
}

func TestHistoryChart_EmptySnapshots(t *testing.T) {
	cv := NewContextVisualizer(200_000)
	result := cv.HistoryChart(nil, 20)
	if result != "" {
		t.Errorf("expected empty string for nil snapshots, got: %q", result)
	}
}

func TestTakeSnapshot(t *testing.T) {
	cv := NewContextVisualizer(100_000)
	cv.Update([]ContextSection{
		{Name: "system_prompt", Tokens: 5000},
		{Name: "conversation", Tokens: 35000},
	})

	snap := cv.TakeSnapshot(7)

	if snap.Turn != 7 {
		t.Errorf("expected Turn=7, got %d", snap.Turn)
	}
	if snap.TotalTokens != 40000 {
		t.Errorf("expected TotalTokens=40000, got %d", snap.TotalTokens)
	}
	if snap.Percentage != 40.0 {
		t.Errorf("expected Percentage=40.0, got %.1f", snap.Percentage)
	}
	if snap.Compacted {
		t.Error("snapshot should not be marked as compacted by default")
	}
}

func TestWarnIfCritical_Safe(t *testing.T) {
	cv := NewContextVisualizer(100_000)
	cv.Update([]ContextSection{
		{Name: "conversation", Tokens: 50000},
	})

	warn := cv.WarnIfCritical()
	if warn != "" {
		t.Errorf("50%% usage should not warn, got: %s", warn)
	}
}

func TestWarnIfCritical_Warning(t *testing.T) {
	cv := NewContextVisualizer(100_000)
	cv.Update([]ContextSection{
		{Name: "conversation", Tokens: 87000},
	})

	warn := cv.WarnIfCritical()
	if warn == "" {
		t.Fatal("87%% usage should produce warning")
	}
	if !strings.Contains(warn, icons.Alert()) {
		t.Errorf("warning level should have alert glyph, got: %s", warn)
	}
	if !strings.Contains(warn, "auto-compact") {
		t.Errorf("warning should mention auto-compact, got: %s", warn)
	}
}

func TestWarnIfCritical_Critical(t *testing.T) {
	cv := NewContextVisualizer(100_000)
	cv.Update([]ContextSection{
		{Name: "conversation", Tokens: 96000},
	})

	warn := cv.WarnIfCritical()
	if warn == "" {
		t.Fatal("96%% usage should produce critical warning")
	}
	if !strings.Contains(warn, "compacting now") {
		t.Errorf("critical should say 'compacting now', got: %s", warn)
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{42, "42"},
		{999, "999"},
		{1000, "1,000"},
		{12345, "12,345"},
		{100000, "100,000"},
		{1000000, "1,000,000"},
		{200000, "200,000"},
	}

	for _, tt := range tests {
		got := formatTokens(tt.input)
		if got != tt.expected {
			t.Errorf("formatTokens(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestFormatTokensShort(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{500, "500"},
		{1000, "1K"},
		{116000, "116K"},
		{1500000, "1.5M"},
	}

	for _, tt := range tests {
		got := formatTokensShort(tt.input)
		if got != tt.expected {
			t.Errorf("formatTokensShort(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestSectionShortName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"system_prompt", "sys"},
		{"memory", "mem"},
		{"conversation", "conv"},
		{"tool_results", "tool"},
		{"reserved", "rsv"},
		{"repo_map", "repo"},
		{"readonly_context", "ctx"},
		{"unknown_thing", "unkn"},
		{"hi", "hi"},
	}

	for _, tt := range tests {
		got := sectionShortName(tt.input)
		if got != tt.expected {
			t.Errorf("sectionShortName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestSectionDisplayName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"system_prompt", "System Prompt"},
		{"memory", "Memory (harrier)"},
		{"conversation", "Conversation"},
		{"tool_results", "Tool Results"},
		{"reserved", "Reserved (output)"},
		{"repo_map", "Repo Map"},
		{"readonly_context", "Read-Only Context"},
		{"custom", "custom"},
	}

	for _, tt := range tests {
		got := sectionDisplayName(tt.input)
		if got != tt.expected {
			t.Errorf("sectionDisplayName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestContextViz_ConcurrentAccess(t *testing.T) {
	cv := NewContextVisualizer(200_000)

	done := make(chan struct{})

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			cv.Update([]ContextSection{
				{Name: "conversation", Tokens: i * 1000},
			})
		}
		close(done)
	}()

	// Reader goroutines
	for i := 0; i < 5; i++ {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					cv.RenderBar(20)
					cv.RenderDetailed()
					cv.RenderCompact()
					cv.WarnIfCritical()
					cv.TakeSnapshot(1)
				}
			}
		}()
	}

	<-done
}

func TestFitLabel(t *testing.T) {
	tests := []struct {
		label    string
		width    int
		expected int // expected rune length of result
	}{
		{"sys", 5, 5},
		{"conv", 4, 4},
		{"x", 0, 0},
		{"toolong", 3, 3},
	}

	for _, tt := range tests {
		got := fitLabel(tt.label, tt.width)
		runeLen := len([]rune(got))
		if runeLen != tt.expected {
			t.Errorf("fitLabel(%q, %d) rune length = %d, want %d (got %q)",
				tt.label, tt.width, runeLen, tt.expected, got)
		}
	}
}
