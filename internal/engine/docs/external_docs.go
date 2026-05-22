package docs

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

type DocSource struct {
	Name     string
	BaseURL  string
	Packages []string
	Language string
	Priority int
}

type DocResult struct {
	Source    string
	Title     string
	Content   string
	URL       string
	Relevance float64
	Tokens    int
}

type ExternalDocs struct {
	Sources   []DocSource
	Cache     map[string]*DocResult
	MaxTokens int
	mu        sync.RWMutex
}

func NewExternalDocs() *ExternalDocs {
	ed := &ExternalDocs{
		Cache:     make(map[string]*DocResult),
		MaxTokens: 4096,
	}
	ed.Sources = defaultSources()
	return ed
}

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

	sort.Slice(results, func(i, j int) bool {
		return results[i].Relevance > results[j].Relevance
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results
}

func (ed *ExternalDocs) ExtractPackageRefs(text string) []string {
	if text == "" {
		return nil
	}

	textLower := strings.ToLower(text)

	words := extractWords(textLower)

	ed.mu.RLock()
	defer ed.mu.RUnlock()

	seen := make(map[string]bool)
	var refs []string

	for _, src := range ed.Sources {
		for _, pkg := range src.Packages {
			pkgLower := strings.ToLower(pkg)
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

func (ed *ExternalDocs) BuildDocContext(results []DocResult, budget int) string {
	if len(results) == 0 {
		return ""
	}
	if budget <= 0 {
		budget = ed.MaxTokens
	}

	var b strings.Builder
	b.WriteString("## Relevant Documentation\n\n")
	usedTokens := 10

	for _, r := range results {
		entry := formatDocEntry(r)
		entryTokens := len(entry) / 4
		if usedTokens+entryTokens > budget {
			break
		}
		b.WriteString(entry)
		b.WriteString("\n")
		usedTokens += entryTokens
	}

	return b.String()
}

func (ed *ExternalDocs) RegisterSource(source DocSource) {
	ed.mu.Lock()
	defer ed.mu.Unlock()
	ed.Sources = append(ed.Sources, source)
}

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

var packageRefPatterns = regexp.MustCompile(
	`(?i)\b(?:use|using|import|require|add|install|` +
		`include|depend(?:s|ency)?|with|integrate)\s+([a-zA-Z0-9_\-/.@]+)`,
)

func extractWords(text string) map[string]bool {
	words := make(map[string]bool)
	parts := regexp.MustCompile(`[^a-zA-Z0-9_\-/.@]+`).Split(text, -1)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			words[strings.ToLower(p)] = true
		}
	}
	matches := packageRefPatterns.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		if len(m) > 1 {
			words[strings.ToLower(m[1])] = true
		}
	}
	return words
}

func matchesPackageRef(textLower string, words map[string]bool, pkgLower string) bool {
	if words[pkgLower] {
		return true
	}
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

	score := 0.5

	if strings.Contains(taskLower, pkgLower) {
		score += 0.3
	}

	score += float64(priority) * 0.02

	if score > 1.0 {
		score = 1.0
	}
	return score
}

func estimateDocTokens(pkg string) int {
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

func defaultSources() []DocSource {
	return []DocSource{
		{
			Name:    "pkg.go.dev",
			BaseURL: "https://pkg.go.dev",
			Packages: []string{
				"fmt", "net/http", "os", "io", "context", "sync", "encoding/json",
				"database/sql", "crypto", "testing", "reflect", "strings", "strconv",
				"path/filepath", "regexp", "time", "math", "sort", "errors",
				"bufio", "bytes", "log", "flag", "html/template", "text/template",
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
				"express", "fastify", "koa", "hapi", "nest",
				"next", "nuxt", "remix", "astro", "svelte",
				"react", "vue", "angular", "solid", "preact",
				"lodash", "ramda", "underscore",
				"axios", "got", "node-fetch", "superagent",
				"moment", "dayjs", "date-fns", "luxon",
				"uuid", "nanoid", "cuid",
				"zod", "yup", "joi", "ajv",
				"prisma", "typeorm", "sequelize", "knex", "drizzle",
				"mongoose", "ioredis", "pg", "mysql2", "better-sqlite3",
				"jest", "vitest", "mocha", "chai", "sinon",
				"playwright", "cypress", "puppeteer",
				"supertest", "nock", "msw",
				"webpack", "vite", "esbuild", "rollup", "parcel", "turbopack",
				"typescript", "babel", "swc",
				"eslint", "prettier", "biome",
				"passport", "jsonwebtoken", "bcrypt", "helmet",
				"socket.io", "ws", "bullmq", "amqplib",
				"aws-sdk", "firebase", "supabase",
				"redux", "zustand", "mobx", "jotai", "recoil", "pinia",
				"tailwindcss", "styled-components", "emotion",
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
