package engine

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"sync"
)

// CodeExplanation holds a structured explanation of a code element.
type CodeExplanation struct {
	File         string
	Symbol       string
	Summary      string
	Sections     []ExplanationSection
	Complexity   string
	Dependencies []string
	UsedBy       []string
}

// ExplanationSection is a titled portion of an explanation with optional code reference.
type ExplanationSection struct {
	Title   string
	Content string
	CodeRef string
}

// CodeExplainer generates structured explanations of code using AST analysis
// and pattern recognition, without any LLM calls.
type CodeExplainer struct {
	mu sync.Mutex
}

// NewCodeExplainer creates a new CodeExplainer instance.
func NewCodeExplainer() *CodeExplainer {
	return &CodeExplainer{}
}

// ExplainFunction parses the given file content and generates a structured explanation
// for the named function.
func (ce *CodeExplainer) ExplainFunction(file, content, funcName string) (*CodeExplanation, error) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	var funcDecl *ast.FuncDecl
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fd.Name.Name == funcName {
			funcDecl = fd
			break
		}
	}
	if funcDecl == nil {
		return nil, fmt.Errorf("function %q not found in %s", funcName, file)
	}

	// Extract parameters
	params := explainerExtractParams(funcDecl)
	// Extract return types
	returns := extractReturns(funcDecl)
	// Extract doc comment
	docComment := explainerExtractDocComment(funcDecl)

	// Build purpose
	paramTypes := make([]string, 0, len(params))
	for _, p := range params {
		paramTypes = append(paramTypes, p[1])
	}
	purpose := InferPurpose(funcName, paramTypes, returns)
	if docComment != "" {
		purpose = docComment
	}

	// Build sections
	var sections []ExplanationSection

	// Purpose section
	sections = append(sections, ExplanationSection{
		Title:   "Purpose",
		Content: purpose,
	})

	// Parameters section
	if len(params) > 0 {
		var paramLines []string
		for _, p := range params {
			desc := inferParamPurpose(p[0], p[1])
			paramLines = append(paramLines, fmt.Sprintf("- `%s %s` — %s", p[0], p[1], desc))
		}
		sections = append(sections, ExplanationSection{
			Title:   "Parameters",
			Content: strings.Join(paramLines, "\n"),
		})
	}

	// Returns section
	if len(returns) > 0 {
		sections = append(sections, ExplanationSection{
			Title:   "Returns",
			Content: "`" + strings.Join(returns, ", ") + "`",
		})
	}

	// Control flow section
	funcBody := extractFuncBody(content, funcDecl, fset)
	controlFlow := DescribeControlFlow(funcBody)
	sections = append(sections, ExplanationSection{
		Title:   "Control Flow",
		Content: controlFlow,
	})

	// Error handling section
	errHandling := describeErrorHandling(funcBody)
	sections = append(sections, ExplanationSection{
		Title:   "Error Handling",
		Content: errHandling,
	})

	// Side effects section
	sideEffects := DetectSideEffects(funcBody)
	sideEffectStr := "None (pure function)"
	if len(sideEffects) > 0 {
		sideEffectStr = strings.Join(sideEffects, ", ")
	}
	sections = append(sections, ExplanationSection{
		Title:   "Side Effects",
		Content: sideEffectStr,
	})

	// Complexity
	cc := computeCyclomaticComplexity(funcBody)
	complexity := classifyComplexity(cc)

	// Dependencies
	deps := extractDependencies(funcBody)

	return &CodeExplanation{
		File:         file,
		Symbol:       funcName,
		Summary:      purpose,
		Sections:     sections,
		Complexity:   fmt.Sprintf("%s (CC: %d)", complexity, cc),
		Dependencies: deps,
		UsedBy:       nil,
	}, nil
}

// ExplainType parses the given file content and generates a structured explanation
// for the named type.
func (ce *CodeExplainer) ExplainType(file, content, typeName string) (*CodeExplanation, error) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	var typeSpec *ast.TypeSpec
	var genDecl *ast.GenDecl
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if ts.Name.Name == typeName {
				typeSpec = ts
				genDecl = gd
				break
			}
		}
		if typeSpec != nil {
			break
		}
	}
	if typeSpec == nil {
		return nil, fmt.Errorf("type %q not found in %s", typeName, file)
	}

	var sections []ExplanationSection

	// Doc comment
	docComment := ""
	if genDecl.Doc != nil {
		docComment = strings.TrimSpace(genDecl.Doc.Text())
	}
	if typeSpec.Doc != nil {
		docComment = strings.TrimSpace(typeSpec.Doc.Text())
	}

	purpose := inferTypePurpose(typeName)
	if docComment != "" {
		purpose = docComment
	}
	sections = append(sections, ExplanationSection{
		Title:   "Purpose",
		Content: purpose,
	})

	// Fields (for struct types)
	if st, ok := typeSpec.Type.(*ast.StructType); ok {
		var fieldLines []string
		for _, field := range st.Fields.List {
			typeStr := explainerExprToString(field.Type)
			for _, name := range field.Names {
				desc := inferFieldPurpose(name.Name, typeStr)
				fieldLines = append(fieldLines, fmt.Sprintf("- `%s %s` — %s", name.Name, typeStr, desc))
			}
			if len(field.Names) == 0 {
				// Embedded field
				fieldLines = append(fieldLines, fmt.Sprintf("- `%s` (embedded)", typeStr))
			}
		}
		if len(fieldLines) > 0 {
			sections = append(sections, ExplanationSection{
				Title:   "Fields",
				Content: strings.Join(fieldLines, "\n"),
			})
		}
	}

	// Interface methods
	if iface, ok := typeSpec.Type.(*ast.InterfaceType); ok {
		var methodLines []string
		for _, method := range iface.Methods.List {
			if len(method.Names) > 0 {
				methodLines = append(methodLines, fmt.Sprintf("- `%s`", method.Names[0].Name))
			}
		}
		if len(methodLines) > 0 {
			sections = append(sections, ExplanationSection{
				Title:   "Methods",
				Content: strings.Join(methodLines, "\n"),
			})
		}
	}

	// Methods on this type
	var methods []string
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv == nil {
			continue
		}
		for _, recv := range fd.Recv.List {
			recvType := explainerExprToString(recv.Type)
			// Strip pointer
			recvType = strings.TrimPrefix(recvType, "*")
			if recvType == typeName {
				methods = append(methods, fd.Name.Name)
			}
		}
	}
	if len(methods) > 0 {
		var methodLines []string
		for _, m := range methods {
			methodLines = append(methodLines, fmt.Sprintf("- `%s`", m))
		}
		sections = append(sections, ExplanationSection{
			Title:   "Methods",
			Content: strings.Join(methodLines, "\n"),
		})
	}

	// Constructor pattern detection
	constructor := findConstructor(f, typeName)
	if constructor != "" {
		sections = append(sections, ExplanationSection{
			Title:   "Constructor",
			Content: fmt.Sprintf("Use `%s` to create instances", constructor),
		})
	}

	// Interfaces implemented (heuristic)
	interfaces := detectImplementedInterfaces(f, typeName, methods)
	if len(interfaces) > 0 {
		sections = append(sections, ExplanationSection{
			Title:   "Implements",
			Content: strings.Join(interfaces, ", "),
		})
	}

	return &CodeExplanation{
		File:         file,
		Symbol:       typeName,
		Summary:      purpose,
		Sections:     sections,
		Complexity:   "",
		Dependencies: nil,
		UsedBy:       nil,
	}, nil
}

// ExplainFile generates a structured explanation of an entire file.
func (ce *CodeExplainer) ExplainFile(path, content string) (*CodeExplanation, error) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	var sections []ExplanationSection

	// Package purpose
	pkgName := f.Name.Name
	pkgDoc := ""
	if f.Doc != nil {
		pkgDoc = strings.TrimSpace(f.Doc.Text())
	}
	pkgPurpose := fmt.Sprintf("Package `%s`", pkgName)
	if pkgDoc != "" {
		pkgPurpose = pkgDoc
	}
	sections = append(sections, ExplanationSection{
		Title:   "Package Purpose",
		Content: pkgPurpose,
	})

	// Exported API summary
	var exportedFuncs []string
	var exportedTypes []string
	var internalFuncs []string
	var internalTypes []string

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name.IsExported() {
				sig := funcSignature(d)
				exportedFuncs = append(exportedFuncs, sig)
			} else {
				internalFuncs = append(internalFuncs, d.Name.Name)
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if ts.Name.IsExported() {
					exportedTypes = append(exportedTypes, ts.Name.Name)
				} else {
					internalTypes = append(internalTypes, ts.Name.Name)
				}
			}
		}
	}

	if len(exportedTypes) > 0 || len(exportedFuncs) > 0 {
		var apiLines []string
		for _, t := range exportedTypes {
			apiLines = append(apiLines, fmt.Sprintf("- type `%s`", t))
		}
		for _, fn := range exportedFuncs {
			apiLines = append(apiLines, fmt.Sprintf("- `%s`", fn))
		}
		sections = append(sections, ExplanationSection{
			Title:   "Exported API",
			Content: strings.Join(apiLines, "\n"),
		})
	}

	// Internal structure
	if len(internalTypes) > 0 || len(internalFuncs) > 0 {
		var internalLines []string
		for _, t := range internalTypes {
			internalLines = append(internalLines, fmt.Sprintf("- type `%s`", t))
		}
		for _, fn := range internalFuncs {
			internalLines = append(internalLines, fmt.Sprintf("- `%s`", fn))
		}
		sections = append(sections, ExplanationSection{
			Title:   "Internal Structure",
			Content: strings.Join(internalLines, "\n"),
		})
	}

	// Key patterns
	patterns := detectPatterns(content)
	if len(patterns) > 0 {
		sections = append(sections, ExplanationSection{
			Title:   "Key Patterns",
			Content: strings.Join(patterns, "\n"),
		})
	}

	summary := pkgPurpose
	if len(exportedFuncs)+len(exportedTypes) > 0 {
		summary = fmt.Sprintf("%s — exports %d types, %d functions",
			pkgPurpose, len(exportedTypes), len(exportedFuncs))
	}

	return &CodeExplanation{
		File:     path,
		Symbol:   pkgName,
		Summary:  summary,
		Sections: sections,
	}, nil
}

// InferPurpose infers the purpose of a function from its name, parameter types,
// and return types using heuristic pattern matching.
func InferPurpose(name string, params, returns []string) string {
	lower := strings.ToLower(name)
	words := splitCamelCase(name)
	verb := ""
	if len(words) > 0 {
		verb = strings.ToLower(words[0])
	}
	object := ""
	if len(words) > 1 {
		object = strings.Join(words[1:], " ")
	}

	// Check for common verb patterns
	hasError := containsType(returns, "error")
	hasBool := containsType(returns, "bool")

	switch verb {
	case "new":
		return fmt.Sprintf("Creates a new %s instance", object)
	case "get", "fetch", "load", "read":
		if hasError {
			return fmt.Sprintf("Retrieves %s, returning an error on failure", lowerFirst(object))
		}
		return fmt.Sprintf("Retrieves %s", lowerFirst(object))
	case "set", "update", "put":
		if hasError {
			return fmt.Sprintf("Updates %s, returning an error on failure", lowerFirst(object))
		}
		return fmt.Sprintf("Updates %s", lowerFirst(object))
	case "delete", "remove":
		if hasError {
			return fmt.Sprintf("Removes %s, returning an error on failure", lowerFirst(object))
		}
		return fmt.Sprintf("Removes %s", lowerFirst(object))
	case "validate", "check", "verify":
		if hasError {
			return fmt.Sprintf("Validates %s and returns an error if invalid", lowerFirst(object))
		}
		if hasBool {
			return fmt.Sprintf("Checks whether %s is valid", lowerFirst(object))
		}
		return fmt.Sprintf("Validates %s", lowerFirst(object))
	case "is", "has", "can", "should":
		return fmt.Sprintf("Returns whether %s", lowerFirst(object))
	case "parse":
		if hasError {
			return fmt.Sprintf("Parses %s from input, returning an error if malformed", lowerFirst(object))
		}
		return fmt.Sprintf("Parses %s from input", lowerFirst(object))
	case "convert", "to", "from":
		return fmt.Sprintf("Converts %s between representations", lowerFirst(object))
	case "init", "initialize", "setup":
		return fmt.Sprintf("Initializes %s", lowerFirst(object))
	case "close", "shutdown", "stop":
		return fmt.Sprintf("Shuts down %s and releases resources", lowerFirst(object))
	case "start", "run", "execute":
		return fmt.Sprintf("Starts or executes %s", lowerFirst(object))
	case "write", "save", "store":
		if hasError {
			return fmt.Sprintf("Persists %s, returning an error on failure", lowerFirst(object))
		}
		return fmt.Sprintf("Persists %s", lowerFirst(object))
	case "find", "search", "lookup":
		return fmt.Sprintf("Searches for %s matching the given criteria", lowerFirst(object))
	case "format", "render":
		return fmt.Sprintf("Formats %s for display or output", lowerFirst(object))
	case "handle":
		return fmt.Sprintf("Handles %s events or requests", lowerFirst(object))
	case "register", "add":
		return fmt.Sprintf("Registers or adds %s", lowerFirst(object))
	}

	// Fallback with return type context
	if strings.Contains(lower, "string") && containsType(returns, "string") {
		return fmt.Sprintf("Converts %s to its string representation", lowerFirst(name))
	}

	if hasError && len(params) > 0 {
		return fmt.Sprintf("Performs %s on the given input, returning an error on failure",
			lowerFirst(strings.Join(words, " ")))
	}

	return fmt.Sprintf("Performs %s", lowerFirst(strings.Join(words, " ")))
}

// DescribeControlFlow analyzes function body text and returns a human-readable
// description of its control flow pattern.
func DescribeControlFlow(funcBody string) string {
	hasFor := regexp.MustCompile(`\bfor\b`).MatchString(funcBody)
	hasRange := regexp.MustCompile(`\brange\b`).MatchString(funcBody)
	hasSwitch := regexp.MustCompile(`\bswitch\b`).MatchString(funcBody)
	hasSelect := regexp.MustCompile(`\bselect\b`).MatchString(funcBody)
	hasIf := regexp.MustCompile(`\bif\b`).MatchString(funcBody)
	hasErrReturn := regexp.MustCompile(`return\s+.*err`).MatchString(funcBody)
	hasBreak := regexp.MustCompile(`\bbreak\b`).MatchString(funcBody)
	hasContinue := regexp.MustCompile(`\bcontinue\b`).MatchString(funcBody)
	hasGoto := regexp.MustCompile(`\bgoto\b`).MatchString(funcBody)
	hasDefer := regexp.MustCompile(`\bdefer\b`).MatchString(funcBody)
	hasGo := regexp.MustCompile(`\bgo\s+`).MatchString(funcBody)
	hasRecursion := regexp.MustCompile(`\b\w+\(`).MatchString(funcBody)

	var parts []string

	if hasSelect {
		parts = append(parts, "Channel select with multiple cases")
	} else if hasSwitch {
		parts = append(parts, "Switch dispatch")
	} else if hasFor && hasRange {
		if hasBreak {
			parts = append(parts, "Range loop with conditional break")
		} else if hasContinue {
			parts = append(parts, "Range loop with conditional skip")
		} else {
			parts = append(parts, "Range iteration")
		}
	} else if hasFor {
		if hasBreak {
			parts = append(parts, "Loop with conditional break")
		} else {
			parts = append(parts, "Loop-based processing")
		}
	} else if hasIf && hasErrReturn {
		parts = append(parts, "Linear with early error returns")
	} else if hasIf {
		parts = append(parts, "Conditional branching")
	} else {
		parts = append(parts, "Linear")
	}

	if hasDefer {
		parts = append(parts, "with deferred cleanup")
	}
	if hasGo {
		parts = append(parts, "with concurrent goroutines")
	}
	if hasGoto {
		parts = append(parts, "with goto jumps")
	}
	_ = hasRecursion // used above in broader context

	return strings.Join(parts, " ")
}

// DetectSideEffects analyzes function body text and returns a list of detected
// side effects such as file I/O, network calls, goroutine spawning, etc.
func DetectSideEffects(funcBody string) []string {
	var effects []string

	// File I/O
	filePatterns := []string{
		`os\.Open`, `os\.Create`, `os\.Remove`, `os\.Mkdir`,
		`os\.ReadFile`, `os\.WriteFile`, `os\.Stat`,
		`ioutil\.ReadFile`, `ioutil\.WriteFile`, `ioutil\.TempFile`,
		`filepath\.Walk`,
	}
	for _, p := range filePatterns {
		if regexp.MustCompile(p).MatchString(funcBody) {
			effects = append(effects, "File I/O")
			break
		}
	}

	// Network calls
	netPatterns := []string{
		`http\.Get`, `http\.Post`, `http\.Do`,
		`net\.Dial`, `net\.Listen`,
		`\.Do\(req`, `client\.`,
		`grpc\.`, `websocket\.`,
	}
	for _, p := range netPatterns {
		if regexp.MustCompile(p).MatchString(funcBody) {
			effects = append(effects, "Network calls")
			break
		}
	}

	// Goroutine spawning
	if regexp.MustCompile(`\bgo\s+\w`).MatchString(funcBody) {
		effects = append(effects, "Goroutine spawning")
	}

	// Mutex locking
	if regexp.MustCompile(`\.Lock\(\)|\.RLock\(\)`).MatchString(funcBody) {
		effects = append(effects, "Mutex locking")
	}

	// Channel operations
	if regexp.MustCompile(`<-\s*\w|(\w+)\s*<-`).MatchString(funcBody) {
		effects = append(effects, "Channel communication")
	}

	// Global/package-level mutation
	if regexp.MustCompile(`\b(os\.Setenv|os\.Exit|log\.Fatal)`).MatchString(funcBody) {
		effects = append(effects, "Process-level side effects")
	}

	// Database operations
	dbPatterns := []string{
		`\.Exec\(`, `\.Query\(`, `\.QueryRow\(`,
		`\.Begin\(`, `\.Commit\(`, `\.Rollback\(`,
	}
	for _, p := range dbPatterns {
		if regexp.MustCompile(p).MatchString(funcBody) {
			effects = append(effects, "Database operations")
			break
		}
	}

	// Stdout/stderr writes
	if regexp.MustCompile(`fmt\.Print|fmt\.Fprint|os\.Stdout|os\.Stderr`).MatchString(funcBody) {
		effects = append(effects, "Standard output")
	}

	return effects
}

// FormatExplanation renders a CodeExplanation into a human-readable markdown-style string.
func FormatExplanation(exp *CodeExplanation) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## %s\n\n", exp.Symbol))

	for _, section := range exp.Sections {
		switch section.Title {
		case "Parameters":
			sb.WriteString(fmt.Sprintf("**%s:**\n%s\n\n", section.Title, section.Content))
		case "Returns":
			sb.WriteString(fmt.Sprintf("**%s:** %s\n\n", section.Title, section.Content))
		default:
			sb.WriteString(fmt.Sprintf("**%s:** %s\n\n", section.Title, section.Content))
		}
	}

	if exp.Complexity != "" {
		sb.WriteString(fmt.Sprintf("**Complexity:** %s\n", exp.Complexity))
	}

	return sb.String()
}

// --- Helper functions ---

func explainerExtractParams(fd *ast.FuncDecl) [][2]string {
	var params [][2]string
	if fd.Type.Params == nil {
		return params
	}
	for _, field := range fd.Type.Params.List {
		typeStr := explainerExprToString(field.Type)
		if len(field.Names) == 0 {
			params = append(params, [2]string{"", typeStr})
		}
		for _, name := range field.Names {
			params = append(params, [2]string{name.Name, typeStr})
		}
	}
	return params
}

func extractReturns(fd *ast.FuncDecl) []string {
	var returns []string
	if fd.Type.Results == nil {
		return returns
	}
	for _, field := range fd.Type.Results.List {
		typeStr := explainerExprToString(field.Type)
		if len(field.Names) > 0 {
			for range field.Names {
				returns = append(returns, typeStr)
			}
		} else {
			returns = append(returns, typeStr)
		}
	}
	return returns
}

func explainerExtractDocComment(fd *ast.FuncDecl) string {
	if fd.Doc == nil {
		return ""
	}
	text := strings.TrimSpace(fd.Doc.Text())
	// Remove the leading function name if the doc starts with it
	if strings.HasPrefix(text, fd.Name.Name+" ") {
		text = text[len(fd.Name.Name)+1:]
	}
	// Take first sentence
	if idx := strings.Index(text, ". "); idx > 0 {
		return text[:idx+1]
	}
	return text
}

func extractFuncBody(content string, fd *ast.FuncDecl, fset *token.FileSet) string {
	if fd.Body == nil {
		return ""
	}
	start := fset.Position(fd.Body.Lbrace).Offset
	end := fset.Position(fd.Body.Rbrace).Offset
	if start >= 0 && end > start && end <= len(content) {
		return content[start:end]
	}
	return ""
}

func explainerExprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return explainerExprToString(e.X) + "." + e.Sel.Name
	case *ast.StarExpr:
		return "*" + explainerExprToString(e.X)
	case *ast.ArrayType:
		return "[]" + explainerExprToString(e.Elt)
	case *ast.MapType:
		return "map[" + explainerExprToString(e.Key) + "]" + explainerExprToString(e.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func(...)"
	case *ast.ChanType:
		return "chan " + explainerExprToString(e.Value)
	case *ast.Ellipsis:
		return "..." + explainerExprToString(e.Elt)
	default:
		return "unknown"
	}
}

func inferParamPurpose(name, typeName string) string {
	lower := strings.ToLower(name)
	switch {
	case lower == "ctx" || typeName == "context.Context":
		return "Context for cancellation and deadlines"
	case lower == "err" || typeName == "error":
		return "Error to handle"
	case strings.Contains(lower, "path") || strings.Contains(lower, "file"):
		return "File path"
	case strings.Contains(lower, "name"):
		return "Name identifier"
	case strings.Contains(lower, "id"):
		return "Unique identifier"
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "duration"):
		return "Time duration"
	case strings.Contains(lower, "config") || strings.Contains(lower, "opts"):
		return "Configuration options"
	case strings.Contains(lower, "fn") || strings.Contains(lower, "func") || strings.Contains(lower, "callback"):
		return "Callback function"
	case strings.Contains(lower, "buf") || strings.Contains(lower, "data") || strings.Contains(lower, "bytes"):
		return "Raw data buffer"
	case strings.Contains(lower, "url") || strings.Contains(lower, "addr"):
		return "Network address"
	case strings.Contains(lower, "token"):
		return "Authentication or parsing token"
	case strings.Contains(lower, "key"):
		return "Lookup key"
	case strings.Contains(lower, "val") || strings.Contains(lower, "value"):
		return "Value to process"
	case typeName == "string":
		return fmt.Sprintf("The %s string", name)
	case typeName == "int" || typeName == "int64":
		return fmt.Sprintf("The %s count or index", name)
	case typeName == "bool":
		return fmt.Sprintf("Whether to enable %s", name)
	default:
		return fmt.Sprintf("The %s to use", name)
	}
}

func inferTypePurpose(name string) string {
	words := splitCamelCase(name)
	if len(words) == 0 {
		return "A type"
	}
	last := strings.ToLower(words[len(words)-1])
	prefix := ""
	if len(words) > 1 {
		prefix = strings.Join(words[:len(words)-1], " ")
	}

	switch last {
	case "config", "options", "opts", "settings":
		return fmt.Sprintf("Configuration for %s", lowerFirst(prefix))
	case "handler":
		return fmt.Sprintf("Handles %s operations", lowerFirst(prefix))
	case "service", "server":
		return fmt.Sprintf("Provides %s functionality", lowerFirst(prefix))
	case "client":
		return fmt.Sprintf("Client for communicating with %s", lowerFirst(prefix))
	case "store", "repository", "repo":
		return fmt.Sprintf("Persistent storage for %s", lowerFirst(prefix))
	case "manager":
		return fmt.Sprintf("Manages lifecycle of %s", lowerFirst(prefix))
	case "builder":
		return fmt.Sprintf("Builder pattern for constructing %s", lowerFirst(prefix))
	case "error", "err":
		return fmt.Sprintf("Error type for %s failures", lowerFirst(prefix))
	case "result":
		return fmt.Sprintf("Result of %s operation", lowerFirst(prefix))
	case "request", "req":
		return fmt.Sprintf("Request payload for %s", lowerFirst(prefix))
	case "response", "resp":
		return fmt.Sprintf("Response from %s", lowerFirst(prefix))
	default:
		return fmt.Sprintf("Represents %s", lowerFirst(strings.Join(words, " ")))
	}
}

func inferFieldPurpose(name, typeName string) string {
	lower := strings.ToLower(name)
	switch {
	case lower == "mu" || strings.Contains(typeName, "Mutex"):
		return "Protects concurrent access"
	case lower == "ctx":
		return "Context for cancellation"
	case lower == "id":
		return "Unique identifier"
	case strings.Contains(lower, "err"):
		return "Last error encountered"
	case strings.Contains(lower, "done") || strings.Contains(lower, "closed"):
		return "Signals completion or shutdown"
	case strings.Contains(lower, "count") || strings.Contains(lower, "num"):
		return "Counter value"
	case strings.Contains(lower, "max") || strings.Contains(lower, "limit"):
		return "Upper bound constraint"
	case strings.Contains(lower, "min"):
		return "Lower bound constraint"
	case strings.Contains(lower, "timeout"):
		return "Maximum wait duration"
	case strings.Contains(lower, "name"):
		return "Human-readable name"
	case strings.Contains(lower, "path") || strings.Contains(lower, "dir"):
		return "Filesystem path"
	default:
		return fmt.Sprintf("The %s value", name)
	}
}

func splitCamelCase(s string) []string {
	var words []string
	current := strings.Builder{}
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func containsType(types []string, target string) bool {
	for _, t := range types {
		if t == target || strings.Contains(t, target) {
			return true
		}
	}
	return false
}

func computeCyclomaticComplexity(body string) int {
	cc := 1 // base complexity
	patterns := []string{
		`\bif\b`, `\belse if\b`, `\bfor\b`, `\bcase\b`,
		`&&`, `\|\|`, `\bselect\b`,
	}
	for _, p := range patterns {
		re := regexp.MustCompile(p)
		cc += len(re.FindAllString(body, -1))
	}
	return cc
}

func classifyComplexity(cc int) string {
	switch {
	case cc <= 5:
		return "Low"
	case cc <= 10:
		return "Moderate"
	case cc <= 20:
		return "High"
	default:
		return "Very High"
	}
}

func describeErrorHandling(body string) string {
	hasErrCheck := regexp.MustCompile(`if\s+err\s*!=\s*nil`).MatchString(body)
	hasWrap := regexp.MustCompile(`fmt\.Errorf\(.*%w`).MatchString(body)
	hasPanic := regexp.MustCompile(`\bpanic\(`).MatchString(body)
	hasRecover := regexp.MustCompile(`\brecover\(\)`).MatchString(body)

	if !hasErrCheck && !hasPanic {
		return "No explicit error handling"
	}

	var parts []string
	if hasErrCheck && hasWrap {
		parts = append(parts, "Returns wrapped errors with context")
	} else if hasErrCheck {
		parts = append(parts, "Checks and propagates errors")
	}
	if hasPanic {
		parts = append(parts, "May panic on unrecoverable errors")
	}
	if hasRecover {
		parts = append(parts, "Includes panic recovery")
	}

	if len(parts) == 0 {
		return "Basic error checking"
	}
	return strings.Join(parts, "; ")
}

func extractDependencies(body string) []string {
	var deps []string
	// Look for package.Function patterns
	re := regexp.MustCompile(`\b([a-z][a-z0-9]+)\.\w+`)
	matches := re.FindAllStringSubmatch(body, -1)
	seen := map[string]bool{}
	for _, m := range matches {
		pkg := m[1]
		if !seen[pkg] && pkg != "err" && pkg != "nil" {
			seen[pkg] = true
			deps = append(deps, pkg)
		}
	}
	return deps
}

func funcSignature(fd *ast.FuncDecl) string {
	var sb strings.Builder
	sb.WriteString(fd.Name.Name)
	sb.WriteString("(")
	if fd.Type.Params != nil {
		var params []string
		for _, field := range fd.Type.Params.List {
			typeStr := explainerExprToString(field.Type)
			if len(field.Names) > 0 {
				for _, name := range field.Names {
					params = append(params, name.Name+" "+typeStr)
				}
			} else {
				params = append(params, typeStr)
			}
		}
		sb.WriteString(strings.Join(params, ", "))
	}
	sb.WriteString(")")
	if fd.Type.Results != nil && len(fd.Type.Results.List) > 0 {
		var results []string
		for _, field := range fd.Type.Results.List {
			results = append(results, explainerExprToString(field.Type))
		}
		if len(results) == 1 {
			sb.WriteString(" " + results[0])
		} else {
			sb.WriteString(" (" + strings.Join(results, ", ") + ")")
		}
	}
	return sb.String()
}

func findConstructor(f *ast.File, typeName string) string {
	candidates := []string{
		"New" + typeName,
		"new" + typeName,
	}
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv != nil {
			continue
		}
		for _, c := range candidates {
			if fd.Name.Name == c {
				return c
			}
		}
	}
	return ""
}

func detectImplementedInterfaces(f *ast.File, typeName string, methods []string) []string {
	var ifaces []string
	// Common Go interfaces by method signature
	methodSet := map[string]bool{}
	for _, m := range methods {
		methodSet[m] = true
	}

	if methodSet["String"] {
		ifaces = append(ifaces, "fmt.Stringer")
	}
	if methodSet["Error"] {
		ifaces = append(ifaces, "error")
	}
	if methodSet["Read"] {
		ifaces = append(ifaces, "io.Reader")
	}
	if methodSet["Write"] {
		ifaces = append(ifaces, "io.Writer")
	}
	if methodSet["Close"] {
		ifaces = append(ifaces, "io.Closer")
	}
	if methodSet["ServeHTTP"] {
		ifaces = append(ifaces, "http.Handler")
	}
	if methodSet["MarshalJSON"] {
		ifaces = append(ifaces, "json.Marshaler")
	}
	if methodSet["UnmarshalJSON"] {
		ifaces = append(ifaces, "json.Unmarshaler")
	}
	if methodSet["Len"] && methodSet["Less"] && methodSet["Swap"] {
		ifaces = append(ifaces, "sort.Interface")
	}

	return ifaces
}

func detectPatterns(content string) []string {
	var patterns []string

	if regexp.MustCompile(`sync\.(Mutex|RWMutex)`).MatchString(content) {
		patterns = append(patterns, "- Mutex-based concurrency control")
	}
	if regexp.MustCompile(`sync\.Once`).MatchString(content) {
		patterns = append(patterns, "- Singleton/once initialization")
	}
	if regexp.MustCompile(`chan\s+\w`).MatchString(content) {
		patterns = append(patterns, "- Channel-based communication")
	}
	if regexp.MustCompile(`context\.Context`).MatchString(content) {
		patterns = append(patterns, "- Context propagation for cancellation")
	}
	if regexp.MustCompile(`interface\s*\{`).MatchString(content) {
		patterns = append(patterns, "- Interface-based abstraction")
	}
	if regexp.MustCompile(`func\s+New\w+\(`).MatchString(content) {
		patterns = append(patterns, "- Constructor functions (New* pattern)")
	}
	if regexp.MustCompile(`defer\s+`).MatchString(content) {
		patterns = append(patterns, "- Deferred cleanup")
	}
	if regexp.MustCompile(`fmt\.Errorf\(.*%w`).MatchString(content) {
		patterns = append(patterns, "- Error wrapping with context")
	}
	if regexp.MustCompile(`select\s*\{`).MatchString(content) {
		patterns = append(patterns, "- Select-based multiplexing")
	}
	if regexp.MustCompile(`type\s+\w+\s+struct\s*\{[^}]*\w+\s+interface`).MatchString(content) {
		patterns = append(patterns, "- Dependency injection via interfaces")
	}

	return patterns
}
