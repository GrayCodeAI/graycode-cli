//go:build cgo

package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// matchEngine implements structural matching over the same tree-sitter
// grammars codegraph uses. One parser per language, reused across files.
type matchEngine struct {
	mu        sync.Mutex
	parsers   map[string]*sitter.Parser
	languages map[string]*sitter.Language
}

func newMatchEngine() *matchEngine {
	return &matchEngine{
		parsers:   map[string]*sitter.Parser{},
		languages: map[string]*sitter.Language{},
	}
}

func (e *matchEngine) lang(name string) (*sitter.Language, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if l, ok := e.languages[name]; ok {
		return l, true
	}
	var l *sitter.Language
	switch name {
	case "go":
		l = golang.GetLanguage()
	case "python":
		l = python.GetLanguage()
	case "typescript":
		l = typescript.GetLanguage()
	case "tsx":
		l = tsx.GetLanguage()
	default:
		return nil, false
	}
	e.languages[name] = l
	return l, true
}

func (e *matchEngine) parser(name string) (*sitter.Parser, bool) {
	e.mu.Lock()
	if p, ok := e.parsers[name]; ok {
		e.mu.Unlock()
		return p, true
	}
	e.mu.Unlock()

	l, ok := e.lang(name)
	if !ok {
		return nil, false
	}
	p := sitter.NewParser()
	p.SetLanguage(l)
	e.mu.Lock()
	if existing, exists := e.parsers[name]; exists {
		e.mu.Unlock()
		p.Close()
		return existing, true
	}
	e.parsers[name] = p
	e.mu.Unlock()
	return p, true
}

// compileQuery compiles a tree-sitter query pattern for one language.
func (e *matchEngine) compileQuery(langName, pattern string) (*sitter.Query, error) {
	l, ok := e.lang(langName)
	if !ok {
		return nil, fmt.Errorf("unsupported language %q", langName)
	}
	q, err := sitter.NewQuery([]byte(pattern), l)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern for %s: %w", langName, err)
	}
	return q, nil
}

// matchSource runs the query over one source buffer and returns hits.
func (e *matchEngine) matchSource(ctx context.Context, langName string, q *sitter.Query, path, src string, limit int) ([]codeMatchHit, error) {
	parser, ok := e.parser(langName)
	if !ok {
		return nil, fmt.Errorf("unsupported language %q", langName)
	}
	tree, err := parser.ParseCtx(ctx, nil, []byte(src))
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	qc := sitter.NewQueryCursor()
	defer qc.Close()
	qc.Exec(q, tree.RootNode())

	lines := strings.Split(src, "\n")
	var hits []codeMatchHit
	for {
		m, ok := qc.NextMatch()
		if !ok || (limit > 0 && len(hits) >= limit) {
			break
		}
		if len(m.Captures) == 0 {
			continue
		}
		startByte := int(m.Captures[0].Node.StartByte())
		endByte := int(m.Captures[0].Node.EndByte())
		startLine := int(m.Captures[0].Node.StartPoint().Row) + 1
		endLine := int(m.Captures[0].Node.EndPoint().Row) + 1

		names := make([]string, 0, len(m.Captures))
		seen := map[string]bool{}
		for _, c := range m.Captures {
			if c.Node == nil {
				continue
			}
			cname := q.CaptureNameForId(c.Index)
			if seen[cname] {
				continue
			}
			seen[cname] = true
			names = append(names, cname)
			if int(c.Node.StartByte()) < startByte {
				startByte = int(c.Node.StartByte())
				startLine = int(c.Node.StartPoint().Row) + 1
			}
			if int(c.Node.EndByte()) > endByte {
				endByte = int(c.Node.EndByte())
				endLine = int(c.Node.EndPoint().Row) + 1
			}
		}

		const maxSnippetLines = 12
		lo := startLine - 1
		if lo < 0 {
			lo = 0
		}
		if endLine > len(lines) {
			endLine = len(lines)
		}
		snippetLines := lines[lo:endLine]
		if len(snippetLines) > maxSnippetLines {
			snippetLines = append(append([]string{}, snippetLines[:maxSnippetLines]...), "... (truncated)")
		}

		hits = append(hits, codeMatchHit{
			File:      path,
			StartLine: startLine,
			EndLine:   endLine,
			Captures:  names,
			Snippet:   strings.Join(snippetLines, "\n"),
		})
	}
	return hits, nil
}

func runCodeMatch(ctx context.Context, root, pattern, language string, limit int) (string, error) {
	engine := newMatchEngine()
	langs := []string{}
	if language != "" {
		if _, ok := engine.lang(language); !ok {
			return "", fmt.Errorf("unsupported language %q (supported: go, python, typescript, tsx)", language)
		}
		langs = append(langs, language)
	} else {
		langs = append(langs, "go", "python", "typescript", "tsx")
	}
	queries := make(map[string]*sitter.Query, len(langs))
	for _, ln := range langs {
		q, err := engine.compileQuery(ln, pattern)
		if err != nil {
			return "", err
		}
		queries[ln] = q
		defer q.Close()
	}
	var allHits []codeMatchHit
	filesScanned := 0
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == ".hawk" || name == "__pycache__" || name == "dist" || name == "target" {
				return filepath.SkipDir
			}
			return nil
		}
		langName, ok := codeMatchExtLang[strings.ToLower(filepath.Ext(path))]
		if !ok || queries[langName] == nil || len(allHits) >= limit {
			return nil
		}
		src, err := os.ReadFile(path) // #nosec G304 -- workspace-relative walk path
		if err != nil {
			return nil
		}
		filesScanned++
		hits, err := engine.matchSource(ctx, langName, queries[langName], path, string(src), limit-len(allHits))
		if err == nil {
			allHits = append(allHits, hits...)
		}
		return nil
	})
	if walkErr != nil {
		return "", fmt.Errorf("walk: %w", walkErr)
	}
	sort.SliceStable(allHits, func(i, j int) bool {
		if allHits[i].File != allHits[j].File {
			return allHits[i].File < allHits[j].File
		}
		return allHits[i].StartLine < allHits[j].StartLine
	})
	payload := map[string]interface{}{"pattern": pattern, "files_scanned": filesScanned, "matches": len(allHits), "truncated": len(allHits) >= limit, "hits": allHits}
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}
