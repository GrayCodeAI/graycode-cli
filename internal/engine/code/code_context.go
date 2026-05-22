package code

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

type CodeSnippet struct {
	File      string
	StartLine int
	EndLine   int
	Content   string
	Relevance float64
	Type      string
	Symbol    string
}

type CodeContext struct {
	Snippets    []CodeSnippet
	TotalTokens int
	Query       string
	mu          sync.RWMutex
}

type ContextExtractor struct {
	ProjectDir string
	MaxTokens  int
	mu         sync.Mutex
}

func NewContextExtractor(projectDir string, maxTokens int) *ContextExtractor {
	if maxTokens <= 0 {
		maxTokens = 8000
	}
	return &ContextExtractor{
		ProjectDir: projectDir,
		MaxTokens:  maxTokens,
	}
}

func (ce *ContextExtractor) ExtractForTask(task string) (*CodeContext, error) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	ctx := &CodeContext{
		Query: task,
	}

	symbols := ce.FindRelevantSymbols(task, 20)
	if len(symbols) == 0 {
		return ctx, nil
	}

	ranked := ce.RankSnippets(symbols, task)

	totalTokens := 0
	for _, snip := range ranked {
		tokens := codeCtxEstimateTokens(snip.Content)
		if totalTokens+tokens > ce.MaxTokens {
			break
		}
		totalTokens += tokens
		ctx.Snippets = append(ctx.Snippets, snip)
	}
	ctx.TotalTokens = totalTokens

	return ctx, nil
}

func (ce *ContextExtractor) ExtractFunction(file, funcName string) (*CodeSnippet, error) {
	fullPath := ce.resolvePath(file)
	lines, err := readFileLines(fullPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", file, err)
	}

	funcPattern := regexp.MustCompile(`^func\s+(\([^)]*\)\s+)?` + regexp.QuoteMeta(funcName) + `\s*[\[(]`)

	startLine := -1
	for i, line := range lines {
		if funcPattern.MatchString(line) {
			startLine = i
			break
		}
	}
	if startLine == -1 {
		return nil, fmt.Errorf("function %s not found in %s", funcName, file)
	}

	endLine := findBlockEnd(lines, startLine)

	content := strings.Join(lines[startLine:endLine+1], "\n")
	return &CodeSnippet{
		File:      file,
		StartLine: startLine + 1,
		EndLine:   endLine + 1,
		Content:   content,
		Relevance: 1.0,
		Type:      "function",
		Symbol:    funcName,
	}, nil
}

func (ce *ContextExtractor) ExtractType(file, typeName string) (*CodeSnippet, error) {
	fullPath := ce.resolvePath(file)
	lines, err := readFileLines(fullPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", file, err)
	}

	typePattern := regexp.MustCompile(`^type\s+` + regexp.QuoteMeta(typeName) + `\s+`)
	startLine := -1
	for i, line := range lines {
		if typePattern.MatchString(line) {
			startLine = i
			break
		}
	}
	if startLine == -1 {
		return nil, fmt.Errorf("type %s not found in %s", typeName, file)
	}

	endLine := startLine
	if strings.Contains(lines[startLine], "{") {
		endLine = findBlockEnd(lines, startLine)
	}

	methodPattern := regexp.MustCompile(`^func\s+\([^)]*\*?` + regexp.QuoteMeta(typeName) + `\)\s+`)
	var methodBlocks []string
	for i, line := range lines {
		if i <= endLine {
			continue
		}
		if methodPattern.MatchString(line) {
			mEnd := findBlockEnd(lines, i)
			methodBlocks = append(methodBlocks, strings.Join(lines[i:mEnd+1], "\n"))
		}
	}

	content := strings.Join(lines[startLine:endLine+1], "\n")
	if len(methodBlocks) > 0 {
		content += "\n\n" + strings.Join(methodBlocks, "\n\n")
	}

	return &CodeSnippet{
		File:      file,
		StartLine: startLine + 1,
		EndLine:   endLine + 1,
		Content:   content,
		Relevance: 1.0,
		Type:      "type",
		Symbol:    typeName,
	}, nil
}

func (ce *ContextExtractor) ExtractImports(file string) (*CodeSnippet, error) {
	fullPath := ce.resolvePath(file)
	lines, err := readFileLines(fullPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", file, err)
	}

	startLine := -1
	endLine := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "import (" {
			startLine = i
			for j := i; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) == ")" {
					endLine = j
					break
				}
			}
			break
		} else if strings.HasPrefix(trimmed, "import ") && !strings.Contains(trimmed, "(") {
			startLine = i
			endLine = i
			break
		}
	}

	if startLine == -1 {
		return nil, fmt.Errorf("no import block found in %s", file)
	}

	content := strings.Join(lines[startLine:endLine+1], "\n")
	return &CodeSnippet{
		File:      file,
		StartLine: startLine + 1,
		EndLine:   endLine + 1,
		Content:   content,
		Relevance: 0.5,
		Type:      "import",
		Symbol:    "imports",
	}, nil
}

func (ce *ContextExtractor) ExtractSurrounding(file string, line, contextLines int) (*CodeSnippet, error) {
	fullPath := ce.resolvePath(file)
	lines, err := readFileLines(fullPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", file, err)
	}

	if line < 1 || line > len(lines) {
		return nil, fmt.Errorf("line %d out of range (file has %d lines)", line, len(lines))
	}

	start := line - 1 - contextLines
	if start < 0 {
		start = 0
	}
	end := line - 1 + contextLines
	if end >= len(lines) {
		end = len(lines) - 1
	}

	content := strings.Join(lines[start:end+1], "\n")
	return &CodeSnippet{
		File:      file,
		StartLine: start + 1,
		EndLine:   end + 1,
		Content:   content,
		Relevance: 0.7,
		Type:      "block",
		Symbol:    fmt.Sprintf("L%d±%d", line, contextLines),
	}, nil
}

func (ce *ContextExtractor) FindRelevantSymbols(query string, limit int) []CodeSnippet {
	keywords := extractKeywords(query)
	if len(keywords) == 0 {
		return nil
	}

	var allSnippets []CodeSnippet

	matchedFiles := ce.grepForFiles(keywords)

	for _, file := range matchedFiles {
		snippets := ce.extractSymbolsFromFile(file, keywords)
		allSnippets = append(allSnippets, snippets...)
	}

	seen := make(map[string]bool)
	var unique []CodeSnippet
	for _, s := range allSnippets {
		key := s.File + ":" + s.Symbol
		if !seen[key] {
			seen[key] = true
			unique = append(unique, s)
		}
	}

	if len(unique) > limit {
		unique = unique[:limit]
	}
	return unique
}

func (ce *ContextExtractor) RankSnippets(snippets []CodeSnippet, query string) []CodeSnippet {
	keywords := extractKeywords(query)
	if len(keywords) == 0 {
		return snippets
	}

	for i := range snippets {
		score := scoreSnippet(&snippets[i], keywords)
		snippets[i].Relevance = score
	}

	sort.Slice(snippets, func(i, j int) bool {
		return snippets[i].Relevance > snippets[j].Relevance
	})

	return snippets
}

func FormatContext(ctx *CodeContext) string {
	if ctx == nil || len(ctx.Snippets) == 0 {
		return ""
	}

	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("## Relevant Code Context\n\n")

	for _, snip := range ctx.Snippets {
		header := snip.File
		if snip.Symbol != "" {
			header += ":" + snip.Symbol
		}
		sb.WriteString(fmt.Sprintf("### %s (relevance: %.2f)\n", header, snip.Relevance))
		sb.WriteString("```go\n")
		sb.WriteString(snip.Content)
		if !strings.HasSuffix(snip.Content, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("```\n\n")
	}

	return sb.String()
}

func codeCtxEstimateTokens(content string) int {
	if content == "" {
		return 0
	}
	chars := len(content)
	tokens := (chars + 3) / 4
	if tokens == 0 {
		tokens = 1
	}
	return tokens
}

func (ce *ContextExtractor) resolvePath(file string) string {
	if filepath.IsAbs(file) {
		return file
	}
	return filepath.Join(ce.ProjectDir, file)
}

func readFileLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func findBlockEnd(lines []string, startLine int) int {
	depth := 0
	for i := startLine; i < len(lines); i++ {
		for _, ch := range lines[i] {
			if ch == '{' {
				depth++
			} else if ch == '}' {
				depth--
				if depth == 0 {
					return i
				}
			}
		}
	}
	return len(lines) - 1
}

func extractKeywords(query string) []string {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "shall": true, "can": true, "need": true,
		"to": true, "of": true, "in": true, "for": true, "on": true,
		"with": true, "at": true, "by": true, "from": true, "that": true,
		"this": true, "it": true, "and": true, "or": true, "but": true,
		"not": true, "if": true, "then": true, "else": true, "when": true,
		"up": true, "so": true, "no": true, "as": true, "i": true,
		"me": true, "my": true, "we": true, "our": true, "you": true,
		"your": true, "they": true, "them": true, "their": true,
	}

	splitter := regexp.MustCompile(`[^a-zA-Z0-9_]+`)
	words := splitter.Split(strings.ToLower(query), -1)

	var keywords []string
	seen := make(map[string]bool)
	for _, w := range words {
		if len(w) < 2 {
			continue
		}
		if stopWords[w] {
			continue
		}
		if !seen[w] {
			seen[w] = true
			keywords = append(keywords, w)
		}
	}
	return keywords
}

func (ce *ContextExtractor) grepForFiles(keywords []string) []string {
	if len(keywords) == 0 {
		return nil
	}

	pattern := strings.Join(keywords, "|")

	cmd := exec.CommandContext(context.Background(), "grep", "-rl", "--include=*.go", "-E", pattern, ce.ProjectDir)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var files []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		rel, err := filepath.Rel(ce.ProjectDir, line)
		if err != nil {
			rel = line
		}
		if !seen[rel] {
			seen[rel] = true
			files = append(files, rel)
		}
	}

	if len(files) > 30 {
		files = files[:30]
	}
	return files
}

func (ce *ContextExtractor) extractSymbolsFromFile(file string, keywords []string) []CodeSnippet {
	fullPath := ce.resolvePath(file)
	lines, err := readFileLines(fullPath)
	if err != nil {
		return nil
	}

	funcPattern := regexp.MustCompile(`^func\s+(?:\([^)]*\)\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*[\[(]`)
	typePattern := regexp.MustCompile(`^type\s+([A-Za-z_][A-Za-z0-9_]*)\s+`)

	var snippets []CodeSnippet

	for i, line := range lines {
		if matches := funcPattern.FindStringSubmatch(line); matches != nil {
			funcName := matches[1]
			if symbolMatchesKeywords(funcName, keywords) {
				endLine := findBlockEnd(lines, i)
				content := strings.Join(lines[i:endLine+1], "\n")
				snippets = append(snippets, CodeSnippet{
					File:      file,
					StartLine: i + 1,
					EndLine:   endLine + 1,
					Content:   content,
					Type:      "function",
					Symbol:    funcName,
				})
			}
		}

		if matches := typePattern.FindStringSubmatch(line); matches != nil {
			typeName := matches[1]
			if symbolMatchesKeywords(typeName, keywords) {
				endLine := i
				if strings.Contains(line, "{") {
					endLine = findBlockEnd(lines, i)
				}
				content := strings.Join(lines[i:endLine+1], "\n")
				snippets = append(snippets, CodeSnippet{
					File:      file,
					StartLine: i + 1,
					EndLine:   endLine + 1,
					Content:   content,
					Type:      "type",
					Symbol:    typeName,
				})
			}
		}
	}

	return snippets
}

func symbolMatchesKeywords(symbol string, keywords []string) bool {
	lower := strings.ToLower(symbol)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func scoreSnippet(snip *CodeSnippet, keywords []string) float64 {
	if len(keywords) == 0 {
		return 0.0
	}

	score := 0.0
	lowerContent := strings.ToLower(snip.Content)
	lowerSymbol := strings.ToLower(snip.Symbol)

	matchCount := 0
	for _, kw := range keywords {
		if strings.Contains(lowerContent, kw) {
			matchCount++
		}
		if strings.Contains(lowerSymbol, kw) {
			score += 0.15
		}
	}
	score += float64(matchCount) / float64(len(keywords)) * 0.5

	if len(snip.Symbol) > 0 && snip.Symbol[0] >= 'A' && snip.Symbol[0] <= 'Z' {
		score += 0.1
	}

	if snip.Type == "function" {
		score += 0.05
	}

	lineCount := snip.EndLine - snip.StartLine + 1
	if lineCount > 100 {
		score -= 0.1
	}

	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}
	return score
}
