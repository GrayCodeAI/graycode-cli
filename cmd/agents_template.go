package cmd

import "os"

// detectAgentsProjectType checks files in cwd to determine the project type for AGENTS.md generation.
func detectAgentsProjectType() string {
	checks := []struct {
		file string
		typ  string
	}{
		{"go.mod", "go"},
		{"Cargo.toml", "rust"},
		{"package.json", "node"},
		{"pyproject.toml", "python"},
		{"requirements.txt", "python"},
	}
	for _, c := range checks {
		if _, err := os.Stat(c.file); err == nil {
			return c.typ
		}
	}
	return "generic"
}

// GenerateAgentsTemplate returns an AGENTS.md template for the given project type.
func GenerateAgentsTemplate(projectType string) string {
	var base string
	switch projectType {
	case "go":
		base = goTemplate
	case "node":
		base = nodeTemplate
	case "python":
		base = pythonTemplate
	case "rust":
		base = rustTemplate
	default:
		base = genericTemplate
	}
	return base + behavioralPrinciples
}

const goTemplate = `# Project Instructions

## Scope
- You CAN: read, edit, build, and test Go code in this repo.
- You CANNOT: modify CI/CD configs, push to remote, or change go.mod dependencies without asking.

## Context
- Language: Go
- Build: go build ./...
- Test: go test ./...
- Lint: golangci-lint run ./...

## Conventions
- Follow standard Go style (gofmt, goimports).
- Exported names use PascalCase; unexported use camelCase.
- One package per directory. Package name matches directory name.
- Errors are values — return them, don't panic.
- Table-driven tests with t.Run subtests.

## Workflow
1. Read relevant code before editing.
2. Make the smallest change that solves the problem.
3. Run go build ./... — fix any errors.
4. Run go test ./... — all tests must pass.
5. Commit with a concise message describing what changed and why.
`

const nodeTemplate = `# Project Instructions

## Scope
- You CAN: read, edit, build, and test JavaScript/TypeScript code in this repo.
- You CANNOT: run npm publish, modify CI configs, or add dependencies without asking.

## Context
- Language: JavaScript/TypeScript
- Install: npm install
- Build: npm run build
- Test: npm test
- Lint: npm run lint

## Conventions
- Use const/let, never var.
- Prefer async/await over raw promises.
- Name files in kebab-case. Name exports in camelCase (functions) or PascalCase (classes/components).
- Keep functions small and focused.
- Tests live next to source files or in __tests__ directories.

## Workflow
1. Read relevant code before editing.
2. Make the smallest change that solves the problem.
3. Run npm run build — fix any errors.
4. Run npm test — all tests must pass.
5. Commit with a concise message describing what changed and why.
`

const pythonTemplate = `# Project Instructions

## Scope
- You CAN: read, edit, and test Python code in this repo.
- You CANNOT: modify CI configs, publish packages, or add dependencies without asking.

## Context
- Language: Python
- Install: pip install -e . or pip install -r requirements.txt
- Test: pytest
- Lint: ruff check .
- Format: ruff format .

## Conventions
- Follow PEP 8. Use snake_case for functions/variables, PascalCase for classes.
- Type hints on all public functions.
- Docstrings on all public modules, classes, and functions.
- Tests in tests/ directory, files prefixed with test_.

## Workflow
1. Read relevant code before editing.
2. Make the smallest change that solves the problem.
3. Run pytest — all tests must pass.
4. Run ruff check . — fix any lint issues.
5. Commit with a concise message describing what changed and why.
`

const rustTemplate = `# Project Instructions

## Scope
- You CAN: read, edit, build, and test Rust code in this repo.
- You CANNOT: modify CI configs, publish crates, or add dependencies without asking.

## Context
- Language: Rust
- Build: cargo build
- Test: cargo test
- Lint: cargo clippy -- -D warnings
- Format: cargo fmt

## Conventions
- Follow Rust API guidelines. Use snake_case for functions/variables, PascalCase for types.
- Handle errors with Result — avoid unwrap() in library code.
- Keep unsafe blocks minimal and documented.
- Tests in the same file (#[cfg(test)] mod tests) or in tests/ directory.

## Workflow
1. Read relevant code before editing.
2. Make the smallest change that solves the problem.
3. Run cargo build — fix any errors.
4. Run cargo test — all tests must pass.
5. Run cargo clippy — fix any warnings.
6. Commit with a concise message describing what changed and why.
`

const genericTemplate = `# Project Instructions

## Scope
- You CAN: read, edit, build, and test code in this repo.
- You CANNOT: modify CI/CD configs, deploy, or add dependencies without asking.

## Context
- Language: [FILL IN]
- Build: [FILL IN]
- Test: [FILL IN]
- Lint: [FILL IN]

## Conventions
- Follow the existing code style in this repo.
- Keep changes minimal and focused.
- Write tests for new functionality.

## Workflow
1. Read relevant code before editing.
2. Make the smallest change that solves the problem.
3. Build and fix any errors.
4. Run tests — all must pass.
5. Commit with a concise message describing what changed and why.
`

// behavioralPrinciples is appended to every AGENTS.md template.
// Based on Andrej Karpathy's guidelines for reducing common LLM coding mistakes.
const behavioralPrinciples = `
## Principles

### Think Before Coding
- State assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them — don't pick silently.
- If a simpler approach exists, say so.

### Simplicity First
- No features beyond what was asked.
- No abstractions for single-use code.
- If you write 200 lines and it could be 50, rewrite it.

### Surgical Changes
- Don't "improve" adjacent code, comments, or formatting.
- Match existing style, even if you'd do it differently.
- Every changed line should trace directly to the request.

### Goal-Driven Execution
- Transform tasks into verifiable goals with success criteria.
- Loop until verified: build passes, tests pass, lint clean.
`
