package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// DocUpdate represents a single documentation update suggestion.
type DocUpdate struct {
	File   string
	Line   int
	OldDoc string
	NewDoc string
	Symbol string
	Reason string // "signature_changed", "behavior_changed", "new_params", "outdated_reference"
}

// DocUpdater detects stale documentation and suggests updates.
type DocUpdater struct {
	mu sync.Mutex
}

// NewDocUpdater creates a new DocUpdater instance.
func NewDocUpdater() *DocUpdater {
	return &DocUpdater{}
}

// DetectStaleDocumentation compares old and new file content to find
// documentation that has become stale due to code changes.
func (du *DocUpdater) DetectStaleDocumentation(file, oldContent, newContent string) []DocUpdate {
	du.mu.Lock()
	defer du.mu.Unlock()

	var updates []DocUpdate

	oldFuncs := docUpdParseFunctions(oldContent)
	newFuncs := docUpdParseFunctions(newContent)

	// Find functions whose signature changed but doc didn't update
	for name, newFunc := range newFuncs {
		oldFunc, exists := oldFuncs[name]
		if !exists {
			continue
		}

		// Check signature changes
		if oldFunc.Signature != newFunc.Signature && oldFunc.Doc == newFunc.Doc && newFunc.Doc != "" {
			reason := "signature_changed"
			detail := docUpdDetectSignatureChangeDetail(oldFunc.Signature, newFunc.Signature)
			if detail != "" {
				reason = reason + " (" + detail + ")"
			}

			newDoc := du.GenerateDocUpdate(name, newFunc.Signature, newFunc.Doc)
			updates = append(updates, DocUpdate{
				File:   file,
				Line:   newFunc.Line,
				OldDoc: newFunc.Doc,
				NewDoc: newDoc,
				Symbol: name,
				Reason: reason,
			})
		}

		// Check for new parameters not mentioned in docs
		newParams := docUpdExtractParams(newFunc.Signature)
		oldParams := docUpdExtractParams(oldFunc.Signature)
		addedParams := docUpdDiffSlices(newParams, oldParams)
		if len(addedParams) > 0 && newFunc.Doc != "" {
			missingFromDoc := []string{}
			for _, p := range addedParams {
				paramName := docUpdExtractParamName(p)
				if paramName != "" && !strings.Contains(newFunc.Doc, paramName) {
					missingFromDoc = append(missingFromDoc, paramName)
				}
			}
			if len(missingFromDoc) > 0 {
				// Avoid duplicate if already reported as signature_changed
				alreadyReported := false
				for _, u := range updates {
					if u.Symbol == name && strings.HasPrefix(u.Reason, "signature_changed") {
						alreadyReported = true
						break
					}
				}
				if !alreadyReported {
					newDoc := du.GenerateDocUpdate(name, newFunc.Signature, newFunc.Doc)
					updates = append(updates, DocUpdate{
						File:   file,
						Line:   newFunc.Line,
						OldDoc: newFunc.Doc,
						NewDoc: newDoc,
						Symbol: name,
						Reason: "new_params",
					})
				}
			}
		}
	}

	// Find docs referencing removed symbols
	removedFuncs := []string{}
	for name := range oldFuncs {
		if _, exists := newFuncs[name]; !exists {
			removedFuncs = append(removedFuncs, name)
		}
	}

	if len(removedFuncs) > 0 {
		for name, newFunc := range newFuncs {
			if newFunc.Doc == "" {
				continue
			}
			for _, removed := range removedFuncs {
				if strings.Contains(newFunc.Doc, removed) {
					updates = append(updates, DocUpdate{
						File:   file,
						Line:   newFunc.Line,
						OldDoc: newFunc.Doc,
						NewDoc: strings.ReplaceAll(newFunc.Doc, removed, "[removed:"+removed+"]"),
						Symbol: name,
						Reason: "outdated_reference",
					})
				}
			}
		}
	}

	return updates
}

// GenerateDocUpdate creates an updated doc comment based on the new signature.
func (du *DocUpdater) GenerateDocUpdate(funcName, signature, oldDoc string) string {
	params := docUpdExtractParams(signature)
	returnType := docUpdExtractReturnType(signature)

	// Start with old doc as a base
	newDoc := oldDoc

	// If doc starts with funcName, keep that pattern
	if strings.HasPrefix(oldDoc, "// "+funcName) {
		// Extract the description after the function name
		desc := strings.TrimPrefix(oldDoc, "// "+funcName)
		desc = strings.TrimSpace(desc)

		// Check for new parameters that should be mentioned
		for _, p := range params {
			paramName := docUpdExtractParamName(p)
			if paramName == "" {
				continue
			}
			paramType := docUpdExtractParamType(p)
			if paramType != "" && !strings.Contains(newDoc, paramName) {
				// Add mention of the parameter
				if strings.Contains(paramType, "context.Context") || strings.Contains(paramType, "Context") || paramName == "ctx" {
					desc = desc + " using the provided context"
				} else {
					desc = desc + " with " + paramName
				}
			}
		}

		newDoc = "// " + funcName + " " + strings.TrimSpace(desc)
	} else if strings.HasPrefix(oldDoc, "// ") {
		// Generic doc comment, add parameter info
		base := strings.TrimPrefix(oldDoc, "// ")
		for _, p := range params {
			paramName := docUpdExtractParamName(p)
			if paramName == "" {
				continue
			}
			if !strings.Contains(oldDoc, paramName) {
				paramType := docUpdExtractParamType(p)
				if paramName == "ctx" || strings.Contains(paramType, "Context") {
					base = base + " using the provided context"
				}
			}
		}
		newDoc = "// " + strings.TrimSpace(base)
	}

	// Add return type info if not already mentioned
	if returnType != "" && !strings.Contains(newDoc, "returns") && !strings.Contains(newDoc, "Returns") {
		if strings.Contains(returnType, "error") && !strings.Contains(returnType, ",") {
			// Only error return, might not need mention
		} else if returnType != "" && returnType != "error" {
			// Has meaningful return type
		}
	}

	return newDoc
}

// ScanProjectForStaleDocs walks the project directory and finds documentation
// that references non-existent symbols.
func (du *DocUpdater) ScanProjectForStaleDocs(projectDir string) []DocUpdate {
	du.mu.Lock()
	defer du.mu.Unlock()

	var updates []DocUpdate

	// First pass: collect all defined symbols
	allSymbols := make(map[string]bool)
	var goFiles []string

	_ = filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			goFiles = append(goFiles, path)
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			funcs := docUpdParseFunctions(string(data))
			for name := range funcs {
				allSymbols[name] = true
			}
		}
		return nil
	})

	// Second pass: check doc comments for references to non-existent symbols
	symbolRefPattern := regexp.MustCompile(`\b([A-Z][a-zA-Z0-9]+)\b`)

	for _, path := range goFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)
		funcs := docUpdParseFunctions(content)

		relPath, err := filepath.Rel(projectDir, path)
		if err != nil {
			relPath = path
		}

		for name, fn := range funcs {
			if fn.Doc == "" {
				continue
			}
			// Find symbol references in the doc comment
			matches := symbolRefPattern.FindAllString(fn.Doc, -1)
			for _, ref := range matches {
				// Skip common words and the function's own name
				if ref == name || docUpdIsCommonWord(ref) {
					continue
				}
				// Check if reference looks like a symbol and doesn't exist
				if len(ref) > 2 && !allSymbols[ref] {
					updates = append(updates, DocUpdate{
						File:   relPath,
						Line:   fn.Line,
						OldDoc: fn.Doc,
						NewDoc: "",
						Symbol: name,
						Reason: "outdated_reference",
					})
					break // One report per function is enough
				}
			}
		}
	}

	return updates
}

// FormatUpdates formats documentation updates into a human-readable report.
func (du *DocUpdater) FormatUpdates(updates []DocUpdate) string {
	if len(updates) == 0 {
		return "No stale documentation found."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Stale Documentation (%d items):\n", len(updates)))
	sb.WriteString("───────────────────────────────\n")

	for i, u := range updates {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("%s:%d — %s\n", u.File, u.Line, u.Symbol))
		sb.WriteString(fmt.Sprintf("  Reason: %s\n", u.Reason))
		if u.OldDoc != "" {
			sb.WriteString(fmt.Sprintf("  Old: %q\n", u.OldDoc))
		}
		if u.NewDoc != "" {
			sb.WriteString(fmt.Sprintf("  New: %q\n", u.NewDoc))
		}
	}

	return sb.String()
}

// ApplyUpdates applies documentation fixes to file content.
func (du *DocUpdater) ApplyUpdates(updates []DocUpdate, content string) string {
	du.mu.Lock()
	defer du.mu.Unlock()

	// Sort updates by line number descending to apply from bottom to top
	// so line numbers remain valid
	sorted := make([]DocUpdate, len(updates))
	copy(sorted, updates)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Line > sorted[i].Line {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	lines := strings.Split(content, "\n")

	for _, u := range sorted {
		if u.OldDoc == "" || u.NewDoc == "" {
			continue
		}
		if u.Line < 1 || u.Line > len(lines) {
			continue
		}

		// Find the doc comment above the function line
		funcLineIdx := u.Line - 1
		// Search upward for the doc comment
		for idx := funcLineIdx - 1; idx >= 0; idx-- {
			trimmed := strings.TrimSpace(lines[idx])
			if strings.HasPrefix(trimmed, "//") {
				if strings.TrimSpace(lines[idx]) == strings.TrimSpace(u.OldDoc) {
					// Preserve indentation
					indent := lines[idx][:len(lines[idx])-len(strings.TrimLeft(lines[idx], " \t"))]
					lines[idx] = indent + u.NewDoc
					break
				}
			} else {
				break
			}
		}
	}

	return strings.Join(lines, "\n")
}

// --- internal helpers ---

type docUpdParsedFunc struct {
	Name      string
	Signature string
	Doc       string
	Line      int
}

var docUpdFuncPattern = regexp.MustCompile(`^func\s+(?:\([^)]*\)\s+)?(\w+)\s*(\([^)]*\)(?:\s*(?:\([^)]*\)|[\w.*\[\]]+))?)`)

func docUpdParseFunctions(content string) map[string]docUpdParsedFunc {
	funcs := make(map[string]docUpdParsedFunc)
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "func ") {
			continue
		}

		matches := docUpdFuncPattern.FindStringSubmatch(trimmed)
		if matches == nil {
			continue
		}

		name := matches[1]
		signature := matches[2]

		// Look for doc comment above
		doc := ""
		if i > 0 {
			docLine := i - 1
			for docLine >= 0 {
				dt := strings.TrimSpace(lines[docLine])
				if strings.HasPrefix(dt, "//") {
					doc = dt
					docLine--
				} else {
					break
				}
			}
			// Take just the last (closest to func) doc line for simplicity
			if i > 0 {
				dt := strings.TrimSpace(lines[i-1])
				if strings.HasPrefix(dt, "//") {
					doc = dt
				}
			}
		}

		funcs[name] = docUpdParsedFunc{
			Name:      name,
			Signature: signature,
			Doc:       doc,
			Line:      i + 1, // 1-indexed
		}
	}

	return funcs
}

func docUpdExtractParams(signature string) []string {
	// Extract content between first ( and matching )
	if !strings.HasPrefix(signature, "(") {
		return nil
	}

	depth := 0
	start := -1
	end := -1
	for i, ch := range signature {
		if ch == '(' {
			if depth == 0 {
				start = i + 1
			}
			depth++
		} else if ch == ')' {
			depth--
			if depth == 0 {
				end = i
				break
			}
		}
	}

	if start < 0 || end < 0 || start >= end {
		return nil
	}

	paramStr := signature[start:end]
	if strings.TrimSpace(paramStr) == "" {
		return nil
	}

	// Split by comma but respect nested parens/brackets
	params := docUpdSplitParams(paramStr)
	return params
}

func docUpdSplitParams(s string) []string {
	var params []string
	depth := 0
	current := ""
	for _, ch := range s {
		if ch == '(' || ch == '[' || ch == '{' {
			depth++
			current += string(ch)
		} else if ch == ')' || ch == ']' || ch == '}' {
			depth--
			current += string(ch)
		} else if ch == ',' && depth == 0 {
			trimmed := strings.TrimSpace(current)
			if trimmed != "" {
				params = append(params, trimmed)
			}
			current = ""
		} else {
			current += string(ch)
		}
	}
	trimmed := strings.TrimSpace(current)
	if trimmed != "" {
		params = append(params, trimmed)
	}
	return params
}

func docUpdExtractParamName(param string) string {
	parts := strings.Fields(param)
	if len(parts) == 0 {
		return ""
	}
	// In Go, parameter name comes first
	name := parts[0]
	// Skip if it looks like a type (starts with *, [], etc.)
	if strings.HasPrefix(name, "*") || strings.HasPrefix(name, "[") || strings.HasPrefix(name, "...") {
		return ""
	}
	return name
}

func docUpdExtractParamType(param string) string {
	parts := strings.Fields(param)
	if len(parts) < 2 {
		return param // might be just a type
	}
	return strings.Join(parts[1:], " ")
}

func docUpdExtractReturnType(signature string) string {
	// Find the closing paren of params, then get whatever follows
	depth := 0
	for i, ch := range signature {
		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
			if depth == 0 {
				rest := strings.TrimSpace(signature[i+1:])
				return rest
			}
		}
	}
	return ""
}

func docUpdDetectSignatureChangeDetail(oldSig, newSig string) string {
	oldParams := docUpdExtractParams(oldSig)
	newParams := docUpdExtractParams(newSig)

	added := docUpdDiffSlices(newParams, oldParams)
	removed := docUpdDiffSlices(oldParams, newParams)

	details := []string{}
	for _, p := range added {
		name := docUpdExtractParamName(p)
		if name != "" {
			details = append(details, "added "+name+" parameter")
		}
	}
	for _, p := range removed {
		name := docUpdExtractParamName(p)
		if name != "" {
			details = append(details, "removed "+name+" parameter")
		}
	}

	if len(details) > 0 {
		return strings.Join(details, ", ")
	}
	return ""
}

func docUpdDiffSlices(a, b []string) []string {
	bSet := make(map[string]bool)
	for _, item := range b {
		bSet[item] = true
	}
	var diff []string
	for _, item := range a {
		if !bSet[item] {
			diff = append(diff, item)
		}
	}
	return diff
}

func docUpdIsCommonWord(s string) bool {
	common := map[string]bool{
		"The": true, "This": true, "That": true, "These": true,
		"String": true, "Int": true, "Bool": true, "Error": true,
		"Context": true, "TODO": true, "NOTE": true, "FIXME": true,
		"See": true, "Returns": true, "Return": true, "New": true,
		"Get": true, "Set": true, "Delete": true, "Update": true,
		"Create": true, "Read": true, "Write": true, "Close": true,
		"Open": true, "Init": true, "Start": true, "Stop": true,
		"Run": true, "True": true, "False": true, "Nil": true,
		"If": true, "For": true, "Each": true, "All": true,
		"Any": true, "Not": true, "Use": true, "Used": true,
		"May": true, "Must": true, "Should": true, "Can": true,
		"Will": true, "Does": true, "Has": true, "Have": true,
		"Are": true, "Is": true, "Was": true, "Were": true,
		"Be": true, "Been": true, "Being": true, "Do": true,
	}
	return common[s]
}
