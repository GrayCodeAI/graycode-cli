package engine

import (
	"strings"
	"testing"
)

func TestNewHallucinationGuard(t *testing.T) {
	hg := NewHallucinationGuard()
	if !hg.Enabled {
		t.Error("expected Enabled to be true by default")
	}
	if hg.Threshold != 0.7 {
		t.Errorf("expected Threshold 0.7, got %f", hg.Threshold)
	}
	if hg.MaxRetries != 2 {
		t.Errorf("expected MaxRetries 2, got %d", hg.MaxRetries)
	}
}

func TestCheck_AllGrounded(t *testing.T) {
	hg := NewHallucinationGuard()
	context := []string{
		"The function ProcessData returns a map[string]int containing word counts.",
		"It is defined in pkg/analysis/processor.go at line 42.",
		"The module was introduced in version 2.3.",
	}
	response := "The function ProcessData returns a map[string]int. It is defined in pkg/analysis/processor.go."

	result := hg.Check(response, context)

	if !result.Grounded {
		t.Errorf("expected grounded, got score %f", result.Score)
	}
	if result.TotalClaims == 0 {
		t.Error("expected claims to be extracted")
	}
	if len(result.UnsupportedClaims) > 0 {
		t.Errorf("expected no unsupported claims, got: %v", result.UnsupportedClaims)
	}
}

func TestCheck_NotGrounded(t *testing.T) {
	hg := NewHallucinationGuard()
	context := []string{
		"The server listens on port 8080.",
		"Configuration is stored in config.yaml.",
	}
	response := "The server runs on port 9090. It stores data in /var/lib/mydb. The binary is compiled with Go 1.21."

	result := hg.Check(response, context)

	if result.Grounded {
		t.Errorf("expected not grounded with fabricated claims, got score %f", result.Score)
	}
	if len(result.UnsupportedClaims) == 0 {
		t.Error("expected unsupported claims to be detected")
	}
}

func TestCheck_EmptyResponse(t *testing.T) {
	hg := NewHallucinationGuard()
	result := hg.Check("", []string{"some context"})

	if !result.Grounded {
		t.Error("empty response should be grounded (no claims)")
	}
	if result.Score != 1.0 {
		t.Errorf("expected score 1.0 for empty response, got %f", result.Score)
	}
}

func TestCheck_NoClaims(t *testing.T) {
	hg := NewHallucinationGuard()
	// Only questions and hedged statements — no factual claims
	response := "What do you think? It might work. Maybe we should try?"
	result := hg.Check(response, []string{"irrelevant context"})

	if !result.Grounded {
		t.Error("response with no factual claims should be grounded")
	}
	if result.TotalClaims != 0 {
		t.Errorf("expected 0 claims, got %d", result.TotalClaims)
	}
}

func TestExtractClaims_FiltersQuestions(t *testing.T) {
	hg := NewHallucinationGuard()
	text := "What is the return type? The function returns int. Should we refactor?"
	claims := hg.ExtractClaims(text)

	for _, c := range claims {
		if strings.HasSuffix(c, "?") {
			t.Errorf("question should be filtered out: %s", c)
		}
	}
}

func TestExtractClaims_FiltersHedged(t *testing.T) {
	hg := NewHallucinationGuard()
	text := "It probably returns an error. The function might crash. The handler accepts 3 arguments."
	claims := hg.ExtractClaims(text)

	for _, c := range claims {
		lower := strings.ToLower(c)
		if strings.Contains(lower, "probably") || strings.Contains(lower, "might") {
			t.Errorf("hedged statement should be filtered out: %s", c)
		}
	}
	// The factual claim about 3 arguments should remain
	found := false
	for _, c := range claims {
		if strings.Contains(c, "3 arguments") {
			found = true
		}
	}
	if !found {
		t.Error("expected factual claim about '3 arguments' to be extracted")
	}
}

func TestExtractClaims_KeepsFactual(t *testing.T) {
	hg := NewHallucinationGuard()
	text := "The file is located at /src/main.go. It implements the Handler interface."
	claims := hg.ExtractClaims(text)

	if len(claims) == 0 {
		t.Error("expected factual claims to be extracted")
	}
}

func TestVerifyClaim_Supported(t *testing.T) {
	hg := NewHallucinationGuard()
	claim := "The function ProcessData is in processor.go"
	context := []string{
		"func ProcessData handles parsing and lives in processor.go",
	}

	if !hg.VerifyClaim(claim, context) {
		t.Error("claim should be verified against context")
	}
}

func TestVerifyClaim_Unsupported(t *testing.T) {
	hg := NewHallucinationGuard()
	claim := "The ConfigManager is defined in settings/manager.go at line 87"
	context := []string{
		"The server uses port 8080 for HTTP traffic.",
		"Logging is configured in log.yaml.",
	}

	if hg.VerifyClaim(claim, context) {
		t.Error("claim should not be verified — key terms not in context")
	}
}

func TestVerifyClaim_EmptyKeyTerms(t *testing.T) {
	hg := NewHallucinationGuard()
	// Very short generic claim with no verifiable terms
	claim := "It is so"
	context := []string{"unrelated context data"}

	// Should return true since no key terms to verify
	if !hg.VerifyClaim(claim, context) {
		t.Error("claim with no key terms should default to supported")
	}
}

func TestExtractKeyTerms(t *testing.T) {
	hg := NewHallucinationGuard()

	tests := []struct {
		input    string
		contains []string
	}{
		{
			input:    "The function ProcessData returns map[string]int",
			contains: []string{"ProcessData"},
		},
		{
			input:    "File is at /src/pkg/handler.go line 42",
			contains: []string{"/src/pkg/handler.go", "42"},
		},
		{
			input:    "Added in version v2.3",
			contains: []string{"v2.3"},
		},
	}

	for _, tt := range tests {
		terms := hg.ExtractKeyTerms(tt.input)
		for _, expected := range tt.contains {
			found := false
			for _, term := range terms {
				if term == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ExtractKeyTerms(%q): expected %q in terms %v", tt.input, expected, terms)
			}
		}
	}
}

func TestExtractKeyTerms_RemovesStopWords(t *testing.T) {
	hg := NewHallucinationGuard()
	terms := hg.ExtractKeyTerms("the function is in a file")

	for _, term := range terms {
		lower := strings.ToLower(term)
		if lower == "the" || lower == "is" || lower == "in" || lower == "a" {
			t.Errorf("stop word %q should not be in key terms", term)
		}
	}
}

func TestBuildRejectionMessage(t *testing.T) {
	result := &GroundingResult{
		Score: 0.33,
		UnsupportedClaims: []string{
			"The function returns a map[string]int",
			"This was added in v2.3",
		},
		SupportedClaims: []string{"The file exists"},
		TotalClaims:     3,
		Grounded:        false,
	}

	msg := BuildRejectionMessage(result)

	if !strings.Contains(msg, "unsupported claims") {
		t.Error("message should mention unsupported claims")
	}
	if !strings.Contains(msg, "map[string]int") {
		t.Error("message should include the unsupported claim text")
	}
	if !strings.Contains(msg, "v2.3") {
		t.Error("message should include all unsupported claims")
	}
	if !strings.Contains(msg, "verify") {
		t.Error("message should ask to verify or rephrase")
	}
}

func TestFormatGroundingResult_Grounded(t *testing.T) {
	result := &GroundingResult{
		Score:           1.0,
		SupportedClaims: []string{"The server runs on port 8080"},
		TotalClaims:     1,
		Grounded:        true,
	}

	formatted := FormatGroundingResult(result)
	if !strings.Contains(formatted, "GROUNDED") {
		t.Error("formatted result should say GROUNDED")
	}
	if strings.Contains(formatted, "NOT GROUNDED") {
		t.Error("formatted result should not say NOT GROUNDED")
	}
	if !strings.Contains(formatted, "1.00") {
		t.Error("formatted result should show score")
	}
}

func TestFormatGroundingResult_NotGrounded(t *testing.T) {
	result := &GroundingResult{
		Score:             0.25,
		SupportedClaims:   []string{"One correct thing"},
		UnsupportedClaims: []string{"False claim 1", "False claim 2", "False claim 3"},
		TotalClaims:       4,
		Grounded:          false,
	}

	formatted := FormatGroundingResult(result)
	if !strings.Contains(formatted, "NOT GROUNDED") {
		t.Error("formatted result should say NOT GROUNDED")
	}
	if !strings.Contains(formatted, "1/4") {
		t.Error("formatted result should show claim ratio")
	}
}

func TestCheck_ThresholdBoundary(t *testing.T) {
	hg := NewHallucinationGuard()
	hg.Threshold = 0.5

	// Provide context that supports some claims but not others
	context := []string{
		"The UserService struct handles authentication and is in auth/service.go.",
	}
	// Two claims: one supported, one not
	response := "The UserService handles authentication. The PaymentGateway processes refunds in billing/gateway.go."

	result := hg.Check(response, context)

	// At least we can verify the threshold logic works
	if result.TotalClaims == 0 {
		t.Fatal("expected claims to be extracted")
	}
	// With 0.5 threshold, having roughly half supported should be on the boundary
	t.Logf("Score: %f, Supported: %d, Unsupported: %d",
		result.Score, len(result.SupportedClaims), len(result.UnsupportedClaims))
}

func TestCheck_ConcurrentSafe(t *testing.T) {
	hg := NewHallucinationGuard()
	context := []string{"The server uses port 8080 and runs HTTP."}
	response := "The server listens on port 8080."

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			result := hg.Check(response, context)
			if result.TotalClaims < 0 {
				t.Error("impossible negative claims")
			}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestSplitSentences(t *testing.T) {
	text := "First sentence. Second sentence! Third one?"
	sentences := splitSentences(text)

	if len(sentences) != 3 {
		t.Errorf("expected 3 sentences, got %d: %v", len(sentences), sentences)
	}
}

func TestIsHedged(t *testing.T) {
	hedged := []string{
		"It might work for this case",
		"This is probably the right approach",
		"Perhaps we should try another way",
		"I think this is correct",
	}
	for _, s := range hedged {
		if !isHedged(s) {
			t.Errorf("expected hedged: %q", s)
		}
	}

	notHedged := []string{
		"The function returns an error",
		"This is defined in main.go",
		"The server runs on port 8080",
	}
	for _, s := range notHedged {
		if isHedged(s) {
			t.Errorf("expected not hedged: %q", s)
		}
	}
}

func TestIsFactualClaim(t *testing.T) {
	factual := []string{
		"The function is at line 42",
		"It returns map[string]int",
		"Located in /src/handler.go",
		"The config_file stores settings",
		"This uses `json.Marshal` internally",
	}
	for _, s := range factual {
		if !isFactualClaim(s) {
			t.Errorf("expected factual: %q", s)
		}
	}

	notFactual := []string{
		"This is a good approach",
		"We can do it",
		"Here is the answer",
	}
	for _, s := range notFactual {
		if isFactualClaim(s) {
			t.Errorf("expected not factual: %q", s)
		}
	}
}

func TestIsStopWord(t *testing.T) {
	stops := []string{"the", "a", "is", "in", "of", "The", "IS"}
	for _, w := range stops {
		if !isStopWord(w) {
			t.Errorf("expected stop word: %q", w)
		}
	}

	notStops := []string{"function", "server", "handler", "ProcessData"}
	for _, w := range notStops {
		if isStopWord(w) {
			t.Errorf("expected not stop word: %q", w)
		}
	}
}
