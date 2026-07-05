---
name: go-review
description: Reviews Go code for idioms, error handling, concurrency, and performance patterns
version: "1.0.0"
author: graycode
license: MIT
category: engineering
tags: ["go", "review", "code-quality"]
allowed-tools: Read Grep Glob
---

# Go Code Review

## When to Use
- Reviewing Go code for idiomatic patterns
- Checking error handling completeness
- Auditing concurrency safety (goroutines, channels, mutexes)
- Identifying performance issues

## Workflow
1. Read the target files
2. Check error handling: every error must be checked, no `_ = err`
3. Verify naming: MixedCaps, not underscores; acronyms all-caps (HTTP, ID)
4. Check for goroutine leaks: every goroutine must have a shutdown path
5. Verify context propagation: functions accepting context.Context as first param
6. Check for unnecessary allocations in hot paths
7. Ensure interfaces are small (1-3 methods)
8. Verify test coverage for exported functions

## Patterns

### Error wrapping
```go
// Good
return fmt.Errorf("open config: %w", err)

// Bad
return err
```

### Context propagation
```go
// Good
func DoWork(ctx context.Context, id string) error {

// Bad
func DoWork(id string) error {
```

### Goroutine lifecycle
```go
// Good — goroutine has shutdown path
go func() {
    select {
    case <-ctx.Done():
        return
    case msg := <-ch:
        process(msg)
    }
}()
```

## Verification
- All errors are handled or explicitly ignored with comment
- No data races (run `go vet -race`)
- Exported functions have doc comments
- No `init()` functions unless absolutely necessary
