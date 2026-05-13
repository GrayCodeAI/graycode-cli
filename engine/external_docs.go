package engine

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// DocSource represents an external documentation source for a language ecosystem.
type DocSource struct {
	Name     string   // e.g. "pkg.go.dev", "MDN"
	BaseURL  string   // e.g. "https://pkg.go.dev"
	Packages []string // known packages served by this source
	Language string   // "go", "python", "javascript", "typescript", "rust"
	Priority int      // higher = preferred when multiple sources match
}

// DocResult represents a documentation reference matched for a task.
type DocResult struct {
	Source    string  // source name that provided this result
	Title     string  // human-readable title
	Content   string  // documentation excerpt or summary
	URL       string  // full URL to the documentation page
	Relevance float64 // 0.0-1.0 relevance score
	Tokens    int     // estimated token count
}

// ExternalDocs manages external documentation sources and finds relevant docs
// for coding tasks. Inspired by gpt-pilot's external_docs agent.
type ExternalDocs struct {
	Sources   []DocSource
	Cache     map[string]*DocResult
	MaxTokens int
	mu        sync.RWMutex
}

// NewExternalDocs creates an ExternalDocs instance pre-loaded with known sources
// for Go, Python, JavaScript/TypeScript, and Rust ecosystems.
func NewExternalDocs() *ExternalDocs {
	ed := &ExternalDocs{
		Cache:     make(map[string]*DocResult),
		MaxTokens: 4096,
	}
	ed.Sources = defaultSources()
	return ed
}

// FindRelevant analyzes a task description and returns relevant documentation
// references, limited to the specified count.
func (ed *ExternalDocs) FindRelevant(task string, language string, limit int) []DocResult {
	if limit <= 0 {
		limit = 5
	}

	refs := ed.ExtractPackageRefs(task)
	if len(refs) == 0 {
		return nil
	}

	ed.mu.RLock()
	defer ed.mu.RUnlock()

	var results []DocResult

	for _, ref := range refs {
		refLower := strings.ToLower(ref)
		for _, src := range ed.Sources {
			if language != "" && src.Language != language && src.Language != "common" {
				continue
			}
			for _, pkg := range src.Packages {
				if strings.ToLower(pkg) == refLower {
					result := DocResult{
						Source:    src.Name,
						Title:     fmt.Sprintf("%s - %s", pkg, src.Name),
						Content:   fmt.Sprintf("Documentation for %s package from %s", pkg, src.Name),
						URL:       buildDocURL(src, pkg),
						Relevance: computeRelevance(task, pkg, src.Priority),
						Tokens:    estimateDocTokens(pkg),
					}

					// Check cache
					cacheKey := src.Name + ":" + pkg
					if cached, ok := ed.Cache[cacheKey]; ok {
						result.Content = cached.Content
						result.Tokens = cached.Tokens
					}

					results = append(results, result)
					break
				}
			}
		}
	}

	// Sort by relevance descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Relevance > results[j].Relevance
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results
}

// ExtractPackageRefs finds package/library names mentioned in text by matching
// against the known package database.
func (ed *ExternalDocs) ExtractPackageRefs(text string) []string {
	if text == "" {
		return nil
	}

	textLower := strings.ToLower(text)

	// Extract words and multi-word tokens from the text
	words := extractWords(textLower)

	ed.mu.RLock()
	defer ed.mu.RUnlock()

	seen := make(map[string]bool)
	var refs []string

	// Match against known packages across all sources
	for _, src := range ed.Sources {
		for _, pkg := range src.Packages {
			pkgLower := strings.ToLower(pkg)
			// Check if the package name appears in extracted words
			// or as a substring in common patterns
			if matchesPackageRef(textLower, words, pkgLower) {
				if !seen[pkg] {
					seen[pkg] = true
					refs = append(refs, pkg)
				}
			}
		}
	}

	return refs
}

// BuildDocContext formats documentation results into a string suitable for
// prompt injection, staying within the given token budget.
func (ed *ExternalDocs) BuildDocContext(results []DocResult, budget int) string {
	if len(results) == 0 {
		return ""
	}
	if budget <= 0 {
		budget = ed.MaxTokens
	}

	var b strings.Builder
	b.WriteString("## Relevant Documentation\n\n")
	usedTokens := 10 // header overhead

	for _, r := range results {
		entry := formatDocEntry(r)
		entryTokens := len(entry) / 4 // rough token estimate
		if usedTokens+entryTokens > budget {
			break
		}
		b.WriteString(entry)
		b.WriteString("\n")
		usedTokens += entryTokens
	}

	return b.String()
}

// RegisterSource adds a new documentation source to the registry.
func (ed *ExternalDocs) RegisterSource(source DocSource) {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	ed.Sources = append(ed.Sources, source)
}

// FormatResults produces a human-readable summary of documentation results.
func (ed *ExternalDocs) FormatResults(results []DocResult) string {
	if len(results) == 0 {
		return "No relevant documentation found."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Found %d relevant documentation references:\n\n", len(results)))

	for i, r := range results {
		b.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, r.Source, r.Title))
		b.WriteString(fmt.Sprintf("   URL: %s\n", r.URL))
		b.WriteString(fmt.Sprintf("   Relevance: %.0f%%\n", r.Relevance*100))
		if r.Content != "" {
			// Truncate content for display
			content := r.Content
			if len(content) > 120 {
				content = content[:120] + "..."
			}
			b.WriteString(fmt.Sprintf("   %s\n", content))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// --- Internal helpers ---

// packageRefPatterns match common patterns like "use X", "import X", "add X"
var packageRefPatterns = regexp.MustCompile(
	`(?i)\b(?:use|using|import|require|add|install|` +
		`include|depend(?:s|ency)?|with|integrate)\s+([a-zA-Z0-9_\-/.@]+)`)

func extractWords(text string) map[string]bool {
	words := make(map[string]bool)
	// Split on common separators
	parts := regexp.MustCompile(`[^a-zA-Z0-9_\-/.@]+`).Split(text, -1)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			words[strings.ToLower(p)] = true
		}
	}
	// Also extract from patterns like "use chi router" -> "chi"
	matches := packageRefPatterns.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		if len(m) > 1 {
			words[strings.ToLower(m[1])] = true
		}
	}
	return words
}

func matchesPackageRef(textLower string, words map[string]bool, pkgLower string) bool {
	// Direct word match
	if words[pkgLower] {
		return true
	}
	// Check if the package appears after a keyword in the text
	patterns := []string{
		"use " + pkgLower,
		"using " + pkgLower,
		"import " + pkgLower,
		"add " + pkgLower,
		"install " + pkgLower,
		"with " + pkgLower,
		"require " + pkgLower,
		"integrate " + pkgLower,
	}
	for _, p := range patterns {
		if strings.Contains(textLower, p) {
			return true
		}
	}
	return false
}

func buildDocURL(src DocSource, pkg string) string {
	switch src.Name {
	case "pkg.go.dev":
		return fmt.Sprintf("https://pkg.go.dev/%s", pkg)
	case "docs.python.org":
		return fmt.Sprintf("https://docs.python.org/3/library/%s.html", pkg)
	case "pypi":
		return fmt.Sprintf("https://pypi.org/project/%s/", pkg)
	case "MDN":
		return fmt.Sprintf("https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/%s", pkg)
	case "nodejs.org":
		return fmt.Sprintf("https://nodejs.org/api/%s.html", pkg)
	case "npmjs.com":
		return fmt.Sprintf("https://www.npmjs.com/package/%s", pkg)
	case "docs.rs":
		return fmt.Sprintf("https://docs.rs/%s/latest/%s/", pkg, pkg)
	case "github":
		return fmt.Sprintf("https://github.com/%s", pkg)
	default:
		return src.BaseURL + "/" + pkg
	}
}

func computeRelevance(task string, pkg string, priority int) float64 {
	taskLower := strings.ToLower(task)
	pkgLower := strings.ToLower(pkg)

	score := 0.5 // base relevance

	// Boost if package name is directly mentioned
	if strings.Contains(taskLower, pkgLower) {
		score += 0.3
	}

	// Boost based on source priority
	score += float64(priority) * 0.02

	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}
	return score
}

func estimateDocTokens(pkg string) int {
	// Base token estimate for a doc page reference
	return 200 + len(pkg)*2
}

func formatDocEntry(r DocResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("### %s\n", r.Title))
	b.WriteString(fmt.Sprintf("Source: %s | URL: %s\n", r.Source, r.URL))
	if r.Content != "" {
		b.WriteString(r.Content)
		b.WriteString("\n")
	}
	return b.String()
}

// defaultSources returns the built-in documentation sources covering 200+ packages.
func defaultSources() []DocSource {
	return []DocSource{
		// Go ecosystem
		{
			Name:    "pkg.go.dev",
			BaseURL: "https://pkg.go.dev",
			Packages: []string{
				// stdlib
				"fmt", "net/http", "os", "io", "context", "sync", "encoding/json",
				"database/sql", "crypto", "testing", "reflect", "strings", "strconv",
				"path/filepath", "regexp", "time", "math", "sort", "errors",
				"bufio", "bytes", "log", "flag", "html/template", "text/template",
				// popular third-party
				"chi", "gin", "echo", "fiber", "mux",
				"cobra", "viper", "pflag",
				"zap", "logrus", "zerolog",
				"testify", "gomock", "ginkgo", "gomega",
				"sqlx", "gorm", "ent", "sqlc", "pgx",
				"wire", "fx", "dig",
				"grpc", "protobuf", "twirp",
				"redis", "go-redis",
				"sarama", "confluent-kafka-go",
				"prometheus", "otel", "opentelemetry",
				"aws-sdk-go", "azure-sdk-for-go", "google-cloud-go",
				"jwt-go", "golang-jwt",
				"validator", "go-playground-validator",
				"uuid", "ulid",
				"fsnotify", "viper",
				"colly", "goquery",
				"excelize", "go-pdf",
				"badger", "bbolt", "pebble",
				"ristretto", "groupcache",
				"chromedp", "rod",
			},
			Language: "go",
			Priority: 9,
		},
		// Python ecosystem
		{
			Name:    "docs.python.org",
			BaseURL: "https://docs.python.org/3",
			Packages: []string{
				"os", "sys", "json", "re", "typing", "pathlib", "collections",
				"itertools", "functools", "dataclasses", "abc", "enum",
				"asyncio", "threading", "multiprocessing", "concurrent",
				"unittest", "logging", "argparse", "configparser",
				"urllib", "http", "socket", "ssl", "email",
				"sqlite3", "csv", "xml", "html",
				"datetime", "math", "random", "statistics",
				"subprocess", "shutil", "tempfile", "glob",
				"inspect", "importlib", "pkgutil",
				"hashlib", "hmac", "secrets",
				"struct", "array", "ctypes",
			},
			Language: "python",
			Priority: 8,
		},
		{
			Name:    "pypi",
			BaseURL: "https://pypi.org",
			Packages: []string{
				"flask", "django", "fastapi", "starlette", "sanic", "tornado",
				"requests", "httpx", "aiohttp", "urllib3",
				"pandas", "numpy", "scipy", "matplotlib", "seaborn", "plotly",
				"scikit-learn", "tensorflow", "pytorch", "keras", "xgboost",
				"sqlalchemy", "alembic", "peewee", "tortoise-orm",
				"celery", "rq", "dramatiq", "huey",
				"pytest", "hypothesis", "tox", "nox", "coverage",
				"pydantic", "marshmallow", "attrs", "cattrs",
				"click", "typer", "rich", "textual",
				"pillow", "opencv-python", "imageio",
				"boto3", "google-cloud", "azure",
				"beautifulsoup4", "scrapy", "selenium", "playwright",
				"redis", "pymongo", "motor", "elasticsearch",
				"uvicorn", "gunicorn", "hypercorn",
				"poetry", "pip", "setuptools", "wheel",
				"black", "ruff", "mypy", "pylint", "flake8",
				"jinja2", "mako",
				"cryptography", "pyjwt", "passlib",
				"arrow", "pendulum", "python-dateutil",
				"pyyaml", "toml", "orjson",
				"loguru", "structlog",
			},
			Language: "python",
			Priority: 7,
		},
		// JavaScript/TypeScript ecosystem
		{
			Name:    "MDN",
			BaseURL: "https://developer.mozilla.org",
			Packages: []string{
				"fetch", "Promise", "Map", "Set", "WeakMap", "WeakSet",
				"Proxy", "Reflect", "Symbol", "Iterator", "Generator",
				"ArrayBuffer", "SharedArrayBuffer", "DataView",
				"WebSocket", "EventSource", "Worker", "ServiceWorker",
				"IntersectionObserver", "MutationObserver", "ResizeObserver",
				"URL", "URLSearchParams", "FormData", "Headers",
				"AbortController", "ReadableStream", "WritableStream",
				"Intl", "Temporal",
			},
			Language: "javascript",
			Priority: 9,
		},
		{
			Name:    "nodejs.org",
			BaseURL: "https://nodejs.org/api",
			Packages: []string{
				"fs", "path", "http", "https", "net", "crypto",
				"stream", "buffer", "events", "child_process",
				"cluster", "worker_threads", "os", "util",
				"assert", "test", "readline", "url", "querystring",
				"zlib", "dns", "tls", "dgram",
			},
			Language: "javascript",
			Priority: 8,
		},
		{
			Name:    "npmjs.com",
			BaseURL: "https://www.npmjs.com",
			Packages: []string{
				// Frameworks
				"express", "fastify", "koa", "hapi", "nest",
				"next", "nuxt", "remix", "astro", "svelte",
				"react", "vue", "angular", "solid", "preact",
				// Utilities
				"lodash", "ramda", "underscore",
				"axios", "got", "node-fetch", "superagent",
				"moment", "dayjs", "date-fns", "luxon",
				"uuid", "nanoid", "cuid",
				"zod", "yup", "joi", "ajv",
				// Database
				"prisma", "typeorm", "sequelize", "knex", "drizzle",
				"mongoose", "ioredis", "pg", "mysql2", "better-sqlite3",
				// Testing
				"jest", "vitest", "mocha", "chai", "sinon",
				"playwright", "cypress", "puppeteer",
				"supertest", "nock", "msw",
				// Build / Tooling
				"webpack", "vite", "esbuild", "rollup", "parcel", "turbopack",
				"typescript", "babel", "swc",
				"eslint", "prettier", "biome",
				// Auth / Security
				"passport", "jsonwebtoken", "bcrypt", "helmet",
				// Messaging
				"socket.io", "ws", "bullmq", "amqplib",
				// Cloud / Infra
				"aws-sdk", "firebase", "supabase",
				// State management
				"redux", "zustand", "mobx", "jotai", "recoil", "pinia",
				// CSS
				"tailwindcss", "styled-components", "emotion",
				// Misc
				"dotenv", "commander", "chalk", "inquirer", "ora",
				"winston", "pino", "morgan",
				"sharp", "jimp", "canvas",
				"cheerio", "puppeteer", "jsdom",
				"glob", "chokidar", "fs-extra",
				"rxjs", "immer",
			},
			Language: "javascript",
			Priority: 7,
		},
		// Rust ecosystem
		{
			Name:    "docs.rs",
			BaseURL: "https://docs.rs",
			Packages: []string{
				"tokio", "async-std", "smol",
				"serde", "serde_json", "serde_yaml", "toml",
				"actix-web", "axum", "warp", "rocket", "hyper",
				"reqwest", "surf",
				"sqlx", "diesel", "sea-orm",
				"clap", "structopt",
				"tracing", "log", "env_logger",
				"anyhow", "thiserror", "eyre",
				"rayon", "crossbeam",
				"regex", "once_cell", "lazy_static",
				"rand", "uuid",
				"chrono", "time",
				"itertools", "num",
				"bytes", "nom", "pest",
				"tonic", "prost",
				"rusqlite", "redis",
				"tower", "tower-http",
				"dashmap", "parking_lot",
				"tempfile", "walkdir", "globset",
			},
			Language: "rust",
			Priority: 8,
		},
		// Common / cross-language (GitHub READMEs)
		{
			Name:    "github",
			BaseURL: "https://github.com",
			Packages: []string{
				"docker", "kubernetes", "terraform", "ansible",
				"graphql", "grpc", "protobuf", "openapi",
				"postgres", "mysql", "mongodb", "redis", "elasticsearch",
				"kafka", "rabbitmq", "nats",
				"nginx", "envoy", "traefik",
				"prometheus", "grafana", "jaeger",
				"github-actions", "gitlab-ci", "jenkins",
			},
			Language: "common",
			Priority: 5,
		},
	}
}
