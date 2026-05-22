package validation

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// SchemaValidator validates LLM output against registered schemas.
type SchemaValidator struct {
	Schemas map[string]*Schema
	mu      sync.RWMutex
}

// Schema defines the expected structure of an LLM output.
type Schema struct {
	Name     string
	Type     string // "json", "yaml", "code", "structured"
	Required []string
	Fields   map[string]FieldSpec
	Pattern  *regexp.Regexp
	Examples []string
}

// FieldSpec describes constraints on a single field.
type FieldSpec struct {
	Type        string // "string", "number", "boolean", "array", "object"
	Required    bool
	MinLength   int
	MaxLength   int
	Pattern     string
	Enum        []string
	Description string
}

// SchemaValidationResult holds the outcome of a schema validation pass.
type SchemaValidationResult struct {
	Valid     bool
	Errors    []SchemaValidationError
	Warnings  []string
	Extracted interface{}
}

// SchemaValidationError describes a single schema validation failure.
type SchemaValidationError struct {
	Field    string
	Message  string
	Expected string
	Got      string
}

// NewSchemaValidator creates a SchemaValidator pre-loaded with built-in schemas.
func NewSchemaValidator() *SchemaValidator {
	sv := &SchemaValidator{
		Schemas: make(map[string]*Schema),
	}
	sv.registerBuiltins()
	return sv
}

// Register adds or replaces a schema by name.
func (sv *SchemaValidator) Register(name string, schema *Schema) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	sv.Schemas[name] = schema
}

// Validate dispatches validation based on the schema's Type.
func (sv *SchemaValidator) Validate(schemaName string, output string) *SchemaValidationResult {
	sv.mu.RLock()
	schema, ok := sv.Schemas[schemaName]
	sv.mu.RUnlock()
	if !ok {
		return &SchemaValidationResult{
			Valid:  false,
			Errors: []SchemaValidationError{{Field: "", Message: fmt.Sprintf("schema %q not found", schemaName)}},
		}
	}

	switch schema.Type {
	case "json":
		return sv.ValidateJSON(output, schema)
	case "code":
		return sv.validateCode(output, schema)
	case "structured":
		return sv.validateStructured(output, schema)
	case "yaml":
		return sv.validateStructured(output, schema)
	default:
		return &SchemaValidationResult{
			Valid:  false,
			Errors: []SchemaValidationError{{Field: "", Message: fmt.Sprintf("unknown schema type %q", schema.Type)}},
		}
	}
}

// ValidateJSON extracts JSON from output and validates against the schema.
func (sv *SchemaValidator) ValidateJSON(output string, schema *Schema) *SchemaValidationResult {
	result := &SchemaValidationResult{Valid: true}

	raw, err := ExtractJSONFromOutput(output)
	if err != nil {
		// try repair
		repaired, repairErr := RepairJSON(output)
		if repairErr != nil {
			result.Valid = false
			result.Errors = append(result.Errors, SchemaValidationError{
				Field:   "",
				Message: "failed to extract JSON: " + err.Error(),
			})
			return result
		}
		raw = repaired
		result.Warnings = append(result.Warnings, "JSON required repair")
	}

	var parsed interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, SchemaValidationError{
			Field:   "",
			Message: "invalid JSON: " + err.Error(),
		})
		return result
	}
	result.Extracted = parsed

	obj, isObj := parsed.(map[string]interface{})
	if !isObj {
		// If schema has required fields, the output must be an object.
		if len(schema.Required) > 0 {
			result.Valid = false
			result.Errors = append(result.Errors, SchemaValidationError{
				Field:    "",
				Message:  "expected JSON object",
				Expected: "object",
				Got:      fmt.Sprintf("%T", parsed),
			})
		}
		return result
	}

	// Check required fields.
	for _, req := range schema.Required {
		if _, exists := obj[req]; !exists {
			result.Valid = false
			result.Errors = append(result.Errors, SchemaValidationError{
				Field:   req,
				Message: "required field missing",
			})
		}
	}

	// Check field specs.
	for fieldName, spec := range schema.Fields {
		val, exists := obj[fieldName]
		if !exists {
			if spec.Required {
				result.Valid = false
				result.Errors = append(result.Errors, SchemaValidationError{
					Field:   fieldName,
					Message: "required field missing",
				})
			}
			continue
		}
		sv.validateField(fieldName, val, spec, result)
	}

	return result
}

func (sv *SchemaValidator) validateField(fieldName string, val interface{}, spec FieldSpec, result *SchemaValidationResult) {
	switch spec.Type {
	case "string":
		s, ok := val.(string)
		if !ok {
			result.Valid = false
			result.Errors = append(result.Errors, SchemaValidationError{
				Field:    fieldName,
				Message:  "wrong type",
				Expected: "string",
				Got:      fmt.Sprintf("%T", val),
			})
			return
		}
		if spec.MinLength > 0 && len(s) < spec.MinLength {
			result.Valid = false
			result.Errors = append(result.Errors, SchemaValidationError{
				Field:    fieldName,
				Message:  fmt.Sprintf("too short (min %d)", spec.MinLength),
				Expected: fmt.Sprintf("length >= %d", spec.MinLength),
				Got:      strconv.Itoa(len(s)),
			})
		}
		if spec.MaxLength > 0 && len(s) > spec.MaxLength {
			result.Valid = false
			result.Errors = append(result.Errors, SchemaValidationError{
				Field:    fieldName,
				Message:  fmt.Sprintf("too long (max %d)", spec.MaxLength),
				Expected: fmt.Sprintf("length <= %d", spec.MaxLength),
				Got:      strconv.Itoa(len(s)),
			})
		}
		if spec.Pattern != "" {
			re, err := regexp.Compile(spec.Pattern)
			if err == nil && !re.MatchString(s) {
				result.Valid = false
				result.Errors = append(result.Errors, SchemaValidationError{
					Field:    fieldName,
					Message:  "does not match pattern",
					Expected: spec.Pattern,
					Got:      s,
				})
			}
		}
		if len(spec.Enum) > 0 {
			found := false
			for _, allowed := range spec.Enum {
				if s == allowed {
					found = true
					break
				}
			}
			if !found {
				result.Valid = false
				result.Errors = append(result.Errors, SchemaValidationError{
					Field:    fieldName,
					Message:  "value not in enum",
					Expected: strings.Join(spec.Enum, ", "),
					Got:      s,
				})
			}
		}
	case "number":
		switch val.(type) {
		case float64, json.Number:
			// ok
		default:
			result.Valid = false
			result.Errors = append(result.Errors, SchemaValidationError{
				Field:    fieldName,
				Message:  "wrong type",
				Expected: "number",
				Got:      fmt.Sprintf("%T", val),
			})
		}
	case "boolean":
		if _, ok := val.(bool); !ok {
			result.Valid = false
			result.Errors = append(result.Errors, SchemaValidationError{
				Field:    fieldName,
				Message:  "wrong type",
				Expected: "boolean",
				Got:      fmt.Sprintf("%T", val),
			})
		}
	case "array":
		if _, ok := val.([]interface{}); !ok {
			result.Valid = false
			result.Errors = append(result.Errors, SchemaValidationError{
				Field:    fieldName,
				Message:  "wrong type",
				Expected: "array",
				Got:      fmt.Sprintf("%T", val),
			})
		}
	case "object":
		if _, ok := val.(map[string]interface{}); !ok {
			result.Valid = false
			result.Errors = append(result.Errors, SchemaValidationError{
				Field:    fieldName,
				Message:  "wrong type",
				Expected: "object",
				Got:      fmt.Sprintf("%T", val),
			})
		}
	}
}

func (sv *SchemaValidator) validateCode(output string, schema *Schema) *SchemaValidationResult {
	result := &SchemaValidationResult{Valid: true}

	code, err := ExtractCodeFromOutput(output, "")
	if err != nil {
		// Try raw output as code
		code = output
	}
	result.Extracted = code

	// Check balanced braces and parentheses.
	braces := 0
	parens := 0
	brackets := 0
	for _, ch := range code {
		switch ch {
		case '{':
			braces++
		case '}':
			braces--
		case '(':
			parens++
		case ')':
			parens--
		case '[':
			brackets++
		case ']':
			brackets--
		}
		if braces < 0 || parens < 0 || brackets < 0 {
			result.Valid = false
			result.Errors = append(result.Errors, SchemaValidationError{
				Field:   "",
				Message: "unbalanced delimiters",
			})
			return result
		}
	}
	if braces != 0 || parens != 0 || brackets != 0 {
		result.Valid = false
		result.Errors = append(result.Errors, SchemaValidationError{
			Field:   "",
			Message: "unbalanced delimiters",
		})
	}

	// Check pattern if present.
	if schema.Pattern != nil && !schema.Pattern.MatchString(code) {
		result.Valid = false
		result.Errors = append(result.Errors, SchemaValidationError{
			Field:    "",
			Message:  "does not match pattern",
			Expected: schema.Pattern.String(),
		})
	}

	return result
}

func (sv *SchemaValidator) validateStructured(output string, schema *Schema) *SchemaValidationResult {
	result := &SchemaValidationResult{Valid: true, Extracted: output}

	if schema.Pattern != nil {
		if !schema.Pattern.MatchString(output) {
			result.Valid = false
			result.Errors = append(result.Errors, SchemaValidationError{
				Field:    "",
				Message:  "does not match pattern",
				Expected: schema.Pattern.String(),
				Got:      output,
			})
		}
	}

	return result
}

// ExtractJSONFromOutput finds JSON content in LLM output text.
func ExtractJSONFromOutput(text string) (string, error) {
	text = strings.TrimSpace(text)

	// Try code fence first: ```json ... ```
	re := regexp.MustCompile("(?s)```(?:json)?\\s*\\n?(.*?)\\n?\\s*```")
	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		candidate := strings.TrimSpace(matches[1])
		if json.Valid([]byte(candidate)) {
			return candidate, nil
		}
	}

	// Try whichever comes first: { or [
	idxObj := strings.Index(text, "{")
	idxArr := strings.Index(text, "[")

	// Determine order of attempts based on which appears first.
	type attempt struct {
		idx   int
		open  rune
		close rune
	}
	var attempts []attempt
	if idxObj >= 0 && idxArr >= 0 {
		if idxArr < idxObj {
			attempts = []attempt{{idxArr, '[', ']'}, {idxObj, '{', '}'}}
		} else {
			attempts = []attempt{{idxObj, '{', '}'}, {idxArr, '[', ']'}}
		}
	} else if idxObj >= 0 {
		attempts = []attempt{{idxObj, '{', '}'}}
	} else if idxArr >= 0 {
		attempts = []attempt{{idxArr, '[', ']'}}
	}

	for _, a := range attempts {
		candidate := extractBalancedDelimiters(text[a.idx:], a.open, a.close)
		if candidate != "" && json.Valid([]byte(candidate)) {
			return candidate, nil
		}
	}

	// Try the entire text as JSON.
	if json.Valid([]byte(text)) {
		return text, nil
	}

	return "", fmt.Errorf("no valid JSON found in output")
}

func extractBalancedDelimiters(s string, open, close rune) string {
	depth := 0
	inString := false
	escaped := false
	start := -1
	for i, ch := range s {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if ch == open {
			if depth == 0 {
				start = i
			}
			depth++
		} else if ch == close {
			depth--
			if depth == 0 && start >= 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// ExtractCodeFromOutput finds code blocks in LLM output.
func ExtractCodeFromOutput(text string, language string) (string, error) {
	text = strings.TrimSpace(text)

	// Try fenced code block with language.
	if language != "" {
		pattern := fmt.Sprintf("(?s)```%s\\s*\\n(.*?)\\n\\s*```", regexp.QuoteMeta(language))
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1]), nil
		}
	}

	// Try any fenced code block.
	re := regexp.MustCompile("(?s)```[a-zA-Z]*\\s*\\n(.*?)\\n\\s*```")
	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1]), nil
	}

	// Fall back to indented blocks (4 spaces or 1 tab).
	lines := strings.Split(text, "\n")
	var codeLines []string
	inBlock := false
	for _, line := range lines {
		if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			inBlock = true
			// Remove one level of indent.
			if strings.HasPrefix(line, "    ") {
				codeLines = append(codeLines, line[4:])
			} else {
				codeLines = append(codeLines, line[1:])
			}
		} else if inBlock && strings.TrimSpace(line) == "" {
			codeLines = append(codeLines, "")
		} else if inBlock {
			break
		}
	}
	if len(codeLines) > 0 {
		return strings.TrimSpace(strings.Join(codeLines, "\n")), nil
	}

	return "", fmt.Errorf("no code block found in output")
}

// RepairJSON attempts to fix common LLM JSON mistakes.
func RepairJSON(broken string) (string, error) {
	s := strings.TrimSpace(broken)

	// Extract from code fences if present.
	re := regexp.MustCompile("(?s)```(?:json)?\\s*\\n?(.*?)\\n?\\s*```")
	if matches := re.FindStringSubmatch(s); len(matches) > 1 {
		s = strings.TrimSpace(matches[1])
	}

	// Find JSON start.
	startObj := strings.Index(s, "{")
	startArr := strings.Index(s, "[")
	if startObj < 0 && startArr < 0 {
		return "", fmt.Errorf("no JSON structure found")
	}
	if startObj >= 0 && (startArr < 0 || startObj < startArr) {
		s = s[startObj:]
	} else if startArr >= 0 {
		s = s[startArr:]
	}

	// Remove single-line comments.
	commentRe := regexp.MustCompile(`//[^\n]*`)
	s = commentRe.ReplaceAllString(s, "")

	// Remove multi-line comments.
	blockCommentRe := regexp.MustCompile(`(?s)/\*.*?\*/`)
	s = blockCommentRe.ReplaceAllString(s, "")

	// Replace single quotes with double quotes.
	s = replaceSingleQuotes(s)

	// Quote unquoted keys.
	s = quoteUnquotedKeys(s)

	// Remove trailing commas before } or ].
	trailingCommaRe := regexp.MustCompile(`,\s*([}\]])`)
	s = trailingCommaRe.ReplaceAllString(s, "$1")

	// Try to add missing closing brackets.
	s = balanceBrackets(s)

	if json.Valid([]byte(s)) {
		return s, nil
	}

	return "", fmt.Errorf("unable to repair JSON")
}

func replaceSingleQuotes(s string) string {
	var buf strings.Builder
	inDouble := false
	inSingle := false
	escaped := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			buf.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			buf.WriteByte(ch)
			escaped = true
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			buf.WriteByte(ch)
			continue
		}
		if ch == '\'' && !inDouble {
			if !inSingle {
				inSingle = true
				buf.WriteByte('"')
			} else {
				inSingle = false
				buf.WriteByte('"')
			}
			continue
		}
		buf.WriteByte(ch)
	}
	return buf.String()
}

func quoteUnquotedKeys(s string) string {
	// Match unquoted keys: word characters followed by optional whitespace and a colon.
	re := regexp.MustCompile(`(?m)([\{\,]\s*)([a-zA-Z_][a-zA-Z0-9_]*)\s*:`)
	return re.ReplaceAllString(s, `$1"$2":`)
}

func balanceBrackets(s string) string {
	braces := 0
	brackets := 0
	inString := false
	escaped := false

	for _, ch := range s {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			braces++
		case '}':
			braces--
		case '[':
			brackets++
		case ']':
			brackets--
		}
	}

	for braces > 0 {
		s += "}"
		braces--
	}
	for brackets > 0 {
		s += "]"
		brackets--
	}
	return s
}

// BuildSchemaPrompt generates LLM instructions describing the expected output format.
func BuildSchemaPrompt(schema *Schema) string {
	var sb strings.Builder

	switch schema.Type {
	case "json":
		sb.WriteString("Respond with JSON matching this schema:\n")
		sb.WriteString("{\n")
		var fields []string
		// Put required fields first.
		for _, req := range schema.Required {
			if spec, ok := schema.Fields[req]; ok {
				fields = append(fields, formatFieldPrompt(req, spec, true))
			} else {
				fields = append(fields, fmt.Sprintf("  %q: \"(required)\"", req))
			}
		}
		// Then optional fields.
		for name, spec := range schema.Fields {
			if !schemaContains(schema.Required, name) {
				fields = append(fields, formatFieldPrompt(name, spec, false))
			}
		}
		sb.WriteString(strings.Join(fields, ",\n"))
		sb.WriteString("\n}\n")
	case "code":
		sb.WriteString("Respond with a code block.\n")
		if schema.Pattern != nil {
			sb.WriteString(fmt.Sprintf("The code must match: %s\n", schema.Pattern.String()))
		}
	case "structured":
		sb.WriteString("Respond with structured text.\n")
		if schema.Pattern != nil {
			sb.WriteString(fmt.Sprintf("Format pattern: %s\n", schema.Pattern.String()))
		}
	default:
		sb.WriteString("Respond in the expected format.\n")
	}

	if len(schema.Examples) > 0 {
		sb.WriteString("\nExample:\n")
		sb.WriteString(schema.Examples[0])
		sb.WriteString("\n")
	}

	return sb.String()
}

func formatFieldPrompt(name string, spec FieldSpec, required bool) string {
	parts := []string{spec.Type}
	if required {
		parts = append(parts, "required")
	} else {
		parts = append(parts, "optional")
	}
	if len(spec.Enum) > 0 {
		parts = append(parts, "one of: "+strings.Join(spec.Enum, ", "))
	}
	if spec.Description != "" {
		parts = append(parts, spec.Description)
	}
	return fmt.Sprintf("  %q: %q", name, strings.Join(parts, " - "))
}

func schemaContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (sv *SchemaValidator) registerBuiltins() {
	// commit_message schema: conventional commit format.
	sv.Schemas["commit_message"] = &Schema{
		Name:    "commit_message",
		Type:    "structured",
		Pattern: regexp.MustCompile(`^(feat|fix|docs|style|refactor|perf|test|chore|ci|build|revert)(\(.+\))?!?:\s.+`),
		Examples: []string{
			"feat(auth): add OAuth2 login support",
		},
	}

	// plan schema.
	sv.Schemas["plan"] = &Schema{
		Name:     "plan",
		Type:     "json",
		Required: []string{"goal", "steps", "files"},
		Fields: map[string]FieldSpec{
			"goal": {
				Type:        "string",
				Required:    true,
				MinLength:   5,
				Description: "what the plan aims to achieve",
			},
			"steps": {
				Type:        "array",
				Required:    true,
				Description: "ordered list of steps",
			},
			"files": {
				Type:        "array",
				Required:    true,
				Description: "files to be modified",
			},
		},
		Examples: []string{
			`{"goal": "Add user authentication", "steps": ["Add auth middleware", "Create login endpoint"], "files": ["auth.go", "routes.go"]}`,
		},
	}

	// review_finding schema.
	sv.Schemas["review_finding"] = &Schema{
		Name:     "review_finding",
		Type:     "json",
		Required: []string{"severity", "message", "file", "line"},
		Fields: map[string]FieldSpec{
			"severity": {
				Type:     "string",
				Required: true,
				Enum:     []string{"error", "warning", "info", "hint"},
			},
			"message": {
				Type:      "string",
				Required:  true,
				MinLength: 5,
			},
			"file": {
				Type:     "string",
				Required: true,
			},
			"line": {
				Type:     "number",
				Required: true,
			},
		},
	}

	// tool_call schema.
	sv.Schemas["tool_call"] = &Schema{
		Name:     "tool_call",
		Type:     "json",
		Required: []string{"name", "arguments"},
		Fields: map[string]FieldSpec{
			"name": {
				Type:     "string",
				Required: true,
			},
			"arguments": {
				Type:     "object",
				Required: true,
			},
		},
	}
}
