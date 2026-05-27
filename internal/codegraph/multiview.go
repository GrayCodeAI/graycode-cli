package codegraph

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// MultiViewGraph represents code from multiple perspectives:
// AST (syntax), DFG (data flow), CFG (control flow), and Call Graph.
// Research shows multi-view representation improves all downstream tasks.
type MultiViewGraph struct {
	AST  *ASTView  `json:"ast"`
	DFG  *DFGView  `json:"dfg"`
	CFG  *CFGView  `json:"cfg"`
	Call *CallView `json:"call"`
}

// ASTView represents the abstract syntax tree view.
type ASTView struct {
	Nodes []ASTNode `json:"nodes"`
	Edges []ASTEdge `json:"edges"`
}

type ASTNode struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // "func", "type", "var", "const", "import"
	Name     string `json:"name"`
	File     string `json:"file"`
	StartPos int    `json:"start_pos"`
	EndPos   int    `json:"end_pos"`
}

type ASTEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"` // "contains", "defines", "uses"
}

// DFGView represents the data flow graph view.
type DFGView struct {
	Nodes []DFGNode `json:"nodes"`
	Edges []DFGEdge `json:"edges"`
}

type DFGNode struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // "definition", "use", "parameter", "return"
	Name     string `json:"name"`
	Variable string `json:"variable"`
	File     string `json:"file"`
	Line     int    `json:"line"`
}

type DFGEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"` // "defines", "uses", "flows_to", "depends_on"
}

// CFGView represents the control flow graph view.
type CFGView struct {
	Nodes []CFGNode `json:"nodes"`
	Edges []CFGEdge `json:"edges"`
}

type CFGNode struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // "entry", "exit", "branch", "loop", "call", "return"
	Function string `json:"function"`
	File     string `json:"file"`
	Line     int    `json:"line"`
}

type CFGEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"` // "sequential", "branch_true", "branch_false", "loop_body", "loop_exit"
}

// CallView represents the call graph view.
type CallView struct {
	Nodes []CallNode `json:"nodes"`
	Edges []CallEdge `json:"edges"`
}

type CallNode struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	IsExport bool   `json:"is_export"`
}

type CallEdge struct {
	Caller string `json:"caller"`
	Callee string `json:"callee"`
	File   string `json:"file"`
	Line   int    `json:"line"`
}

// BuildMultiViewGraph constructs a multi-view graph from Go source code.
func BuildMultiViewGraph(filePath string, source []byte) (*MultiViewGraph, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, source, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	mvg := &MultiViewGraph{
		AST:  &ASTView{},
		DFG:  &DFGView{},
		CFG:  &CFGView{},
		Call: &CallView{},
	}

	// Build AST view
	ast.Walk(&astBuilder{view: mvg.AST, file: filePath, fset: fset}, node)

	// Build DFG view (data flow)
	buildDFG(mvg.DFG, node, filePath, fset)

	// Build CFG view (control flow)
	buildCFG(mvg.CFG, node, filePath, fset)

	// Build Call view
	buildCallGraph(mvg.Call, node, filePath, fset)

	return mvg, nil
}

// astBuilder implements ast.Visitor for building AST view.
type astBuilder struct {
	view *ASTView
	file string
	fset *token.FileSet
}

func (b *astBuilder) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		return nil
	}

	switch node := n.(type) {
	case *ast.FuncDecl:
		b.view.Nodes = append(b.view.Nodes, ASTNode{
			ID:       b.file + ":" + node.Name.Name,
			Type:     "func",
			Name:     node.Name.Name,
			File:     b.file,
			StartPos: b.fset.Position(node.Pos()).Offset,
			EndPos:   b.fset.Position(node.End()).Offset,
		})
	case *ast.GenDecl:
		for _, spec := range node.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				b.view.Nodes = append(b.view.Nodes, ASTNode{
					ID:   b.file + ":" + s.Name.Name,
					Type: "type",
					Name: s.Name.Name,
					File: b.file,
				})
			case *ast.ValueSpec:
				for _, name := range s.Names {
					b.view.Nodes = append(b.view.Nodes, ASTNode{
						ID:   b.file + ":" + name.Name,
						Type: "var",
						Name: name.Name,
						File: b.file,
					})
				}
			}
		}
	}
	return b
}

// buildDFG constructs data flow edges from variable definitions to uses.
func buildDFG(dfg *DFGView, node *ast.File, filePath string, fset *token.FileSet) {
	// Track variable definitions
	defs := make(map[string]*DFGNode)

	ast.Inspect(node, func(n ast.Node) bool {
		switch t := n.(type) {
		case *ast.AssignStmt:
			// Definition via assignment
			for _, lhs := range t.Lhs {
				if id, ok := lhs.(*ast.Ident); ok {
					dfgNode := &DFGNode{
						ID:       filePath + ":def:" + id.Name + ":" + itoa(fset.Position(id.Pos()).Line),
						Type:     "definition",
						Name:     id.Name,
						Variable: id.Name,
						File:     filePath,
						Line:     fset.Position(id.Pos()).Line,
					}
					dfg.Nodes = append(dfg.Nodes, *dfgNode)
					defs[id.Name+":"+itoa(fset.Position(id.Pos()).Line)] = dfgNode
				}
			}
		case *ast.Ident:
			// Use of variable
			if t.Obj != nil && t.Obj.Kind == ast.Var {
				dfgNode := DFGNode{
					ID:       filePath + ":use:" + t.Name + ":" + itoa(fset.Position(t.Pos()).Line),
					Type:     "use",
					Name:     t.Name,
					Variable: t.Name,
					File:     filePath,
					Line:     fset.Position(t.Pos()).Line,
				}
				dfg.Nodes = append(dfg.Nodes, dfgNode)

				// Connect to nearest definition
				for key, def := range defs {
					if strings.HasPrefix(key, t.Name+":") {
						dfg.Edges = append(dfg.Edges, DFGEdge{
							Source: def.ID,
							Target: dfgNode.ID,
							Type:   "flows_to",
						})
					}
				}
			}
		}
		return true
	})
}

// buildCFG constructs control flow edges.
func buildCFG(cfg *CFGView, node *ast.File, filePath string, fset *token.FileSet) {
	ast.Inspect(node, func(n ast.Node) bool {
		switch t := n.(type) {
		case *ast.FuncDecl:
			// Entry node
			entryID := filePath + ":entry:" + t.Name.Name
			cfg.Nodes = append(cfg.Nodes, CFGNode{
				ID:       entryID,
				Type:     "entry",
				Function: t.Name.Name,
				File:     filePath,
				Line:     fset.Position(t.Pos()).Line,
			})

			// Walk function body for branches and loops
			if t.Body != nil {
				buildCFGFromBody(cfg, t.Body, t.Name.Name, filePath, entryID, fset)
			}
		case *ast.IfStmt:
			// Branch node
			branchID := filePath + ":branch:" + itoa(fset.Position(t.Pos()).Line)
			cfg.Nodes = append(cfg.Nodes, CFGNode{
				ID:   branchID,
				Type: "branch",
				File: filePath,
				Line: fset.Position(t.Pos()).Line,
			})
		case *ast.ForStmt, *ast.RangeStmt:
			// Loop node
			loopID := filePath + ":loop:" + itoa(fset.Position(n.Pos()).Line)
			cfg.Nodes = append(cfg.Nodes, CFGNode{
				ID:   loopID,
				Type: "loop",
				File: filePath,
				Line: fset.Position(n.Pos()).Line,
			})
		}
		return true
	})
}

func buildCFGFromBody(cfg *CFGView, body *ast.BlockStmt, funcName, filePath, prevID string, fset *token.FileSet) {
	currentID := prevID

	for _, stmt := range body.List {
		switch s := stmt.(type) {
		case *ast.IfStmt:
			branchID := filePath + ":branch:" + itoa(fset.Position(s.Pos()).Line)
			cfg.Edges = append(cfg.Edges, CFGEdge{
				Source: currentID,
				Target: branchID,
				Type:   "branch_true",
			})
			currentID = branchID
		case *ast.ForStmt:
			loopID := filePath + ":loop:" + itoa(fset.Position(s.Pos()).Line)
			cfg.Edges = append(cfg.Edges, CFGEdge{
				Source: currentID,
				Target: loopID,
				Type:   "loop_body",
			})
			currentID = loopID
		case *ast.ReturnStmt:
			exitID := filePath + ":exit:" + funcName
			cfg.Edges = append(cfg.Edges, CFGEdge{
				Source: currentID,
				Target: exitID,
				Type:   "sequential",
			})
		}
	}
}

// buildCallGraph constructs call edges between functions.
func buildCallGraph(call *CallView, node *ast.File, filePath string, fset *token.FileSet) {
	// Track function definitions
	funcDefs := make(map[string]*CallNode)

	// First pass: collect all function definitions
	ast.Inspect(node, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			callNode := CallNode{
				ID:       filePath + ":" + fn.Name.Name,
				Name:     fn.Name.Name,
				File:     filePath,
				Line:     fset.Position(fn.Pos()).Line,
				IsExport: fn.Name.IsExported(),
			}
			call.Nodes = append(call.Nodes, callNode)
			funcDefs[fn.Name.Name] = &callNode
		}
		return true
	})

	// Second pass: find call expressions
	ast.Inspect(node, func(n ast.Node) bool {
		if callExpr, ok := n.(*ast.CallExpr); ok {
			if fn, ok := callExpr.Fun.(*ast.Ident); ok {
				if target, exists := funcDefs[fn.Name]; exists {
					// Find the enclosing function
					enclosing := findEnclosingFunc(node, callExpr.Pos(), fset)
					if enclosing != "" {
						call.Edges = append(call.Edges, CallEdge{
							Caller: filePath + ":" + enclosing,
							Callee: target.ID,
							File:   filePath,
							Line:   fset.Position(callExpr.Pos()).Line,
						})
					}
				}
			}
		}
		return true
	})
}

func findEnclosingFunc(node *ast.File, pos token.Pos, fset *token.FileSet) string {
	var result string
	ast.Inspect(node, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			if fn.Pos() <= pos && pos <= fn.End() {
				result = fn.Name.Name
			}
		}
		return true
	})
	return result
}

func itoa(i int) string {
	return strings.TrimLeft(strings.Replace(string(rune(i/10+'0'))+string(rune(i%10+'0')), "", "", -1), "")
}
