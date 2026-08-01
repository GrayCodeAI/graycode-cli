# Hawk Architecture - Technical Plan

## Overview

This plan defines the technical approach for implementing the hawk architecture specification. The architecture is already largely implemented; this plan documents the existing design decisions and identifies gaps.

## Architecture Decisions

### AD-1: God-Object Decomposition

**Decision:** Decompose the Session god-object into 7 cohesive sub-services,
with the safety boundary extracted first.

**Rationale:** The Session struct accumulated 35+ collaborators over time. The decomposition phases (1-7) extract these into focused sub-services while maintaining backward compatibility via legacy field accessors.

**Trade-offs:**
- Pro: Clear responsibility boundaries, easier testing
- Pro: Sub-services can be nil-checked independently
- Con: Legacy fields remain for backward compatibility during migration
- Con: The remaining services still need extraction and interface seams

**Implementation:**
```
Session
  ├─ llm *ChatService        (Phase 1: LLM transport)
  ├─ perms *PermissionService (Phase 2: safety/approval)
  ├─ life *LifecycleService   (Phase 3: self-improvement)
  ├─ memory *MemoryService    (Phase 4: yaad bridge)
  ├─ persist *PersistenceService (Phase 5: conversation store)
  └─ tools *ToolService       (Phase 6: tool execution)
```

**Current implementation boundary:** `PermissionService` now owns the
authoritative policy state and exposes an immutable per-tool `PolicySnapshot`.
Tool approval and execution consume that same snapshot. `PersistenceService`
also protects transcript ownership with deep-copy snapshots. The diagram above
remains the target for the unextracted Chat, Memory, Lifecycle, and Tool
services; it is not a claim that those fields have already moved.

### AD-2: Spec-Driven Development Gate

**Decision:** Gate all write/edit/bash tools behind spec workflow stages.

**Rationale:** Forces structured planning before code changes. The spec gate is checked first in PermissionEngine.CheckTool, before trust-tier or autonomy-level logic.

**Trade-offs:**
- Pro: Prevents premature coding
- Pro: Forces requirements clarity
- Con: Adds workflow overhead for simple tasks
- Con: User must explicitly approve before implementation

**Implementation:** Perm.Stage enum controls tool access:
- Specify/Plan/Tasks: only spec tools + read-only
- ApproveImplementation: prompts user
- Implementing: all tools unblocked

### AD-3: Context Compaction Strategies

**Decision:** Implement four compaction strategies (collapse, micro, smart, truncate) with automatic selection.

**Rationale:** Context windows are finite. Different situations require different compaction approaches.

**Trade-offs:**
- Pro: Graceful degradation under token pressure
- Pro: Pinned messages protected from compaction
- Con: Compaction can lose nuance from older messages
- Con: Smart compaction requires LLM call (cost)

**Strategy Selection:**
1. Collapse: Merge old tool results into summaries
2. Micro: Summarize recent messages
3. Smart: Keep pinned, compact rest (requires LLM)
4. Truncate: Last resort, drop oldest messages

### AD-4: Self-Improvement Feedback Loops

**Decision:** Implement three feedback loops (session-end, session-start, per-turn).

**Rationale:** The agent should learn from each session and improve over time.

**Trade-offs:**
- Pro: Accumulates successful patterns
- Pro: Adapts to user preferences
- Con: Storage overhead for memories/patterns
- Con: Potential for stale/incorrect learnings

**Loop Implementation:**
- Session-end: Extract patterns, learn corrections, distill skills
- Session-start: Inject guidelines, examples, preferences
- Per-turn: Recall memories, match skills, refresh adaptive prompt

### AD-5: Resilience Patterns

**Decision:** Implement circuit breaker, rate limiting, retry with backoff, and health checks.

**Rationale:** LLM APIs are unreliable. The system must degrade gracefully.

**Trade-offs:**
- Pro: Automatic recovery from transient failures
- Pro: Provider failover on circuit breaker open
- Con: Retry adds latency
- Con: Circuit breaker can block healthy requests during recovery

**Pattern Implementation:**
- Circuit breaker: Track failures, open after threshold, half-open on timer
- Rate limiting: Token bucket per provider
- Retry: Exponential backoff with jitter
- Health checks: Periodic provider availability checks

## Technical Approach

### Phase 1: Documentation (This Spec)

**Goal:** Document the complete architecture for developer onboarding.

**Tasks:**
1. Create spec.md with all requirements
2. Create plan.md with technical decisions
3. Create tasks.md with implementation checklist
4. Update AGENTS.md with architecture overview

### Phase 2: Gap Analysis

**Goal:** Identify gaps between spec and implementation.

**Approach:**
1. Compare spec requirements against existing code
2. Identify missing features or incomplete implementations
3. Prioritize gaps by impact and effort
4. Create remediation tasks

### Phase 3: Implementation

**Goal:** Close identified gaps.

**Approach:**
1. Implement missing features
2. Add tests for new functionality
3. Update documentation
4. Verify all requirements are met

## Risk Assessment

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| God-object decomposition breaks existing code | High | Medium | Legacy field accessors, backward compat |
| Spec gate blocks legitimate quick fixes | Medium | Low | Allow /bypass command for emergency fixes |
| Context compaction loses critical information | High | Low | Pinned messages, smart compaction |
| Self-improvement accumulates incorrect learnings | Medium | Medium | Human-in-the-loop for critical learnings |
| Circuit breaker blocks healthy providers | Medium | Low | Half-open state, configurable thresholds |

## Dependencies

- **eyrie:** LLM provider engine behind `eyrie/engine` (external submodule)
- **yaad:** Graph-based persistent memory (external submodule)
- **tok:** Tokenizer, compression (external submodule)
- **hawk-core-contracts:** Shared types (external submodule)

## Success Metrics

| Metric | Target |
|--------|--------|
| Tool execution success rate | >99% |
| Context overflow rate | <1% |
| Spec workflow completion rate | >90% |
| Memory recall accuracy | >80% |
| Error recovery success rate | >95% |
