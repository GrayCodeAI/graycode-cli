---
name: test-driven-development
description: Drives development with tests. Use when implementing any logic, fixing any bug, or changing any behavior. Use when you need to prove that code works, when a bug report arrives, or when you're about to modify existing functionality.
version: "1.0.0"
author: "graycode"
license: "MIT"
category: testing
tags: ["testing", "tdd", "workflow"]
allowed-tools: ["Read", "Write", "Edit", "Glob", "Grep", "Bash"]
---

# Test-Driven Development

## Overview

Write a failing test before writing the code that makes it pass. For bug fixes, reproduce the bug with a test before attempting a fix. Tests are proof — "seems right" is not done. A codebase with good tests is an AI agent's superpower; a codebase without tests is a liability.

## When to Use

- Implementing any new logic or behavior
- Fixing any bug (the Prove-It Pattern)
- Modifying existing functionality
- Adding edge case handling
- Any change that could break existing behavior

**When NOT to use:** Pure configuration changes, documentation updates, or static content changes that have no behavioral impact.

## The TDD Cycle

```
    RED                GREEN              REFACTOR
 Write a test    Write minimal code    Clean up the
 that fails  -->  to make it pass  -->  implementation  -->  (repeat)
      |                  |                    |
      v                  v                    v
   Test FAILS        Test PASSES         Tests still PASS
```

### Step 1: RED — Write a Failing Test

Write the test first. It must fail. A test that passes immediately proves nothing.

```go
// RED: This test fails because CreateTask doesn't exist yet
func TestCreateTask(t *testing.T) {
    task, err := CreateTask(TaskInput{Title: "Buy groceries"})
    require.NoError(t, err)

    assert.NotEmpty(t, task.ID)
    assert.Equal(t, "Buy groceries", task.Title)
    assert.Equal(t, "pending", task.Status)
    assert.False(t, task.CreatedAt.IsZero())
}
```

### Step 2: GREEN — Make It Pass

Write the minimum code to make the test pass. Don't over-engineer:

```go
// GREEN: Minimal implementation
func CreateTask(input TaskInput) (*Task, error) {
    task := &Task{
        ID:        generateID(),
        Title:     input.Title,
        Status:    "pending",
        CreatedAt: time.Now(),
    }
    if err := db.Insert(task); err != nil {
        return nil, fmt.Errorf("create task: %w", err)
    }
    return task, nil
}
```

### Step 3: REFACTOR — Clean Up

With tests green, improve the code without changing behavior:

- Extract shared logic
- Improve naming
- Remove duplication
- Optimize if necessary

Run tests after every refactor step to confirm nothing broke.

## The Prove-It Pattern (Bug Fixes)

When a bug is reported, **do not start by trying to fix it.** Start by writing a test that reproduces it.

```
Bug report arrives
       |
       v
  Write a test that demonstrates the bug
       |
       v
  Test FAILS (confirming the bug exists)
       |
       v
  Implement the fix
       |
       v
  Test PASSES (proving the fix works)
       |
       v
  Run full test suite (no regressions)
```

**Example:**

```go
// Bug: "Completing a task doesn't update the completedAt timestamp"

// Step 1: Write the reproduction test (it should FAIL)
func TestCompleteTask_SetsCompletedAt(t *testing.T) {
    task, _ := CreateTask(TaskInput{Title: "Test"})
    completed, err := CompleteTask(task.ID)

    require.NoError(t, err)
    assert.Equal(t, "completed", completed.Status)
    assert.False(t, completed.CompletedAt.IsZero()) // This fails, bug confirmed
}

// Step 2: Fix the bug
func CompleteTask(id string) (*Task, error) {
    return db.Update(id, map[string]interface{}{
        "status":       "completed",
        "completed_at": time.Now(), // This was missing
    })
}

// Step 3: Test passes, bug fixed, regression guarded
```

## The Test Pyramid

Invest testing effort according to the pyramid — most tests should be small and fast, with progressively fewer tests at higher levels:

```
          /\
         /  \         E2E Tests (~5%)
        /    \        Full user flows
       /------\
      /        \      Integration Tests (~15%)
     /          \     Component interactions, API boundaries
    /------------\
   /              \   Unit Tests (~80%)
  /                \  Pure logic, isolated, milliseconds each
 /------------------\
```

**The Beyonce Rule:** If you liked it, you should have put a test on it. Infrastructure changes, refactoring, and migrations are not responsible for catching your bugs — your tests are.

### Test Sizes (Resource Model)

| Size | Constraints | Speed | Example |
|------|------------|-------|---------|
| **Small** | Single process, no I/O, no network, no database | Milliseconds | Pure function tests, data transforms |
| **Medium** | Multi-process OK, localhost only, no external services | Seconds | API tests with test DB, component tests |
| **Large** | Multi-machine OK, external services allowed | Minutes | E2E tests, performance benchmarks |

Small tests should make up the vast majority of your suite. They're fast, reliable, and easy to debug when they fail.

### Decision Guide

```
Is it pure logic with no side effects?
  -> Unit test (small)

Does it cross a boundary (API, database, file system)?
  -> Integration test (medium)

Is it a critical user flow that must work end-to-end?
  -> E2E test (large) — limit these to critical paths
```

## Writing Good Tests

### Test State, Not Interactions

Assert on the *outcome* of an operation, not on which methods were called internally. Tests that verify method call sequences break when you refactor, even if the behavior is unchanged.

### DAMP Over DRY in Tests

In production code, DRY (Don't Repeat Yourself) is usually right. In tests, **DAMP (Descriptive And Meaningful Phrases)** is better. A test should read like a specification — each test should tell a complete story without requiring the reader to trace through shared helpers.

```go
// DAMP: Each test is self-contained and readable
func TestCreateTask_RejectsEmptyTitle(t *testing.T) {
    _, err := CreateTask(TaskInput{Title: ""})
    assert.ErrorContains(t, err, "title is required")
}

func TestCreateTask_TrimsWhitespace(t *testing.T) {
    task, _ := CreateTask(TaskInput{Title: "  Buy groceries  "})
    assert.Equal(t, "Buy groceries", task.Title)
}
```

Duplication in tests is acceptable when it makes each test independently understandable.

### Prefer Real Implementations Over Mocks

Use the simplest test double that gets the job done. The more your tests use real code, the more confidence they provide.

```
Preference order (most to least preferred):
1. Real implementation  -> Highest confidence, catches real bugs
2. Fake                 -> In-memory version of a dependency (e.g., fake DB)
3. Stub                 -> Returns canned data, no behavior
4. Mock (interaction)   -> Verifies method calls — use sparingly
```

**Use mocks only when:** the real implementation is too slow, non-deterministic, or has side effects you can't control (external APIs, email sending). Over-mocking creates tests that pass while production breaks.

### Use the Arrange-Act-Assert Pattern

```go
func TestCheckOverdue(t *testing.T) {
    // Arrange: Set up the test scenario
    task := &Task{
        Title:    "Test",
        Deadline: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
    }

    // Act: Perform the action being tested
    result := CheckOverdue(task, time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC))

    // Assert: Verify the outcome
    assert.True(t, result.IsOverdue)
}
```

### One Assertion Per Concept

```go
// Good: Each test verifies one behavior
func TestCreateTask_RejectsEmptyTitle(t *testing.T)   { ... }
func TestCreateTask_TrimsWhitespace(t *testing.T)      { ... }
func TestCreateTask_EnforcesMaxLength(t *testing.T)    { ... }
```

### Name Tests Descriptively

```go
// Good: Reads like a specification
func TestCompleteTask_SetsStatusAndRecordsTimestamp(t *testing.T)    { ... }
func TestCompleteTask_ReturnsErrorForNonExistentTask(t *testing.T)   { ... }
func TestCompleteTask_IsIdempotent(t *testing.T)                     { ... }
```

## Test Anti-Patterns to Avoid

| Anti-Pattern | Problem | Fix |
|---|---|---|
| Testing implementation details | Tests break when refactoring even if behavior is unchanged | Test inputs and outputs, not internal structure |
| Flaky tests (timing, order-dependent) | Erode trust in the test suite | Use deterministic assertions, isolate test state |
| Testing framework code | Wastes time testing third-party behavior | Only test YOUR code |
| No test isolation | Tests pass individually but fail together | Each test sets up and tears down its own state |
| Mocking everything | Tests pass but production breaks | Prefer real implementations over mocks |

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "I'll write tests after the code works" | You won't. And tests written after the fact test implementation, not behavior. |
| "This is too simple to test" | Simple code gets complicated. The test documents the expected behavior. |
| "Tests slow me down" | Tests slow you down now. They speed you up every time you change the code later. |
| "I tested it manually" | Manual testing doesn't persist. Tomorrow's change might break it with no way to know. |
| "The code is self-explanatory" | Tests ARE the specification. They document what the code should do, not what it does. |
| "It's just a prototype" | Prototypes become production code. Tests from day one prevent the "test debt" crisis. |

## Red Flags

- Writing code without any corresponding tests
- Tests that pass on the first run (they may not be testing what you think)
- "All tests pass" but no tests were actually run
- Bug fixes without reproduction tests
- Tests that test framework behavior instead of application behavior
- Test names that don't describe the expected behavior
- Skipping tests to make the suite pass

## Verification

After completing any implementation:

- [ ] Every new behavior has a corresponding test
- [ ] All tests pass: `go test -race ./...`
- [ ] Bug fixes include a reproduction test that failed before the fix
- [ ] Test names describe the behavior being verified
- [ ] No tests were skipped or disabled
- [ ] Coverage hasn't decreased (if tracked)
