package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewFixtureGenerator(t *testing.T) {
	fg := NewFixtureGenerator()
	if fg == nil {
		t.Fatal("NewFixtureGenerator returned nil")
	}
}

func TestGenerateForType_SimpleStruct(t *testing.T) {
	fg := NewFixtureGenerator()

	typeDef := `type User struct {
	ID        string
	Name      string
	Age       int
	Active    bool
	CreatedAt time.Time
}`

	fixture := fg.GenerateForType(typeDef)

	if fixture.TypeName != "User" {
		t.Errorf("expected TypeName 'User', got %q", fixture.TypeName)
	}

	if fixture.Fields["ID"] != "test_value" {
		t.Errorf("expected ID to be 'test_value', got %v", fixture.Fields["ID"])
	}
	if fixture.Fields["Name"] != "test_value" {
		t.Errorf("expected Name to be 'test_value', got %v", fixture.Fields["Name"])
	}
	if fixture.Fields["Age"] != 42 {
		t.Errorf("expected Age to be 42, got %v", fixture.Fields["Age"])
	}
	if fixture.Fields["Active"] != true {
		t.Errorf("expected Active to be true, got %v", fixture.Fields["Active"])
	}
	if _, ok := fixture.Fields["CreatedAt"].(time.Time); !ok {
		t.Errorf("expected CreatedAt to be time.Time, got %T", fixture.Fields["CreatedAt"])
	}
}

func TestGenerateForType_WithPointers(t *testing.T) {
	fg := NewFixtureGenerator()

	typeDef := `type Config struct {
	Host    string
	Port    int
	Debug   *bool
	Timeout *int
}`

	fixture := fg.GenerateForType(typeDef)

	if fixture.TypeName != "Config" {
		t.Errorf("expected TypeName 'Config', got %q", fixture.TypeName)
	}

	if fixture.Fields["Host"] != "test_value" {
		t.Errorf("expected Host default, got %v", fixture.Fields["Host"])
	}
	if fixture.Fields["Port"] != 42 {
		t.Errorf("expected Port default, got %v", fixture.Fields["Port"])
	}
}

func TestGenerateForType_WithSlices(t *testing.T) {
	fg := NewFixtureGenerator()

	typeDef := `type Project struct {
	Name  string
	Tags  []string
	Items []int
}`

	fixture := fg.GenerateForType(typeDef)

	if fixture.TypeName != "Project" {
		t.Errorf("expected TypeName 'Project', got %q", fixture.TypeName)
	}

	tags, ok := fixture.Fields["Tags"].([]interface{})
	if !ok {
		t.Fatalf("expected Tags to be []interface{}, got %T", fixture.Fields["Tags"])
	}
	if len(tags) != 1 {
		t.Errorf("expected single element slice for Tags, got %d elements", len(tags))
	}
	if tags[0] != "test_value" {
		t.Errorf("expected Tags[0] to be 'test_value', got %v", tags[0])
	}
}

func TestGenerateForType_Variants(t *testing.T) {
	fg := NewFixtureGenerator()

	typeDef := `type Item struct {
	ID    string
	Value int
	Tags  []string
}`

	fixture := fg.GenerateForType(typeDef)

	if len(fixture.Variants) != 3 {
		t.Fatalf("expected 3 variants, got %d", len(fixture.Variants))
	}

	// Check variant names
	names := make(map[string]bool)
	for _, v := range fixture.Variants {
		names[v.Name] = true
	}
	if !names["minimal"] {
		t.Error("missing 'minimal' variant")
	}
	if !names["full"] {
		t.Error("missing 'full' variant")
	}
	if !names["edge"] {
		t.Error("missing 'edge' variant")
	}

	// Minimal should only have required fields (non-pointer, non-slice)
	var minimal FixtureVariant
	for _, v := range fixture.Variants {
		if v.Name == "minimal" {
			minimal = v
			break
		}
	}
	// ID and Value are required; Tags (slice) is not
	if _, ok := minimal.Values["ID"]; !ok {
		t.Error("minimal variant should include required field ID")
	}
	if _, ok := minimal.Values["Value"]; !ok {
		t.Error("minimal variant should include required field Value")
	}

	// Edge variant should have zero values
	var edge FixtureVariant
	for _, v := range fixture.Variants {
		if v.Name == "edge" {
			edge = v
			break
		}
	}
	if edge.Values["ID"] != "" {
		t.Errorf("edge ID should be empty string, got %v", edge.Values["ID"])
	}
	if edge.Values["Value"] != 0 {
		t.Errorf("edge Value should be 0, got %v", edge.Values["Value"])
	}
}

func TestGenerateGoCode(t *testing.T) {
	fg := NewFixtureGenerator()

	typeDef := `type User struct {
	ID    string
	Name  string
	Email string
}`

	fixture := fg.GenerateForType(typeDef)
	code := fg.GenerateGoCode(fixture)

	if !strings.Contains(code, "func TestFixture_User() *User") {
		t.Error("expected function signature in generated code")
	}
	if !strings.Contains(code, "return &User{") {
		t.Error("expected struct initialization in generated code")
	}
	if !strings.Contains(code, `"test_value"`) {
		t.Error("expected string literal in generated code")
	}
}

func TestGenerateGoCode_WithDifferentTypes(t *testing.T) {
	fg := NewFixtureGenerator()

	typeDef := `type Config struct {
	Host    string
	Port    int
	Debug   bool
	Rate    float64
}`

	fixture := fg.GenerateForType(typeDef)
	code := fg.GenerateGoCode(fixture)

	if !strings.Contains(code, "func TestFixture_Config() *Config") {
		t.Error("expected function signature")
	}
	if !strings.Contains(code, "42") {
		t.Error("expected int literal")
	}
	if !strings.Contains(code, "true") {
		t.Error("expected bool literal")
	}
	if !strings.Contains(code, "3.14") {
		t.Error("expected float literal")
	}
}

func TestGenerateTableTestData(t *testing.T) {
	fg := NewFixtureGenerator()

	sig := "func ParseURL(input string) (string, error)"
	code := fg.GenerateTableTestData(sig)

	if !strings.Contains(code, "func TestParseURL(t *testing.T)") {
		t.Error("expected test function declaration")
	}
	if !strings.Contains(code, "tests := []struct {") {
		t.Error("expected test table struct")
	}
	if !strings.Contains(code, "name string") {
		t.Error("expected name field in test struct")
	}
	if !strings.Contains(code, "input string") {
		t.Error("expected input field in test struct")
	}
	if !strings.Contains(code, "wantErr bool") {
		t.Error("expected wantErr field in test struct")
	}
	if !strings.Contains(code, "t.Run(tt.name") {
		t.Error("expected subtest with t.Run")
	}
	if !strings.Contains(code, "ParseURL(tt.input)") {
		t.Error("expected function call with test input")
	}
	if !strings.Contains(code, `"valid input"`) {
		t.Error("expected valid input test case")
	}
	if !strings.Contains(code, `"empty input"`) {
		t.Error("expected empty input test case")
	}
}

func TestGenerateTableTestData_MultipleParams(t *testing.T) {
	fg := NewFixtureGenerator()

	sig := "func Add(a int, b int) int"
	code := fg.GenerateTableTestData(sig)

	if !strings.Contains(code, "func TestAdd(t *testing.T)") {
		t.Error("expected test function declaration")
	}
	if !strings.Contains(code, "a int") {
		t.Error("expected field a")
	}
	if !strings.Contains(code, "b int") {
		t.Error("expected field b")
	}
	if !strings.Contains(code, "Add(tt.a, tt.b)") {
		t.Error("expected function call with both args")
	}
}

func TestGenerateTableTestData_NoReturn(t *testing.T) {
	fg := NewFixtureGenerator()

	sig := "func LogMessage(msg string)"
	code := fg.GenerateTableTestData(sig)

	if !strings.Contains(code, "func TestLogMessage(t *testing.T)") {
		t.Error("expected test function declaration")
	}
	if !strings.Contains(code, "LogMessage(tt.msg)") {
		t.Error("expected function call")
	}
}

func TestGenerateEdgeCases(t *testing.T) {
	fg := NewFixtureGenerator()

	typeDef := `type Record struct {
	ID      string
	Count   int
	Score   float64
	Active  bool
	Data    *string
}`

	variants := fg.GenerateEdgeCases(typeDef)

	if len(variants) != 5 {
		t.Fatalf("expected 5 edge case variants, got %d", len(variants))
	}

	// Check variant names
	names := make(map[string]bool)
	for _, v := range variants {
		names[v.Name] = true
	}

	expectedNames := []string{"nil_values", "empty_strings", "zero_integers", "max_values", "unicode_strings"}
	for _, name := range expectedNames {
		if !names[name] {
			t.Errorf("missing edge case variant %q", name)
		}
	}
}

func TestGenerateEdgeCases_NilValues(t *testing.T) {
	fg := NewFixtureGenerator()

	typeDef := `type Item struct {
	Name   string
	Ref    *string
}`

	variants := fg.GenerateEdgeCases(typeDef)

	var nilVariant FixtureVariant
	for _, v := range variants {
		if v.Name == "nil_values" {
			nilVariant = v
			break
		}
	}

	if nilVariant.Values["Ref"] != nil {
		t.Errorf("expected nil for pointer field Ref, got %v", nilVariant.Values["Ref"])
	}
	// Non-pointer string should still have its default
	if nilVariant.Values["Name"] != "test_value" {
		t.Errorf("expected 'test_value' for Name, got %v", nilVariant.Values["Name"])
	}
}

func TestGenerateEdgeCases_EmptyStrings(t *testing.T) {
	fg := NewFixtureGenerator()

	typeDef := `type User struct {
	Name  string
	Email string
	Age   int
}`

	variants := fg.GenerateEdgeCases(typeDef)

	var emptyStrings FixtureVariant
	for _, v := range variants {
		if v.Name == "empty_strings" {
			emptyStrings = v
			break
		}
	}

	if emptyStrings.Values["Name"] != "" {
		t.Errorf("expected empty string for Name, got %v", emptyStrings.Values["Name"])
	}
	if emptyStrings.Values["Email"] != "" {
		t.Errorf("expected empty string for Email, got %v", emptyStrings.Values["Email"])
	}
	if emptyStrings.Values["Age"] != 42 {
		t.Errorf("expected default 42 for Age, got %v", emptyStrings.Values["Age"])
	}
}

func TestGenerateEdgeCases_MaxValues(t *testing.T) {
	fg := NewFixtureGenerator()

	typeDef := `type Limits struct {
	Count  int
	Size   int32
	Name   string
}`

	variants := fg.GenerateEdgeCases(typeDef)

	var maxVariant FixtureVariant
	for _, v := range variants {
		if v.Name == "max_values" {
			maxVariant = v
			break
		}
	}

	if maxVariant.Values["Count"] != int(9223372036854775807) {
		t.Errorf("expected max int for Count, got %v", maxVariant.Values["Count"])
	}
	if maxVariant.Values["Size"] != int(2147483647) {
		t.Errorf("expected max int32 for Size, got %v", maxVariant.Values["Size"])
	}
	// String should be long
	nameStr, ok := maxVariant.Values["Name"].(string)
	if !ok {
		t.Fatalf("expected string for Name, got %T", maxVariant.Values["Name"])
	}
	if len(nameStr) != 1000 {
		t.Errorf("expected 1000-char string for Name, got %d chars", len(nameStr))
	}
}

func TestGenerateEdgeCases_UnicodeStrings(t *testing.T) {
	fg := NewFixtureGenerator()

	typeDef := `type Profile struct {
	Username string
	Bio      string
	Age      int
}`

	variants := fg.GenerateEdgeCases(typeDef)

	var unicodeVariant FixtureVariant
	for _, v := range variants {
		if v.Name == "unicode_strings" {
			unicodeVariant = v
			break
		}
	}

	usernameStr, ok := unicodeVariant.Values["Username"].(string)
	if !ok {
		t.Fatalf("expected string for Username, got %T", unicodeVariant.Values["Username"])
	}
	if !strings.Contains(usernameStr, "世界") {
		t.Error("expected unicode characters in Username")
	}
	if unicodeVariant.Values["Age"] != 42 {
		t.Errorf("expected default 42 for Age, got %v", unicodeVariant.Values["Age"])
	}
}

func TestFormatFixture(t *testing.T) {
	fg := NewFixtureGenerator()

	typeDef := `type Item struct {
	ID   string
	Name string
}`

	fixture := fg.GenerateForType(typeDef)
	formatted := fg.FormatFixture(fixture)

	if !strings.Contains(formatted, "Fixture: Item") {
		t.Error("expected fixture type name in formatted output")
	}
	if !strings.Contains(formatted, "Default Fields:") {
		t.Error("expected 'Default Fields:' in formatted output")
	}
	if !strings.Contains(formatted, "Variants:") {
		t.Error("expected 'Variants:' in formatted output")
	}
	if !strings.Contains(formatted, "[minimal]") {
		t.Error("expected minimal variant in formatted output")
	}
	if !strings.Contains(formatted, "[full]") {
		t.Error("expected full variant in formatted output")
	}
	if !strings.Contains(formatted, "[edge]") {
		t.Error("expected edge variant in formatted output")
	}
}

func TestFixtureGeneratorTool_Interface(t *testing.T) {
	tool := NewFixtureGeneratorTool()

	if tool.Name() != "TestFixtures" {
		t.Errorf("expected Name() = 'TestFixtures', got %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
	params := tool.Parameters()
	if params == nil {
		t.Fatal("expected non-nil parameters")
	}
}

func TestFixtureGeneratorTool_Generate(t *testing.T) {
	tool := NewFixtureGeneratorTool()
	ctx := context.Background()

	input := fixtureGenInput{
		Action:  "generate",
		TypeDef: "type User struct {\n\tID string\n\tName string\n}",
	}
	data, _ := json.Marshal(input)
	output, err := tool.Execute(ctx, data)
	if err != nil {
		t.Fatalf("Execute generate: %v", err)
	}
	if !strings.Contains(output, "Fixture: User") {
		t.Error("expected fixture output to contain type name")
	}
}

func TestFixtureGeneratorTool_TableTest(t *testing.T) {
	tool := NewFixtureGeneratorTool()
	ctx := context.Background()

	input := fixtureGenInput{
		Action:        "table_test",
		FuncSignature: "func Validate(input string) error",
	}
	data, _ := json.Marshal(input)
	output, err := tool.Execute(ctx, data)
	if err != nil {
		t.Fatalf("Execute table_test: %v", err)
	}
	if !strings.Contains(output, "func TestValidate") {
		t.Error("expected test function in output")
	}
	if !strings.Contains(output, "wantErr") {
		t.Error("expected wantErr in output")
	}
}

func TestFixtureGeneratorTool_EdgeCases(t *testing.T) {
	tool := NewFixtureGeneratorTool()
	ctx := context.Background()

	input := fixtureGenInput{
		Action:  "edge_cases",
		TypeDef: "type Data struct {\n\tValue int\n\tLabel string\n}",
	}
	data, _ := json.Marshal(input)
	output, err := tool.Execute(ctx, data)
	if err != nil {
		t.Fatalf("Execute edge_cases: %v", err)
	}
	if !strings.Contains(output, "Edge Cases:") {
		t.Error("expected 'Edge Cases:' header")
	}
	if !strings.Contains(output, "nil_values") {
		t.Error("expected nil_values variant")
	}
	if !strings.Contains(output, "unicode_strings") {
		t.Error("expected unicode_strings variant")
	}
}

func TestFixtureGeneratorTool_GoCode(t *testing.T) {
	tool := NewFixtureGeneratorTool()
	ctx := context.Background()

	input := fixtureGenInput{
		Action:  "go_code",
		TypeDef: "type Config struct {\n\tHost string\n\tPort int\n}",
	}
	data, _ := json.Marshal(input)
	output, err := tool.Execute(ctx, data)
	if err != nil {
		t.Fatalf("Execute go_code: %v", err)
	}
	if !strings.Contains(output, "func TestFixture_Config() *Config") {
		t.Error("expected Go code function declaration")
	}
	if !strings.Contains(output, "return &Config{") {
		t.Error("expected struct initialization")
	}
}

func TestFixtureGeneratorTool_Format(t *testing.T) {
	tool := NewFixtureGeneratorTool()
	ctx := context.Background()

	input := fixtureGenInput{
		Action:  "format",
		TypeDef: "type Item struct {\n\tID string\n}",
	}
	data, _ := json.Marshal(input)
	output, err := tool.Execute(ctx, data)
	if err != nil {
		t.Fatalf("Execute format: %v", err)
	}
	if !strings.Contains(output, "Fixture: Item") {
		t.Error("expected formatted output")
	}
}

func TestFixtureGeneratorTool_Errors(t *testing.T) {
	tool := NewFixtureGeneratorTool()
	ctx := context.Background()

	tests := []struct {
		name  string
		input fixtureGenInput
		errIn string
	}{
		{
			name:  "unknown action",
			input: fixtureGenInput{Action: "unknown"},
			errIn: "unknown action",
		},
		{
			name:  "generate without type_def",
			input: fixtureGenInput{Action: "generate"},
			errIn: "type_def is required",
		},
		{
			name:  "table_test without func_signature",
			input: fixtureGenInput{Action: "table_test"},
			errIn: "func_signature is required",
		},
		{
			name:  "edge_cases without type_def",
			input: fixtureGenInput{Action: "edge_cases"},
			errIn: "type_def is required",
		},
		{
			name:  "go_code without type_def",
			input: fixtureGenInput{Action: "go_code"},
			errIn: "type_def is required",
		},
		{
			name:  "format without type_def",
			input: fixtureGenInput{Action: "format"},
			errIn: "type_def is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, _ := json.Marshal(tt.input)
			_, err := tool.Execute(ctx, data)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.errIn) {
				t.Errorf("expected error containing %q, got: %v", tt.errIn, err)
			}
		})
	}
}

func TestFixtureGeneratorTool_InvalidJSON(t *testing.T) {
	tool := NewFixtureGeneratorTool()
	ctx := context.Background()

	_, err := tool.Execute(ctx, json.RawMessage(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGenerateForType_WithStructTags(t *testing.T) {
	fg := NewFixtureGenerator()

	typeDef := `type APIResponse struct {
	ID     string ` + "`json:\"id\"`" + `
	Status string ` + "`json:\"status\"`" + `
	Count  int    ` + "`json:\"count\"`" + `
}`

	fixture := fg.GenerateForType(typeDef)

	if fixture.TypeName != "APIResponse" {
		t.Errorf("expected TypeName 'APIResponse', got %q", fixture.TypeName)
	}
	if fixture.Fields["ID"] != "test_value" {
		t.Errorf("expected ID 'test_value', got %v", fixture.Fields["ID"])
	}
	if fixture.Fields["Count"] != 42 {
		t.Errorf("expected Count 42, got %v", fixture.Fields["Count"])
	}
}

func TestGenerateForType_MapField(t *testing.T) {
	fg := NewFixtureGenerator()

	typeDef := `type Settings struct {
	Name   string
	Values map[string]interface{}
}`

	fixture := fg.GenerateForType(typeDef)

	if fixture.TypeName != "Settings" {
		t.Errorf("expected TypeName 'Settings', got %q", fixture.TypeName)
	}
	values, ok := fixture.Fields["Values"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map for Values, got %T", fixture.Fields["Values"])
	}
	if values["key"] != "value" {
		t.Errorf("expected map default value, got %v", values)
	}
}

func TestGenerateForType_FloatField(t *testing.T) {
	fg := NewFixtureGenerator()

	typeDef := `type Metrics struct {
	Name  string
	Score float64
}`

	fixture := fg.GenerateForType(typeDef)

	if fixture.Fields["Score"] != 3.14 {
		t.Errorf("expected Score 3.14, got %v", fixture.Fields["Score"])
	}
}

func TestConcurrentFixtureGeneration(t *testing.T) {
	fg := NewFixtureGenerator()

	typeDef := `type User struct {
	ID   string
	Name string
	Age  int
}`

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			fixture := fg.GenerateForType(typeDef)
			if fixture.TypeName != "User" {
				t.Errorf("expected 'User', got %q", fixture.TypeName)
			}
			_ = fg.GenerateGoCode(fixture)
			_ = fg.FormatFixture(fixture)
			_ = fg.GenerateEdgeCases(typeDef)
			_ = fg.GenerateTableTestData("func Process(input string) (string, error)")
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestGenerateGoCode_NilValue(t *testing.T) {
	fg := NewFixtureGenerator()

	fixture := &Fixture{
		TypeName: "Container",
		Fields: map[string]interface{}{
			"Data": nil,
			"Name": "test",
		},
	}

	code := fg.GenerateGoCode(fixture)

	if !strings.Contains(code, "nil") {
		t.Error("expected nil in generated code")
	}
	if !strings.Contains(code, `"test"`) {
		t.Error("expected string literal in generated code")
	}
}

func TestGenerateGoCode_TimeField(t *testing.T) {
	fg := NewFixtureGenerator()

	fixture := &Fixture{
		TypeName: "Event",
		Fields: map[string]interface{}{
			"Name":      "test",
			"CreatedAt": time.Now(),
		},
	}

	code := fg.GenerateGoCode(fixture)

	if !strings.Contains(code, "time.Now()") {
		t.Error("expected time.Now() in generated code")
	}
}

func TestGenerateGoCode_ZeroTime(t *testing.T) {
	fg := NewFixtureGenerator()

	fixture := &Fixture{
		TypeName: "Event",
		Fields: map[string]interface{}{
			"CreatedAt": time.Time{},
		},
	}

	code := fg.GenerateGoCode(fixture)

	if !strings.Contains(code, "time.Time{}") {
		t.Error("expected time.Time{} in generated code for zero time")
	}
}

func TestFixtureGeneratorTool_ImplementsInterface(t *testing.T) {
	// Compile-time check that FixtureGeneratorTool implements Tool
	var _ Tool = (*FixtureGeneratorTool)(nil)
}

func TestGenerateTableTestData_ErrorOnlyReturn(t *testing.T) {
	fg := NewFixtureGenerator()

	sig := "func Save(name string) error"
	code := fg.GenerateTableTestData(sig)

	if !strings.Contains(code, "func TestSave(t *testing.T)") {
		t.Error("expected test function declaration")
	}
	if !strings.Contains(code, "wantErr bool") {
		t.Error("expected wantErr field")
	}
	if !strings.Contains(code, "Save(tt.name)") {
		t.Error("expected function call")
	}
}

func TestGenerateForType_Comments(t *testing.T) {
	fg := NewFixtureGenerator()

	typeDef := `type Item struct {
	// ID is the unique identifier
	ID   string
	// Name is the display name
	Name string
}`

	fixture := fg.GenerateForType(typeDef)

	if fixture.TypeName != "Item" {
		t.Errorf("expected TypeName 'Item', got %q", fixture.TypeName)
	}
	if len(fixture.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(fixture.Fields))
	}
}

func TestGeneratorAccessor(t *testing.T) {
	tool := NewFixtureGeneratorTool()
	gen := tool.Generator()
	if gen == nil {
		t.Fatal("Generator() returned nil")
	}
}
