package codegraph

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	tstype "github.com/smacker/go-tree-sitter/typescript/typescript"
)

// CodeGraph is a tree-sitter based code knowledge graph.
// It parses source code into a graph of symbols and edges,
// stored in SQLite with FTS5 for fast search.
type CodeGraph struct {
	db       *sql.DB
	mu       sync.RWMutex
	root     string
	parser   *sitter.Parser
	extracts map[string]*LanguageExtractor
}

// Node represents a code symbol (function, class, method, etc.).
type Node struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	QualifiedName string `json:"qualified_name"`
	FilePath      string `json:"file_path"`
	Language      string `json:"language"`
	StartLine     int    `json:"start_line"`
	EndLine       int    `json:"end_line"`
	Signature     string `json:"signature"`
	Docstring     string `json:"docstring"`
	Visibility    string `json:"visibility"`
	IsExported    bool   `json:"is_exported"`
}

// Edge represents a relationship between two nodes.
type Edge struct {
	ID       int    `json:"id"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	Kind     string `json:"kind"`
	Line     int    `json:"line"`
	Metadata string `json:"metadata"`
}

// LanguageExtractor defines how to extract symbols from a language's AST.
type LanguageExtractor struct {
	FunctionTypes  []string
	ClassTypes     []string
	MethodTypes    []string
	InterfaceTypes []string
	StructTypes    []string
	EnumTypes      []string
	TypeAliasTypes []string
	ImportTypes    []string
	CallTypes      []string
	VariableTypes  []string

	NameField   string
	BodyField   string
	ParamsField string

	GetSignature  func(node *sitter.Node, source []byte) string
	GetVisibility func(node *sitter.Node, source []byte) string
	IsExported    func(node *sitter.Node, source []byte) bool
	ExtractImport func(node *sitter.Node, source []byte) (fromPath string, names []string)
}

// Open opens or creates a CodeGraph database at the given path.
func Open(root string) (*CodeGraph, error) {
	dbPath := filepath.Join(root, ".codegraph", "codegraph.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}

	cg := &CodeGraph{
		db:       db,
		root:     root,
		parser:   sitter.NewParser(),
		extracts: make(map[string]*LanguageExtractor),
	}

	if err := cg.createSchema(); err != nil {
		return nil, err
	}

	cg.registerLanguages()
	return cg, nil
}

// Close closes the database connection.
func (cg *CodeGraph) Close() error {
	return cg.db.Close()
}

// createSchema creates the SQLite tables and indexes.
func (cg *CodeGraph) createSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY,
		kind TEXT NOT NULL,
		name TEXT NOT NULL,
		qualified_name TEXT NOT NULL,
		file_path TEXT NOT NULL,
		language TEXT NOT NULL,
		start_line INTEGER,
		end_line INTEGER,
		signature TEXT,
		docstring TEXT,
		visibility TEXT,
		is_exported INTEGER DEFAULT 0,
		updated_at INTEGER
	);

	CREATE INDEX IF NOT EXISTS idx_nodes_kind ON nodes(kind);
	CREATE INDEX IF NOT EXISTS idx_nodes_name ON nodes(name);
	CREATE INDEX IF NOT EXISTS idx_nodes_qualified ON nodes(qualified_name);
	CREATE INDEX IF NOT EXISTS idx_nodes_file ON nodes(file_path);
	CREATE INDEX IF NOT EXISTS idx_nodes_language ON nodes(language);

	CREATE VIRTUAL TABLE IF NOT EXISTS nodes_fts USING fts5(
		id, name, qualified_name, docstring, signature,
		content='nodes', content_rowid='rowid'
	);

	CREATE TABLE IF NOT EXISTS edges (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
		target TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
		kind TEXT NOT NULL,
		line INTEGER,
		metadata TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source, kind);
	CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target, kind);
	CREATE INDEX IF NOT EXISTS idx_edges_kind ON edges(kind);

	CREATE TABLE IF NOT EXISTS files (
		path TEXT PRIMARY KEY,
		content_hash TEXT NOT NULL,
		language TEXT NOT NULL,
		size INTEGER,
		modified_at INTEGER,
		indexed_at INTEGER,
		node_count INTEGER DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS unresolved_refs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		from_node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
		reference_name TEXT NOT NULL,
		reference_kind TEXT NOT NULL,
		line INTEGER,
		file_path TEXT,
		language TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_unresolved_from ON unresolved_refs(from_node_id);
	CREATE INDEX IF NOT EXISTS idx_unresolved_name ON unresolved_refs(reference_name);

	-- Triggers for FTS5 sync
	CREATE TRIGGER IF NOT EXISTS nodes_ai AFTER INSERT ON nodes BEGIN
		INSERT INTO nodes_fts(rowid, id, name, qualified_name, docstring, signature)
		VALUES (new.rowid, new.id, new.name, new.qualified_name, new.docstring, new.signature);
	END;
	CREATE TRIGGER IF NOT EXISTS nodes_ad AFTER DELETE ON nodes BEGIN
		INSERT INTO nodes_fts(nodes_fts, rowid, id, name, qualified_name, docstring, signature)
		VALUES ('delete', old.rowid, old.id, old.name, old.qualified_name, old.docstring, old.signature);
	END;
	CREATE TRIGGER IF NOT EXISTS nodes_au AFTER UPDATE ON nodes BEGIN
		INSERT INTO nodes_fts(nodes_fts, rowid, id, name, qualified_name, docstring, signature)
		VALUES ('delete', old.rowid, old.id, old.name, old.qualified_name, old.docstring, old.signature);
		INSERT INTO nodes_fts(rowid, id, name, qualified_name, docstring, signature)
		VALUES (new.rowid, new.id, new.name, new.qualified_name, new.docstring, new.signature);
	END;
	`
	_, err := cg.db.Exec(schema)
	return err
}

// registerLanguages registers language-specific extractors.
func (cg *CodeGraph) registerLanguages() {
	cg.extracts[".go"] = &LanguageExtractor{
		FunctionTypes:  []string{"function_declaration"},
		MethodTypes:    []string{"method_declaration"},
		TypeAliasTypes: []string{"type_spec"},
		ImportTypes:    []string{"import_declaration"},
		CallTypes:      []string{"call_expression"},
		VariableTypes:  []string{"var_declaration", "short_var_declaration", "const_declaration"},
		NameField:      "name",
		BodyField:      "body",
		GetSignature: func(node *sitter.Node, source []byte) string {
			return extractGoSignature(node, source)
		},
		IsExported: func(node *sitter.Node, source []byte) bool {
			name := node.ChildByFieldName("name")
			if name == nil {
				return false
			}
			n := string(source[name.StartByte():name.EndByte()])
			return len(n) > 0 && n[0] >= 'A' && n[0] <= 'Z'
		},
	}

	cg.extracts[".py"] = &LanguageExtractor{
		FunctionTypes: []string{"function_definition"},
		ClassTypes:    []string{"class_definition"},
		ImportTypes:   []string{"import_statement", "import_from_statement"},
		CallTypes:     []string{"call"},
		NameField:     "name",
		BodyField:     "body",
		IsExported: func(node *sitter.Node, source []byte) bool {
			return true // Python is always "exported"
		},
	}

	cg.extracts[".ts"] = &LanguageExtractor{
		FunctionTypes:  []string{"function_declaration"},
		ClassTypes:     []string{"class_declaration"},
		MethodTypes:    []string{"method_definition"},
		InterfaceTypes: []string{"interface_declaration"},
		TypeAliasTypes: []string{"type_alias_declaration"},
		ImportTypes:    []string{"import_statement"},
		CallTypes:      []string{"call_expression"},
		VariableTypes:  []string{"lexical_declaration", "variable_declaration"},
		NameField:      "name",
		BodyField:      "body",
		IsExported: func(node *sitter.Node, source []byte) bool {
			// Check for export keyword
			parent := node.Parent()
			if parent != nil && parent.Type() == "export_statement" {
				return true
			}
			return false
		},
	}

	cg.extracts[".tsx"] = cg.extracts[".ts"]
	cg.extracts[".js"] = cg.extracts[".ts"]
	cg.extracts[".jsx"] = cg.extracts[".ts"]

	cg.extracts[".rs"] = &LanguageExtractor{
		FunctionTypes:  []string{"function_item"},
		StructTypes:    []string{"struct_item"},
		EnumTypes:      []string{"enum_item"},
		InterfaceTypes: []string{"trait_item"},
		ImportTypes:    []string{"use_declaration"},
		CallTypes:      []string{"call_expression"},
		NameField:      "name",
		BodyField:      "body",
		IsExported: func(node *sitter.Node, source []byte) bool {
			return true // Rust uses pub keyword, simplified here
		},
	}
}

// IndexFile parses a file and stores its symbols in the graph.
func (cg *CodeGraph) IndexFile(filePath string) error {
	ext := filepath.Ext(filePath)
	extractor, ok := cg.extracts[ext]
	if !ok {
		return nil // unsupported language
	}

	lang := getLanguage(ext)
	cg.parser.SetLanguage(lang)

	source, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	tree, err := cg.parser.ParseCtx(nil, nil, source)
	if err != nil {
		return err
	}
	defer tree.Close()

	// Delete old nodes for this file
	cg.mu.Lock()
	_, _ = cg.db.Exec("DELETE FROM nodes WHERE file_path = ?", filePath)
	_, _ = cg.db.Exec("DELETE FROM edges WHERE source IN (SELECT id FROM nodes WHERE file_path = ?)", filePath)
	cg.mu.Unlock()

	// Walk AST and extract symbols
	var nodes []Node
	var edges []Edge
	var unresolvedRefs []UnresolvedRef
	var nodeStack []string // qualified name stack

	var walk func(node *sitter.Node, depth int)
	walk = func(node *sitter.Node, depth int) {
		nodeType := node.Type()

		// Try to extract a symbol
		if n := cg.extractNode(node, source, filePath, ext, extractor, nodeStack); n != nil {
			nodes = append(nodes, *n)

			// Create contains edge from parent
			if len(nodeStack) > 0 {
				parentID := generateNodeID(filePath, "container", nodeStack[len(nodeStack)-1], 0)
				edges = append(edges, Edge{
					Source: parentID,
					Target: n.ID,
					Kind:   "contains",
					Line:   n.StartLine,
				})
			}

			// Push to stack for children
			nodeStack = append(nodeStack, n.QualifiedName)
			defer func() { nodeStack = nodeStack[:len(nodeStack)-1] }()
		}

		// Extract calls
		if contains(extractor.CallTypes, nodeType) {
			if len(nodeStack) > 0 {
				callee := extractCalleeName(node, source)
				if callee != "" {
					fromID := generateNodeID(filePath, "function", nodeStack[len(nodeStack)-1], 0)
					unresolvedRefs = append(unresolvedRefs, UnresolvedRef{
						FromNodeID:    fromID,
						ReferenceName: callee,
						ReferenceKind: "calls",
						Line:          int(node.StartPoint().Row) + 1,
						FilePath:      filePath,
						Language:      ext,
					})
				}
			}
		}

		// Recurse into children
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			walk(child, depth+1)
		}
	}

	root := tree.RootNode()
	walk(root, 0)

	// Store in database
	cg.mu.Lock()
	defer cg.mu.Unlock()

	for _, n := range nodes {
		_, err := cg.db.Exec(
			`INSERT OR REPLACE INTO nodes (id, kind, name, qualified_name, file_path, language, start_line, end_line, signature, docstring, visibility, is_exported, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			n.ID, n.Kind, n.Name, n.QualifiedName, n.FilePath, n.Language,
			n.StartLine, n.EndLine, n.Signature, n.Docstring, n.Visibility,
			boolToInt(n.IsExported), time.Now().Unix(),
		)
		if err != nil {
			return err
		}
	}

	for _, e := range edges {
		_, err := cg.db.Exec(
			`INSERT INTO edges (source, target, kind, line, metadata) VALUES (?, ?, ?, ?, ?)`,
			e.Source, e.Target, e.Kind, e.Line, e.Metadata,
		)
		if err != nil {
			return err
		}
	}

	for _, ref := range unresolvedRefs {
		_, err := cg.db.Exec(
			`INSERT INTO unresolved_refs (from_node_id, reference_name, reference_kind, line, file_path, language)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			ref.FromNodeID, ref.ReferenceName, ref.ReferenceKind, ref.Line, ref.FilePath, ref.Language,
		)
		if err != nil {
			return err
		}
	}

	// Update file record
	hash := sha256Sum(source)
	_, _ = cg.db.Exec(
		`INSERT OR REPLACE INTO files (path, content_hash, language, size, modified_at, indexed_at, node_count)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		filePath, hash, ext, len(source), time.Now().Unix(), time.Now().Unix(), len(nodes),
	)

	return nil
}

// IndexDir indexes all supported source files in a directory.
func (cg *CodeGraph) IndexDir(dir string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "node_modules" || base == "vendor" || base == "dist" || base == "build" || base == ".codegraph" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if cg.extracts[ext] != nil {
			return cg.IndexFile(path)
		}
		return nil
	})
}

// Search finds symbols by name using FTS5.
func (cg *CodeGraph) Search(query string, limit int) ([]Node, error) {
	if limit <= 0 {
		limit = 20
	}

	cg.mu.RLock()
	defer cg.mu.RUnlock()

	rows, err := cg.db.Query(
		`SELECT n.id, n.kind, n.name, n.qualified_name, n.file_path, n.language,
		        n.start_line, n.end_line, n.signature, n.docstring, n.visibility, n.is_exported
		 FROM nodes_fts fts
		 JOIN nodes n ON n.id = fts.id
		 WHERE nodes_fts MATCH ?
		 ORDER BY rank
		 LIMIT ?`, query+"*", limit)
	if err != nil {
		// Fallback to LIKE search
		rows, err = cg.db.Query(
			`SELECT id, kind, name, qualified_name, file_path, language,
			        start_line, end_line, signature, docstring, visibility, is_exported
			 FROM nodes WHERE name LIKE ? ORDER BY name LIMIT ?`,
			"%"+query+"%", limit)
		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()

	return scanNodes(rows)
}

// GetCallers returns nodes that call the given node.
func (cg *CodeGraph) GetCallers(nodeID string, maxDepth int) ([]Node, error) {
	if maxDepth <= 0 {
		maxDepth = 3
	}

	cg.mu.RLock()
	defer cg.mu.RUnlock()

	var result []Node
	visited := make(map[string]bool)
	queue := []string{nodeID}

	for len(queue) > 0 && len(result) < 100 {
		current := queue[0]
		queue = queue[1:]

		if visited[current] {
			continue
		}
		visited[current] = true

		rows, err := cg.db.Query(
			`SELECT n.id, n.kind, n.name, n.qualified_name, n.file_path, n.language,
			        n.start_line, n.end_line, n.signature, n.docstring, n.visibility, n.is_exported
			 FROM edges e JOIN nodes n ON n.id = e.source
			 WHERE e.target = ? AND e.kind IN ('calls', 'references')
			 LIMIT 50`, current)
		if err != nil {
			continue
		}

		nodes, _ := scanNodes(rows)
		rows.Close()

		for _, n := range nodes {
			if !visited[n.ID] {
				result = append(result, n)
				queue = append(queue, n.ID)
			}
		}
	}

	return result, nil
}

// GetCallees returns nodes that the given node calls.
func (cg *CodeGraph) GetCallees(nodeID string, maxDepth int) ([]Node, error) {
	if maxDepth <= 0 {
		maxDepth = 3
	}

	cg.mu.RLock()
	defer cg.mu.RUnlock()

	var result []Node
	visited := make(map[string]bool)
	queue := []string{nodeID}

	for len(queue) > 0 && len(result) < 100 {
		current := queue[0]
		queue = queue[1:]

		if visited[current] {
			continue
		}
		visited[current] = true

		rows, err := cg.db.Query(
			`SELECT n.id, n.kind, n.name, n.qualified_name, n.file_path, n.language,
			        n.start_line, n.end_line, n.signature, n.docstring, n.visibility, n.is_exported
			 FROM edges e JOIN nodes n ON n.id = e.target
			 WHERE e.source = ? AND e.kind IN ('calls', 'references')
			 LIMIT 50`, current)
		if err != nil {
			continue
		}

		nodes, _ := scanNodes(rows)
		rows.Close()

		for _, n := range nodes {
			if !visited[n.ID] {
				result = append(result, n)
				queue = append(queue, n.ID)
			}
		}
	}

	return result, nil
}

// GetImpactRadius returns all nodes affected by changing the given node.
func (cg *CodeGraph) GetImpactRadius(nodeID string, maxDepth int) ([]Node, error) {
	if maxDepth <= 0 {
		maxDepth = 3
	}

	cg.mu.RLock()
	defer cg.mu.RUnlock()

	// Get the focal node
	var focalNode Node
	err := cg.db.QueryRow(
		`SELECT id, kind, name, qualified_name, file_path, language,
		        start_line, end_line, signature, docstring, visibility, is_exported
		 FROM nodes WHERE id = ?`, nodeID).Scan(
		&focalNode.ID, &focalNode.Kind, &focalNode.Name, &focalNode.QualifiedName,
		&focalNode.FilePath, &focalNode.Language, &focalNode.StartLine, &focalNode.EndLine,
		&focalNode.Signature, &focalNode.Docstring, &focalNode.Visibility, &focalNode.IsExported,
	)
	if err != nil {
		return nil, err
	}

	// Container expansion: if this is a class/struct, include children
	containerKinds := map[string]bool{
		"class": true, "interface": true, "struct": true, "trait": true, "enum": true,
	}

	var result []Node
	visited := make(map[string]bool)
	type step struct {
		nodeID string
		depth  int
	}
	queue := []step{{nodeID: nodeID, depth: 0}}

	for len(queue) > 0 && len(result) < 200 {
		s := queue[0]
		queue = queue[1:]

		if visited[s.nodeID] || s.depth > maxDepth {
			continue
		}
		visited[s.nodeID] = true

		// Get node
		var n Node
		err := cg.db.QueryRow(
			`SELECT id, kind, name, qualified_name, file_path, language,
			        start_line, end_line, signature, docstring, visibility, is_exported
			 FROM nodes WHERE id = ?`, s.nodeID).Scan(
			&n.ID, &n.Kind, &n.Name, &n.QualifiedName, &n.FilePath, &n.Language,
			&n.StartLine, &n.EndLine, &n.Signature, &n.Docstring, &n.Visibility, &n.IsExported,
		)
		if err != nil {
			continue
		}
		result = append(result, n)

		// If container, expand children at same depth
		if containerKinds[n.Kind] {
			childRows, _ := cg.db.Query(
				`SELECT target FROM edges WHERE source = ? AND kind = 'contains'`, s.nodeID)
			if childRows != nil {
				for childRows.Next() {
					var childID string
					childRows.Scan(&childID)
					if !visited[childID] {
						queue = append(queue, step{nodeID: childID, depth: s.depth})
					}
				}
				childRows.Close()
			}
		}

		// Traverse incoming edges (things that depend on this node)
		depRows, _ := cg.db.Query(
			`SELECT source FROM edges WHERE target = ? AND kind IN ('calls', 'references', 'imports', 'extends', 'implements')`, s.nodeID)
		if depRows != nil {
			for depRows.Next() {
				var depID string
				depRows.Scan(&depID)
				if !visited[depID] {
					queue = append(queue, step{nodeID: depID, depth: s.depth + 1})
				}
			}
			depRows.Close()
		}
	}

	return result, nil
}

// BuildContext builds relevant context for a natural language query.
func (cg *CodeGraph) BuildContext(query string, maxNodes int) (string, error) {
	if maxNodes <= 0 {
		maxNodes = 30
	}

	// Step 1: Extract symbols from query
	terms := extractSearchTerms(query)

	// Step 2: Hybrid search
	entryPoints := make(map[string]Node)
	for _, term := range terms {
		nodes, err := cg.Search(term, 5)
		if err != nil {
			continue
		}
		for _, n := range nodes {
			entryPoints[n.ID] = n
		}
	}

	if len(entryPoints) == 0 {
		return "", fmt.Errorf("no symbols found for query: %s", query)
	}

	// Step 3: BFS expansion from entry points
	visited := make(map[string]bool)
	var contextNodes []Node

	for id := range entryPoints {
		radius, _ := cg.GetImpactRadius(id, 2)
		for _, n := range radius {
			if !visited[n.ID] && len(contextNodes) < maxNodes {
				visited[n.ID] = true
				contextNodes = append(contextNodes, n)
			}
		}
	}

	// Step 4: Format as markdown
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Code Context for: %s\n\n", query))

	// Group by file
	byFile := make(map[string][]Node)
	for _, n := range contextNodes {
		byFile[n.FilePath] = append(byFile[n.FilePath], n)
	}

	for filePath, nodes := range byFile {
		b.WriteString(fmt.Sprintf("### %s\n", filePath))
		for _, n := range nodes {
			rel := n.QualifiedName
			if n.Signature != "" {
				rel = n.Signature
			}
			b.WriteString(fmt.Sprintf("- **%s** `%s` (line %d)\n", n.Kind, rel, n.StartLine))
			if n.Docstring != "" {
				b.WriteString(fmt.Sprintf("  %s\n", truncate(n.Docstring, 100)))
			}
		}
		b.WriteString("\n")
	}

	return b.String(), nil
}

// ResolveRefs resolves unresolved references to build call graph edges.
func (cg *CodeGraph) ResolveRefs() error {
	cg.mu.Lock()
	defer cg.mu.Unlock()

	rows, err := cg.db.Query(`SELECT id, from_node_id, reference_name, reference_kind, line, file_path, language FROM unresolved_refs`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type ref struct {
		id       int
		fromID   string
		name     string
		kind     string
		line     int
		filePath string
		lang     string
	}

	var refs []ref
	for rows.Next() {
		var r ref
		rows.Scan(&r.id, &r.fromID, &r.name, &r.kind, &r.line, &r.filePath, &r.lang)
		refs = append(refs, r)
	}

	resolved := 0
	for _, r := range refs {
		// Try to find target by name
		var targetID string
		err := cg.db.QueryRow(
			`SELECT id FROM nodes WHERE name = ? OR qualified_name LIKE ? LIMIT 1`,
			r.name, "%"+r.name,
		).Scan(&targetID)
		if err != nil {
			continue
		}

		// Create edge
		_, err = cg.db.Exec(
			`INSERT INTO edges (source, target, kind, line) VALUES (?, ?, ?, ?)`,
			r.fromID, targetID, r.kind, r.line,
		)
		if err == nil {
			resolved++
		}
	}

	// Clean up resolved refs
	_, _ = cg.db.Exec(`DELETE FROM unresolved_refs`)

	return nil
}

// Stats returns graph statistics.
func (cg *CodeGraph) Stats() (map[string]interface{}, error) {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	stats := make(map[string]interface{})

	var nodeCount, edgeCount, fileCount int
	cg.db.QueryRow("SELECT COUNT(*) FROM nodes").Scan(&nodeCount)
	cg.db.QueryRow("SELECT COUNT(*) FROM edges").Scan(&edgeCount)
	cg.db.QueryRow("SELECT COUNT(*) FROM files").Scan(&fileCount)

	stats["nodes"] = nodeCount
	stats["edges"] = edgeCount
	stats["files"] = fileCount

	// Nodes by kind
	rows, _ := cg.db.Query("SELECT kind, COUNT(*) FROM nodes GROUP BY kind ORDER BY COUNT(*) DESC")
	if rows != nil {
		byKind := make(map[string]int)
		for rows.Next() {
			var kind string
			var count int
			rows.Scan(&kind, &count)
			byKind[kind] = count
		}
		rows.Close()
		stats["nodes_by_kind"] = byKind
	}

	return stats, nil
}

// Helper types and functions

type UnresolvedRef struct {
	FromNodeID    string
	ReferenceName string
	ReferenceKind string
	Line          int
	FilePath      string
	Language      string
}

func (cg *CodeGraph) extractNode(node *sitter.Node, source []byte, filePath, ext string, extractor *LanguageExtractor, nodeStack []string) *Node {
	nodeType := node.Type()

	var kind, name string
	switch {
	case contains(extractor.FunctionTypes, nodeType):
		kind = "function"
	case contains(extractor.ClassTypes, nodeType):
		kind = "class"
	case contains(extractor.MethodTypes, nodeType):
		kind = "method"
	case contains(extractor.InterfaceTypes, nodeType):
		kind = "interface"
	case contains(extractor.StructTypes, nodeType):
		kind = "struct"
	case contains(extractor.EnumTypes, nodeType):
		kind = "enum"
	case contains(extractor.TypeAliasTypes, nodeType):
		kind = "type_alias"
	default:
		return nil
	}

	// Get name
	nameNode := node.ChildByFieldName(extractor.NameField)
	if nameNode == nil {
		return nil
	}
	name = string(source[nameNode.StartByte():nameNode.EndByte()])

	// Build qualified name
	qn := name
	if len(nodeStack) > 0 {
		qn = strings.Join(nodeStack, "::") + "::" + name
	}

	// Get signature
	sig := ""
	if extractor.GetSignature != nil {
		sig = extractor.GetSignature(node, source)
	}

	// Get visibility
	vis := ""
	if extractor.GetVisibility != nil {
		vis = extractor.GetVisibility(node, source)
	}

	// Is exported
	exported := false
	if extractor.IsExported != nil {
		exported = extractor.IsExported(node, source)
	}

	// Get docstring (look for comment sibling above)
	docstring := extractDocstring(node, source)

	return &Node{
		ID:            generateNodeID(filePath, kind, qn, int(node.StartPoint().Row)+1),
		Kind:          kind,
		Name:          name,
		QualifiedName: qn,
		FilePath:      filePath,
		Language:      ext,
		StartLine:     int(node.StartPoint().Row) + 1,
		EndLine:       int(node.EndPoint().Row) + 1,
		Signature:     sig,
		Docstring:     docstring,
		Visibility:    vis,
		IsExported:    exported,
	}
}

func extractGoSignature(node *sitter.Node, source []byte) string {
	// Extract the full function signature line
	start := node.StartByte()
	bodyNode := node.ChildByFieldName("body")
	if bodyNode != nil {
		// Get everything before the body
		return strings.TrimSpace(string(source[start:bodyNode.StartByte()]))
	}
	// Fallback: get the first line
	text := string(source[start:node.EndByte()])
	lines := strings.SplitN(text, "\n", 2)
	return strings.TrimSpace(lines[0])
}

func extractCalleeName(node *sitter.Node, source []byte) string {
	// For call_expression, get the function name
	funcNode := node.ChildByFieldName("function")
	if funcNode == nil {
		// Try first child
		if node.NamedChildCount() > 0 {
			funcNode = node.NamedChild(0)
		}
	}
	if funcNode == nil {
		return ""
	}

	text := string(source[funcNode.StartByte():funcNode.EndByte()])
	// Remove arguments part
	if idx := strings.Index(text, "("); idx > 0 {
		text = text[:idx]
	}
	return strings.TrimSpace(text)
}

func extractDocstring(node *sitter.Node, source []byte) string {
	// Look for comment node before this node
	parent := node.Parent()
	if parent == nil {
		return ""
	}

	for i := 0; i < int(parent.NamedChildCount()); i++ {
		child := parent.NamedChild(i)
		if child.Equal(node) && i > 0 {
			prev := parent.NamedChild(i - 1)
			if prev.Type() == "comment" || prev.Type() == "block_comment" {
				text := string(source[prev.StartByte():prev.EndByte()])
				// Clean comment markers
				text = strings.TrimPrefix(text, "//")
				text = strings.TrimPrefix(text, "/*")
				text = strings.TrimSuffix(text, "*/")
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

func generateNodeID(filePath, kind, name string, line int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s:%d", filePath, kind, name, line)))
	return fmt.Sprintf("%x", h[:8])
}

func sha256Sum(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:8])
}

func getLanguage(ext string) *sitter.Language {
	switch ext {
	case ".go":
		return golang.GetLanguage()
	case ".py":
		return python.GetLanguage()
	case ".ts":
		return tstype.GetLanguage()
	case ".tsx", ".js", ".jsx":
		return tsx.GetLanguage()
	default:
		return golang.GetLanguage() // fallback
	}
}

func extractSearchTerms(query string) []string {
	// Split on spaces and camelCase boundaries
	var terms []string
	words := strings.Fields(query)
	for _, w := range words {
		// Split camelCase
		parts := splitCamelCase(w)
		terms = append(terms, parts...)
	}
	return terms
}

func splitCamelCase(s string) []string {
	var parts []string
	var current strings.Builder

	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func scanNodes(rows *sql.Rows) ([]Node, error) {
	var nodes []Node
	for rows.Next() {
		var n Node
		err := rows.Scan(
			&n.ID, &n.Kind, &n.Name, &n.QualifiedName, &n.FilePath, &n.Language,
			&n.StartLine, &n.EndLine, &n.Signature, &n.Docstring, &n.Visibility, &n.IsExported,
		)
		if err != nil {
			continue
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// SyncResult holds the result of an incremental sync.
type SyncResult struct {
	FilesChecked  int `json:"files_checked"`
	FilesAdded    int `json:"files_added"`
	FilesModified int `json:"files_modified"`
	FilesRemoved  int `json:"files_removed"`
	NodesUpdated  int `json:"nodes_updated"`
	DurationMs    int `json:"duration_ms"`
}

// Sync performs an incremental sync — only re-indexes files whose content hash
// has changed since the last index. Removes files that no longer exist.
func (cg *CodeGraph) Sync() (*SyncResult, error) {
	start := time.Now()
	result := &SyncResult{}

	// Get currently tracked files
	trackedFiles := make(map[string]string) // path -> content_hash
	rows, err := cg.db.Query("SELECT path, content_hash FROM files")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var path, hash string
		rows.Scan(&path, &hash)
		trackedFiles[path] = hash
	}
	rows.Close()

	// Scan current files
	currentFiles := make(map[string]bool)
	err = filepath.WalkDir(cg.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "node_modules" || base == "vendor" || base == "dist" || base == "build" || base == ".codegraph" || base == "target" || base == "__pycache__" || base == ".venv" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if cg.extracts[ext] != nil {
			relPath, _ := filepath.Rel(cg.root, path)
			currentFiles[relPath] = true
			result.FilesChecked++

			// Check if file changed
			source, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			hash := sha256Sum(source)

			oldHash, exists := trackedFiles[relPath]
			if !exists {
				// New file
				if err := cg.IndexFile(path); err == nil {
					result.FilesAdded++
					result.NodesUpdated++
				}
			} else if oldHash != hash {
				// Modified file
				if err := cg.IndexFile(path); err == nil {
					result.FilesModified++
					result.NodesUpdated++
				}
			}
			// else: unchanged, skip
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Remove files that no longer exist
	cg.mu.Lock()
	for trackedPath := range trackedFiles {
		if !currentFiles[trackedPath] {
			absPath := filepath.Join(cg.root, trackedPath)
			relForDelete := trackedPath
			cg.db.Exec("DELETE FROM nodes WHERE file_path = ?", absPath)
			cg.db.Exec("DELETE FROM edges WHERE source IN (SELECT id FROM nodes WHERE file_path = ?)", absPath)
			cg.db.Exec("DELETE FROM files WHERE path = ?", relForDelete)
			result.FilesRemoved++
		}
	}
	cg.mu.Unlock()

	result.DurationMs = int(time.Since(start).Milliseconds())
	return result, nil
}

// Trace finds the shortest call path between two symbols.
// Returns the chain of nodes from 'from' to 'to', or nil if no path exists.
func (cg *CodeGraph) Trace(fromName, toName string) ([]Node, error) {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	// Find source nodes
	fromNodes, err := cg.searchByName(fromName, 5)
	if err != nil || len(fromNodes) == 0 {
		return nil, fmt.Errorf("symbol %q not found", fromName)
	}

	// Find target nodes
	toNodes, err := cg.searchByName(toName, 5)
	if err != nil || len(toNodes) == 0 {
		return nil, fmt.Errorf("symbol %q not found", toName)
	}

	toIDs := make(map[string]bool)
	for _, n := range toNodes {
		toIDs[n.ID] = true
	}

	// BFS from each source to find shortest path
	type step struct {
		nodeID string
		path   []string
	}

	for _, from := range fromNodes {
		visited := make(map[string]bool)
		queue := []step{{nodeID: from.ID, path: []string{from.ID}}}

		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]

			if visited[current.nodeID] {
				continue
			}
			visited[current.nodeID] = true

			if toIDs[current.nodeID] {
				// Found path — load full nodes
				var path []Node
				for _, id := range current.path {
					var n Node
					err := cg.db.QueryRow(
						`SELECT id, kind, name, qualified_name, file_path, language,
						        start_line, end_line, signature, docstring, visibility, is_exported
						 FROM nodes WHERE id = ?`, id).Scan(
						&n.ID, &n.Kind, &n.Name, &n.QualifiedName, &n.FilePath, &n.Language,
						&n.StartLine, &n.EndLine, &n.Signature, &n.Docstring, &n.Visibility, &n.IsExported,
					)
					if err == nil {
						path = append(path, n)
					}
				}
				return path, nil
			}

			// Expand via call edges
			edgeRows, _ := cg.db.Query(
				`SELECT target FROM edges WHERE source = ? AND kind IN ('calls', 'references') LIMIT 20`, current.nodeID)
			if edgeRows != nil {
				for edgeRows.Next() {
					var nextID string
					edgeRows.Scan(&nextID)
					if !visited[nextID] {
						newPath := make([]string, len(current.path)+1)
						copy(newPath, current.path)
						newPath[len(current.path)] = nextID
						queue = append(queue, step{nodeID: nextID, path: newPath})
					}
				}
				edgeRows.Close()
			}
		}
	}

	return nil, fmt.Errorf("no call path from %q to %q", fromName, toName)
}

// ExploreResult holds source code for multiple symbols grouped by file.
type ExploreResult struct {
	Files       map[string][]Node `json:"files"`
	SourceLines map[string]string `json:"source_lines"` // file:line -> source snippet
}

// Explore returns source code for several related symbols grouped by file.
func (cg *CodeGraph) Explore(query string, maxFiles int) (*ExploreResult, error) {
	if maxFiles <= 0 {
		maxFiles = 10
	}

	// Search for symbols
	nodes, err := cg.Search(query, maxFiles*3)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no symbols found for %q", query)
	}

	// Group by file
	byFile := make(map[string][]Node)
	for _, n := range nodes {
		byFile[n.FilePath] = append(byFile[n.FilePath], n)
	}

	// Limit files
	result := &ExploreResult{
		Files:       make(map[string][]Node),
		SourceLines: make(map[string]string),
	}

	count := 0
	for filePath, fileNodes := range byFile {
		if count >= maxFiles {
			break
		}

		// Read source file
		absPath := filePath
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(cg.root, filePath)
		}
		source, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		lines := strings.Split(string(source), "\n")

		result.Files[filePath] = fileNodes

		// Extract source snippets for each node
		for _, n := range fileNodes {
			startIdx := n.StartLine - 1
			endIdx := n.EndLine
			if startIdx >= 0 && endIdx <= len(lines) {
				snippet := strings.Join(lines[startIdx:endIdx], "\n")
				if len(snippet) > 2000 {
					snippet = snippet[:2000] + "\n... (truncated)"
				}
				key := fmt.Sprintf("%s:%d", filePath, n.StartLine)
				result.SourceLines[key] = snippet
			}
		}
		count++
	}

	return result, nil
}

// FileEntry represents a tracked file in the index.
type FileEntry struct {
	Path      string `json:"path"`
	Language  string `json:"language"`
	Size      int    `json:"size"`
	NodeCount int    `json:"node_count"`
	IndexedAt int    `json:"indexed_at"`
}

// Files returns the list of all indexed files.
func (cg *CodeGraph) Files(dirFilter string) ([]FileEntry, error) {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	query := "SELECT path, language, size, node_count, indexed_at FROM files"
	args := []interface{}{}

	if dirFilter != "" {
		query += " WHERE path LIKE ?"
		args = append(args, dirFilter+"%")
	}
	query += " ORDER BY path"

	rows, err := cg.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []FileEntry
	for rows.Next() {
		var f FileEntry
		rows.Scan(&f.Path, &f.Language, &f.Size, &f.NodeCount, &f.IndexedAt)
		files = append(files, f)
	}
	return files, nil
}

// StatusResult holds detailed index health information.
type StatusResult struct {
	ProjectRoot  string         `json:"project_root"`
	DBPath       string         `json:"db_path"`
	DBSizeBytes  int64          `json:"db_size_bytes"`
	Files        int            `json:"files"`
	Nodes        int            `json:"nodes"`
	Edges        int            `json:"edges"`
	Unresolved   int            `json:"unresolved_refs"`
	NodesByKind  map[string]int `json:"nodes_by_kind"`
	FilesByLang  map[string]int `json:"files_by_lang"`
	JournalMode  string         `json:"journal_mode"`
	UpToDate     bool           `json:"up_to_date"`
}

// Status returns detailed index health and statistics.
func (cg *CodeGraph) Status() (*StatusResult, error) {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	status := &StatusResult{
		ProjectRoot: cg.root,
		DBPath:      filepath.Join(cg.root, ".codegraph", "codegraph.db"),
		NodesByKind: make(map[string]int),
		FilesByLang: make(map[string]int),
	}

	// DB size
	if info, err := os.Stat(status.DBPath); err == nil {
		status.DBSizeBytes = info.Size()
	}

	// Counts
	cg.db.QueryRow("SELECT COUNT(*) FROM nodes").Scan(&status.Nodes)
	cg.db.QueryRow("SELECT COUNT(*) FROM edges").Scan(&status.Edges)
	cg.db.QueryRow("SELECT COUNT(*) FROM files").Scan(&status.Files)
	cg.db.QueryRow("SELECT COUNT(*) FROM unresolved_refs").Scan(&status.Unresolved)

	// Nodes by kind
	rows, _ := cg.db.Query("SELECT kind, COUNT(*) FROM nodes GROUP BY kind ORDER BY COUNT(*) DESC")
	if rows != nil {
		for rows.Next() {
			var kind string
			var count int
			rows.Scan(&kind, &count)
			status.NodesByKind[kind] = count
		}
		rows.Close()
	}

	// Files by language
	rows, _ = cg.db.Query("SELECT language, COUNT(*) FROM files GROUP BY language ORDER BY COUNT(*) DESC")
	if rows != nil {
		for rows.Next() {
			var lang string
			var count int
			rows.Scan(&lang, &count)
			status.FilesByLang[lang] = count
		}
		rows.Close()
	}

	// Journal mode
	cg.db.QueryRow("PRAGMA journal_mode").Scan(&status.JournalMode)

	// Check if up to date (no pending changes)
	status.UpToDate = true
	fileRows, _ := cg.db.Query("SELECT path, content_hash FROM files")
	if fileRows != nil {
		for fileRows.Next() {
			var path, hash string
			fileRows.Scan(&path, &hash)
			absPath := filepath.Join(cg.root, path)
			source, err := os.ReadFile(absPath)
			if err != nil {
				status.UpToDate = false
				break
			}
			if sha256Sum(source) != hash {
				status.UpToDate = false
				break
			}
		}
		fileRows.Close()
	}

	return status, nil
}

// searchByName is an internal search that returns nodes matching a name.
func (cg *CodeGraph) searchByName(name string, limit int) ([]Node, error) {
	rows, err := cg.db.Query(
		`SELECT id, kind, name, qualified_name, file_path, language,
		        start_line, end_line, signature, docstring, visibility, is_exported
		 FROM nodes WHERE name = ? OR name LIKE ? LIMIT ?`,
		name, "%"+name+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNodes(rows)
}
