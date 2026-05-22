package validation

import (
	"regexp"
	"strings"
	"testing"
)

func TestValidateJSON_ValidInput(t *testing.T) {
	sv := NewSchemaValidator()
	schema := &Schema{
		Name:     "test",
		Type:     "json",
		Required: []string{"name", "age"},
		Fields: map[string]FieldSpec{
			"name": {Type: "string", Required: true, MinLength: 1},
			"age":  {Type: "number", Required: true},
		},
	}

	result := sv.ValidateJSON(`{"name": "Alice", "age": 30}`, schema)
	if !result.Valid {
		t.Errorf("expected valid, got errors: %v", result.Errors)
	}
	if result.Extracted == nil {
		t.Error("expected extracted value")
	}
}

func TestValidateJSON_MissingRequiredField(t *testing.T) {
	sv := NewSchemaValidator()
	schema := &Schema{
		Name:     "test",
		Type:     "json",
		Required: []string{"name", "age"},
		Fields: map[string]FieldSpec{
			"name": {Type: "string", Required: true},
			"age":  {Type: "number", Required: true},
		},
	}

	result := sv.ValidateJSON(`{"name": "Alice"}`, schema)
	if result.Valid {
		t.Error("expected invalid for missing required field")
	}
	found := false
	for _, e := range result.Errors {
		if e.Field == "age" && strings.Contains(e.Message, "required") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error about missing 'age' field, got: %v", result.Errors)
	}
}

func TestValidateJSON_WrongType(t *testing.T) {
	sv := NewSchemaValidator()
	schema := &Schema{
		Name:     "test",
		Type:     "json",
		Required: []string{"count"},
		Fields: map[string]FieldSpec{
			"count": {Type: "number", Required: true},
		},
	}

	result := sv.ValidateJSON(`{"count": "not a number"}`, schema)
	if result.Valid {
		t.Error("expected invalid for wrong type")
	}
	found := false
	for _, e := range result.Errors {
		if e.Field == "count" && strings.Contains(e.Message, "wrong type") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected type error for 'count', got: %v", result.Errors)
	}
}

func TestValidateJSON_EnumViolation(t *testing.T) {
	sv := NewSchemaValidator()
	schema := &Schema{
		Name:     "test",
		Type:     "json",
		Required: []string{"status"},
		Fields: map[string]FieldSpec{
			"status": {Type: "string", Required: true, Enum: []string{"active", "inactive"}},
		},
	}

	result := sv.ValidateJSON(`{"status": "unknown"}`, schema)
	if result.Valid {
		t.Error("expected invalid for enum violation")
	}
	found := false
	for _, e := range result.Errors {
		if e.Field == "status" && strings.Contains(e.Message, "enum") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected enum error for 'status', got: %v", result.Errors)
	}
}

func TestExtractJSONFromOutput_FromCodeFence(t *testing.T) {
	input := "Here is the result:\n```json\n{\"key\": \"value\"}\n```\nDone."
	got, err := ExtractJSONFromOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `{"key": "value"}` {
		t.Errorf("got %q, want %q", got, `{"key": "value"}`)
	}
}

func TestExtractJSONFromOutput_FromRawText(t *testing.T) {
	input := `The answer is {"foo": 42, "bar": true} and that's it.`
	got, err := ExtractJSONFromOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `{"foo": 42, "bar": true}` {
		t.Errorf("got %q, want %q", got, `{"foo": 42, "bar": true}`)
	}
}

func TestRepairJSON_TrailingComma(t *testing.T) {
	input := `{"name": "test", "value": 1,}`
	got, err := RepairJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, `"name"`) || strings.Contains(got, ",}") {
		t.Errorf("trailing comma not removed: %s", got)
	}
}

func TestRepairJSON_SingleQuotes(t *testing.T) {
	input := `{'name': 'test', 'value': 1}`
	got, err := RepairJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "'") {
		t.Errorf("single quotes not replaced: %s", got)
	}
	if !strings.Contains(got, `"name"`) {
		t.Errorf("expected double-quoted keys: %s", got)
	}
}

func TestRepairJSON_UnquotedKeys(t *testing.T) {
	input := `{name: "test", value: 1}`
	got, err := RepairJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, `"name"`) || !strings.Contains(got, `"value"`) {
		t.Errorf("unquoted keys not fixed: %s", got)
	}
}

func TestExtractCodeFromOutput_FindsLanguageBlock(t *testing.T) {
	input := "Here is the code:\n```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```\n"
	got, err := ExtractCodeFromOutput(input, "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "func main()") {
		t.Errorf("expected go code, got: %s", got)
	}
}

func TestBuildSchemaPrompt(t *testing.T) {
	schema := &Schema{
		Name:     "test",
		Type:     "json",
		Required: []string{"name", "status"},
		Fields: map[string]FieldSpec{
			"name":   {Type: "string", Required: true, Description: "user name"},
			"status": {Type: "string", Required: true, Enum: []string{"pending", "active", "done"}},
			"notes":  {Type: "string", Required: false},
		},
		Examples: []string{`{"name": "Alice", "status": "active"}`},
	}

	prompt := BuildSchemaPrompt(schema)
	if !strings.Contains(prompt, "Respond with JSON") {
		t.Error("expected JSON instruction in prompt")
	}
	if !strings.Contains(prompt, "required") {
		t.Error("expected 'required' annotation in prompt")
	}
	if !strings.Contains(prompt, "pending") {
		t.Error("expected enum values in prompt")
	}
	if !strings.Contains(prompt, "Example") {
		t.Error("expected example in prompt")
	}
}

func TestRegisterAndRetrieveSchema(t *testing.T) {
	sv := NewSchemaValidator()
	schema := &Schema{
		Name:     "custom",
		Type:     "json",
		Required: []string{"id"},
		Fields: map[string]FieldSpec{
			"id": {Type: "string", Required: true},
		},
	}

	sv.Register("custom", schema)
	result := sv.Validate("custom", `{"id": "abc123"}`)
	if !result.Valid {
		t.Errorf("expected valid, got errors: %v", result.Errors)
	}
}

func TestBuiltinSchemas_CommitMessage(t *testing.T) {
	sv := NewSchemaValidator()

	// Valid conventional commit.
	result := sv.Validate("commit_message", "feat(auth): add login support")
	if !result.Valid {
		t.Errorf("expected valid commit message, got errors: %v", result.Errors)
	}

	// Invalid commit message.
	result = sv.Validate("commit_message", "added some stuff")
	if result.Valid {
		t.Error("expected invalid for non-conventional commit")
	}
}

func TestBuiltinSchemas_Plan(t *testing.T) {
	sv := NewSchemaValidator()

	valid := `{"goal": "Refactor the authentication module", "steps": ["Extract interface", "Write tests"], "files": ["auth.go"]}`
	result := sv.Validate("plan", valid)
	if !result.Valid {
		t.Errorf("expected valid plan, got errors: %v", result.Errors)
	}

	// Missing steps field.
	invalid := `{"goal": "Do stuff", "files": ["a.go"]}`
	result = sv.Validate("plan", invalid)
	if result.Valid {
		t.Error("expected invalid for missing steps")
	}
}

func TestBuiltinSchemas_ReviewFinding(t *testing.T) {
	sv := NewSchemaValidator()

	valid := `{"severity": "warning", "message": "Unused variable detected", "file": "main.go", "line": 42}`
	result := sv.Validate("review_finding", valid)
	if !result.Valid {
		t.Errorf("expected valid review finding, got errors: %v", result.Errors)
	}

	// Invalid severity enum.
	invalid := `{"severity": "critical", "message": "Something bad", "file": "main.go", "line": 1}`
	result = sv.Validate("review_finding", invalid)
	if result.Valid {
		t.Error("expected invalid for bad severity enum")
	}
}

func TestBuiltinSchemas_ToolCall(t *testing.T) {
	sv := NewSchemaValidator()

	valid := `{"name": "read_file", "arguments": {"path": "/tmp/foo.txt"}}`
	result := sv.Validate("tool_call", valid)
	if !result.Valid {
		t.Errorf("expected valid tool_call, got errors: %v", result.Errors)
	}
}

func TestInvalidJSON_Handling(t *testing.T) {
	sv := NewSchemaValidator()
	schema := &Schema{
		Name:     "test",
		Type:     "json",
		Required: []string{"x"},
		Fields: map[string]FieldSpec{
			"x": {Type: "string", Required: true},
		},
	}

	result := sv.ValidateJSON("this is not json at all", schema)
	if result.Valid {
		t.Error("expected invalid for garbage input")
	}
	if len(result.Errors) == 0 {
		t.Error("expected at least one error")
	}
}

func TestNestedObjectValidation(t *testing.T) {
	sv := NewSchemaValidator()
	schema := &Schema{
		Name:     "test",
		Type:     "json",
		Required: []string{"metadata"},
		Fields: map[string]FieldSpec{
			"metadata": {Type: "object", Required: true},
		},
	}

	// Valid nested object.
	result := sv.ValidateJSON(`{"metadata": {"key": "value", "count": 3}}`, schema)
	if !result.Valid {
		t.Errorf("expected valid for nested object, got errors: %v", result.Errors)
	}

	// metadata is not an object.
	result = sv.ValidateJSON(`{"metadata": "just a string"}`, schema)
	if result.Valid {
		t.Error("expected invalid when metadata is a string instead of object")
	}
	found := false
	for _, e := range result.Errors {
		if e.Field == "metadata" && strings.Contains(e.Message, "wrong type") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected type error for 'metadata', got: %v", result.Errors)
	}
}

func TestValidateJSON_ArrayField(t *testing.T) {
	sv := NewSchemaValidator()
	schema := &Schema{
		Name:     "test",
		Type:     "json",
		Required: []string{"items"},
		Fields: map[string]FieldSpec{
			"items": {Type: "array", Required: true},
		},
	}

	result := sv.ValidateJSON(`{"items": [1, 2, 3]}`, schema)
	if !result.Valid {
		t.Errorf("expected valid, got errors: %v", result.Errors)
	}

	result = sv.ValidateJSON(`{"items": "not an array"}`, schema)
	if result.Valid {
		t.Error("expected invalid when items is string")
	}
}

func TestValidateJSON_BooleanField(t *testing.T) {
	sv := NewSchemaValidator()
	schema := &Schema{
		Name:     "test",
		Type:     "json",
		Required: []string{"active"},
		Fields: map[string]FieldSpec{
			"active": {Type: "boolean", Required: true},
		},
	}

	result := sv.ValidateJSON(`{"active": true}`, schema)
	if !result.Valid {
		t.Errorf("expected valid, got errors: %v", result.Errors)
	}

	result = sv.ValidateJSON(`{"active": "yes"}`, schema)
	if result.Valid {
		t.Error("expected invalid when active is string")
	}
}

func TestValidateJSON_StringLength(t *testing.T) {
	sv := NewSchemaValidator()
	schema := &Schema{
		Name: "test",
		Type: "json",
		Fields: map[string]FieldSpec{
			"title": {Type: "string", MinLength: 3, MaxLength: 10},
		},
	}

	result := sv.ValidateJSON(`{"title": "hello"}`, schema)
	if !result.Valid {
		t.Errorf("expected valid, got errors: %v", result.Errors)
	}

	result = sv.ValidateJSON(`{"title": "hi"}`, schema)
	if result.Valid {
		t.Error("expected invalid for too-short string")
	}

	result = sv.ValidateJSON(`{"title": "this is way too long"}`, schema)
	if result.Valid {
		t.Error("expected invalid for too-long string")
	}
}

func TestValidateJSON_Pattern(t *testing.T) {
	sv := NewSchemaValidator()
	schema := &Schema{
		Name: "test",
		Type: "json",
		Fields: map[string]FieldSpec{
			"email": {Type: "string", Pattern: `^[^@]+@[^@]+\.[^@]+$`},
		},
	}

	result := sv.ValidateJSON(`{"email": "user@example.com"}`, schema)
	if !result.Valid {
		t.Errorf("expected valid, got errors: %v", result.Errors)
	}

	result = sv.ValidateJSON(`{"email": "not-an-email"}`, schema)
	if result.Valid {
		t.Error("expected invalid for bad email pattern")
	}
}

func TestValidate_SchemaNotFound(t *testing.T) {
	sv := NewSchemaValidator()
	result := sv.Validate("nonexistent", "anything")
	if result.Valid {
		t.Error("expected invalid for missing schema")
	}
	if len(result.Errors) == 0 || !strings.Contains(result.Errors[0].Message, "not found") {
		t.Error("expected 'not found' error")
	}
}

func TestValidateCode_Balanced(t *testing.T) {
	sv := NewSchemaValidator()
	schema := &Schema{
		Name: "test_code",
		Type: "code",
	}
	sv.Register("test_code", schema)

	result := sv.Validate("test_code", "```go\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n```")
	if !result.Valid {
		t.Errorf("expected valid code, got errors: %v", result.Errors)
	}
}

func TestValidateCode_Unbalanced(t *testing.T) {
	sv := NewSchemaValidator()
	schema := &Schema{
		Name: "test_code",
		Type: "code",
	}
	sv.Register("test_code", schema)

	// Use a code block with an explicit unbalanced brace (missing closing }).
	result := sv.Validate("test_code", "```go\nfunc main() {\nfmt.Println(\"hi\")\n```")
	if result.Valid {
		t.Error("expected invalid for unbalanced braces")
	}
}

func TestValidateStructured_Pattern(t *testing.T) {
	sv := NewSchemaValidator()
	schema := &Schema{
		Name:    "version",
		Type:    "structured",
		Pattern: regexp.MustCompile(`^\d+\.\d+\.\d+$`),
	}
	sv.Register("version", schema)

	result := sv.Validate("version", "1.2.3")
	if !result.Valid {
		t.Errorf("expected valid, got errors: %v", result.Errors)
	}

	result = sv.Validate("version", "not a version")
	if result.Valid {
		t.Error("expected invalid for non-version string")
	}
}

func TestExtractJSONFromOutput_Array(t *testing.T) {
	input := `Here are the results: [{"id": 1}, {"id": 2}]`
	got, err := ExtractJSONFromOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(got, "[") {
		t.Errorf("expected array, got: %s", got)
	}
}

func TestExtractCodeFromOutput_Indented(t *testing.T) {
	input := "Here is the code:\n\n    def hello():\n        print(\"hi\")\n\nThat's it."
	got, err := ExtractCodeFromOutput(input, "python")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "def hello()") {
		t.Errorf("expected indented code, got: %s", got)
	}
}

func TestRepairJSON_MissingBracket(t *testing.T) {
	input := `{"name": "test", "items": [1, 2, 3]`
	got, err := RepairJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "}") {
		t.Errorf("expected missing brace to be added: %s", got)
	}
}

func TestRepairJSON_Comments(t *testing.T) {
	input := `{
		// this is a comment
		"name": "test",
		/* block comment */
		"value": 42
	}`
	got, err := RepairJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "//") || strings.Contains(got, "/*") {
		t.Errorf("comments not removed: %s", got)
	}
}

func TestBuildSchemaPrompt_CodeType(t *testing.T) {
	schema := &Schema{
		Name:    "code_out",
		Type:    "code",
		Pattern: regexp.MustCompile(`func \w+`),
	}
	prompt := BuildSchemaPrompt(schema)
	if !strings.Contains(prompt, "code block") {
		t.Error("expected code block instruction")
	}
}

func TestBuildSchemaPrompt_StructuredType(t *testing.T) {
	schema := &Schema{
		Name:    "structured_out",
		Type:    "structured",
		Pattern: regexp.MustCompile(`^\w+: .+`),
	}
	prompt := BuildSchemaPrompt(schema)
	if !strings.Contains(prompt, "structured text") {
		t.Error("expected structured text instruction")
	}
}

func TestValidateJSON_OptionalFieldNotRequired(t *testing.T) {
	sv := NewSchemaValidator()
	schema := &Schema{
		Name:     "test",
		Type:     "json",
		Required: []string{"id"},
		Fields: map[string]FieldSpec{
			"id":    {Type: "string", Required: true},
			"notes": {Type: "string", Required: false},
		},
	}

	// Should be valid even without optional "notes" field.
	result := sv.ValidateJSON(`{"id": "abc"}`, schema)
	if !result.Valid {
		t.Errorf("expected valid without optional field, got errors: %v", result.Errors)
	}
}

func TestExtractJSONFromOutput_PlainJSON(t *testing.T) {
	input := `{"simple": true}`
	got, err := ExtractJSONFromOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != input {
		t.Errorf("got %q, want %q", got, input)
	}
}
