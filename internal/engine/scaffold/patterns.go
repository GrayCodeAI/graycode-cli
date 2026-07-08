package scaffold

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// PromptPattern represents a composable, reusable prompt pattern for common tasks.
type PromptPattern struct {
	Name         string
	Description  string
	SystemPrompt string
	UserTemplate string
	OutputFormat string
	Tags         []string
	Version      string
	Author       string
}

// PatternLibrary holds a collection of prompt patterns and supports loading,
// searching, and chaining them.
type PatternLibrary struct {
	Patterns map[string]*PromptPattern
	Dir      string
	mu       sync.RWMutex
}

// NewPatternLibrary creates a new PatternLibrary with the given directory for
// custom pattern storage.
func NewPatternLibrary(dir string) *PatternLibrary {
	return &PatternLibrary{
		Patterns: make(map[string]*PromptPattern),
		Dir:      dir,
	}
}

// LoadBuiltins populates the library with built-in prompt patterns.
func (pl *PatternLibrary) LoadBuiltins() {
	builtins := []*PromptPattern{
		{
			Name:         "summarize",
			Description:  "Summarize the following text in bullet points",
			SystemPrompt: "You are a concise summarizer. Extract the key points and present them as clear bullet points.",
			UserTemplate: "Summarize the following text in bullet points:\n\n{{INPUT}}",
			OutputFormat: "markdown-bullets",
			Tags:         []string{"text", "summary", "general"},
			Version:      "1.0.0",
			Author:       "hawk",
		},
		{
			Name:         "explain_code",
			Description:  "Explain this code clearly for a junior developer",
			SystemPrompt: "You are a patient programming mentor. Explain code clearly, avoiding jargon where possible, and provide context for why things are done a certain way.",
			UserTemplate: "Explain this code clearly for a junior developer:\n\n{{INPUT}}",
			OutputFormat: "markdown",
			Tags:         []string{"code", "explanation", "education"},
			Version:      "1.0.0",
			Author:       "hawk",
		},
		{
			Name:         "find_bugs",
			Description:  "Find potential bugs in this code",
			SystemPrompt: "You are a meticulous code reviewer focused on finding bugs. Look for logic errors, edge cases, null pointer issues, race conditions, and other common mistakes.",
			UserTemplate: "Find potential bugs in this code:\n\n{{INPUT}}",
			OutputFormat: "markdown-list",
			Tags:         []string{"code", "bugs", "review", "quality"},
			Version:      "1.0.0",
			Author:       "hawk",
		},
		{
			Name:         "improve_code",
			Description:  "Suggest improvements for this code",
			SystemPrompt: "You are a senior engineer focused on code quality. Suggest improvements for readability, maintainability, performance, and adherence to best practices.",
			UserTemplate: "Suggest improvements for this code:\n\n{{INPUT}}",
			OutputFormat: "markdown",
			Tags:         []string{"code", "improvement", "refactor", "quality"},
			Version:      "1.0.0",
			Author:       "hawk",
		},
		{
			Name:         "write_tests",
			Description:  "Write comprehensive tests for this code",
			SystemPrompt: "You are a testing expert. Write comprehensive unit tests that cover happy paths, edge cases, and error conditions. Use table-driven tests where appropriate.",
			UserTemplate: "Write comprehensive tests for this code:\n\n{{INPUT}}",
			OutputFormat: "code",
			Tags:         []string{"code", "testing", "quality"},
			Version:      "1.0.0",
			Author:       "hawk",
		},
		{
			Name:         "extract_todos",
			Description:  "Extract all TODO/FIXME items from this code",
			SystemPrompt: "You are a project tracker. Extract all TODO, FIXME, HACK, and XXX comments from code, organize them by priority, and summarize what needs to be done.",
			UserTemplate: "Extract all TODO/FIXME items from this code:\n\n{{INPUT}}",
			OutputFormat: "markdown-list",
			Tags:         []string{"code", "todos", "project-management"},
			Version:      "1.0.0",
			Author:       "hawk",
		},
		{
			Name:         "security_review",
			Description:  "Review this code for security vulnerabilities",
			SystemPrompt: "You are a security expert. Review code for common vulnerabilities including injection attacks, authentication issues, data exposure, and insecure configurations. Reference CWE IDs where applicable.",
			UserTemplate: "Review this code for security vulnerabilities:\n\n{{INPUT}}",
			OutputFormat: "markdown",
			Tags:         []string{"code", "security", "review", "vulnerabilities"},
			Version:      "1.0.0",
			Author:       "hawk",
		},
		{
			Name:         "api_docs",
			Description:  "Generate API documentation for this code",
			SystemPrompt: "You are a technical writer specializing in API documentation. Generate clear, complete documentation including endpoint descriptions, parameters, request/response examples, and error codes.",
			UserTemplate: "Generate API documentation for this code:\n\n{{INPUT}}",
			OutputFormat: "markdown",
			Tags:         []string{"code", "documentation", "api"},
			Version:      "1.0.0",
			Author:       "hawk",
		},
		{
			Name:         "commit_message",
			Description:  "Generate a commit message for this diff",
			SystemPrompt: "You are a Git expert. Write clear, conventional commit messages that explain the what and why of changes. Follow the Conventional Commits specification.",
			UserTemplate: "Generate a commit message for this diff:\n\n{{INPUT}}",
			OutputFormat: "text",
			Tags:         []string{"git", "commit", "workflow"},
			Version:      "1.0.0",
			Author:       "hawk",
		},
		{
			Name:         "review_pr",
			Description:  "Review this pull request diff",
			SystemPrompt: "You are a thorough code reviewer. Review pull requests for correctness, style, performance, and potential issues. Provide constructive feedback with specific suggestions.",
			UserTemplate: "Review this pull request diff:\n\n{{INPUT}}",
			OutputFormat: "markdown",
			Tags:         []string{"code", "review", "git", "pr"},
			Version:      "1.0.0",
			Author:       "hawk",
		},
		{
			Name:         "debug_error",
			Description:  "Help debug this error",
			SystemPrompt: "You are a debugging expert. Analyze error messages and stack traces to identify root causes. Provide step-by-step debugging guidance and potential fixes.",
			UserTemplate: "Help debug this error:\n\n{{INPUT}}",
			OutputFormat: "markdown",
			Tags:         []string{"code", "debugging", "errors"},
			Version:      "1.0.0",
			Author:       "hawk",
		},
		{
			Name:         "refactor_plan",
			Description:  "Create a refactoring plan for this code",
			SystemPrompt: "You are a software architect specializing in refactoring. Create detailed, step-by-step refactoring plans that minimize risk and maintain backward compatibility.",
			UserTemplate: "Create a refactoring plan for this code:\n\n{{INPUT}}",
			OutputFormat: "markdown",
			Tags:         []string{"code", "refactor", "architecture", "planning"},
			Version:      "1.0.0",
			Author:       "hawk",
		},
		{
			Name:         "architecture_doc",
			Description:  "Document the architecture of this code",
			SystemPrompt: "You are a software architect. Document the architecture including component relationships, data flow, design patterns used, and key design decisions.",
			UserTemplate: "Document the architecture of this code:\n\n{{INPUT}}",
			OutputFormat: "markdown",
			Tags:         []string{"code", "architecture", "documentation"},
			Version:      "1.0.0",
			Author:       "hawk",
		},
		{
			Name:         "performance_review",
			Description:  "Find performance issues in this code",
			SystemPrompt: "You are a performance engineer. Identify performance bottlenecks, inefficient algorithms, unnecessary allocations, and N+1 query patterns. Suggest specific optimizations with expected impact.",
			UserTemplate: "Find performance issues in this code:\n\n{{INPUT}}",
			OutputFormat: "markdown-list",
			Tags:         []string{"code", "performance", "optimization"},
			Version:      "1.0.0",
			Author:       "hawk",
		},
		{
			Name:         "accessibility_check",
			Description:  "Check this UI code for accessibility issues",
			SystemPrompt: "You are an accessibility expert. Review UI code for WCAG compliance, proper ARIA attributes, keyboard navigation, screen reader support, and color contrast issues.",
			UserTemplate: "Check this UI code for accessibility issues:\n\n{{INPUT}}",
			OutputFormat: "markdown-list",
			Tags:         []string{"ui", "accessibility", "a11y", "frontend"},
			Version:      "1.0.0",
			Author:       "hawk",
		},
	}

	pl.mu.Lock()
	defer pl.mu.Unlock()
	for _, p := range builtins {
		pl.Patterns[p.Name] = p
	}
}

// Get retrieves a pattern by name. Returns nil if not found.
func (pl *PatternLibrary) Get(name string) *PromptPattern {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	return pl.Patterns[name]
}

// Apply resolves a pattern by name and injects the input into its template.
// Returns the system prompt, composed user prompt, and any error.
func (pl *PatternLibrary) Apply(patternName string, input string) (string, string, error) {
	pl.mu.RLock()
	p, ok := pl.Patterns[patternName]
	pl.mu.RUnlock()
	if !ok {
		return "", "", fmt.Errorf("pattern not found: %s", patternName)
	}

	userPrompt := strings.ReplaceAll(p.UserTemplate, "{{INPUT}}", input)
	return p.SystemPrompt, userPrompt, nil
}

// Search performs a fuzzy search across pattern names, descriptions, and tags.
// Returns all patterns that match the query substring (case-insensitive).
func (pl *PatternLibrary) Search(query string) []*PromptPattern {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	query = strings.ToLower(query)
	var results []*PromptPattern

	for _, p := range pl.Patterns {
		if matchesPattern(p, query) {
			results = append(results, p)
		}
	}
	return results
}

// matchesPattern checks if a pattern matches a search query by looking at
// name, description, and tags.
func matchesPattern(p *PromptPattern, query string) bool {
	if strings.Contains(strings.ToLower(p.Name), query) {
		return true
	}
	if strings.Contains(strings.ToLower(p.Description), query) {
		return true
	}
	for _, tag := range p.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}

// Register adds a new pattern to the library. If a pattern with the same
// name exists, it is overwritten.
func (pl *PatternLibrary) Register(pattern *PromptPattern) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	pl.Patterns[pattern.Name] = pattern
}

// Remove deletes a pattern from the library by name.
func (pl *PatternLibrary) Remove(name string) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	delete(pl.Patterns, name)
}

// Chain applies multiple patterns sequentially, where the output prompt of one
// becomes the input for the next. Returns the list of user prompts generated
// at each step.
func (pl *PatternLibrary) Chain(patterns []string, input string) []string {
	var results []string
	current := input

	for _, name := range patterns {
		_, userPrompt, err := pl.Apply(name, current)
		if err != nil {
			results = append(results, fmt.Sprintf("[error: %s]", err.Error()))
			break
		}
		results = append(results, userPrompt)
		current = userPrompt
	}
	return results
}

// LoadFromDir loads custom patterns from markdown files with YAML frontmatter
// in the specified directory. Each .md file is parsed for frontmatter fields
// (name, description, system_prompt, output_format, tags, version, author)
// and the body becomes the UserTemplate.
func (pl *PatternLibrary) LoadFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading pattern directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".md" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		pattern, err := parsePatternFile(path)
		if err != nil {
			continue // skip malformed files
		}

		pl.mu.Lock()
		pl.Patterns[pattern.Name] = pattern
		pl.mu.Unlock()
	}
	return nil
}

// parsePatternFile reads a markdown file with YAML frontmatter and returns
// a PromptPattern. The frontmatter is delimited by "---" lines.
func parsePatternFile(path string) (*PromptPattern, error) {
	f, err := os.Open(path) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	p := &PromptPattern{}

	// Check for frontmatter start
	if !scanner.Scan() {
		return nil, fmt.Errorf("empty file: %s", path)
	}
	firstLine := strings.TrimSpace(scanner.Text())
	if firstLine != "---" {
		return nil, fmt.Errorf("no frontmatter in %s", path)
	}

	// Parse frontmatter
	inFrontmatter := true
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			inFrontmatter = false
			break
		}
		if inFrontmatter {
			parseFrontmatterLine(p, trimmed)
		}
	}

	if inFrontmatter {
		return nil, fmt.Errorf("unclosed frontmatter in %s", path)
	}

	// Rest is the user template body
	var body strings.Builder
	for scanner.Scan() {
		if body.Len() > 0 {
			body.WriteString("\n")
		}
		body.WriteString(scanner.Text())
	}

	if body.Len() > 0 {
		p.UserTemplate = body.String()
	}

	// Default name from filename if not set
	if p.Name == "" {
		base := filepath.Base(path)
		p.Name = strings.TrimSuffix(base, filepath.Ext(base))
	}

	return p, nil
}

// parseFrontmatterLine parses a single "key: value" line from YAML frontmatter.
func parseFrontmatterLine(p *PromptPattern, line string) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return
	}
	key := strings.TrimSpace(line[:idx])
	value := strings.TrimSpace(line[idx+1:])

	// Remove surrounding quotes if present
	value = strings.Trim(value, "\"'")

	switch key {
	case "name":
		p.Name = value
	case "description":
		p.Description = value
	case "system_prompt":
		p.SystemPrompt = value
	case "output_format":
		p.OutputFormat = value
	case "version":
		p.Version = value
	case "author":
		p.Author = value
	case "tags":
		// Parse comma-separated or YAML-style list
		value = strings.Trim(value, "[]")
		parts := strings.Split(value, ",")
		for _, part := range parts {
			tag := strings.TrimSpace(part)
			tag = strings.Trim(tag, "\"'")
			if tag != "" {
				p.Tags = append(p.Tags, tag)
			}
		}
	}
}

// FormatPattern returns a human-readable string representation of a pattern.
func FormatPattern(p *PromptPattern) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Pattern: %s\n", p.Name))
	sb.WriteString(fmt.Sprintf("  Description:  %s\n", p.Description))
	sb.WriteString(fmt.Sprintf("  Version:      %s\n", p.Version))
	sb.WriteString(fmt.Sprintf("  Author:       %s\n", p.Author))
	sb.WriteString(fmt.Sprintf("  Output:       %s\n", p.OutputFormat))
	if len(p.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("  Tags:         %s\n", strings.Join(p.Tags, ", ")))
	}
	sb.WriteString(fmt.Sprintf("  System:       %s\n", patternTruncate(p.SystemPrompt, 80)))
	sb.WriteString(fmt.Sprintf("  Template:     %s\n", patternTruncate(p.UserTemplate, 80)))
	return sb.String()
}

// patternTruncate shortens a string to max length, appending "..." if truncated.
func patternTruncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// ListByTag returns all patterns that have the specified tag.
func (pl *PatternLibrary) ListByTag(tag string) []*PromptPattern {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	tag = strings.ToLower(tag)
	var results []*PromptPattern
	for _, p := range pl.Patterns {
		for _, t := range p.Tags {
			if strings.ToLower(t) == tag {
				results = append(results, p)
				break
			}
		}
	}
	return results
}
