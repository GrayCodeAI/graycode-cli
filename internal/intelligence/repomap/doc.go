// Package repomap is the deep code-analysis engine that powers hawk's
// repository-mapping, symbol-extraction, and code-quality features.
//
// The package is built around the entry point Generate (in repomap.go), which
// walks a directory, dispatches each supported source file to a language-aware
// parser, and returns a RepoMap: a token-budgeted summary of files and their
// top-level symbols suitable for injection into LLM prompts. Around that core
// it accumulates a number of specialised analyses that all share the same
// parsed-symbol substrate:
//
//   - Static analysis: call graph, import/dep graph, type hierarchy,
//     interface extraction, code ownership, co-change statistics, and a
//     Shapley-value ranker.
//   - Search and navigation: BM25 semantic search, symbol-level PageRank,
//     reranking, and a "go to definition / find references / find
//     implementations" index that works without an LSP server.
//   - Quality signals: cyclomatic complexity, code smells, repository health
//     score, doc linter, dead-code detector, and a migration detector for
//     deprecated language idioms.
//   - API surface: a per-framework HTTP route scanner (Chi, net/http, Gin,
//     Echo, Gorilla, Fiber) with OpenAPI export.
//   - Incremental indexing: file-hash-keyed caches and an fsnotify-based
//     watcher so that regeneration only re-processes files that changed.
//
// The package is intentionally stdlib-only at its core (go/parser, go/ast,
// go/token, encoding/*); the only third-party dependency is github.com/fsnotify
// /fsnotify for file watching, plus hawk's own internal/scoring and
// internal/ui/icons helpers where they are used. Tree-sitter is not required:
// Go is parsed with go/ast and other languages are handled by an enhanced
// regex extractor (see treesitter.go and parser_langs.go).
//
// # Dual-package relationship
//
// hawk ships a second package, internal/context/repomap, that exposes a much
// narrower surface - essentially just RepoMap(root, budget) (string, error).
// That package is the prompt-injection shim: it is what hawk's context layer
// calls when it needs a budgeted overview for the system prompt. It does its
// own AST parsing, PageRank pass, and rendering, and shares no code with this
// package beyond the name. The package you are reading is the deeper
// analysis engine that drives the higher-level navigators, quality tools, and
// search index, not the prompt-injection shim. See internal/context/repomap's
// own package comment for the shim side of the boundary.
//
// # Extension points
//
//   - Add a new language parser by adding a new case to parseFileSymbols in
//     repomap.go and a parseX function in parser_langs.go (or a new
//     TreeSitterParser method in treesitter.go for languages that need
//     scope-aware extraction).
//   - Add a new code smell or quality heuristic by adding a Detector field
//     on SmellDetector in smells.go and wiring it into ScanDirectory.
//   - Add a new HTTP framework scanner by adding a ScanX function in
//     api_scanner.go and a case in DetectFramework.
//
// # Performance and scaling
//
// Generate is O(N) in the number of files with a hard cap of Options.MaxFiles
// (default 500). The symbol cache (cache.go) is an in-process LRU keyed by
// (path, modtime) that is cleared on process exit; the IncrementalMap
// (incremental_map.go) provides a persistent on-disk cache at
// .hawk/repomap-cache.json keyed by SHA-256. Callers that need to operate on
// repositories with tens of thousands of files should set MaxFiles
// appropriately and prefer the incremental path. Static-analysis passes such
// as BuildCallGraph, BuildDepGraph, and the PageRank iteration in pagerank.go
// are O(V + E) per iteration over the symbol graph and scale linearly with
// the number of declarations, not the number of lines.
package repomap
