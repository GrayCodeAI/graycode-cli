package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractTicketRef_BranchName(t *testing.T) {
	tc := NewTicketCompliance()

	tests := []struct {
		name     string
		branch   string
		prDesc   string
		expected []string
	}{
		{
			name:     "JIRA-style from branch",
			branch:   "feature/HAWK-123-add-auth",
			prDesc:   "",
			expected: []string{"HAWK-123"},
		},
		{
			name:     "branch with multiple segments",
			branch:   "bugfix/PROJ-456-fix-login",
			prDesc:   "",
			expected: []string{"PROJ-456"},
		},
		{
			name:     "branch without ticket ref",
			branch:   "feature/add-new-widget",
			prDesc:   "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := tc.ExtractTicketRef(tt.branch, tt.prDesc)
			if len(refs) != len(tt.expected) {
				t.Fatalf("expected %d refs, got %d: %v", len(tt.expected), len(refs), refs)
			}
			for i, ref := range refs {
				if ref != tt.expected[i] {
					t.Errorf("ref[%d]: expected %q, got %q", i, tt.expected[i], ref)
				}
			}
		})
	}
}

func TestExtractTicketRef_PRDescription(t *testing.T) {
	tc := NewTicketCompliance()

	tests := []struct {
		name     string
		prDesc   string
		contains []string
	}{
		{
			name:     "fixes hash reference",
			prDesc:   "This PR fixes #42 by adding validation",
			contains: []string{"#42"},
		},
		{
			name:     "closes hash reference",
			prDesc:   "Closes #101",
			contains: []string{"#101"},
		},
		{
			name:     "resolves JIRA reference",
			prDesc:   "Resolves HAWK-99",
			contains: []string{"HAWK-99"},
		},
		{
			name:     "multiple references",
			prDesc:   "Fixes #42 and also resolves HAWK-99\nRelated to PROJ-200",
			contains: []string{"#42", "HAWK-99", "PROJ-200"},
		},
		{
			name:     "no references",
			prDesc:   "Just a small update to the README",
			contains: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := tc.ExtractTicketRef("", tt.prDesc)
			for _, expected := range tt.contains {
				found := false
				for _, ref := range refs {
					if ref == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected refs to contain %q, got %v", expected, refs)
				}
			}
		})
	}
}

func TestExtractTicketRef_Combined(t *testing.T) {
	tc := NewTicketCompliance()

	refs := tc.ExtractTicketRef("feature/HAWK-123-jwt", "Fixes #42\nResolves HAWK-123")
	// HAWK-123 should only appear once.
	count := 0
	for _, r := range refs {
		if r == "HAWK-123" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected HAWK-123 to appear once, appeared %d times in %v", count, refs)
	}
	// #42 should be present.
	found := false
	for _, r := range refs {
		if r == "#42" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected #42 in refs, got %v", refs)
	}
}

func TestParseTicket_BasicContent(t *testing.T) {
	tc := NewTicketCompliance()

	content := `# Add JWT authentication

Implement JWT-based authentication for the API.
Users should be able to log in and receive a token.

## Acceptance Criteria

- [ ] Token validation endpoint
- [ ] Middleware for protected routes
- [ ] Unit tests for token parsing
- [ ] Documentation for API changes
`

	ticket := tc.ParseTicket(content)

	if ticket.Title != "Add JWT authentication" {
		t.Errorf("expected title 'Add JWT authentication', got %q", ticket.Title)
	}

	expectedCriteria := []string{
		"Token validation endpoint",
		"Middleware for protected routes",
		"Unit tests for token parsing",
		"Documentation for API changes",
	}

	if len(ticket.AcceptanceCriteria) != len(expectedCriteria) {
		t.Fatalf("expected %d criteria, got %d: %v", len(expectedCriteria), len(ticket.AcceptanceCriteria), ticket.AcceptanceCriteria)
	}

	for i, expected := range expectedCriteria {
		if ticket.AcceptanceCriteria[i] != expected {
			t.Errorf("criterion[%d]: expected %q, got %q", i, expected, ticket.AcceptanceCriteria[i])
		}
	}
}

func TestParseTicket_NumberedList(t *testing.T) {
	tc := NewTicketCompliance()

	content := `Add rate limiting

We need to add rate limiting to the API.

Acceptance Criteria:
1. Implement token bucket algorithm
2. Add rate limit headers to responses
3. Return 429 status when exceeded
`

	ticket := tc.ParseTicket(content)

	if ticket.Title != "Add rate limiting" {
		t.Errorf("expected title 'Add rate limiting', got %q", ticket.Title)
	}

	if len(ticket.AcceptanceCriteria) != 3 {
		t.Fatalf("expected 3 criteria, got %d: %v", len(ticket.AcceptanceCriteria), ticket.AcceptanceCriteria)
	}

	if ticket.AcceptanceCriteria[0] != "Implement token bucket algorithm" {
		t.Errorf("criterion[0]: got %q", ticket.AcceptanceCriteria[0])
	}
}

func TestParseTicket_ShouldStatements(t *testing.T) {
	tc := NewTicketCompliance()

	content := `Improve error handling

The system should return structured error responses.
The API should include error codes in responses.
`

	ticket := tc.ParseTicket(content)

	if len(ticket.AcceptanceCriteria) < 2 {
		t.Fatalf("expected at least 2 criteria from should statements, got %d: %v",
			len(ticket.AcceptanceCriteria), ticket.AcceptanceCriteria)
	}
}

func TestParseTicket_EmptyContent(t *testing.T) {
	tc := NewTicketCompliance()

	ticket := tc.ParseTicket("")
	if ticket.Title != "" {
		t.Errorf("expected empty title, got %q", ticket.Title)
	}
	if len(ticket.AcceptanceCriteria) != 0 {
		t.Errorf("expected no criteria, got %v", ticket.AcceptanceCriteria)
	}
}

func TestCheckCompliance_AllSatisfied(t *testing.T) {
	tc := NewTicketCompliance()

	ticket := &Ticket{
		ID:    "HAWK-123",
		Title: "Add JWT authentication",
		AcceptanceCriteria: []string{
			"Token validation endpoint",
			"Middleware for protected routes",
			"Unit tests for token parsing",
		},
	}

	diff := `
+func ValidateToken(token string) (*Claims, error) {
+    // Token validation logic
+}
+
+func AuthMiddleware(next http.Handler) http.Handler {
+    // Protected routes middleware
+}
`
	commits := "feat: add token validation endpoint\nfeat: add auth middleware for protected routes\ntest: add unit tests for token parsing"

	result := tc.CheckCompliance(ticket, diff, commits)

	if result.Score != 1.0 {
		t.Errorf("expected score 1.0, got %f", result.Score)
	}
	if len(result.Unsatisfied) != 0 {
		t.Errorf("expected no unsatisfied criteria, got %v", result.Unsatisfied)
	}
	if len(result.Satisfied) != 3 {
		t.Errorf("expected 3 satisfied criteria, got %d", len(result.Satisfied))
	}
}

func TestCheckCompliance_PartiallySatisfied(t *testing.T) {
	tc := NewTicketCompliance()

	ticket := &Ticket{
		ID:    "HAWK-123",
		Title: "Add JWT authentication",
		AcceptanceCriteria: []string{
			"Token validation endpoint",
			"Middleware for protected routes",
			"Unit tests for token parsing",
			"Documentation for API changes",
		},
	}

	diff := `
+func ValidateToken(token string) (*Claims, error) {
+    // Token validation logic
+}
+
+func AuthMiddleware(next http.Handler) http.Handler {
+    // Protected routes middleware
+}
`
	commits := "feat: add token validation endpoint\nfeat: add auth middleware for protected routes\ntest: add unit tests for token parsing"

	result := tc.CheckCompliance(ticket, diff, commits)

	if result.Score != 0.75 {
		t.Errorf("expected score 0.75, got %f", result.Score)
	}
	if len(result.Satisfied) != 3 {
		t.Errorf("expected 3 satisfied, got %d: %v", len(result.Satisfied), result.Satisfied)
	}
	if len(result.Unsatisfied) != 1 {
		t.Errorf("expected 1 unsatisfied, got %d: %v", len(result.Unsatisfied), result.Unsatisfied)
	}
	if len(result.Unsatisfied) > 0 && result.Unsatisfied[0] != "Documentation for API changes" {
		t.Errorf("expected unsatisfied criterion 'Documentation for API changes', got %q", result.Unsatisfied[0])
	}
}

func TestCheckCompliance_NoCriteria(t *testing.T) {
	tc := NewTicketCompliance()

	ticket := &Ticket{
		ID:                 "HAWK-1",
		Title:              "Quick fix",
		AcceptanceCriteria: []string{},
	}

	result := tc.CheckCompliance(ticket, "some diff", "some commit")

	if result.Score != 1.0 {
		t.Errorf("expected score 1.0 when no criteria, got %f", result.Score)
	}
}

func TestCheckCompliance_NoneSatisfied(t *testing.T) {
	tc := NewTicketCompliance()

	ticket := &Ticket{
		ID:    "HAWK-50",
		Title: "Database migration",
		AcceptanceCriteria: []string{
			"PostgreSQL schema migration script",
			"Rollback procedure documented",
		},
	}

	diff := "+func HelloWorld() string { return \"hello\" }"
	commits := "chore: update readme"

	result := tc.CheckCompliance(ticket, diff, commits)

	if result.Score != 0.0 {
		t.Errorf("expected score 0.0, got %f", result.Score)
	}
	if len(result.Unsatisfied) != 2 {
		t.Errorf("expected 2 unsatisfied, got %d", len(result.Unsatisfied))
	}
}

func TestFormatComplianceResult(t *testing.T) {
	result := &ComplianceResult{
		Ticket: &Ticket{
			ID:    "HAWK-123",
			Title: "Add JWT authentication",
		},
		Satisfied: []string{
			"Token validation endpoint",
			"Middleware for protected routes",
			"Unit tests for token parsing",
		},
		Unsatisfied: []string{
			"Documentation for API changes",
		},
		Score: 0.75,
		Suggestions: []string{
			"Add documentation for: Documentation for API changes",
		},
	}

	output := FormatComplianceResult(result)

	// Check key components are present.
	if !strings.Contains(output, "HAWK-123") {
		t.Error("output should contain ticket ID")
	}
	if !strings.Contains(output, "Add JWT authentication") {
		t.Error("output should contain ticket title")
	}
	if !strings.Contains(output, "75%") {
		t.Error("output should contain percentage score")
	}
	if !strings.Contains(output, "3/4 criteria met") {
		t.Error("output should contain criteria count")
	}
	if !strings.Contains(output, "✓ Token validation endpoint") {
		t.Error("output should contain satisfied criterion with checkmark")
	}
	if !strings.Contains(output, "✗ Documentation for API changes") {
		t.Error("output should contain unsatisfied criterion with X mark")
	}
	if !strings.Contains(output, "Suggestions:") {
		t.Error("output should contain suggestions section")
	}
}

func TestFormatComplianceResult_PerfectScore(t *testing.T) {
	result := &ComplianceResult{
		Ticket: &Ticket{
			ID:    "#42",
			Title: "Simple fix",
		},
		Satisfied: []string{"Fix the bug"},
		Score:     1.0,
	}

	output := FormatComplianceResult(result)

	if !strings.Contains(output, "100%") {
		t.Error("expected 100% in output")
	}
	if !strings.Contains(output, "1/1 criteria met") {
		t.Error("expected '1/1 criteria met' in output")
	}
}

func TestTicketComplianceTool_Interface(t *testing.T) {
	tool := &TicketComplianceTool{}

	if tool.Name() != "ticket_compliance" {
		t.Errorf("expected name 'ticket_compliance', got %q", tool.Name())
	}

	if tool.Description() == "" {
		t.Error("description should not be empty")
	}

	params := tool.Parameters()
	if params == nil {
		t.Fatal("parameters should not be nil")
	}

	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("parameters should have properties")
	}

	requiredFields := []string{"ticket_content", "diff", "branch_name", "pr_description", "ticket_id", "ticket_source", "commit_messages"}
	for _, field := range requiredFields {
		if _, exists := props[field]; !exists {
			t.Errorf("missing parameter: %s", field)
		}
	}
}

func TestTicketComplianceTool_Execute(t *testing.T) {
	tool := &TicketComplianceTool{}

	input := map[string]string{
		"branch_name":    "feature/HAWK-123-jwt",
		"pr_description": "Fixes HAWK-123\n\nAdds JWT authentication",
		"ticket_content": `# Add JWT authentication

## Acceptance Criteria

- [ ] Token validation endpoint
- [ ] Middleware for protected routes
- [ ] Unit tests for token parsing
- [ ] Documentation for API changes
`,
		"ticket_id":       "HAWK-123",
		"ticket_source":   "jira",
		"diff":            "+func ValidateToken(t string) error {}\n+func AuthMiddleware() {}\n",
		"commit_messages": "feat: add token validation endpoint\nfeat: add middleware for protected routes\ntest: unit tests for token parsing",
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), inputJSON)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(result, "HAWK-123") {
		t.Error("result should contain ticket ID")
	}
	if !strings.Contains(result, "75%") {
		t.Error("result should show 75% compliance")
	}
}

func TestTicketComplianceTool_Execute_MissingRequired(t *testing.T) {
	tool := &TicketComplianceTool{}

	tests := []struct {
		name  string
		input map[string]string
	}{
		{
			name:  "missing ticket_content",
			input: map[string]string{"diff": "some diff"},
		},
		{
			name:  "missing diff",
			input: map[string]string{"ticket_content": "some content"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputJSON, _ := json.Marshal(tt.input)
			_, err := tool.Execute(context.Background(), inputJSON)
			if err == nil {
				t.Error("expected error for missing required field")
			}
		})
	}
}

func TestExtractKeywords(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
		excludes []string
	}{
		{
			name:     "filters stop words",
			input:    "The system should validate tokens",
			contains: []string{"system", "validate", "tokens"},
			excludes: []string{"the", "should"},
		},
		{
			name:     "filters short words",
			input:    "Add a new API endpoint",
			contains: []string{"new", "api", "endpoint"},
			excludes: []string{"a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keywords := extractKeywords(tt.input)
			for _, expected := range tt.contains {
				found := false
				for _, k := range keywords {
					if k == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected keywords to contain %q, got %v", expected, keywords)
				}
			}
			for _, excluded := range tt.excludes {
				for _, k := range keywords {
					if k == excluded {
						t.Errorf("expected keywords to NOT contain %q, got %v", excluded, keywords)
					}
				}
			}
		})
	}
}

func TestGenerateSuggestion(t *testing.T) {
	tests := []struct {
		criterion string
		contains  string
	}{
		{"Unit tests for parsing", "Add tests"},
		{"Documentation for API", "Add documentation"},
		{"Error handling for invalid input", "error handling/validation"},
		{"Logging for authentication failures", "Add logging"},
		{"Rate limiting for endpoints", "Address missing requirement"},
	}

	for _, tt := range tests {
		t.Run(tt.criterion, func(t *testing.T) {
			suggestion := generateSuggestion(tt.criterion)
			if !strings.Contains(suggestion, tt.contains) {
				t.Errorf("expected suggestion to contain %q, got %q", tt.contains, suggestion)
			}
		})
	}
}

func TestCriterionSatisfied(t *testing.T) {
	tests := []struct {
		name      string
		criterion string
		corpus    string
		expected  bool
	}{
		{
			name:      "keywords present in corpus",
			criterion: "Token validation endpoint",
			corpus:    "added a token validation endpoint handler",
			expected:  true,
		},
		{
			name:      "keywords not present",
			criterion: "Database migration script",
			corpus:    "updated the readme file with usage info",
			expected:  false,
		},
		{
			name:      "partial match below threshold",
			criterion: "PostgreSQL schema migration rollback",
			corpus:    "just a regular code update with postgresql mention",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := criterionSatisfied(tt.criterion, tt.corpus)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
