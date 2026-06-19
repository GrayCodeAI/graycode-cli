package code

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"sync"
)

type CodeExplanation struct {
	File         string
	Symbol       string
	Summary      string
	Sections     []ExplanationSection
	Complexity   string
	Dependencies []string
	UsedBy       []string
}

type ExplanationSection struct {
	Title   string
	Content string
	CodeRef string
}

type CodeExplainer struct {
	mu sync.Mutex
}

func NewCodeExplainer() *CodeExplainer {
	return &CodeExplainer{}
}

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

	params := explainerExtractParams(funcDecl)
	returns := extractReturns(funcDecl)
	docComment := explainerExtractDocComment(funcDecl)

	paramTypes := make([]string, 0, len(params))
	for _, p := range params {
		paramTypes = append(paramTypes, p[1])
	}
	purpose := InferPurpose(funcName, paramTypes, returns)
	if docComment != "" {
		purpose = docComment
	}

	var sections []ExplanationSection

	sections = append(sections, ExplanationSection{
		Title:   "Purpose",
		Content: purpose,
	})

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

	if len(returns) > 0 {
		sections = append(sections, ExplanationSection{
			Title:   "Returns",
			Content: "`" + strings.Join(returns, ", ") + "`",
		})
	}

	funcBody := extractFuncBody(content, funcDecl, fset)
	controlFlow := DescribeControlFlow(funcBody)
	sections = append(sections, ExplanationSection{
		Title:   "Control Flow",
		Content: controlFlow,
	})

	errHandling := describeErrorHandling(funcBody)
	sections = append(sections, ExplanationSection{
		Title:   "Error Handling",
		Content: errHandling,
	})

	sideEffects := DetectSideEffects(funcBody)
	sideEffectStr := "None (pure function)"
	if len(sideEffects) > 0 {
		sideEffectStr = strings.Join(sideEffects, ", ")
	}
	sections = append(sections, ExplanationSection{
		Title:   "Side Effects",
		Content: sideEffectStr,
	})

	cc := computeCyclomaticComplexity(funcBody)
	complexity := classifyComplexity(cc)

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

	if st, ok := typeSpec.Type.(*ast.StructType); ok {
		var fieldLines []string
		for _, field := range st.Fields.List {
			typeStr := explainerExprToString(field.Type)
			for _, name := range field.Names {
				desc := inferFieldPurpose(name.Name, typeStr)
				fieldLines = append(fieldLines, fmt.Sprintf("- `%s %s` — %s", name.Name, typeStr, desc))
			}
			if len(field.Names) == 0 {
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

	var methods []string
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv == nil {
			continue
		}
		for _, recv := range fd.Recv.List {
			recvType := explainerExprToString(recv.Type)
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

	constructor := findConstructor(f, typeName)
	if constructor != "" {
		sections = append(sections, ExplanationSection{
			Title:   "Constructor",
			Content: fmt.Sprintf("Use `%s` to create instances", constructor),
		})
	}

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

func (ce *CodeExplainer) ExplainFile(path, content string) (*CodeExplanation, error) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	var sections []ExplanationSection

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

	if strings.Contains(lower, "string") && containsType(returns, "string") {
		return fmt.Sprintf("Converts %s to its string representation", lowerFirst(name))
	}

	if hasError && len(params) > 0 {
		return fmt.Sprintf("Performs %s on the given input, returning an error on failure",
			lowerFirst(strings.Join(words, " ")))
	}

	return fmt.Sprintf("Performs %s", lowerFirst(strings.Join(words, " ")))
}

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
	_ = hasRecursion

	return strings.Join(parts, " ")
}

func DetectSideEffects(funcBody string) []string {
	var effects []string

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

	if regexp.MustCompile(`\bgo\s+\w`).MatchString(funcBody) {
		effects = append(effects, "Goroutine spawning")
	}

	if regexp.MustCompile(`\.Lock\(\)|\.RLock\(\)`).MatchString(funcBody) {
		effects = append(effects, "Mutex locking")
	}

	if regexp.MustCompile(`<-\s*\w|(\w+)\s*<-`).MatchString(funcBody) {
		effects = append(effects, "Channel communication")
	}

	if regexp.MustCompile(`\b(os\.Setenv|os\.Exit|log\.Fatal)`).MatchString(funcBody) {
		effects = append(effects, "Process-level side effects")
	}

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

	if regexp.MustCompile(`fmt\.Print|fmt\.Fprint|os\.Stdout|os\.Stderr`).MatchString(funcBody) {
		effects = append(effects, "Standard output")
	}

	return effects
}

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
