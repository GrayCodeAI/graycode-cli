package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Fixture represents generated test data for a type.
type Fixture struct {
	TypeName string
	Fields   map[string]interface{}
	Variants []FixtureVariant
}

// FixtureVariant represents a named variant of a fixture with specific values.
type FixtureVariant struct {
	Name        string
	Values      map[string]interface{}
	Description string
}

// FixtureGenerator generates test fixtures from type definitions.
type FixtureGenerator struct {
	mu sync.Mutex
}

// NewFixtureGenerator creates a new FixtureGenerator.
func NewFixtureGenerator() *FixtureGenerator {
	return &FixtureGenerator{}
}

// fieldInfo holds parsed field information from a type definition.
type fieldInfo struct {
	Name     string
	Type     string
	Required bool
}

// GenerateForType parses a type definition and generates a Fixture with default values and variants.
func (fg *FixtureGenerator) GenerateForType(typeDef string) *Fixture {
	fg.mu.Lock()
	defer fg.mu.Unlock()

	typeName, fields := fg.parseTypeDef(typeDef)

	fixture := &Fixture{
		TypeName: typeName,
		Fields:   make(map[string]interface{}),
	}

	// Generate default values for each field
	for _, f := range fields {
		fixture.Fields[f.Name] = fg.defaultValue(f.Type)
	}

	// Create variants
	fixture.Variants = fg.generateVariants(fields)

	return fixture
}

// parseTypeDef extracts the type name and fields from a Go type definition string.
func (fg *FixtureGenerator) parseTypeDef(typeDef string) (string, []fieldInfo) {
	var typeName string
	var fields []fieldInfo

	lines := strings.Split(typeDef, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Parse type declaration line
		if strings.HasPrefix(line, "type ") && strings.Contains(line, "struct") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				typeName = parts[1]
			}
			continue
		}

		// Skip braces and empty lines
		if line == "{" || line == "}" || line == "" {
			continue
		}

		// Skip comments
		if strings.HasPrefix(line, "//") {
			continue
		}

		// Parse field: Name Type `tags`
		// Remove struct tags
		if idx := strings.Index(line, "`"); idx > 0 {
			line = strings.TrimSpace(line[:idx])
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			fieldName := parts[0]
			fieldType := parts[1]

			// Determine if required (pointer types and slices are optional)
			required := !strings.HasPrefix(fieldType, "*") && !strings.HasPrefix(fieldType, "[]")

			fields = append(fields, fieldInfo{
				Name:     fieldName,
				Type:     fieldType,
				Required: required,
			})
		}
	}

	if typeName == "" {
		typeName = "Unknown"
	}

	return typeName, fields
}

// defaultValue returns a sensible default value for the given Go type.
func (fg *FixtureGenerator) defaultValue(goType string) interface{} {
	// Strip pointer prefix
	cleanType := strings.TrimPrefix(goType, "*")

	switch cleanType {
	case "string":
		return "test_value"
	case "int", "int8", "int16", "int32", "int64":
		return 42
	case "uint", "uint8", "uint16", "uint32", "uint64":
		return 42
	case "float32", "float64":
		return 3.14
	case "bool":
		return true
	case "time.Time":
		return time.Now()
	case "error":
		return nil
	}

	// Handle slices
	if strings.HasPrefix(cleanType, "[]") {
		elemType := strings.TrimPrefix(cleanType, "[]")
		elemVal := fg.defaultValue(elemType)
		return []interface{}{elemVal}
	}

	// Handle maps
	if strings.HasPrefix(cleanType, "map[") {
		return map[string]interface{}{"key": "value"}
	}

	// Default for unknown types (struct references, interfaces, etc.)
	return nil
}

// generateVariants creates the standard variants: minimal, full, and edge.
func (fg *FixtureGenerator) generateVariants(fields []fieldInfo) []FixtureVariant {
	minimal := FixtureVariant{
		Name:        "minimal",
		Values:      make(map[string]interface{}),
		Description: "Only required fields populated",
	}
	full := FixtureVariant{
		Name:        "full",
		Values:      make(map[string]interface{}),
		Description: "All fields populated with sensible defaults",
	}
	edge := FixtureVariant{
		Name:        "edge",
		Values:      make(map[string]interface{}),
		Description: "Edge case values (empty/zero values)",
	}

	for _, f := range fields {
		// Full variant gets all fields
		full.Values[f.Name] = fg.defaultValue(f.Type)

		// Minimal variant gets only required fields
		if f.Required {
			minimal.Values[f.Name] = fg.defaultValue(f.Type)
		}

		// Edge variant gets zero/empty values
		edge.Values[f.Name] = fg.zeroValue(f.Type)
	}

	return []FixtureVariant{minimal, full, edge}
}

// zeroValue returns the zero/empty value for a Go type.
func (fg *FixtureGenerator) zeroValue(goType string) interface{} {
	cleanType := strings.TrimPrefix(goType, "*")

	switch cleanType {
	case "string":
		return ""
	case "int", "int8", "int16", "int32", "int64":
		return 0
	case "uint", "uint8", "uint16", "uint32", "uint64":
		return 0
	case "float32", "float64":
		return 0.0
	case "bool":
		return false
	case "time.Time":
		return time.Time{}
	}

	if strings.HasPrefix(cleanType, "[]") {
		return []interface{}{}
	}

	if strings.HasPrefix(cleanType, "map[") {
		return map[string]interface{}{}
	}

	return nil
}

// GenerateGoCode produces Go code that creates the fixture as a function.
func (fg *FixtureGenerator) GenerateGoCode(fixture *Fixture) string {
	fg.mu.Lock()
	defer fg.mu.Unlock()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("func TestFixture_%s() *%s {\n", fixture.TypeName, fixture.TypeName))
	sb.WriteString(fmt.Sprintf("\treturn &%s{\n", fixture.TypeName))

	for name, value := range fixture.Fields {
		sb.WriteString(fmt.Sprintf("\t\t%s: %s,\n", name, fg.goLiteral(value)))
	}

	sb.WriteString("\t}\n")
	sb.WriteString("}\n")

	return sb.String()
}

// goLiteral converts a Go value to its literal representation in source code.
func (fg *FixtureGenerator) goLiteral(value interface{}) string {
	if value == nil {
		return "nil"
	}

	switch v := value.(type) {
	case string:
		return fmt.Sprintf("%q", v)
	case int:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%g", v)
	case bool:
		return fmt.Sprintf("%t", v)
	case time.Time:
		if v.IsZero() {
			return "time.Time{}"
		}
		return "time.Now()"
	case []interface{}:
		if len(v) == 0 {
			return "nil"
		}
		// Single element slice
		elemStr := fg.goLiteral(v[0])
		return fmt.Sprintf("[]interface{}{%s}", elemStr)
	case map[string]interface{}:
		if len(v) == 0 {
			return "nil"
		}
		var pairs []string
		for k, val := range v {
			pairs = append(pairs, fmt.Sprintf("%q: %s", k, fg.goLiteral(val)))
		}
		return fmt.Sprintf("map[string]interface{}{%s}", strings.Join(pairs, ", "))
	default:
		return fmt.Sprintf("%v", v)
	}
}

// GenerateTableTestData generates a table-driven test structure for a function signature.
func (fg *FixtureGenerator) GenerateTableTestData(funcSignature string) string {
	fg.mu.Lock()
	defer fg.mu.Unlock()

	funcName, params, returns := fg.parseFuncSignature(funcSignature)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("func Test%s(t *testing.T) {\n", funcName))
	sb.WriteString("\ttests := []struct {\n")
	sb.WriteString("\t\tname string\n")

	// Input fields
	for _, p := range params {
		sb.WriteString(fmt.Sprintf("\t\t%s %s\n", p.Name, p.Type))
	}

	// Output fields
	for i, r := range returns {
		if r.Type == "error" {
			sb.WriteString("\t\twantErr bool\n")
		} else {
			name := r.Name
			if name == "" {
				name = fmt.Sprintf("want%d", i)
				if i == 0 {
					name = "want"
				}
			}
			sb.WriteString(fmt.Sprintf("\t\t%s %s\n", name, r.Type))
		}
	}

	sb.WriteString("\t}{\n")

	// Generate test cases
	sb.WriteString("\t\t{\n")
	sb.WriteString("\t\t\tname: \"valid input\",\n")
	for _, p := range params {
		sb.WriteString(fmt.Sprintf("\t\t\t%s: %s,\n", p.Name, fg.goLiteral(fg.defaultValue(p.Type))))
	}
	for i, r := range returns {
		if r.Type == "error" {
			sb.WriteString("\t\t\twantErr: false,\n")
		} else {
			name := r.Name
			if name == "" {
				name = fmt.Sprintf("want%d", i)
				if i == 0 {
					name = "want"
				}
			}
			sb.WriteString(fmt.Sprintf("\t\t\t%s: %s,\n", name, fg.goLiteral(fg.defaultValue(r.Type))))
		}
	}
	sb.WriteString("\t\t},\n")

	// Edge case
	sb.WriteString("\t\t{\n")
	sb.WriteString("\t\t\tname: \"empty input\",\n")
	for _, p := range params {
		sb.WriteString(fmt.Sprintf("\t\t\t%s: %s,\n", p.Name, fg.goLiteral(fg.zeroValue(p.Type))))
	}
	for _, r := range returns {
		if r.Type == "error" {
			sb.WriteString("\t\t\twantErr: true,\n")
		}
	}
	sb.WriteString("\t\t},\n")

	sb.WriteString("\t}\n\n")

	// Test loop
	sb.WriteString("\tfor _, tt := range tests {\n")
	sb.WriteString("\t\tt.Run(tt.name, func(t *testing.T) {\n")

	// Build function call
	var argNames []string
	for _, p := range params {
		argNames = append(argNames, "tt."+p.Name)
	}

	var returnVars []string
	hasErr := false
	for i, r := range returns {
		if r.Type == "error" {
			returnVars = append(returnVars, "err")
			hasErr = true
		} else {
			name := fmt.Sprintf("got%d", i)
			if i == 0 {
				name = "got"
			}
			returnVars = append(returnVars, name)
		}
	}

	if len(returnVars) > 0 {
		sb.WriteString(fmt.Sprintf("\t\t\t%s := %s(%s)\n", strings.Join(returnVars, ", "), funcName, strings.Join(argNames, ", ")))
	} else {
		sb.WriteString(fmt.Sprintf("\t\t\t%s(%s)\n", funcName, strings.Join(argNames, ", ")))
	}

	if hasErr {
		sb.WriteString("\t\t\tif (err != nil) != tt.wantErr {\n")
		sb.WriteString(fmt.Sprintf("\t\t\t\tt.Errorf(\"%s() error = %%v, wantErr %%v\", err, tt.wantErr)\n", funcName))
		sb.WriteString("\t\t\t\treturn\n")
		sb.WriteString("\t\t\t}\n")
	}

	sb.WriteString("\t\t})\n")
	sb.WriteString("\t}\n")
	sb.WriteString("}\n")

	return sb.String()
}

// parseFuncSignature parses a function signature like "func Foo(x string, y int) (string, error)".
func (fg *FixtureGenerator) parseFuncSignature(sig string) (string, []fieldInfo, []fieldInfo) {
	sig = strings.TrimSpace(sig)
	sig = strings.TrimPrefix(sig, "func ")

	// Extract function name
	parenIdx := strings.Index(sig, "(")
	if parenIdx < 0 {
		return sig, nil, nil
	}
	funcName := strings.TrimSpace(sig[:parenIdx])

	// Remove receiver if present (e.g., "(s *Store) Create")
	if strings.HasPrefix(funcName, "(") {
		closeIdx := strings.Index(funcName, ")")
		if closeIdx >= 0 {
			remaining := strings.TrimSpace(funcName[closeIdx+1:])
			funcName = remaining
			// Re-find the opening paren after the function name
			rest := sig[parenIdx+1:]
			sig = funcName + "(" + rest
			parenIdx = strings.Index(sig, "(")
			if parenIdx < 0 {
				return funcName, nil, nil
			}
		}
	}

	rest := sig[parenIdx:]

	// Find matching close paren for params
	params, afterParams := fg.extractParenContent(rest)
	paramFields := fg.parseParamList(params)

	// Parse return types
	var returnFields []fieldInfo
	afterParams = strings.TrimSpace(afterParams)
	if strings.HasPrefix(afterParams, "(") {
		returns, _ := fg.extractParenContent(afterParams)
		returnFields = fg.parseParamList(returns)
	} else if afterParams != "" {
		// Single return value without parens
		retType := strings.TrimSpace(afterParams)
		if retType != "" {
			returnFields = append(returnFields, fieldInfo{Name: "", Type: retType})
		}
	}

	return funcName, paramFields, returnFields
}

// extractParenContent extracts content within parentheses and returns it plus the remainder.
func (fg *FixtureGenerator) extractParenContent(s string) (string, string) {
	if !strings.HasPrefix(s, "(") {
		return "", s
	}

	depth := 0
	for i, ch := range s {
		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
			if depth == 0 {
				return s[1:i], strings.TrimSpace(s[i+1:])
			}
		}
	}
	return s[1:], ""
}

// parseParamList parses a comma-separated param list like "x string, y int".
func (fg *FixtureGenerator) parseParamList(paramStr string) []fieldInfo {
	paramStr = strings.TrimSpace(paramStr)
	if paramStr == "" {
		return nil
	}

	var fields []fieldInfo
	parts := strings.Split(paramStr, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tokens := strings.Fields(part)
		if len(tokens) >= 2 {
			fields = append(fields, fieldInfo{
				Name: tokens[0],
				Type: strings.Join(tokens[1:], " "),
			})
		} else if len(tokens) == 1 {
			// Unnamed parameter (just a type)
			fields = append(fields, fieldInfo{
				Name: "",
				Type: tokens[0],
			})
		}
	}

	return fields
}

// GenerateEdgeCases generates edge case fixture variants for a type definition.
func (fg *FixtureGenerator) GenerateEdgeCases(typeDef string) []FixtureVariant {
	fg.mu.Lock()
	defer fg.mu.Unlock()

	_, fields := fg.parseTypeDef(typeDef)

	var variants []FixtureVariant

	// Nil values variant
	nilVariant := FixtureVariant{
		Name:        "nil_values",
		Values:      make(map[string]interface{}),
		Description: "All pointer and interface fields set to nil",
	}
	for _, f := range fields {
		if strings.HasPrefix(f.Type, "*") || f.Type == "error" || f.Type == "interface{}" {
			nilVariant.Values[f.Name] = nil
		} else {
			nilVariant.Values[f.Name] = fg.defaultValue(f.Type)
		}
	}
	variants = append(variants, nilVariant)

	// Empty strings variant
	emptyStrings := FixtureVariant{
		Name:        "empty_strings",
		Values:      make(map[string]interface{}),
		Description: "All string fields set to empty string",
	}
	for _, f := range fields {
		cleanType := strings.TrimPrefix(f.Type, "*")
		if cleanType == "string" {
			emptyStrings.Values[f.Name] = ""
		} else {
			emptyStrings.Values[f.Name] = fg.defaultValue(f.Type)
		}
	}
	variants = append(variants, emptyStrings)

	// Zero integers variant
	zeroInts := FixtureVariant{
		Name:        "zero_integers",
		Values:      make(map[string]interface{}),
		Description: "All integer fields set to zero",
	}
	for _, f := range fields {
		cleanType := strings.TrimPrefix(f.Type, "*")
		if fg.isIntType(cleanType) {
			zeroInts.Values[f.Name] = 0
		} else {
			zeroInts.Values[f.Name] = fg.defaultValue(f.Type)
		}
	}
	variants = append(variants, zeroInts)

	// Max values variant
	maxValues := FixtureVariant{
		Name:        "max_values",
		Values:      make(map[string]interface{}),
		Description: "Numeric fields set to maximum values",
	}
	for _, f := range fields {
		cleanType := strings.TrimPrefix(f.Type, "*")
		switch cleanType {
		case "int", "int64":
			maxValues.Values[f.Name] = int(9223372036854775807)
		case "int32":
			maxValues.Values[f.Name] = int(2147483647)
		case "int16":
			maxValues.Values[f.Name] = int(32767)
		case "int8":
			maxValues.Values[f.Name] = int(127)
		case "uint", "uint64":
			maxValues.Values[f.Name] = uint64(18446744073709551615)
		case "uint32":
			maxValues.Values[f.Name] = uint64(4294967295)
		case "uint16":
			maxValues.Values[f.Name] = uint64(65535)
		case "uint8":
			maxValues.Values[f.Name] = uint64(255)
		case "float32":
			maxValues.Values[f.Name] = float64(3.4028235e+38)
		case "float64":
			maxValues.Values[f.Name] = float64(1.7976931348623157e+308)
		case "string":
			maxValues.Values[f.Name] = strings.Repeat("a", 1000)
		default:
			maxValues.Values[f.Name] = fg.defaultValue(f.Type)
		}
	}
	variants = append(variants, maxValues)

	// Unicode strings variant
	unicodeStrings := FixtureVariant{
		Name:        "unicode_strings",
		Values:      make(map[string]interface{}),
		Description: "String fields with unicode characters",
	}
	for _, f := range fields {
		cleanType := strings.TrimPrefix(f.Type, "*")
		if cleanType == "string" {
			unicodeStrings.Values[f.Name] = "世界こんにちは🌍éèê"
		} else {
			unicodeStrings.Values[f.Name] = fg.defaultValue(f.Type)
		}
	}
	variants = append(variants, unicodeStrings)

	return variants
}

// isIntType checks if a type string is an integer type.
func (fg *FixtureGenerator) isIntType(t string) bool {
	switch t {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64":
		return true
	}
	return false
}

// FormatFixture formats a fixture into a human-readable string representation.
func (fg *FixtureGenerator) FormatFixture(fixture *Fixture) string {
	fg.mu.Lock()
	defer fg.mu.Unlock()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Fixture: %s\n", fixture.TypeName))
	sb.WriteString("Default Fields:\n")
	for name, value := range fixture.Fields {
		sb.WriteString(fmt.Sprintf("  %s: %v\n", name, value))
	}

	if len(fixture.Variants) > 0 {
		sb.WriteString("\nVariants:\n")
		for _, v := range fixture.Variants {
			sb.WriteString(fmt.Sprintf("  [%s] %s\n", v.Name, v.Description))
			for name, value := range v.Values {
				sb.WriteString(fmt.Sprintf("    %s: %v\n", name, value))
			}
		}
	}

	return sb.String()
}

// FixtureGeneratorTool implements the Tool interface for test fixture generation.
type FixtureGeneratorTool struct {
	generator *FixtureGenerator
}

// NewFixtureGeneratorTool creates a new FixtureGeneratorTool.
func NewFixtureGeneratorTool() *FixtureGeneratorTool {
	return &FixtureGeneratorTool{generator: NewFixtureGenerator()}
}

func (t *FixtureGeneratorTool) Name() string {
	return "TestFixtures"
}

func (t *FixtureGeneratorTool) Description() string {
	return "Generate test fixtures, table test data, and edge cases from Go type definitions and function signatures."
}

func (t *FixtureGeneratorTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"generate", "table_test", "edge_cases", "go_code", "format"},
				"description": "Action to perform: generate fixture, generate table test, generate edge cases, produce Go code, or format fixture",
			},
			"type_def": map[string]interface{}{
				"type":        "string",
				"description": "Go type definition (e.g., 'type User struct { ID string; Name string }')",
			},
			"func_signature": map[string]interface{}{
				"type":        "string",
				"description": "Go function signature for table test generation",
			},
		},
		"required": []string{"action"},
	}
}

type fixtureGenInput struct {
	Action        string `json:"action"`
	TypeDef       string `json:"type_def"`
	FuncSignature string `json:"func_signature"`
}

func (t *FixtureGeneratorTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in fixtureGenInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	switch in.Action {
	case "generate":
		if in.TypeDef == "" {
			return "", fmt.Errorf("type_def is required for generate action")
		}
		fixture := t.generator.GenerateForType(in.TypeDef)
		return t.generator.FormatFixture(fixture), nil

	case "table_test":
		if in.FuncSignature == "" {
			return "", fmt.Errorf("func_signature is required for table_test action")
		}
		return t.generator.GenerateTableTestData(in.FuncSignature), nil

	case "edge_cases":
		if in.TypeDef == "" {
			return "", fmt.Errorf("type_def is required for edge_cases action")
		}
		variants := t.generator.GenerateEdgeCases(in.TypeDef)
		var sb strings.Builder
		sb.WriteString("Edge Cases:\n")
		for _, v := range variants {
			sb.WriteString(fmt.Sprintf("\n  [%s] %s\n", v.Name, v.Description))
			for name, value := range v.Values {
				sb.WriteString(fmt.Sprintf("    %s: %v\n", name, value))
			}
		}
		return sb.String(), nil

	case "go_code":
		if in.TypeDef == "" {
			return "", fmt.Errorf("type_def is required for go_code action")
		}
		fixture := t.generator.GenerateForType(in.TypeDef)
		return t.generator.GenerateGoCode(fixture), nil

	case "format":
		if in.TypeDef == "" {
			return "", fmt.Errorf("type_def is required for format action")
		}
		fixture := t.generator.GenerateForType(in.TypeDef)
		return t.generator.FormatFixture(fixture), nil

	default:
		return "", fmt.Errorf("unknown action: %s (valid: generate, table_test, edge_cases, go_code, format)", in.Action)
	}
}

// Generator returns the underlying FixtureGenerator for direct use.
func (t *FixtureGeneratorTool) Generator() *FixtureGenerator {
	return t.generator
}
