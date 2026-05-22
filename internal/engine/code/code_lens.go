package code

import (
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

type CodeLens struct {
	File     string
	Line     int
	Label    string
	Category string
	Command  string
	Tooltip  string
}

type LensGenerator func(file, content string) []CodeLens

type CodeLensProvider struct {
	Providers map[string]LensGenerator
	mu        sync.RWMutex
}

func NewCodeLensProvider() *CodeLensProvider {
	p := &CodeLensProvider{
		Providers: make(map[string]LensGenerator),
	}
	p.Providers["test_status"] = GenerateTestLens
	p.Providers["complexity"] = GenerateComplexityLens
	p.Providers["references"] = GenerateReferenceLens
	p.Providers["age"] = GenerateAgeLens
	p.Providers["coverage"] = GenerateCoverageLens
	return p
}

func (p *CodeLensProvider) Register(name string, generator LensGenerator) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Providers[name] = generator
}

func (p *CodeLensProvider) Generate(file, content string) []CodeLens {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var all []CodeLens
	for _, gen := range p.Providers {
		lenses := gen(file, content)
		all = append(all, lenses...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Line != all[j].Line {
			return all[i].Line < all[j].Line
		}
		return all[i].Category < all[j].Category
	})
	return all
}

func FilterByCategory(lenses []CodeLens, category string) []CodeLens {
	var result []CodeLens
	for _, l := range lenses {
		if l.Category == category {
			result = append(result, l)
		}
	}
	return result
}

func FormatLenses(file string, lenses []CodeLens) string {
	if len(lenses) == 0 {
		return fmt.Sprintf("Code Lenses for %s:\n  (none)", file)
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Code Lenses for %s:\n", file))
	for _, l := range lenses {
		b.WriteString(fmt.Sprintf("L%-3d [%s] %s\n", l.Line, l.Label, l.Tooltip))
	}
	return strings.TrimRight(b.String(), "\n")
}

var testFuncRe = regexp.MustCompile(`(?m)^func\s+(Test\w+)\s*\(`)

func GenerateTestLens(file, content string) []CodeLens {
	if !strings.HasSuffix(file, "_test.go") {
		return nil
	}

	var lenses []CodeLens
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		matches := testFuncRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		funcName := matches[1]
		status := lookupTestStatus(file, funcName)
		label := fmt.Sprintf("test: %s", status)
		tooltip := fmt.Sprintf("func %s", funcName)
		if status == "PASS" {
			tooltip += " — passed"
		} else if status == "FAIL" {
			tooltip += " — failed"
		} else {
			tooltip += " — status unknown"
		}
		lenses = append(lenses, CodeLens{
			File:     file,
			Line:     i + 1,
			Label:    label,
			Category: "test_status",
			Command:  fmt.Sprintf("go test -run ^%s$ %s", funcName, file),
			Tooltip:  tooltip,
		})
	}
	return lenses
}

func lookupTestStatus(file, funcName string) string {
	dir := file
	if idx := strings.LastIndex(file, "/"); idx >= 0 {
		dir = file[:idx]
	}
	cmd := exec.Command("go", "test", "-run", "^"+funcName+"$", "-count=1", "-timeout=10s", dir)
	err := cmd.Run()
	if err == nil {
		return "PASS"
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() == 1 {
			return "FAIL"
		}
	}
	return "UNKNOWN"
}

var funcDeclRe = regexp.MustCompile(`(?m)^func\s+(?:\(\s*\w+\s+\*?\w+\s*\)\s+)?(\w+)\s*\(`)

func GenerateComplexityLens(file, content string) []CodeLens {
	const threshold = 5
	var lenses []CodeLens

	functions := extractFunctions(content)
	for _, fn := range functions {
		cc := calculateCyclomaticComplexity(fn.body)
		if cc > threshold {
			label := fmt.Sprintf("complexity: %d", cc)
			tooltip := fmt.Sprintf("func %s — consider splitting", fn.name)
			lenses = append(lenses, CodeLens{
				File:     file,
				Line:     fn.line,
				Label:    label,
				Category: "complexity",
				Command:  "",
				Tooltip:  tooltip,
			})
		}
	}
	return lenses
}

func GenerateReferenceLens(file, content string) []CodeLens {
	var lenses []CodeLens

	lines := strings.Split(content, "\n")
	symbols := extractExportedSymbols(content)

	for _, sym := range symbols {
		count := countReferences(content, sym.name)
		if count > 0 {
			label := fmt.Sprintf("references: %d", count)
			tooltip := fmt.Sprintf("func %s — ", sym.name)
			if count >= 5 {
				tooltip += "widely used"
			} else {
				tooltip += fmt.Sprintf("referenced %d times", count)
			}
			_ = lines
			lenses = append(lenses, CodeLens{
				File:     file,
				Line:     sym.line,
				Label:    label,
				Category: "references",
				Command:  "",
				Tooltip:  tooltip,
			})
		}
	}
	return lenses
}

func GenerateAgeLens(file, content string) []CodeLens {
	var lenses []CodeLens

	functions := extractFunctions(content)
	blameData := getGitBlame(file)

	for _, fn := range functions {
		age := lookupAge(blameData, fn.line)
		if age == "" {
			continue
		}
		label := fmt.Sprintf("age: %s", age)
		tooltip := fmt.Sprintf("func %s — ", fn.name)
		if isRecent(age) {
			tooltip += "recently modified"
		} else {
			tooltip += "last modified " + age + " ago"
		}
		lenses = append(lenses, CodeLens{
			File:     file,
			Line:     fn.line,
			Label:    label,
			Category: "age",
			Command:  fmt.Sprintf("git log --oneline -1 -L %d,%d:%s", fn.line, fn.endLine, file),
			Tooltip:  tooltip,
		})
	}
	return lenses
}

func GenerateCoverageLens(file, content string) []CodeLens {
	var lenses []CodeLens

	functions := extractFunctions(content)
	coverageData := loadCoverageData(file)
	if coverageData == nil {
		return nil
	}

	for _, fn := range functions {
		pct, ok := coverageData[fn.name]
		if !ok {
			continue
		}
		label := fmt.Sprintf("coverage: %.0f%%", pct)
		tooltip := fmt.Sprintf("func %s — ", fn.name)
		if pct >= 80 {
			tooltip += "well covered"
		} else if pct >= 50 {
			tooltip += "partially covered"
		} else {
			tooltip += "needs more tests"
		}
		lenses = append(lenses, CodeLens{
			File:     file,
			Line:     fn.line,
			Label:    label,
			Category: "coverage",
			Command:  fmt.Sprintf("go test -coverprofile=coverage.out -run . %s", file),
			Tooltip:  tooltip,
		})
	}
	return lenses
}

type funcInfo struct {
	name    string
	line    int
	endLine int
	body    string
}

type symbolInfo struct {
	name string
	line int
}

func extractFunctions(content string) []funcInfo {
	var funcs []funcInfo
	lines := strings.Split(content, "\n")

	for i := 0; i < len(lines); i++ {
		matches := funcDeclRe.FindStringSubmatch(lines[i])
		if matches == nil {
			continue
		}
		name := matches[1]
		startLine := i + 1
		braceCount := 0
		started := false
		var bodyLines []string

		for j := i; j < len(lines); j++ {
			for _, ch := range lines[j] {
				if ch == '{' {
					braceCount++
					started = true
				} else if ch == '}' {
					braceCount--
				}
			}
			bodyLines = append(bodyLines, lines[j])
			if started && braceCount == 0 {
				funcs = append(funcs, funcInfo{
					name:    name,
					line:    startLine,
					endLine: j + 1,
					body:    strings.Join(bodyLines, "\n"),
				})
				break
			}
		}
	}
	return funcs
}

func calculateCyclomaticComplexity(body string) int {
	cc := 1
	decisionPatterns := []string{
		`\bif\b`,
		`\belse if\b`,
		`\bfor\b`,
		`\bcase\b`,
		`\b&&\b`,
		`\b\|\|\b`,
		`\bselect\b`,
	}
	words := strings.Fields(body)
	for _, w := range words {
		switch w {
		case "if", "for", "case", "select":
			cc++
		}
	}
	_ = decisionPatterns
	cc += strings.Count(body, "&&")
	cc += strings.Count(body, "||")
	return cc
}

func extractExportedSymbols(content string) []symbolInfo {
	var symbols []symbolInfo
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		matches := funcDeclRe.FindStringSubmatch(line)
		if matches != nil {
			name := matches[1]
			if len(name) > 0 && unicode.IsUpper(rune(name[0])) {
				symbols = append(symbols, symbolInfo{name: name, line: i + 1})
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "type ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				name := parts[1]
				if len(name) > 0 && unicode.IsUpper(rune(name[0])) {
					symbols = append(symbols, symbolInfo{name: name, line: i + 1})
				}
			}
		}
	}
	return symbols
}

func countReferences(content, symbol string) int {
	count := strings.Count(content, symbol)
	if count > 0 {
		count--
	}
	return count
}

type blameEntry struct {
	line int
	date time.Time
}

func getGitBlame(file string) []blameEntry {
	cmd := exec.Command("git", "blame", "--porcelain", file)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var entries []blameEntry
	lines := strings.Split(string(out), "\n")
	lineNum := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "author-time ") {
			ts := strings.TrimPrefix(l, "author-time ")
			var epoch int64
			_, _ = fmt.Sscanf(ts, "%d", &epoch)
			if epoch > 0 {
				entries = append(entries, blameEntry{
					line: lineNum,
					date: time.Unix(epoch, 0),
				})
			}
		}
		parts := strings.Fields(l)
		if len(parts) >= 3 && len(parts[0]) == 40 {
			_, _ = fmt.Sscanf(parts[2], "%d", &lineNum)
		}
	}
	return entries
}

func lookupAge(entries []blameEntry, line int) string {
	if len(entries) == 0 {
		return ""
	}

	var newest time.Time
	for _, e := range entries {
		if e.line >= line && e.line <= line+20 {
			if e.date.After(newest) {
				newest = e.date
			}
		}
	}

	if newest.IsZero() {
		for _, e := range entries {
			if e.date.After(newest) {
				newest = e.date
			}
		}
	}

	if newest.IsZero() {
		return ""
	}

	return lensFormatDuration(time.Since(newest))
}

func lensFormatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days == 0 {
		hours := int(d.Hours())
		if hours == 0 {
			return "just now"
		}
		return fmt.Sprintf("%dh", hours)
	}
	if days < 7 {
		return fmt.Sprintf("%dd", days)
	}
	if days < 30 {
		return fmt.Sprintf("%dw", days/7)
	}
	if days < 365 {
		return fmt.Sprintf("%dmo", days/30)
	}
	return fmt.Sprintf("%dy", days/365)
}

func isRecent(age string) bool {
	if age == "just now" {
		return true
	}
	if strings.HasSuffix(age, "h") {
		return true
	}
	if strings.HasSuffix(age, "d") {
		var days int
		_, _ = fmt.Sscanf(age, "%dd", &days)
		return days < 7
	}
	return false
}

func loadCoverageData(file string) map[string]float64 {
	dir := file
	if idx := strings.LastIndex(file, "/"); idx >= 0 {
		dir = file[:idx]
	}
	coverFile := dir + "/coverage.out"

	cmd := exec.Command("cat", coverFile)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	return parseCoverageProfile(string(out), file)
}

func parseCoverageProfile(profile, file string) map[string]float64 {
	result := make(map[string]float64)
	lines := strings.Split(profile, "\n")

	type blockInfo struct {
		startLine int
		endLine   int
		stmts     int
		count     int
	}

	var blocks []blockInfo
	for _, line := range lines {
		if !strings.Contains(line, ":") || strings.HasPrefix(line, "mode:") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		loc := parts[0]
		if !strings.Contains(loc, file) {
			continue
		}
		colonIdx := strings.LastIndex(loc, ":")
		if colonIdx < 0 {
			continue
		}
		span := loc[colonIdx+1:]
		rangeParts := strings.Split(span, ",")
		if len(rangeParts) != 2 {
			continue
		}
		var startLine, endLine, stmts, count int
		_, _ = fmt.Sscanf(rangeParts[0], "%d", &startLine)
		_, _ = fmt.Sscanf(rangeParts[1], "%d", &endLine)
		_, _ = fmt.Sscanf(parts[1], "%d", &stmts)
		_, _ = fmt.Sscanf(parts[2], "%d", &count)
		blocks = append(blocks, blockInfo{startLine, endLine, stmts, count})
	}

	if len(blocks) == 0 {
		return nil
	}

	_ = result
	_ = blocks
	return nil
}
