package engine

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/GrayCodeAI/eyrie/client"
)

// filePathPatterns are regex patterns used to detect file paths in LLM responses.
var filePathPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?:^|\s|` + "`" + `)([a-zA-Z0-9_./-]+\.[a-zA-Z]{1,10})(?:\s|$|` + "`" + `|:|,|\))`),
	regexp.MustCompile(`"([a-zA-Z0-9_./-]+\.[a-zA-Z]{1,10})"`),
	regexp.MustCompile(`([a-zA-Z0-9_/-]+/[a-zA-Z0-9_./-]+)`),
}

// fileLinePattern matches file:line references like main.go:42.
var fileLinePattern = regexp.MustCompile(`^(.+):(\d+)$`)

// urlPattern matches HTTP/HTTPS URLs to filter them out.
var urlPattern = regexp.MustCompile(`https?://`)

// falsePositivePaths are common paths that should be ignored.
var falsePositivePaths = map[string]bool{
	"/dev/null":    true,
	"/etc/passwd":  true,
	"/etc/hosts":   true,
	"/tmp":         true,
	"/dev/random":  true,
	"/dev/urandom": true,
	"/dev/zero":    true,
	"/dev/stdin":   true,
	"/dev/stdout":  true,
	"/dev/stderr":  true,
}

// FileMentionDetector scans LLM response text for file path mentions,
// validates them against the filesystem, and suggests additions to context.
type FileMentionDetector struct {
	projectRoot string
	knownFiles  map[string]bool // cache of files we know exist
	mu          sync.RWMutex
}

// NewFileMentionDetector creates a new detector rooted at the given project directory.
func NewFileMentionDetector(projectRoot string) *FileMentionDetector {
	return &FileMentionDetector{
		projectRoot: projectRoot,
		knownFiles:  make(map[string]bool),
	}
}

// DetectMentions scans the given text for file paths, validates them on disk
// (relative to projectRoot), and returns a deduplicated list of valid paths.
func (d *FileMentionDetector) DetectMentions(text string) []string {
	candidates := d.extractCandidates(text)
	seen := make(map[string]bool)
	var results []string

	for _, candidate := range candidates {
		// Strip file:line suffix if present.
		candidate = stripLineNumber(candidate)

		// Normalize: remove leading/trailing whitespace.
		candidate = strings.TrimSpace(candidate)

		// Skip empty candidates.
		if candidate == "" {
			continue
		}

		// Skip URLs.
		if urlPattern.MatchString(candidate) {
			continue
		}

		// Skip known false positives.
		if falsePositivePaths[candidate] {
			continue
		}

		// Skip if it starts with a common false-positive prefix.
		if isFalsePositivePrefix(candidate) {
			continue
		}

		// Deduplicate.
		if seen[candidate] {
			continue
		}
		seen[candidate] = true

		// Validate: check if file exists on disk.
		if d.fileExists(candidate) {
			results = append(results, candidate)
		}
	}

	return results
}

// FilterNew removes paths that are already present in the context map.
func (d *FileMentionDetector) FilterNew(paths []string, alreadyInContext map[string]bool) []string {
	var filtered []string
	for _, p := range paths {
		if !alreadyInContext[p] {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// BuildSuggestion formats a human-readable suggestion message listing
// the given files that are mentioned but not yet in context.
func (d *FileMentionDetector) BuildSuggestion(newFiles []string) string {
	if len(newFiles) == 0 {
		return ""
	}
	return "Files mentioned but not in context: " + strings.Join(newFiles, ", ") + "\nConsider reading these for better context."
}

// InjectFileMentionContext detects file mentions in the given text,
// filters out files already discussed in the message history, and returns
// a system context string if new files are found. Returns "" if none.
func (d *FileMentionDetector) InjectFileMentionContext(text string, messages []client.EyrieMessage) string {
	mentions := d.DetectMentions(text)
	if len(mentions) == 0 {
		return ""
	}

	// Build a set of files already referenced in message history.
	alreadyDiscussed := make(map[string]bool)
	for _, msg := range messages {
		mentioned := d.extractCandidates(msg.Content)
		for _, m := range mentioned {
			m = stripLineNumber(m)
			m = strings.TrimSpace(m)
			alreadyDiscussed[m] = true
		}
	}

	newFiles := d.FilterNew(mentions, alreadyDiscussed)
	if len(newFiles) == 0 {
		return ""
	}

	return d.BuildSuggestion(newFiles)
}

// extractCandidates runs all regex patterns against the text and collects
// raw candidate path strings.
func (d *FileMentionDetector) extractCandidates(text string) []string {
	var candidates []string
	for _, pat := range filePathPatterns {
		matches := pat.FindAllStringSubmatch(text, -1)
		for _, m := range matches {
			if len(m) > 1 {
				candidates = append(candidates, m[1])
			}
		}
	}
	return candidates
}

// fileExists checks whether the given path exists on disk, either as an
// absolute path or relative to the project root. Results are cached.
func (d *FileMentionDetector) fileExists(path string) bool {
	// Resolve the full path.
	var fullPath string
	if filepath.IsAbs(path) {
		fullPath = path
	} else {
		fullPath = filepath.Join(d.projectRoot, path)
	}

	// Check cache first.
	d.mu.RLock()
	if exists, ok := d.knownFiles[fullPath]; ok {
		d.mu.RUnlock()
		return exists
	}
	d.mu.RUnlock()

	// Check filesystem.
	_, err := os.Stat(fullPath)
	exists := err == nil

	// Cache result.
	d.mu.Lock()
	d.knownFiles[fullPath] = exists
	d.mu.Unlock()

	return exists
}

// stripLineNumber removes a trailing :NNN line number from a path.
func stripLineNumber(path string) string {
	if m := fileLinePattern.FindStringSubmatch(path); len(m) > 1 {
		return m[1]
	}
	return path
}

// isFalsePositivePrefix checks for common system path prefixes that
// should be excluded from file mention detection.
func isFalsePositivePrefix(path string) bool {
	prefixes := []string{"/dev/", "/etc/", "/tmp/", "/proc/", "/sys/"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
