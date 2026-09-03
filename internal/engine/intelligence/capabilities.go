package intelligence

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Capability represents a single thing the agent can do.
type Capability struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Category         string   `json:"category"`
	Tools            []string `json:"tools"`
	Examples         []string `json:"examples"`
	Complexity       string   `json:"complexity"` // "trivial", "simple", "moderate", "complex"
	RequiresApproval bool     `json:"requires_approval"`
	Enabled          bool     `json:"enabled"`
}

// CapabilityRegistry manages the set of capabilities the agent advertises.
type CapabilityRegistry struct {
	Capabilities map[string]*Capability
	Categories   map[string][]string
	mu           sync.RWMutex
}

// NewCapabilityRegistry creates a registry pre-populated with built-in capabilities.
func NewCapabilityRegistry() *CapabilityRegistry {
	r := &CapabilityRegistry{
		Capabilities: make(map[string]*Capability),
		Categories:   make(map[string][]string),
	}

	builtins := []*Capability{
		{
			ID:          "code_write",
			Name:        "Write Code",
			Description: "Write new code files",
			Category:    "Code",
			Tools:       []string{"file_write", "file_create"},
			Examples:    []string{"Create a new Go HTTP handler", "Write a Python script to parse CSV"},
			Complexity:  "moderate",
			Enabled:     true,
		},
		{
			ID:          "code_edit",
			Name:        "Edit Code",
			Description: "Modify existing code",
			Category:    "Code",
			Tools:       []string{"file_edit", "file_write"},
			Examples:    []string{"Add error handling to this function", "Rename variable across file"},
			Complexity:  "moderate",
			Enabled:     true,
		},
		{
			ID:          "code_review",
			Name:        "Review Code",
			Description: "Review code for issues",
			Category:    "Code",
			Tools:       []string{"file_read", "grep"},
			Examples:    []string{"Review this PR for bugs", "Check for security issues"},
			Complexity:  "moderate",
			Enabled:     true,
		},
		{
			ID:          "bug_fix",
			Name:        "Fix Bugs",
			Description: "Debug and fix bugs",
			Category:    "Code",
			Tools:       []string{"file_read", "file_edit", "shell_exec", "grep"},
			Examples:    []string{"Fix the nil pointer panic in handler.go", "Debug why tests are failing"},
			Complexity:  "complex",
			Enabled:     true,
		},
		{
			ID:          "test_write",
			Name:        "Write Tests",
			Description: "Write unit/integration tests",
			Category:    "Testing",
			Tools:       []string{"file_write", "file_read", "shell_exec"},
			Examples:    []string{"Write unit tests for the parser package", "Add integration test for API"},
			Complexity:  "moderate",
			Enabled:     true,
		},
		{
			ID:          "refactor",
			Name:        "Refactor",
			Description: "Restructure code",
			Category:    "Code",
			Tools:       []string{"file_read", "file_edit", "grep", "file_write"},
			Examples:    []string{"Extract method from this function", "Split large file into modules"},
			Complexity:  "complex",
			Enabled:     true,
		},
		{
			ID:               "git_commit",
			Name:             "Git Commit",
			Description:      "Stage and commit changes",
			Category:         "Git",
			Tools:            []string{"shell_exec"},
			Examples:         []string{"Commit these changes with a good message", "Stage and commit the fix"},
			Complexity:       "simple",
			RequiresApproval: true,
			Enabled:          true,
		},
		{
			ID:          "git_branch",
			Name:        "Git Branch",
			Description: "Create/switch branches",
			Category:    "Git",
			Tools:       []string{"shell_exec"},
			Examples:    []string{"Create a feature branch", "Switch to main branch"},
			Complexity:  "simple",
			Enabled:     true,
		},
		{
			ID:          "search_code",
			Name:        "Search Code",
			Description: "Search codebase",
			Category:    "Navigation",
			Tools:       []string{"grep", "file_read"},
			Examples:    []string{"Find all usages of UserService", "Search for TODO comments"},
			Complexity:  "trivial",
			Enabled:     true,
		},
		{
			ID:          "explain_code",
			Name:        "Explain Code",
			Description: "Explain how code works",
			Category:    "Navigation",
			Tools:       []string{"file_read"},
			Examples:    []string{"Explain this function", "How does the auth middleware work?"},
			Complexity:  "simple",
			Enabled:     true,
		},
		{
			ID:          "run_tests",
			Name:        "Run Tests",
			Description: "Execute test suites",
			Category:    "Testing",
			Tools:       []string{"shell_exec"},
			Examples:    []string{"Run all tests", "Run tests for the auth package"},
			Complexity:  "simple",
			Enabled:     true,
		},
		{
			ID:          "run_lint",
			Name:        "Run Linters",
			Description: "Run linters",
			Category:    "Testing",
			Tools:       []string{"shell_exec"},
			Examples:    []string{"Run golangci-lint", "Check for style violations"},
			Complexity:  "trivial",
			Enabled:     true,
		},
		{
			ID:          "file_create",
			Name:        "Create Files",
			Description: "Create new files",
			Category:    "Files",
			Tools:       []string{"file_write", "file_create"},
			Examples:    []string{"Create a new config file", "Add a Dockerfile"},
			Complexity:  "trivial",
			Enabled:     true,
		},
		{
			ID:               "file_delete",
			Name:             "Delete Files",
			Description:      "Delete files",
			Category:         "Files",
			Tools:            []string{"shell_exec"},
			Examples:         []string{"Remove the old migration", "Delete unused test fixtures"},
			Complexity:       "trivial",
			RequiresApproval: true,
			Enabled:          true,
		},
		{
			ID:          "web_fetch",
			Name:        "Fetch Web Content",
			Description: "Fetch web content",
			Category:    "External",
			Tools:       []string{"web_fetch"},
			Examples:    []string{"Fetch the API docs from this URL", "Download the schema"},
			Complexity:  "simple",
			Enabled:     true,
		},
		{
			ID:               "shell_exec",
			Name:             "Shell Commands",
			Description:      "Run shell commands",
			Category:         "System",
			Tools:            []string{"shell_exec"},
			Examples:         []string{"List files in the directory", "Check disk usage"},
			Complexity:       "simple",
			RequiresApproval: true,
			Enabled:          true,
		},
		{
			ID:          "project_scaffold",
			Name:        "Scaffold Project",
			Description: "Generate project structure",
			Category:    "Code",
			Tools:       []string{"file_write", "file_create", "shell_exec"},
			Examples:    []string{"Create a new Go module", "Scaffold a React app"},
			Complexity:  "complex",
			Enabled:     true,
		},
		{
			ID:               "dependency_manage",
			Name:             "Manage Dependencies",
			Description:      "Add/update dependencies",
			Category:         "System",
			Tools:            []string{"shell_exec", "file_edit"},
			Examples:         []string{"Add the cobra library", "Update all dependencies"},
			Complexity:       "moderate",
			RequiresApproval: true,
			Enabled:          true,
		},
		{
			ID:          "doc_generate",
			Name:        "Generate Docs",
			Description: "Generate documentation",
			Category:    "Documentation",
			Tools:       []string{"file_read", "file_write"},
			Examples:    []string{"Generate API docs", "Write README for this package"},
			Complexity:  "moderate",
			Enabled:     true,
		},
		{
			ID:          "config_manage",
			Name:        "Manage Config",
			Description: "Manage configuration",
			Category:    "System",
			Tools:       []string{"file_read", "file_edit", "file_write"},
			Examples:    []string{"Update the database config", "Add new environment variable"},
			Complexity:  "simple",
			Enabled:     true,
		},
		{
			ID:               "git_merge",
			Name:             "Git Merge",
			Description:      "Merge branches",
			Category:         "Git",
			Tools:            []string{"shell_exec"},
			Examples:         []string{"Merge feature branch into main", "Resolve merge conflicts"},
			Complexity:       "moderate",
			RequiresApproval: true,
			Enabled:          true,
		},
		{
			ID:               "git_rebase",
			Name:             "Git Rebase",
			Description:      "Rebase branches",
			Category:         "Git",
			Tools:            []string{"shell_exec"},
			Examples:         []string{"Rebase onto main", "Interactive rebase last 3 commits"},
			Complexity:       "complex",
			RequiresApproval: true,
			Enabled:          true,
		},
		{
			ID:          "git_stash",
			Name:        "Git Stash",
			Description: "Stash/unstash changes",
			Category:    "Git",
			Tools:       []string{"shell_exec"},
			Examples:    []string{"Stash current changes", "Apply last stash"},
			Complexity:  "trivial",
			Enabled:     true,
		},
		{
			ID:          "api_design",
			Name:        "Design API",
			Description: "Design REST/gRPC APIs",
			Category:    "Architecture",
			Tools:       []string{"file_write", "file_read"},
			Examples:    []string{"Design a REST API for user management", "Create OpenAPI spec"},
			Complexity:  "complex",
			Enabled:     true,
		},
		{
			ID:               "db_migration",
			Name:             "Database Migration",
			Description:      "Create database migrations",
			Category:         "Database",
			Tools:            []string{"file_write", "shell_exec"},
			Examples:         []string{"Create migration for users table", "Add index on email column"},
			Complexity:       "moderate",
			RequiresApproval: true,
			Enabled:          true,
		},
		{
			ID:          "db_query",
			Name:        "Database Query",
			Description: "Write/optimize SQL queries",
			Category:    "Database",
			Tools:       []string{"file_read", "file_edit"},
			Examples:    []string{"Optimize this slow query", "Write a join for user orders"},
			Complexity:  "moderate",
			Enabled:     true,
		},
		{
			ID:          "error_handle",
			Name:        "Error Handling",
			Description: "Add/improve error handling",
			Category:    "Code",
			Tools:       []string{"file_read", "file_edit"},
			Examples:    []string{"Add proper error handling", "Wrap errors with context"},
			Complexity:  "simple",
			Enabled:     true,
		},
		{
			ID:          "perf_optimize",
			Name:        "Optimize Performance",
			Description: "Profile and optimize performance",
			Category:    "Code",
			Tools:       []string{"file_read", "file_edit", "shell_exec"},
			Examples:    []string{"Find performance bottleneck", "Optimize this hot loop"},
			Complexity:  "complex",
			Enabled:     true,
		},
		{
			ID:          "security_audit",
			Name:        "Security Audit",
			Description: "Audit code for vulnerabilities",
			Category:    "Security",
			Tools:       []string{"file_read", "grep", "shell_exec"},
			Examples:    []string{"Check for SQL injection", "Audit auth implementation"},
			Complexity:  "complex",
			Enabled:     true,
		},
		{
			ID:          "ci_configure",
			Name:        "Configure CI",
			Description: "Set up CI/CD pipelines",
			Category:    "DevOps",
			Tools:       []string{"file_write", "file_read"},
			Examples:    []string{"Create GitHub Actions workflow", "Add test stage to pipeline"},
			Complexity:  "moderate",
			Enabled:     true,
		},
		{
			ID:          "docker_manage",
			Name:        "Docker Management",
			Description: "Create/manage Dockerfiles and compose",
			Category:    "DevOps",
			Tools:       []string{"file_write", "file_edit", "shell_exec"},
			Examples:    []string{"Create a multi-stage Dockerfile", "Set up docker-compose"},
			Complexity:  "moderate",
			Enabled:     true,
		},
		{
			ID:          "type_annotate",
			Name:        "Type Annotations",
			Description: "Add/fix type annotations",
			Category:    "Code",
			Tools:       []string{"file_read", "file_edit"},
			Examples:    []string{"Add TypeScript types", "Fix type errors"},
			Complexity:  "simple",
			Enabled:     true,
		},
		{
			ID:          "log_add",
			Name:        "Add Logging",
			Description: "Add structured logging",
			Category:    "Code",
			Tools:       []string{"file_read", "file_edit"},
			Examples:    []string{"Add logging to this service", "Improve log messages"},
			Complexity:  "simple",
			Enabled:     true,
		},
	}

	for _, cap := range builtins {
		r.Capabilities[cap.ID] = cap
		r.Categories[cap.Category] = append(r.Categories[cap.Category], cap.ID)
	}

	return r
}

// GetCapability returns a capability by ID, or nil if not found.
func (r *CapabilityRegistry) GetCapability(id string) *Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Capabilities[id]
}

// ListByCategory returns all capabilities in the given category.
func (r *CapabilityRegistry) ListByCategory(category string) []*Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids, ok := r.Categories[category]
	if !ok {
		return nil
	}

	caps := make([]*Capability, 0, len(ids))
	for _, id := range ids {
		if cap, exists := r.Capabilities[id]; exists {
			caps = append(caps, cap)
		}
	}
	return caps
}

// CanDo matches a task description against capabilities and returns relevant ones.
func (r *CapabilityRegistry) CanDo(taskDescription string) []*Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()

	lower := strings.ToLower(taskDescription)
	words := strings.Fields(lower)

	type scored struct {
		cap   *Capability
		score int
	}

	var results []scored

	for _, cap := range r.Capabilities {
		if !cap.Enabled {
			continue
		}

		score := 0

		// Match against name
		nameLower := strings.ToLower(cap.Name)
		for _, w := range words {
			if strings.Contains(nameLower, w) {
				score += 3
			}
		}

		// Match against description
		descLower := strings.ToLower(cap.Description)
		for _, w := range words {
			if strings.Contains(descLower, w) {
				score += 2
			}
		}

		// Match against ID
		idLower := strings.ToLower(cap.ID)
		for _, w := range words {
			if strings.Contains(idLower, w) {
				score += 2
			}
		}

		// Match against examples
		for _, ex := range cap.Examples {
			exLower := strings.ToLower(ex)
			for _, w := range words {
				if strings.Contains(exLower, w) {
					score++
				}
			}
		}

		// Match against category
		catLower := strings.ToLower(cap.Category)
		for _, w := range words {
			if strings.Contains(catLower, w) {
				score++
			}
		}

		if score > 0 {
			results = append(results, scored{cap: cap, score: score})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	caps := make([]*Capability, 0, len(results))
	for _, r := range results {
		caps = append(caps, r.cap)
	}
	return caps
}

// FormatHelp returns a formatted help string listing all capabilities by category.
func (r *CapabilityRegistry) FormatHelp() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("graycode Capabilities:\n")
	sb.WriteString(strings.Repeat("═", 19))
	sb.WriteString("\n")

	categories := r.sortedCategories()

	for _, cat := range categories {
		ids := r.Categories[cat]
		if len(ids) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("\n%s:\n", cat))
		for _, id := range ids {
			cap := r.Capabilities[id]
			if cap == nil {
				continue
			}
			sb.WriteString(fmt.Sprintf("  • %s — %s\n", cap.ID, cap.Description))
		}
	}

	return sb.String()
}

// FormatCapability returns a detailed formatted string for a single capability.
func (r *CapabilityRegistry) FormatCapability(cap *Capability) string {
	if cap == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Capability: %s\n", cap.Name))
	sb.WriteString(fmt.Sprintf("  ID:          %s\n", cap.ID))
	sb.WriteString(fmt.Sprintf("  Category:    %s\n", cap.Category))
	sb.WriteString(fmt.Sprintf("  Description: %s\n", cap.Description))
	sb.WriteString(fmt.Sprintf("  Complexity:  %s\n", cap.Complexity))
	sb.WriteString(fmt.Sprintf("  Approval:    %v\n", cap.RequiresApproval))
	sb.WriteString(fmt.Sprintf("  Enabled:     %v\n", cap.Enabled))

	if len(cap.Tools) > 0 {
		sb.WriteString(fmt.Sprintf("  Tools:       %s\n", strings.Join(cap.Tools, ", ")))
	}

	if len(cap.Examples) > 0 {
		sb.WriteString("  Examples:\n")
		for _, ex := range cap.Examples {
			sb.WriteString(fmt.Sprintf("    - %s\n", ex))
		}
	}

	return sb.String()
}

// Enable enables a capability by ID.
func (r *CapabilityRegistry) Enable(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cap, ok := r.Capabilities[id]; ok {
		cap.Enabled = true
	}
}

// Disable disables a capability by ID.
func (r *CapabilityRegistry) Disable(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cap, ok := r.Capabilities[id]; ok {
		cap.Enabled = false
	}
}

// Search finds capabilities matching the query string against ID, name, description, and examples.
func (r *CapabilityRegistry) Search(query string) []*Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()

	lower := strings.ToLower(query)
	var results []*Capability

	for _, cap := range r.Capabilities {
		if strings.Contains(strings.ToLower(cap.ID), lower) ||
			strings.Contains(strings.ToLower(cap.Name), lower) ||
			strings.Contains(strings.ToLower(cap.Description), lower) ||
			strings.Contains(strings.ToLower(cap.Category), lower) {
			results = append(results, cap)
			continue
		}

		for _, ex := range cap.Examples {
			if strings.Contains(strings.ToLower(ex), lower) {
				results = append(results, cap)
				break
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].ID < results[j].ID
	})

	return results
}

// GetCategories returns a sorted list of all category names.
func (r *CapabilityRegistry) GetCategories() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sortedCategories()
}

// sortedCategories returns category names in sorted order. Must be called with lock held.
func (r *CapabilityRegistry) sortedCategories() []string {
	cats := make([]string, 0, len(r.Categories))
	for cat := range r.Categories {
		cats = append(cats, cat)
	}
	sort.Strings(cats)
	return cats
}
