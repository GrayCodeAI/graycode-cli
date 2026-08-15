# Spec-Driven Workflow — Phase 2 Plan (Remaining Features)

## Current State
- Constitution enforced with phase gates
- EARS notation + REQ-XXX.Y.Z IDs
- Orphan REQ hallucination detection
- Task dependency analysis (ParseTasks + AnalyzeTaskGroups)
- Parallel group identification

## Target: Complete All Remaining Features

### Phase A: Parallel Subagent Execution
**Goal:** Execute independent task groups in parallel using existing MultiAgentTool infrastructure.

**Changes:**
1. Create `internal/tool/spec_parallel.go` — SpecParallelTool
   - Reads tasks.md, parses with spec.ParseTasks
   - Groups with spec.AnalyzeTaskGroups
   - Executes parallel groups via MultiAgentTool
   - Collects results and reports

2. Leverage existing infrastructure:
   - MultiAgentTool (already supports parallel execution)
   - SpawnController.SpawnBackground (async spawning)
   - taskruntime.Registry (result collection)

### Phase B: Reactive Test Execution on File Save
**Goal:** When a source file is saved, run related tests automatically.

**Changes:**
1. Enhance `internal/hooks/file_watcher.go`:
   - On .go file save: run `go test ./<pkg>/...`
   - On .ts/.js file save: run related test file
   - On _test.go file save: run that specific test
2. Fire new `EventTestResult` hook with pass/fail status
3. Report results back to agent for fix iteration

### Phase C: Task Checklist Auto-Update
**Goal:** When code changes implement a task, auto-mark it complete.

**Changes:**
1. Create `internal/tool/spec_progress.go` — SpecProgressTool
   - Scans code for REQ IDs
   - Matches against tasks.md REQ references
   - Marks tasks complete when implementation detected
   - Reports progress stats

### Phase D: Multiple Plan Variations
**Goal:** Generate and compare multiple implementation approaches.

**Changes:**
1. Extend PlanTool with `variations` parameter
   - Generate N different plan approaches
   - Compare tradeoffs (performance, simplicity, risk)
   - Let user choose preferred approach

### Phase E: Context Grounding Hooks
**Goal:** Before each spec stage, probe the repository for relevant context.

**Changes:**
1. Create `internal/tool/spec_ground.go` — SpecGroundTool
   - Before Specify: gather codebase structure, related files
   - Before Design: gather existing patterns, dependencies
   - Before Plan: gather API contracts, test coverage
   - Inject findings into system prompt

### Phase F: Spec Versioning in VCS
**Goal:** Ensure specs are committed alongside code.

**Changes:**
1. Create `internal/tool/spec_version.go` — SpecVersionTool
   - Stage .hawk/specs/ changes
   - Generate commit message referencing REQ IDs
   - Link code commits to spec requirements

## Verification Strategy
1. After each phase: run `go test ./...` — must pass
2. After all phases: run `make lint` — must pass
3. Compare feature matrix against target
4. Iterate on any gaps found
