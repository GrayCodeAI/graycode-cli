# Contributing to Hawk

Thanks for your interest in contributing to hawk! This guide will help you get started.

## Development Setup

```bash
# Clone
git clone https://github.com/GrayCodeAI/hawk.git
cd hawk

# Build
make build

# Run tests
make test

# Run linter
make lint
```

## Making Changes

1. Fork the repo and create a branch from `dev`
2. Make your changes
3. Add tests for new functionality
4. Ensure `make test` and `make lint` pass
5. Open a PR against `dev`

## Code Standards

- Go 1.26+ with modules
- All errors must be handled (no unchecked return values)
- Use `context.Context` for cancellation and timeouts
- Use structured logging via `log/slog`
- Table-driven tests with `t.Parallel()` where safe
- No global mutable state

## Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(engine): add token budget tracking
fix(session): prevent WAL corruption on crash
docs(readme): update install instructions
test(tool): add fuzz tests for bash command parsing
```

## Testing

```bash
make test          # Unit tests with race detector
make test-coverage # Tests with coverage report
make bench         # Benchmarks
```

## Architecture

See `CLAUDE.md` for a complete architecture overview.

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
