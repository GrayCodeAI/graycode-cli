// api_scanner.go discovers HTTP endpoints in a project
// by detecting the framework (Chi, net/http, Gin, Echo, Gorilla mux, or
// Fiber) and extracting route declarations into an APIMap. FormatAPIMap
// renders the routes as text; GenerateOpenAPI produces a 3.x OpenAPI
// document for the same set.
package repomap

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// APIEndpoint represents a single HTTP endpoint discovered in the codebase.
type APIEndpoint struct {
	Method      string
	Path        string
	Handler     string
	File        string
	Line        int
	Middleware  []string
	Description string
}

// APIMap holds all discovered endpoints and metadata about the API.
type APIMap struct {
	Endpoints []APIEndpoint
	BaseURL   string
	Version   string
	mu        sync.RWMutex
}

// FindEndpointByPath returns the first endpoint matching the given path, or nil.
func (m *APIMap) FindEndpointByPath(path string) *APIEndpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.Endpoints {
		if m.Endpoints[i].Path == path {
			return &m.Endpoints[i]
		}
	}
	return nil
}

// APIScanner discovers HTTP endpoints in Go projects by analyzing router registrations.
type APIScanner struct {
	mu sync.Mutex
}

// NewAPIScanner creates a new APIScanner instance.
func NewAPIScanner() *APIScanner {
	return &APIScanner{}
}

// ScanProject walks Go files in dir looking for router registrations and returns
// an APIMap with all discovered endpoints. It auto-detects the framework in use.
func (s *APIScanner) ScanProject(dir string) (*APIMap, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	apiMap := &APIMap{}

	framework := DetectFramework(dir)

	// Verify the directory exists before walking
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("scanning project: %w", err)
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == "node_modules" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		relPath, _ := filepath.Rel(dir, path)
		if relPath == "" {
			relPath = path
		}

		var endpoints []APIEndpoint

		switch framework {
		case "chi":
			endpoints = ScanChi(content, relPath)
		case "gin":
			endpoints = ScanGin(content, relPath)
		case "echo":
			endpoints = ScanEcho(content, relPath)
		case "gorilla":
			endpoints = ScanGorilla(content, relPath)
		case "fiber":
			endpoints = ScanFiber(content, relPath)
		default:
			// Try net/http as fallback, plus any framework-specific patterns
			endpoints = ScanNetHTTP(content, relPath)
			endpoints = append(endpoints, ScanChi(content, relPath)...)
			endpoints = append(endpoints, ScanGin(content, relPath)...)
			endpoints = append(endpoints, ScanEcho(content, relPath)...)
			endpoints = append(endpoints, ScanGorilla(content, relPath)...)
			endpoints = append(endpoints, ScanFiber(content, relPath)...)
		}

		apiMap.mu.Lock()
		apiMap.Endpoints = append(apiMap.Endpoints, endpoints...)
		apiMap.mu.Unlock()

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scanning project: %w", err)
	}

	// Deduplicate endpoints
	apiMap.Endpoints = deduplicateEndpoints(apiMap.Endpoints)

	// Sort endpoints by path then method
	sort.Slice(apiMap.Endpoints, func(i, j int) bool {
		if apiMap.Endpoints[i].Path == apiMap.Endpoints[j].Path {
			return apiMap.Endpoints[i].Method < apiMap.Endpoints[j].Method
		}
		return apiMap.Endpoints[i].Path < apiMap.Endpoints[j].Path
	})

	return apiMap, nil
}

func deduplicateEndpoints(endpoints []APIEndpoint) []APIEndpoint {
	seen := make(map[string]bool)
	var result []APIEndpoint
	for _, ep := range endpoints {
		key := ep.Method + " " + ep.Path + " " + ep.Handler + " " + ep.File
		if !seen[key] {
			seen[key] = true
			result = append(result, ep)
		}
	}
	return result
}

// ScanChi extracts endpoints from chi router registrations.
// Patterns: r.Get("/path", handler), r.Post(...), r.Put(...), r.Delete(...),
// r.Route("/prefix", func(r chi.Router) { ... }), r.Group(func(r chi.Router) { ... })
func ScanChi(content, file string) []APIEndpoint {
	var endpoints []APIEndpoint

	// Match r.Method("/path", handler) patterns for chi
	// chi uses: r.Get, r.Post, r.Put, r.Delete, r.Patch, r.Head, r.Options
	methodPattern := regexp.MustCompile(`(?m)(\w+)\.(Get|Post|Put|Delete|Patch|Head|Options)\(\s*"([^"]+)"\s*,\s*([^)]+)\)`)
	matches := methodPattern.FindAllStringSubmatchIndex(content, -1)

	for _, loc := range matches {
		fullMatch := content[loc[0]:loc[1]]
		_ = fullMatch
		method := strings.ToUpper(content[loc[4]:loc[5]])
		path := content[loc[6]:loc[7]]
		handler := strings.TrimSpace(content[loc[8]:loc[9]])
		line := countLinesAt(content, loc[0])

		// Skip if this looks like a gin pattern (uppercase method names are gin)
		// chi uses Title case (Get, Post), gin uses all-caps (GET, POST)
		rawMethod := content[loc[4]:loc[5]]
		if rawMethod == strings.ToUpper(rawMethod) {
			continue
		}

		endpoints = append(endpoints, APIEndpoint{
			Method:  method,
			Path:    path,
			Handler: cleanHandler(handler),
			File:    file,
			Line:    line,
		})
	}

	// Match r.Route("/prefix", func(...) { ... }) to extract group prefixes
	routePattern := regexp.MustCompile(`(?m)(\w+)\.Route\(\s*"([^"]+)"`)
	routeMatches := routePattern.FindAllStringSubmatch(content, -1)
	_ = routeMatches

	return endpoints
}

// ScanNetHTTP extracts endpoints from net/http standard library registrations.
// Patterns: http.HandleFunc("/path", handler), http.Handle("/path", handler),
// mux.HandleFunc("/path", handler), mux.Handle("/path", handler)
func ScanNetHTTP(content, file string) []APIEndpoint {
	var endpoints []APIEndpoint

	// http.HandleFunc("/path", handler) or http.Handle("/path", handler)
	// Also mux.HandleFunc, mux.Handle, serveMux.HandleFunc, etc.
	pattern := regexp.MustCompile(`(?m)(\w+)\.(HandleFunc|Handle)\(\s*"([^"]+)"\s*,\s*([^)]+)\)`)
	matches := pattern.FindAllStringSubmatchIndex(content, -1)

	for _, loc := range matches {
		path := content[loc[6]:loc[7]]
		handler := strings.TrimSpace(content[loc[8]:loc[9]])
		line := countLinesAt(content, loc[0])

		endpoints = append(endpoints, APIEndpoint{
			Method:  "ANY",
			Path:    path,
			Handler: cleanHandler(handler),
			File:    file,
			Line:    line,
		})
	}

	// http.HandleFunc with method pattern in Go 1.22+: mux.HandleFunc("GET /path", handler)
	methodPathPattern := regexp.MustCompile(`(?m)(\w+)\.(HandleFunc|Handle)\(\s*"(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\s+([^"]+)"\s*,\s*([^)]+)\)`)
	methodMatches := methodPathPattern.FindAllStringSubmatchIndex(content, -1)

	for _, loc := range methodMatches {
		method := content[loc[6]:loc[7]]
		path := content[loc[8]:loc[9]]
		handler := strings.TrimSpace(content[loc[10]:loc[11]])
		line := countLinesAt(content, loc[0])

		// Remove the ANY entry we may have added above for the same location
		for i := len(endpoints) - 1; i >= 0; i-- {
			if endpoints[i].Line == line && endpoints[i].File == file {
				endpoints = append(endpoints[:i], endpoints[i+1:]...)
				break
			}
		}

		endpoints = append(endpoints, APIEndpoint{
			Method:  method,
			Path:    path,
			Handler: cleanHandler(handler),
			File:    file,
			Line:    line,
		})
	}

	return endpoints
}

// ScanGin extracts endpoints from gin router registrations.
// Patterns: r.GET("/path", handler), r.POST(...), group := r.Group("/api")
func ScanGin(content, file string) []APIEndpoint {
	var endpoints []APIEndpoint

	// Gin uses all-uppercase method names: r.GET, r.POST, r.PUT, r.DELETE, r.PATCH, r.HEAD, r.OPTIONS
	methodPattern := regexp.MustCompile(`(?m)(\w+)\.(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\(\s*"([^"]+)"\s*,\s*([^)]+)\)`)
	matches := methodPattern.FindAllStringSubmatchIndex(content, -1)

	for _, loc := range matches {
		method := content[loc[4]:loc[5]]
		path := content[loc[6]:loc[7]]
		handler := strings.TrimSpace(content[loc[8]:loc[9]])
		line := countLinesAt(content, loc[0])

		endpoints = append(endpoints, APIEndpoint{
			Method:  method,
			Path:    path,
			Handler: cleanHandler(handler),
			File:    file,
			Line:    line,
		})
	}

	// Detect group prefixes: group := r.Group("/api")
	groupPattern := regexp.MustCompile(`(?m)(\w+)\s*:?=\s*\w+\.Group\(\s*"([^"]+)"`)
	groupMatches := groupPattern.FindAllStringSubmatch(content, -1)

	// Build a map of group variable -> prefix
	groupPrefixes := make(map[string]string)
	for _, match := range groupMatches {
		varName := match[1]
		prefix := match[2]
		groupPrefixes[varName] = prefix
	}

	// Resolve group prefixes for endpoints
	for i := range endpoints {
		for varName, prefix := range groupPrefixes {
			// Check if the endpoint's receiver matches a group variable
			line := getLineAt(content, endpoints[i].Line)
			if strings.Contains(line, varName+".") {
				if !strings.HasPrefix(endpoints[i].Path, prefix) {
					endpoints[i].Path = prefix + endpoints[i].Path
				}
			}
		}
	}

	return endpoints
}

// ScanEcho extracts endpoints from echo framework registrations.
// Patterns: e.GET("/path", handler), e.POST(...), g := e.Group("/api")
func ScanEcho(content, file string) []APIEndpoint {
	var endpoints []APIEndpoint

	// Echo uses uppercase: e.GET, e.POST, e.PUT, e.DELETE, e.PATCH
	methodPattern := regexp.MustCompile(`(?m)(\w+)\.(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\(\s*"([^"]+)"\s*,\s*([^)]+)\)`)
	matches := methodPattern.FindAllStringSubmatchIndex(content, -1)

	// Only include if echo is imported
	if !strings.Contains(content, "labstack/echo") && !strings.Contains(content, `"echo"`) {
		return nil
	}

	for _, loc := range matches {
		method := content[loc[4]:loc[5]]
		path := content[loc[6]:loc[7]]
		handler := strings.TrimSpace(content[loc[8]:loc[9]])
		line := countLinesAt(content, loc[0])

		endpoints = append(endpoints, APIEndpoint{
			Method:  method,
			Path:    path,
			Handler: cleanHandler(handler),
			File:    file,
			Line:    line,
		})
	}

	return endpoints
}

// ScanGorilla extracts endpoints from gorilla/mux registrations.
// Patterns: r.HandleFunc("/path", handler).Methods("GET")
func ScanGorilla(content, file string) []APIEndpoint {
	var endpoints []APIEndpoint

	if !strings.Contains(content, "gorilla/mux") && !strings.Contains(content, `"mux"`) {
		// Quick check if it might be gorilla based on patterns
		if !strings.Contains(content, ".Methods(") {
			return nil
		}
	}

	// r.HandleFunc("/path", handler).Methods("GET", "POST")
	pattern := regexp.MustCompile(`(?m)(\w+)\.(HandleFunc|Handle)\(\s*"([^"]+)"\s*,\s*([^)]+)\)\s*\.Methods\(\s*"([^"]+)"`)
	matches := pattern.FindAllStringSubmatchIndex(content, -1)

	for _, loc := range matches {
		path := content[loc[6]:loc[7]]
		handler := strings.TrimSpace(content[loc[8]:loc[9]])
		methodsStr := content[loc[10]:loc[11]]
		line := countLinesAt(content, loc[0])

		// Methods can be comma-separated in the string or multiple args
		methods := strings.Split(methodsStr, `", "`)
		for _, method := range methods {
			method = strings.Trim(method, `"' `)
			endpoints = append(endpoints, APIEndpoint{
				Method:  strings.ToUpper(method),
				Path:    path,
				Handler: cleanHandler(handler),
				File:    file,
				Line:    line,
			})
		}
	}

	// Also catch r.HandleFunc("/path", handler) without .Methods (defaults to ANY)
	anyPattern := regexp.MustCompile(`(?m)(\w+)\.(HandleFunc|Handle)\(\s*"([^"]+)"\s*,\s*([^)]+)\)\s*[^.]`)
	anyMatches := anyPattern.FindAllStringSubmatchIndex(content, -1)

	for _, loc := range anyMatches {
		path := content[loc[6]:loc[7]]
		handler := strings.TrimSpace(content[loc[8]:loc[9]])
		line := countLinesAt(content, loc[0])

		// Skip if already captured with Methods
		duplicate := false
		for _, ep := range endpoints {
			if ep.Path == path && ep.Line == line {
				duplicate = true
				break
			}
		}
		if !duplicate {
			endpoints = append(endpoints, APIEndpoint{
				Method:  "ANY",
				Path:    path,
				Handler: cleanHandler(handler),
				File:    file,
				Line:    line,
			})
		}
	}

	return endpoints
}

// ScanFiber extracts endpoints from gofiber/fiber registrations.
// Patterns: app.Get("/path", handler), app.Post(...), api := app.Group("/api")
func ScanFiber(content, file string) []APIEndpoint {
	var endpoints []APIEndpoint

	if !strings.Contains(content, "gofiber/fiber") && !strings.Contains(content, `"fiber"`) {
		return nil
	}

	// Fiber uses Title case: app.Get, app.Post, app.Put, app.Delete, app.Patch
	methodPattern := regexp.MustCompile(`(?m)(\w+)\.(Get|Post|Put|Delete|Patch|Head|Options)\(\s*"([^"]+)"\s*,\s*([^)]+)\)`)
	matches := methodPattern.FindAllStringSubmatchIndex(content, -1)

	for _, loc := range matches {
		method := strings.ToUpper(content[loc[4]:loc[5]])
		path := content[loc[6]:loc[7]]
		handler := strings.TrimSpace(content[loc[8]:loc[9]])
		line := countLinesAt(content, loc[0])

		endpoints = append(endpoints, APIEndpoint{
			Method:  method,
			Path:    path,
			Handler: cleanHandler(handler),
			File:    file,
			Line:    line,
		})
	}

	return endpoints
}

// DetectFramework examines Go files in dir to determine which HTTP framework is used.
// Returns one of: "chi", "gin", "echo", "gorilla", "fiber", "net/http", or "unknown".
func DetectFramework(dir string) string {
	frameworks := map[string]string{
		"github.com/go-chi/chi":    "chi",
		"github.com/gin-gonic/gin": "gin",
		"github.com/labstack/echo": "echo",
		"github.com/gorilla/mux":   "gorilla",
		"github.com/gofiber/fiber": "fiber",
	}

	// First check go.mod if available
	goModPath := filepath.Join(dir, "go.mod")
	if data, err := os.ReadFile(goModPath); err == nil {
		modContent := string(data)
		for importPath, name := range frameworks {
			if strings.Contains(modContent, importPath) {
				return name
			}
		}
	}

	// Scan Go source files for imports
	counts := make(map[string]int)
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		for importPath, name := range frameworks {
			if strings.Contains(content, importPath) {
				counts[name]++
			}
		}
		if strings.Contains(content, "net/http") {
			counts["net/http"]++
		}
		return nil
	})

	// Return the most used framework
	maxCount := 0
	result := "unknown"
	for name, count := range counts {
		if count > maxCount {
			maxCount = count
			result = name
		}
	}

	return result
}

// FormatAPIMap produces a formatted string representation of the API map.
func FormatAPIMap(apiMap *APIMap) string {
	if apiMap == nil || len(apiMap.Endpoints) == 0 {
		return "API Endpoints (0):\n" + strings.Repeat("═", 19) + "\nNo endpoints found."
	}

	var sb strings.Builder

	count := len(apiMap.Endpoints)
	header := fmt.Sprintf("API Endpoints (%d):", count)
	sb.WriteString(header)
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("═", len(header)))
	sb.WriteString("\n")

	// Calculate column widths
	maxMethod := 0
	maxPath := 0
	for _, ep := range apiMap.Endpoints {
		if len(ep.Method) > maxMethod {
			maxMethod = len(ep.Method)
		}
		if len(ep.Path) > maxPath {
			maxPath = len(ep.Path)
		}
	}

	// Format each endpoint
	for _, ep := range apiMap.Endpoints {
		methodPad := maxMethod - len(ep.Method)
		pathPad := maxPath - len(ep.Path)

		sb.WriteString(fmt.Sprintf("%-*s %s%s → %s\n",
			maxMethod, ep.Method,
			ep.Path, strings.Repeat(" ", pathPad),
			ep.Handler))
		_ = methodPad
	}

	return strings.TrimRight(sb.String(), "\n")
}

// GenerateOpenAPI produces a basic OpenAPI 3.0 YAML skeleton from the API map.
func GenerateOpenAPI(apiMap *APIMap) string {
	if apiMap == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("openapi: \"3.0.0\"\n")
	sb.WriteString("info:\n")
	sb.WriteString("  title: API\n")
	if apiMap.Version != "" {
		sb.WriteString(fmt.Sprintf("  version: \"%s\"\n", apiMap.Version))
	} else {
		sb.WriteString("  version: \"1.0.0\"\n")
	}
	if apiMap.BaseURL != "" {
		sb.WriteString("servers:\n")
		sb.WriteString(fmt.Sprintf("  - url: \"%s\"\n", apiMap.BaseURL))
	}
	sb.WriteString("paths:\n")

	// Group endpoints by path
	pathGroups := make(map[string][]APIEndpoint)
	var orderedPaths []string
	for _, ep := range apiMap.Endpoints {
		if _, exists := pathGroups[ep.Path]; !exists {
			orderedPaths = append(orderedPaths, ep.Path)
		}
		pathGroups[ep.Path] = append(pathGroups[ep.Path], ep)
	}
	sort.Strings(orderedPaths)

	for _, path := range orderedPaths {
		sb.WriteString(fmt.Sprintf("  %s:\n", path))
		for _, ep := range pathGroups[path] {
			method := strings.ToLower(ep.Method)
			if method == "any" {
				method = "get"
			}
			sb.WriteString(fmt.Sprintf("    %s:\n", method))
			if ep.Description != "" {
				sb.WriteString(fmt.Sprintf("      summary: \"%s\"\n", ep.Description))
			} else {
				sb.WriteString(fmt.Sprintf("      summary: \"%s\"\n", ep.Handler))
			}
			sb.WriteString("      responses:\n")
			sb.WriteString("        \"200\":\n")
			sb.WriteString("          description: OK\n")
		}
	}

	return sb.String()
}

// countLinesAt returns the 1-based line number of the byte offset pos within content.
func countLinesAt(content string, pos int) int {
	return strings.Count(content[:pos], "\n") + 1
}

// getLineAt returns the content of the line at the given 1-based line number.
func getLineAt(content string, lineNum int) string {
	lines := strings.Split(content, "\n")
	if lineNum < 1 || lineNum > len(lines) {
		return ""
	}
	return lines[lineNum-1]
}

// cleanHandler removes common noise from handler references.
func cleanHandler(handler string) string {
	handler = strings.TrimSpace(handler)
	// Remove trailing commas
	handler = strings.TrimRight(handler, ",")
	// Remove newlines and extra whitespace
	handler = strings.Join(strings.Fields(handler), " ")
	// If it's a multiline handler with multiple args, take the first meaningful one
	if idx := strings.Index(handler, ","); idx > 0 {
		// Keep just the last meaningful part (handler function)
		parts := strings.Split(handler, ",")
		handler = strings.TrimSpace(parts[len(parts)-1])
	}
	return handler
}
