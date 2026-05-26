package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/codegraph"
)

// CodeGraphTool provides tree-sitter based code intelligence.
type CodeGraphTool struct{}

func (CodeGraphTool) Name() string        { return "CodeGraph" }
func (CodeGraphTool) RiskLevel() string   { return "low" }
func (CodeGraphTool) Aliases() []string   { return []string{"cg", "graph"} }
func (CodeGraphTool) Description() string { return "Query the code knowledge graph: search symbols, trace callers/callees, compute impact radius, build context." }
func (CodeGraphTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"search", "callers", "callees", "impact", "context", "index", "sync", "trace", "explore", "files", "status", "stats"},
				"description": "Action: search/find symbols, callers/who calls, callees/what it calls, impact/breakage radius, context/build task context, index/full re-index, sync/incremental update, trace/call path A→B, explore/multi-symbol source, files/list indexed, status/health check, stats/counts",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query, symbol name, or task description (for context/trace use 'from -> to')",
			},
			"node_id": map[string]interface{}{
				"type":        "string",
				"description": "Node ID for callers/callees/impact",
			},
			"max_depth": map[string]interface{}{
				"type":        "integer",
				"description": "Max traversal depth (default: 3)",
			},
			"max_nodes": map[string]interface{}{
				"type":        "integer",
				"description": "Max nodes to return (default: 30)",
			},
			"root": map[string]interface{}{
				"type":        "string",
				"description": "Project root directory (default: current dir)",
			},
			"from": map[string]interface{}{
				"type":        "string",
				"description": "Source symbol for trace action",
			},
			"to": map[string]interface{}{
				"type":        "string",
				"description": "Target symbol for trace action",
			},
			"max_files": map[string]interface{}{
				"type":        "integer",
				"description": "Max files for explore action (default: 10)",
			},
			"dir": map[string]interface{}{
				"type":        "string",
				"description": "Directory filter for files action",
			},
		},
		"required": []string{"action"},
	}
}

func (CodeGraphTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Action   string `json:"action"`
		Query    string `json:"query"`
		NodeID   string `json:"node_id"`
		MaxDepth int    `json:"max_depth"`
		MaxNodes int    `json:"max_nodes"`
		Root     string `json:"root"`
		From     string `json:"from"`
		To       string `json:"to"`
		MaxFiles int    `json:"max_files"`
		Dir      string `json:"dir"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}

	root := p.Root
	if root == "" {
		root = "."
	}
	if err := validatePathAllowed(ctx, root); err != nil {
		return "", err
	}
	if p.MaxDepth <= 0 {
		p.MaxDepth = 3
	}
	if p.MaxNodes <= 0 {
		p.MaxNodes = 30
	}
	if p.MaxFiles <= 0 {
		p.MaxFiles = 10
	}

	cg, err := codegraph.Open(root)
	if err != nil {
		return "", fmt.Errorf("codegraph: %w", err)
	}
	defer cg.Close()

	switch p.Action {
	case "index":
		return indexCodeGraph(cg, root)
	case "sync":
		return syncCodeGraph(cg)
	case "search":
		return searchCodeGraph(cg, p.Query, p.MaxNodes)
	case "callers":
		return callersCodeGraph(cg, p.NodeID, p.MaxDepth)
	case "callees":
		return calleesCodeGraph(cg, p.NodeID, p.MaxDepth)
	case "impact":
		return impactCodeGraph(cg, p.NodeID, p.MaxDepth)
	case "context":
		return contextCodeGraph(cg, p.Query, p.MaxNodes)
	case "trace":
		return traceCodeGraph(cg, p.From, p.To)
	case "explore":
		return exploreCodeGraph(cg, p.Query, p.MaxFiles)
	case "files":
		return filesCodeGraph(cg, p.Dir)
	case "status":
		return statusCodeGraph(cg)
	case "stats":
		return statsCodeGraph(cg)
	default:
		return "", fmt.Errorf("unknown action: %s", p.Action)
	}
}

func indexCodeGraph(cg *codegraph.CodeGraph, root string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}

	// Check if already indexed
	dbPath := filepath.Join(absRoot, ".codegraph", "codegraph.db")
	if _, err := os.Stat(dbPath); err == nil {
		// Already indexed, do incremental sync
		return "CodeGraph index already exists. Use 'search' or 'context' to query it.", nil
	}

	if err := cg.IndexDir(absRoot); err != nil {
		return "", fmt.Errorf("indexing failed: %w", err)
	}

	stats, _ := cg.Stats()
	return fmt.Sprintf("Indexed %d files, %d symbols, %d edges", stats["files"], stats["nodes"], stats["edges"]), nil
}

func searchCodeGraph(cg *codegraph.CodeGraph, query string, limit int) (string, error) {
	if query == "" {
		return "", fmt.Errorf("query is required for search")
	}

	nodes, err := cg.Search(query, limit)
	if err != nil {
		return "", err
	}

	if len(nodes) == 0 {
		return fmt.Sprintf("No symbols found for '%s'", query), nil
	}

	var result string
	result += fmt.Sprintf("## Search Results for '%s' (%d found)\n\n", query, len(nodes))
	for _, n := range nodes {
		sig := n.Signature
		if sig == "" {
			sig = n.QualifiedName
		}
		result += fmt.Sprintf("- **%s** `%s` in %s:%d\n", n.Kind, sig, n.FilePath, n.StartLine)
	}
	return result, nil
}

func callersCodeGraph(cg *codegraph.CodeGraph, nodeID string, depth int) (string, error) {
	if nodeID == "" {
		return "", fmt.Errorf("node_id is required for callers")
	}

	nodes, err := cg.GetCallers(nodeID, depth)
	if err != nil {
		return "", err
	}

	if len(nodes) == 0 {
		return "No callers found", nil
	}

	var result string
	result += fmt.Sprintf("## Callers (%d found)\n\n", len(nodes))
	for _, n := range nodes {
		result += fmt.Sprintf("- **%s** `%s` in %s:%d\n", n.Kind, n.QualifiedName, n.FilePath, n.StartLine)
	}
	return result, nil
}

func calleesCodeGraph(cg *codegraph.CodeGraph, nodeID string, depth int) (string, error) {
	if nodeID == "" {
		return "", fmt.Errorf("node_id is required for callees")
	}

	nodes, err := cg.GetCallees(nodeID, depth)
	if err != nil {
		return "", err
	}

	if len(nodes) == 0 {
		return "No callees found", nil
	}

	var result string
	result += fmt.Sprintf("## Callees (%d found)\n\n", len(nodes))
	for _, n := range nodes {
		result += fmt.Sprintf("- **%s** `%s` in %s:%d\n", n.Kind, n.QualifiedName, n.FilePath, n.StartLine)
	}
	return result, nil
}

func impactCodeGraph(cg *codegraph.CodeGraph, nodeID string, depth int) (string, error) {
	if nodeID == "" {
		return "", fmt.Errorf("node_id is required for impact")
	}

	nodes, err := cg.GetImpactRadius(nodeID, depth)
	if err != nil {
		return "", err
	}

	if len(nodes) == 0 {
		return "No impact found", nil
	}

	var result string
	result += fmt.Sprintf("## Impact Radius (%d nodes affected)\n\n", len(nodes))
	for _, n := range nodes {
		result += fmt.Sprintf("- **%s** `%s` in %s:%d\n", n.Kind, n.QualifiedName, n.FilePath, n.StartLine)
	}
	return result, nil
}

func contextCodeGraph(cg *codegraph.CodeGraph, query string, maxNodes int) (string, error) {
	if query == "" {
		return "", fmt.Errorf("query is required for context")
	}

	return cg.BuildContext(query, maxNodes)
}

func statsCodeGraph(cg *codegraph.CodeGraph) (string, error) {
	stats, err := cg.Stats()
	if err != nil {
		return "", err
	}

	var result string
	result += "## CodeGraph Statistics\n\n"
	result += fmt.Sprintf("- Files: %d\n", stats["files"])
	result += fmt.Sprintf("- Nodes: %d\n", stats["nodes"])
	result += fmt.Sprintf("- Edges: %d\n", stats["edges"])

	if byKind, ok := stats["nodes_by_kind"].(map[string]int); ok {
		result += "\n### Nodes by Kind\n"
		for kind, count := range byKind {
			result += fmt.Sprintf("- %s: %d\n", kind, count)
		}
	}

	return result, nil
}

func syncCodeGraph(cg *codegraph.CodeGraph) (string, error) {
	result, err := cg.Sync()
	if err != nil {
		return "", fmt.Errorf("sync failed: %w", err)
	}

	var msg string
	msg += "## Sync Complete\n\n"
	msg += fmt.Sprintf("- Files checked: %d\n", result.FilesChecked)
	msg += fmt.Sprintf("- Added: %d\n", result.FilesAdded)
	msg += fmt.Sprintf("- Modified: %d\n", result.FilesModified)
	msg += fmt.Sprintf("- Removed: %d\n", result.FilesRemoved)
	msg += fmt.Sprintf("- Nodes updated: %d\n", result.NodesUpdated)
	msg += fmt.Sprintf("- Duration: %dms\n", result.DurationMs)

	if result.FilesAdded == 0 && result.FilesModified == 0 && result.FilesRemoved == 0 {
		msg += "\nIndex is up to date."
	}

	return msg, nil
}

func traceCodeGraph(cg *codegraph.CodeGraph, from, to string) (string, error) {
	if from == "" || to == "" {
		return "", fmt.Errorf("both 'from' and 'to' are required for trace")
	}

	path, err := cg.Trace(from, to)
	if err != nil {
		return fmt.Sprintf("No call path found from %q to %q: %v", from, to, err), nil
	}

	var result string
	result += fmt.Sprintf("## Trace: %s → %s\n\n", from, to)
	result += fmt.Sprintf("%d hops:\n\n", len(path))

	for i, n := range path {
		sig := n.QualifiedName
		if n.Signature != "" {
			sig = n.Signature
		}
		if i == 0 {
			result += fmt.Sprintf("  **START** %s `%s` in %s:%d\n", n.Kind, sig, n.FilePath, n.StartLine)
		} else if i == len(path)-1 {
			result += fmt.Sprintf("  **END** %s `%s` in %s:%d\n", n.Kind, sig, n.FilePath, n.StartLine)
		} else {
			result += fmt.Sprintf("  → %s `%s` in %s:%d\n", n.Kind, sig, n.FilePath, n.StartLine)
		}
	}

	return result, nil
}

func exploreCodeGraph(cg *codegraph.CodeGraph, query string, maxFiles int) (string, error) {
	if query == "" {
		return "", fmt.Errorf("query is required for explore")
	}

	result, err := cg.Explore(query, maxFiles)
	if err != nil {
		return "", err
	}

	var msg string
	msg += fmt.Sprintf("## Explore: %s\n\n", query)

	for filePath, nodes := range result.Files {
		msg += fmt.Sprintf("### %s\n", filePath)
		for _, n := range nodes {
			sig := n.QualifiedName
			if n.Signature != "" {
				sig = n.Signature
			}
			msg += fmt.Sprintf("- **%s** `%s` (line %d)\n", n.Kind, sig, n.StartLine)

			key := fmt.Sprintf("%s:%d", filePath, n.StartLine)
			if source, ok := result.SourceLines[key]; ok {
				msg += fmt.Sprintf("  ```\n  %s\n  ```\n", truncateLines(source, 20))
			}
		}
		msg += "\n"
	}

	return msg, nil
}

func filesCodeGraph(cg *codegraph.CodeGraph, dir string) (string, error) {
	files, err := cg.Files(dir)
	if err != nil {
		return "", err
	}

	var msg string
	msg += "## Indexed Files\n\n"
	if len(files) == 0 {
		msg += "No files indexed. Run 'index' first.\n"
		return msg, nil
	}

	msg += fmt.Sprintf("%d files indexed:\n\n", len(files))
	for _, f := range files {
		msg += fmt.Sprintf("- `%s` [%s] %d symbols\n", f.Path, f.Language, f.NodeCount)
	}

	return msg, nil
}

func statusCodeGraph(cg *codegraph.CodeGraph) (string, error) {
	status, err := cg.Status()
	if err != nil {
		return "", err
	}

	var msg string
	msg += "## CodeGraph Status\n\n"
	msg += fmt.Sprintf("- Project: %s\n", status.ProjectRoot)
	msg += fmt.Sprintf("- DB: %s (%.2f MB)\n", status.DBPath, float64(status.DBSizeBytes)/1024/1024)
	msg += fmt.Sprintf("- Journal: %s\n", status.JournalMode)
	msg += fmt.Sprintf("- Up to date: %v\n", status.UpToDate)
	msg += "\n"
	msg += fmt.Sprintf("- Files: %d\n", status.Files)
	msg += fmt.Sprintf("- Nodes: %d\n", status.Nodes)
	msg += fmt.Sprintf("- Edges: %d\n", status.Edges)
	msg += fmt.Sprintf("- Unresolved refs: %d\n", status.Unresolved)

	if len(status.NodesByKind) > 0 {
		msg += "\n### Nodes by Kind\n"
		for kind, count := range status.NodesByKind {
			msg += fmt.Sprintf("- %s: %d\n", kind, count)
		}
	}

	if len(status.FilesByLang) > 0 {
		msg += "\n### Files by Language\n"
		for lang, count := range status.FilesByLang {
			msg += fmt.Sprintf("- %s: %d\n", lang, count)
		}
	}

	return msg, nil
}

func truncateLines(s string, maxLines int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return s
	}
	return strings.Join(lines[:maxLines], "\n") + "\n..."
}
