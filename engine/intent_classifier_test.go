package engine

import (
	"strings"
	"sync"
	"testing"
)

func TestNewIntentClassifier(t *testing.T) {
	ic := NewIntentClassifier()
	if ic == nil {
		t.Fatal("NewIntentClassifier returned nil")
	}
	if len(ic.Rules) == 0 {
		t.Fatal("expected rules to be populated")
	}
	if ic.History == nil {
		t.Fatal("expected history to be initialized")
	}
}

func TestClassify_CodeWrite(t *testing.T) {
	ic := NewIntentClassifier()
	tests := []struct {
		input   string
		wantCat string
	}{
		{"implement a new auth module", IntentCodeWrite},
		{"create a function to parse JSON", IntentCodeWrite},
		{"add feature for user notifications", IntentCodeWrite},
		{"write a handler for POST requests", IntentCodeWrite},
	}
	for _, tt := range tests {
		intent := ic.Classify(tt.input)
		if intent.Category != tt.wantCat {
			t.Errorf("Classify(%q) = %s, want %s", tt.input, intent.Category, tt.wantCat)
		}
		if intent.Confidence <= 0 {
			t.Errorf("Classify(%q) confidence should be > 0", tt.input)
		}
	}
}

func TestClassify_CodeFix(t *testing.T) {
	ic := NewIntentClassifier()
	tests := []struct {
		input   string
		wantCat string
	}{
		{"fix the null pointer error in main.go", IntentCodeFix},
		{"debug the failing test", IntentCodeFix},
		{"resolve the race condition", IntentCodeFix},
		{"repair broken authentication", IntentCodeFix},
	}
	for _, tt := range tests {
		intent := ic.Classify(tt.input)
		if intent.Category != tt.wantCat {
			t.Errorf("Classify(%q) = %s, want %s", tt.input, intent.Category, tt.wantCat)
		}
	}
}

func TestClassify_Explain(t *testing.T) {
	ic := NewIntentClassifier()
	tests := []struct {
		input   string
		wantCat string
	}{
		{"explain how the auth middleware works", IntentExplain},
		{"what does this function do", IntentExplain},
		{"how does the caching layer work", IntentExplain},
		{"why is this variable global", IntentExplain},
	}
	for _, tt := range tests {
		intent := ic.Classify(tt.input)
		if intent.Category != tt.wantCat {
			t.Errorf("Classify(%q) = %s, want %s", tt.input, intent.Category, tt.wantCat)
		}
	}
}

func TestClassify_Search(t *testing.T) {
	ic := NewIntentClassifier()
	tests := []struct {
		input   string
		wantCat string
	}{
		{"find all usages of the Config struct", IntentSearch},
		{"where is the database connection defined", IntentSearch},
		{"locate the error handler", IntentSearch},
		{"grep for TODO comments", IntentSearch},
	}
	for _, tt := range tests {
		intent := ic.Classify(tt.input)
		if intent.Category != tt.wantCat {
			t.Errorf("Classify(%q) = %s, want %s", tt.input, intent.Category, tt.wantCat)
		}
	}
}

func TestClassify_Git(t *testing.T) {
	ic := NewIntentClassifier()
	tests := []struct {
		input   string
		wantCat string
	}{
		{"commit these changes", IntentGit},
		{"push to remote", IntentGit},
		{"create a new branch for the feature", IntentGit},
		{"merge the dev branch", IntentGit},
	}
	for _, tt := range tests {
		intent := ic.Classify(tt.input)
		if intent.Category != tt.wantCat {
			t.Errorf("Classify(%q) = %s, want %s", tt.input, intent.Category, tt.wantCat)
		}
	}
}

func TestClassify_Refactor(t *testing.T) {
	ic := NewIntentClassifier()
	tests := []struct {
		input   string
		wantCat string
	}{
		{"refactor the user service", IntentRefactor},
		{"restructure the project layout", IntentRefactor},
		{"optimize the database queries", IntentRefactor},
		{"clean up the handler code", IntentRefactor},
	}
	for _, tt := range tests {
		intent := ic.Classify(tt.input)
		if intent.Category != tt.wantCat {
			t.Errorf("Classify(%q) = %s, want %s", tt.input, intent.Category, tt.wantCat)
		}
	}
}

func TestClassify_Test(t *testing.T) {
	ic := NewIntentClassifier()
	tests := []struct {
		input   string
		wantCat string
	}{
		{"add tests for the parser", IntentTest},
		{"check test coverage", IntentTest},
		{"write unit tests for the validator", IntentTest},
	}
	for _, tt := range tests {
		intent := ic.Classify(tt.input)
		if intent.Category != tt.wantCat {
			t.Errorf("Classify(%q) = %s, want %s", tt.input, intent.Category, tt.wantCat)
		}
	}
}

func TestClassify_Config(t *testing.T) {
	ic := NewIntentClassifier()
	tests := []struct {
		input   string
		wantCat string
	}{
		{"configure the database connection", IntentConfig},
		{"setup the development environment", IntentConfig},
		{"install the required dependencies", IntentConfig},
	}
	for _, tt := range tests {
		intent := ic.Classify(tt.input)
		if intent.Category != tt.wantCat {
			t.Errorf("Classify(%q) = %s, want %s", tt.input, intent.Category, tt.wantCat)
		}
	}
}

func TestClassify_Unknown(t *testing.T) {
	ic := NewIntentClassifier()
	intent := ic.Classify("xyzzy foobarbaz")
	if intent.Category != IntentQuestion {
		t.Errorf("expected question for unknown input, got %s", intent.Category)
	}
	if intent.Confidence > 0.5 {
		t.Errorf("expected low confidence for unknown, got %.2f", intent.Confidence)
	}
}

func TestClassify_MultipleKeywords_HigherConfidence(t *testing.T) {
	ic := NewIntentClassifier()
	single := ic.Classify("fix this")
	multi := ic.Classify("fix the error that causes a crash and debug the null pointer")
	if multi.Confidence <= single.Confidence {
		t.Errorf("multiple keywords should yield higher confidence: single=%.2f multi=%.2f",
			single.Confidence, multi.Confidence)
	}
}

func TestClassifyForRouting(t *testing.T) {
	ic := NewIntentClassifier()

	model, tools := ic.ClassifyForRouting("fix the bug")
	if model == "" {
		t.Error("expected non-empty model")
	}
	if tools == "" {
		t.Error("expected non-empty tools")
	}
	if !strings.Contains(tools, "Read") {
		t.Errorf("expected tools to contain Read for code_fix, got %s", tools)
	}

	// Complex input should get advanced model
	model, _ = ic.ClassifyForRouting("implement a full authentication system with OAuth2 integration and also add rate limiting and then write tests for all of it plus configure the CI pipeline")
	if model != "advanced" {
		t.Errorf("expected advanced model for complex input, got %s", model)
	}
}

func TestSuggestTools(t *testing.T) {
	ic := NewIntentClassifier()
	tests := []struct {
		category string
		want     []string
	}{
		{IntentCodeWrite, []string{"Read", "Edit", "Write", "Bash"}},
		{IntentCodeFix, []string{"Read", "Grep", "Edit", "Bash"}},
		{IntentExplain, []string{"Read", "Glob"}},
		{IntentSearch, []string{"Grep", "Glob", "LS"}},
		{IntentTest, []string{"Bash", "Read", "Write"}},
		{IntentGit, []string{"Bash"}},
		{IntentQuestion, nil},
	}
	for _, tt := range tests {
		tools := ic.SuggestTools(&Intent{Category: tt.category})
		if tt.want == nil {
			if tools != nil {
				t.Errorf("SuggestTools(%s) = %v, want nil", tt.category, tools)
			}
			continue
		}
		if len(tools) != len(tt.want) {
			t.Errorf("SuggestTools(%s) = %v, want %v", tt.category, tools, tt.want)
			continue
		}
		for i, tool := range tools {
			if tool != tt.want[i] {
				t.Errorf("SuggestTools(%s)[%d] = %s, want %s", tt.category, i, tool, tt.want[i])
			}
		}
	}
}

func TestEstimateComplexity(t *testing.T) {
	ic := NewIntentClassifier()
	tests := []struct {
		input string
		want  string
	}{
		{"fix bug", "trivial"},
		{"fix the login error in auth.go", "simple"},
		{"refactor the authentication module to use JWT tokens and update the tests", "moderate"},
		{"implement a full OAuth2 authentication system with support for multiple providers and also add rate limiting to all endpoints and then write comprehensive integration tests for each provider and additionally configure the CI/CD pipeline to run these tests on every pull request across multiple environments", "complex"},
	}
	for _, tt := range tests {
		got := ic.EstimateComplexity(tt.input)
		if got != tt.want {
			t.Errorf("EstimateComplexity(%q) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestFormatIntent(t *testing.T) {
	intent := &Intent{
		Category:            IntentCodeFix,
		Confidence:          0.92,
		SubCategory:         "debug",
		Keywords:            []string{"fix", "error", "null pointer"},
		SuggestedTools:      []string{"Read", "Grep", "Edit", "Bash"},
		EstimatedComplexity: "moderate",
	}

	output := FormatIntent(intent)
	if !strings.Contains(output, "code_fix") {
		t.Errorf("FormatIntent missing category")
	}
	if !strings.Contains(output, "0.92") {
		t.Errorf("FormatIntent missing confidence")
	}
	if !strings.Contains(output, "debug") {
		t.Errorf("FormatIntent missing subcategory")
	}
	if !strings.Contains(output, `"fix"`) {
		t.Errorf("FormatIntent missing keyword")
	}
	if !strings.Contains(output, "Read") {
		t.Errorf("FormatIntent missing tool")
	}
	if !strings.Contains(output, "moderate") {
		t.Errorf("FormatIntent missing complexity")
	}
}

func TestRecordClassification(t *testing.T) {
	ic := NewIntentClassifier()
	intent := &Intent{Category: IntentCodeWrite, Confidence: 0.8}
	ic.RecordClassification("create a new module", intent)
	ic.RecordClassification("implement auth", intent)

	if len(ic.History) != 2 {
		t.Errorf("expected 2 history entries, got %d", len(ic.History))
	}
	if ic.History[0].Input != "create a new module" {
		t.Errorf("unexpected history input: %s", ic.History[0].Input)
	}
}

func TestGetPatterns(t *testing.T) {
	ic := NewIntentClassifier()
	ic.RecordClassification("fix bug", &Intent{Category: IntentCodeFix})
	ic.RecordClassification("fix error", &Intent{Category: IntentCodeFix})
	ic.RecordClassification("fix crash", &Intent{Category: IntentCodeFix})
	ic.RecordClassification("create module", &Intent{Category: IntentCodeWrite})

	patterns := ic.GetPatterns()
	if patterns[IntentCodeFix] != 3 {
		t.Errorf("expected 3 code_fix, got %d", patterns[IntentCodeFix])
	}
	if patterns[IntentCodeWrite] != 1 {
		t.Errorf("expected 1 code_write, got %d", patterns[IntentCodeWrite])
	}
}

func TestIntentClassifierConcurrentAccess(t *testing.T) {
	ic := NewIntentClassifier()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			intent := ic.Classify("fix the bug")
			ic.RecordClassification("fix the bug", intent)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = ic.GetPatterns()
		}()
	}

	wg.Wait()

	patterns := ic.GetPatterns()
	if patterns[IntentCodeFix] != 50 {
		t.Errorf("expected 50 recordings, got %d", patterns[IntentCodeFix])
	}
}

func TestClassify_KeywordsPopulated(t *testing.T) {
	ic := NewIntentClassifier()
	intent := ic.Classify("fix the error and debug the crash")
	if len(intent.Keywords) == 0 {
		t.Error("expected keywords to be populated")
	}
	// Should have matched multiple patterns
	found := false
	for _, kw := range intent.Keywords {
		if kw == "fix" || kw == "error" || kw == "debug" || kw == "crash" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected relevant keywords, got %v", intent.Keywords)
	}
}

func TestClassify_SuggestedToolsPopulated(t *testing.T) {
	ic := NewIntentClassifier()
	intent := ic.Classify("find the config file")
	if len(intent.SuggestedTools) == 0 {
		t.Error("expected suggested tools to be populated for search intent")
	}
}
