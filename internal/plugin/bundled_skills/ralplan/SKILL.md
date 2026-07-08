---
name: ralplan
description: Structured planning discipline with self-critique before execution
version: "1.0.0"
author: graycode
license: MIT
category: workflow
tags: ["planning", "review", "critique", "discipline", "methodology"]
allowed-tools: Read Grep Glob Bash
---

# Ralplan

A structured planning method that forces plan self-critique before execution. Prevents premature coding and catches design flaws early.

**R**eview → **A**nalyze → **L**ist → **P**lan → **A**ct → **N**ote

## When to Use

- Complex multi-step tasks that affect multiple files
- Changes with high blast radius (API changes, schema migrations, refactors)
- When you're about to start coding but haven't written a plan
- After receiving a task from deep-interview or mission planning
- Any task where "just start coding" would likely miss edge cases

## Workflow

### Phase 1: Review

Understand the full picture before proposing changes.

```
1. Read the task description and acceptance criteria
2. Read all affected files completely
3. Identify the current architecture and patterns
4. Note existing conventions (naming, error handling, testing)
5. Check for related past decisions (git log, ADRs, comments)
```

**Output:** A "Current State" summary — what exists today and why.

### Phase 2: Analyze

Identify the gap between current state and desired state.

```
1. What needs to change? (specific files, functions, types)
2. What should NOT change? (preservation constraints)
3. What are the risks? (breaking changes, performance regression, security)
4. What are the dependencies? (ordering, external APIs, shared types)
5. What are the unknowns? (things that need investigation before coding)
```

**Output:** A "Gap Analysis" — delta between current and target state.

### Phase 3: List

Break the work into discrete, ordered steps.

```
1. Each step should be independently testable
2. Steps should be ordered to minimize risk (foundational changes first)
3. Each step should have a clear "done" criterion
4. Identify which steps can be parallelized
5. Estimate relative complexity (S/M/L) per step
```

**Output:** An ordered checklist of implementation steps.

### Phase 4: Plan (Self-Critique)

**This is the critical phase.** Review the plan from Phase 3 and actively try to find flaws.

**Critique questions:**
- What breaks if step 3 fails? Is there a rollback?
- Did I miss any files that import the thing I'm changing?
- Will existing tests still pass after step 2?
- Is there a simpler approach that achieves the same goal?
- Am I over-engineering? What's the minimum viable change?
- What would a reviewer flag in a PR review of this plan?

**Revised plan:** Update the step list based on critique findings. Add risk mitigation steps where needed.

**Output:** A "Critiqued Plan" — the revised, reviewed step list.

### Phase 5: Act

Execute the plan step by step.

```
1. For each step:
   a. Implement the change
   b. Run relevant tests
   c. Verify the "done" criterion
   d. If blocked, stop and re-plan (don't skip ahead)
2. After all steps: run full test suite
3. Review the diff as if reviewing someone else's PR
```

**Output:** Working code that passes all tests.

### Phase 6: Note

Document what was done and what was learned.

```
1. Update relevant documentation (README, API docs, changelog)
2. Add inline comments for non-obvious decisions
3. Note any technical debt created or resolved
4. Record any deviations from the plan and why
5. Update acceptance criteria with actual outcomes
```

**Output:** Documentation, commit message, and lessons learned.

## Patterns

### Pattern: API Change

When modifying a public interface:
1. Review: Read all callers of the current interface
2. Analyze: Which callers need updating? Can we maintain backward compat?
3. List: Steps ordered to keep the build green at each step
4. Critique: "Did I miss any callers in vendored/external code?"
5. Act: Implement with deprecation warnings before removal
6. Note: Migration guide for downstream consumers

### Pattern: Refactor

When restructuring existing code:
1. Review: Read the module and its tests completely
2. Analyze: What's the target structure? What's the extraction order?
3. List: Small, atomic moves (one function/struct at a time)
4. Critique: "Will each intermediate state compile and pass tests?"
5. Act: Move code, run tests, commit after each atomic move
6. Note: Update module docs, remove dead code

### Pattern: New Feature

When adding new functionality:
1. Review: Read similar features for patterns to follow
2. Analyze: Where does this fit in the architecture?
3. List: Interface first, then implementation, then integration, then tests
4. Critique: "Is the interface too broad? Too narrow?"
5. Act: Implement bottom-up (types → logic → integration → tests)
6. Note: Usage examples, edge case documentation

## Verification

Before declaring the plan complete:

- [ ] Every step has a testable "done" criterion
- [ ] The plan was self-critiqued (Phase 4 completed)
- [ ] Risk mitigation exists for identified risks
- [ ] The step order keeps the build green at each stage
- [ ] Documentation was updated (Phase 6 completed)
- [ ] The diff was reviewed as if reviewing someone else's PR
