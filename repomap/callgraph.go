package repomap

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// CallGraph maps functions to their callers and callees using Go AST analysis.
// No CGO required — uses go/parser for static analysis.
type CallGraph struct {
	// callers maps function name -> functions that call it
	callers map[string][]string
	// callees maps function name -> functions it calls
	callees map[string][]string
}

// BuildCallGraph parses Go files in root and extracts call relationships.
func BuildCallGraph(root string) (*CallGraph, error) {
	cg := &CallGraph{
		callers: make(map[string][]string),
		callees: make(map[string][]string),
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if shouldSkipDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		cg.parseFile(path)
		return nil
	})

	return cg, err
}

// CallersOf returns functions that call the given function (depth levels up).
func (cg *CallGraph) CallersOf(funcName string, maxDepth int) []string {
	if maxDepth <= 0 {
		maxDepth = 1
	}
	visited := make(map[string]bool)
	var result []string
	cg.bfsReverse(funcName, maxDepth, visited, &result)
	return result
}

// CalleesOf returns functions called by the given function (depth levels down).
func (cg *CallGraph) CalleesOf(funcName string, maxDepth int) []string {
	if maxDepth <= 0 {
		maxDepth = 1
	}
	visited := make(map[string]bool)
	var result []string
	cg.bfsForward(funcName, maxDepth, visited, &result)
	return result
}

// Neighborhood returns callers + callees within depth.
func (cg *CallGraph) Neighborhood(funcName string, depth int) []string {
	callers := cg.CallersOf(funcName, depth)
	callees := cg.CalleesOf(funcName, depth)
	seen := make(map[string]bool)
	var result []string
	for _, f := range append(callers, callees...) {
		if !seen[f] && f != funcName {
			seen[f] = true
			result = append(result, f)
		}
	}
	return result
}

func (cg *CallGraph) parseFile(path string) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return
	}

	// Walk AST to find function declarations and their call expressions
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		funcName := qualifiedName(fn)
		if funcName == "" {
			continue
		}

		// Walk function body for call expressions
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			callee := extractCalleeName(call)
			if callee == "" || callee == funcName {
				return true
			}

			cg.callees[funcName] = appendUniqueStr(cg.callees[funcName], callee)
			cg.callers[callee] = appendUniqueStr(cg.callers[callee], funcName)
			return true
		})
	}
}

func qualifiedName(fn *ast.FuncDecl) string {
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		recv := fn.Recv.List[0]
		typeName := exprName(recv.Type)
		if typeName != "" {
			return typeName + "." + fn.Name.Name
		}
	}
	return fn.Name.Name
}

func exprName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return exprName(t.X)
	}
	return ""
}

func extractCalleeName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		if x, ok := fn.X.(*ast.Ident); ok {
			return x.Name + "." + fn.Sel.Name
		}
		return fn.Sel.Name
	}
	return ""
}

func (cg *CallGraph) bfsForward(start string, maxDepth int, visited map[string]bool, result *[]string) {
	type qItem struct {
		name  string
		depth int
	}
	queue := []qItem{{start, 0}}
	visited[start] = true

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if item.depth > 0 {
			*result = append(*result, item.name)
		}
		if item.depth >= maxDepth {
			continue
		}

		for _, callee := range cg.callees[item.name] {
			if !visited[callee] {
				visited[callee] = true
				queue = append(queue, qItem{callee, item.depth + 1})
			}
		}
	}
}

func (cg *CallGraph) bfsReverse(start string, maxDepth int, visited map[string]bool, result *[]string) {
	type qItem struct {
		name  string
		depth int
	}
	queue := []qItem{{start, 0}}
	visited[start] = true

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if item.depth > 0 {
			*result = append(*result, item.name)
		}
		if item.depth >= maxDepth {
			continue
		}

		for _, caller := range cg.callers[item.name] {
			if !visited[caller] {
				visited[caller] = true
				queue = append(queue, qItem{caller, item.depth + 1})
			}
		}
	}
}

func shouldSkipDir(name string) bool {
	return name == "vendor" || name == "node_modules" || name == ".git" || strings.HasPrefix(name, ".")
}

func appendUniqueStr(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}
