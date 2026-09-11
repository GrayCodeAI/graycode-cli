package history

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Annotation represents a temporary inline annotation added to a file during agent work.
type Annotation struct {
	ID        string
	File      string
	Line      int
	Content   string
	Type      string // "note", "todo", "question", "warning", "context"
	Author    string // "agent", "user"
	CreatedAt time.Time
	Resolved  bool
}

// AnnotationManager manages temporary annotations across files.
type AnnotationManager struct {
	Annotations map[string][]*Annotation // file → annotations
	mu          sync.RWMutex
	nextID      int
}

// NewAnnotationManager creates a new AnnotationManager.
func NewAnnotationManager() *AnnotationManager {
	return &AnnotationManager{
		Annotations: make(map[string][]*Annotation),
	}
}

// Add creates a new annotation for the given file and line.
func (am *AnnotationManager) Add(file string, line int, content, annotationType, author string) *Annotation {
	am.mu.Lock()
	defer am.mu.Unlock()

	am.nextID++
	a := &Annotation{
		ID:        fmt.Sprintf("ann-%d", am.nextID),
		File:      file,
		Line:      line,
		Content:   content,
		Type:      annotationType,
		Author:    author,
		CreatedAt: time.Now(),
		Resolved:  false,
	}

	am.Annotations[file] = append(am.Annotations[file], a)
	return a
}

// Remove deletes an annotation by ID.
func (am *AnnotationManager) Remove(id string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	for file, anns := range am.Annotations {
		for i, a := range anns {
			if a.ID == id {
				am.Annotations[file] = append(anns[:i], anns[i+1:]...)
				if len(am.Annotations[file]) == 0 {
					delete(am.Annotations, file)
				}
				return
			}
		}
	}
}

// Resolve marks an annotation as resolved.
func (am *AnnotationManager) Resolve(id string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	for _, anns := range am.Annotations {
		for _, a := range anns {
			if a.ID == id {
				a.Resolved = true
				return
			}
		}
	}
}

// GetForFile returns all annotations for a given file.
func (am *AnnotationManager) GetForFile(file string) []*Annotation {
	am.mu.RLock()
	defer am.mu.RUnlock()

	result := make([]*Annotation, len(am.Annotations[file]))
	copy(result, am.Annotations[file])
	return result
}

// GetAll returns all annotations across all files.
func (am *AnnotationManager) GetAll() []*Annotation {
	am.mu.RLock()
	defer am.mu.RUnlock()

	var result []*Annotation
	for _, anns := range am.Annotations {
		result = append(result, anns...)
	}
	return result
}

// GetUnresolved returns all unresolved annotations.
func (am *AnnotationManager) GetUnresolved() []*Annotation {
	am.mu.RLock()
	defer am.mu.RUnlock()

	var result []*Annotation
	for _, anns := range am.Annotations {
		for _, a := range anns {
			if !a.Resolved {
				result = append(result, a)
			}
		}
	}
	return result
}

// InjectAnnotations inserts annotation comments into file content at appropriate lines.
// Annotations are inserted ABOVE the target line using the correct comment syntax
// based on file extension.
func (am *AnnotationManager) InjectAnnotations(file, content string) string {
	am.mu.RLock()
	anns := am.Annotations[file]
	am.mu.RUnlock()

	if len(anns) == 0 {
		return content
	}

	// Sort annotations by line number descending so insertion doesn't shift indices.
	sorted := make([]*Annotation, len(anns))
	copy(sorted, anns)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Line > sorted[j].Line
	})

	lines := strings.Split(content, "\n")
	commentPrefix := annotationCommentPrefix(file)

	for _, a := range sorted {
		if a.Resolved {
			continue
		}
		commentLine := fmt.Sprintf("%s [hawk:%s] %s", commentPrefix, a.Type, a.Content)
		idx := a.Line - 1 // convert 1-based to 0-based
		if idx < 0 {
			idx = 0
		}
		if idx > len(lines) {
			idx = len(lines)
		}
		// Insert above the target line.
		lines = append(lines[:idx], append([]string{commentLine}, lines[idx:]...)...)
	}

	return strings.Join(lines, "\n")
}

// annotationCommentPrefix returns the comment prefix for the given file type.
func annotationCommentPrefix(file string) string {
	ext := strings.ToLower(filepath.Ext(file))
	switch ext {
	case ".py", ".rb", ".sh", ".bash", ".zsh", ".yaml", ".yml", ".toml":
		return "#"
	case ".go", ".js", ".ts", ".tsx", ".jsx", ".c", ".cpp", ".h", ".java", ".rs", ".swift", ".kt":
		return "//"
	case ".css", ".scss":
		return "/*"
	case ".html", ".xml":
		return "<!--"
	case ".lua", ".sql":
		return "--"
	default:
		return "//"
	}
}

// hawkAnnotationRe matches hawk annotation comment lines.
var hawkAnnotationRe = regexp.MustCompile(`^\s*(//|#|/\*|<!--|--)\s*\[hawk:(note|todo|question|warning|context)\]\s*(.*)$`)

// StripAnnotations removes all [hawk:*] comment lines from content.
// Used before commit/save to keep annotations agent-only.
func StripAnnotations(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	for _, line := range lines {
		if !hawkAnnotationRe.MatchString(line) {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

// DetectAnnotations finds existing hawk annotations in file content and parses them.
func DetectAnnotations(content string) []*Annotation {
	lines := strings.Split(content, "\n")
	var result []*Annotation

	for i, line := range lines {
		matches := hawkAnnotationRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		annotationType := matches[2]
		annotationContent := strings.TrimSpace(matches[3])
		result = append(result, &Annotation{
			Line:    i + 1,
			Content: annotationContent,
			Type:    annotationType,
		})
	}
	return result
}

// FormatAnnotations formats annotations for display.
func FormatAnnotations(annotations []*Annotation) string {
	if len(annotations) == 0 {
		return ""
	}

	// Group by file.
	byFile := make(map[string][]*Annotation)
	for _, a := range annotations {
		byFile[a.File] = append(byFile[a.File], a)
	}

	var sb strings.Builder
	for file, anns := range byFile {
		// Sort by line number.
		sort.Slice(anns, func(i, j int) bool {
			return anns[i].Line < anns[j].Line
		})

		sb.WriteString(fmt.Sprintf("Annotations for %s:\n", file))
		for _, a := range anns {
			sb.WriteString(fmt.Sprintf("  L%d [%s] %s\n", a.Line, a.Type, a.Content))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// BuildContextFromAnnotations formats annotations for a file for injection into agent context.
func (am *AnnotationManager) BuildContextFromAnnotations(file string) string {
	am.mu.RLock()
	anns := am.Annotations[file]
	am.mu.RUnlock()

	if len(anns) == 0 {
		return ""
	}

	// Sort by line.
	sorted := make([]*Annotation, len(anns))
	copy(sorted, anns)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Line < sorted[j].Line
	})

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Annotations for %s:\n", file))
	for _, a := range sorted {
		status := ""
		if a.Resolved {
			status = " [resolved]"
		}
		sb.WriteString(fmt.Sprintf("  L%d [%s] %s%s\n", a.Line, a.Type, a.Content, status))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// Summary returns a brief summary of all annotations.
func (am *AnnotationManager) Summary() string {
	am.mu.RLock()
	defer am.mu.RUnlock()

	total := 0
	unresolved := 0
	typeCounts := make(map[string]int)

	for _, anns := range am.Annotations {
		for _, a := range anns {
			total++
			if !a.Resolved {
				unresolved++
				typeCounts[a.Type]++
			}
		}
	}

	if total == 0 {
		return "0 annotations"
	}

	if unresolved == 0 {
		return fmt.Sprintf("%d annotations (all resolved)", total)
	}

	// Build breakdown of unresolved types.
	var parts []string
	// Order types consistently.
	typeOrder := []string{"warning", "todo", "note", "question", "context"}
	for _, t := range typeOrder {
		if count, ok := typeCounts[t]; ok {
			label := t + "s"
			if count == 1 {
				label = t
			}
			parts = append(parts, fmt.Sprintf("%d %s", count, label))
		}
	}

	breakdown := strings.Join(parts, ", ")
	return fmt.Sprintf("%d annotations (%d unresolved: %s)", total, unresolved, breakdown)
}
