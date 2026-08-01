//go:build cgo

package codegraph

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	tstype "github.com/smacker/go-tree-sitter/typescript/typescript"
)

// This file holds the standalone extraction/formatting helpers and the
// high-level read/traversal operations (Sync, Trace, Explore, Files, Status,
// searchByName, GetNode) for the CodeGraph. The CodeGraph type, schema setup,
// indexing, and the core query/traversal methods live in codegraph_cgo.go.

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
	// Rune-safe truncation: never split a multibyte UTF-8 sequence.
	if runes := []rune(s); len(runes) > maxLen {
		return string(runes[:maxLen]) + "..."
	}
	return s
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
	rows, err := cg.db.QueryContext(context.Background(), "SELECT path, content_hash FROM files")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var path, hash string
		_ = rows.Scan(&path, &hash)
		trackedFiles[path] = hash
	}
	_ = rows.Close()

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
			source, err := os.ReadFile(path) // #nosec G304,G122 -- read-only codegraph indexing scan
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
			_, _ = cg.db.ExecContext(context.Background(), "DELETE FROM nodes WHERE file_path = ?", absPath)
			_, _ = cg.db.ExecContext(context.Background(), "DELETE FROM edges WHERE source IN (SELECT id FROM nodes WHERE file_path = ?)", absPath)
			_, _ = cg.db.ExecContext(context.Background(), "DELETE FROM files WHERE path = ?", relForDelete)
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
					err := cg.db.QueryRowContext(
						context.Background(),
						`SELECT id, kind, name, qualified_name, file_path, language,
						        start_line, end_line, signature, docstring, visibility, is_exported
						 FROM nodes WHERE id = ?`, id,
					).Scan(
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
			edgeRows, _ := cg.db.QueryContext(
				context.Background(),
				`SELECT target FROM edges WHERE source = ? AND kind IN ('calls', 'references') LIMIT 20`, current.nodeID,
			)
			if edgeRows != nil {
				for edgeRows.Next() {
					var nextID string
					_ = edgeRows.Scan(&nextID)
					if !visited[nextID] {
						newPath := make([]string, len(current.path)+1)
						copy(newPath, current.path)
						newPath[len(current.path)] = nextID
						queue = append(queue, step{nodeID: nextID, path: newPath})
					}
				}
				_ = edgeRows.Close()
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
		source, err := os.ReadFile(absPath) // #nosec G304 -- absPath is derived from indexed file paths under cg.root, the project being explored
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

	rows, err := cg.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // deferred Close on read-only rows is cleanup-only

	var files []FileEntry
	for rows.Next() {
		var f FileEntry
		_ = rows.Scan(&f.Path, &f.Language, &f.Size, &f.NodeCount, &f.IndexedAt)
		files = append(files, f)
	}
	return files, nil
}

// StatusResult holds detailed index health information.
type StatusResult struct {
	ProjectRoot string         `json:"project_root"`
	DBPath      string         `json:"db_path"`
	DBSizeBytes int64          `json:"db_size_bytes"`
	Files       int            `json:"files"`
	Nodes       int            `json:"nodes"`
	Edges       int            `json:"edges"`
	Unresolved  int            `json:"unresolved_refs"`
	NodesByKind map[string]int `json:"nodes_by_kind"`
	FilesByLang map[string]int `json:"files_by_lang"`
	JournalMode string         `json:"journal_mode"`
	UpToDate    bool           `json:"up_to_date"`
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
	_ = cg.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM nodes").Scan(&status.Nodes)
	_ = cg.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM edges").Scan(&status.Edges)
	_ = cg.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM files").Scan(&status.Files)
	_ = cg.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM unresolved_refs").Scan(&status.Unresolved)

	// Nodes by kind
	rows, _ := cg.db.QueryContext(context.Background(), "SELECT kind, COUNT(*) FROM nodes GROUP BY kind ORDER BY COUNT(*) DESC")
	if rows != nil {
		for rows.Next() {
			var kind string
			var count int
			_ = rows.Scan(&kind, &count)
			status.NodesByKind[kind] = count
		}
		_ = rows.Close()
	}

	// Files by language
	rows, _ = cg.db.QueryContext(context.Background(), "SELECT language, COUNT(*) FROM files GROUP BY language ORDER BY COUNT(*) DESC")
	if rows != nil {
		for rows.Next() {
			var lang string
			var count int
			_ = rows.Scan(&lang, &count)
			status.FilesByLang[lang] = count
		}
		_ = rows.Close()
	}

	// Journal mode
	_ = cg.db.QueryRowContext(context.Background(), "PRAGMA journal_mode").Scan(&status.JournalMode)

	// Check if up to date (no pending changes)
	status.UpToDate = true
	fileRows, _ := cg.db.QueryContext(context.Background(), "SELECT path, content_hash FROM files")
	if fileRows != nil {
		for fileRows.Next() {
			var path, hash string
			_ = fileRows.Scan(&path, &hash)
			absPath := filepath.Join(cg.root, path)
			source, err := os.ReadFile(absPath) // #nosec G304 -- absPath is derived from tracked file paths under cg.root, the project being indexed
			if err != nil {
				status.UpToDate = false
				break
			}
			if sha256Sum(source) != hash {
				status.UpToDate = false
				break
			}
		}
		_ = fileRows.Close()
	}

	return status, nil
}

// searchByName is an internal search that returns nodes matching a name.
func (cg *CodeGraph) searchByName(name string, limit int) ([]Node, error) {
	rows, err := cg.db.QueryContext(
		context.Background(),
		`SELECT id, kind, name, qualified_name, file_path, language,
		        start_line, end_line, signature, docstring, visibility, is_exported
		 FROM nodes WHERE name = ? OR name LIKE ? LIMIT ?`,
		name, "%"+name+"%", limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // deferred Close on read-only rows is cleanup-only
	return scanNodes(rows)
}

// GetNode returns a single node by ID.
func (cg *CodeGraph) GetNode(id string) (Node, error) {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	var n Node
	err := cg.db.QueryRowContext(
		context.Background(),
		`SELECT id, kind, name, qualified_name, file_path, language,
		        start_line, end_line, signature, docstring, visibility, is_exported
		 FROM nodes WHERE id = ?`, id,
	).Scan(
		&n.ID, &n.Kind, &n.Name, &n.QualifiedName, &n.FilePath, &n.Language,
		&n.StartLine, &n.EndLine, &n.Signature, &n.Docstring, &n.Visibility, &n.IsExported,
	)
	return n, err
}
