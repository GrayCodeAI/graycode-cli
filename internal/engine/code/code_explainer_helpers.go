package code

import (
	"fmt"
	"go/ast"
	"go/token"
	"regexp"
	"strings"
)

// This file holds the internal helpers for the CodeExplainer: AST extraction,
// purpose inference, complexity/error-handling analysis, and pattern detection.
// The CodeExplainer type and its public Explain*/Infer/Describe/Detect/Format
// entry points live in code_explainer.go.

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
	if strings.HasPrefix(text, fd.Name.Name+" ") {
		text = text[len(fd.Name.Name)+1:]
	}
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
	cc := 1
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
