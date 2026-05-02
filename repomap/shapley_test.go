package repomap

import (
	"strings"
	"testing"
)

func TestShapleyRanker_ComputeScores(t *testing.T) {
	chunks := []CodeChunk{
		{
			Path:      "handler.go",
			StartLine: 1,
			EndLine:   20,
			Content:   "func HandleRequest(w http.ResponseWriter, r *http.Request) {\n\tquery := r.URL.Query()\n}",
		},
		{
			Path:      "model.go",
			StartLine: 1,
			EndLine:   15,
			Content:   "type User struct {\n\tName string\n\tEmail string\n}",
		},
		{
			Path:      "util.go",
			StartLine: 1,
			EndLine:   10,
			Content:   "func FormatDate(t time.Time) string {\n\treturn t.Format(time.RFC3339)\n}",
		},
	}

	sr := NewShapleyRanker(chunks)
	scores := sr.ComputeScores([]string{"handler.go"}, "HandleRequest query http")
	if len(scores) == 0 {
		t.Fatal("expected scores to be computed")
	}

	// The handler.go chunk should score highest since it matches the query and is relevant.
	if scores[0].Path != "handler.go" {
		t.Errorf("expected handler.go to score highest, got %s", scores[0].Path)
	}
}

func TestShapleyRanker_SelectOptimalContext(t *testing.T) {
	chunks := []CodeChunk{
		{Path: "a.go", StartLine: 1, EndLine: 10, Content: "func Alpha() { return alpha value }"},
		{Path: "b.go", StartLine: 1, EndLine: 10, Content: "func Beta() { return beta value }"},
		{Path: "c.go", StartLine: 1, EndLine: 10, Content: "func Gamma() { unrelated content here }"},
	}

	sr := NewShapleyRanker(chunks)
	selected := sr.SelectOptimalContext("alpha beta", 500)

	if len(selected) == 0 {
		t.Fatal("expected at least one chunk selected")
	}
	if len(selected) > len(chunks) {
		t.Errorf("selected more chunks than available: %d", len(selected))
	}
}

func TestShapleyRanker_RedundancyPenalty(t *testing.T) {
	// Two nearly identical chunks should not both be selected.
	chunks := []CodeChunk{
		{Path: "a.go", StartLine: 1, EndLine: 10, Content: "func Process(data string) error { return nil }"},
		{Path: "a.go", StartLine: 11, EndLine: 20, Content: "func Process(data string) error { return nil }"},
		{Path: "b.go", StartLine: 1, EndLine: 10, Content: "func Validate(input int) bool { return true }"},
	}

	sr := NewShapleyRanker(chunks)
	selected := sr.SelectOptimalContext("Process data", 500)

	// Should not select both identical chunks.
	processCount := 0
	for _, c := range selected {
		if strings.Contains(c.Content, "Process") {
			processCount++
		}
	}
	if processCount > 1 {
		t.Errorf("expected at most 1 Process chunk selected (redundancy filter), got %d", processCount)
	}
}

func TestShapleyRanker_Format(t *testing.T) {
	chunks := []CodeChunk{
		{Path: "main.go", StartLine: 1, EndLine: 5, Content: "package main\nfunc main() {}"},
	}

	sr := NewShapleyRanker(chunks)
	formatted := sr.Format(chunks)

	if !strings.Contains(formatted, "main.go") {
		t.Error("formatted output should contain file path")
	}
	if !strings.Contains(formatted, "lines 1-5") {
		t.Error("formatted output should contain line range")
	}
	if !strings.Contains(formatted, "package main") {
		t.Error("formatted output should contain chunk content")
	}
}
