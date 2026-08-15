# Spec-Driven Workflow — Complete Implementation Plan

## Current State (After Initial Implementation)
- Proposal → Specify + Design (parallel) → Plan → Tasks → Implementing
- Constitution tool exists but is optional
- Cross-artifact validation exists (AnalyzeTool)
- Convergence tool exists but is manual
- File-watch hooks bridge exists
- Command hooks with patterns exist

## Target State (Perfect Spec-Driven Workflow)

### Phase 1: Constitution as Enforced Guardrails
**Goal:** Constitution is REQUIRED before spec work begins. Gates enforced at Plan transition.

**Changes:**
1. Constitution must exist before Specify/Design can run
2. Constitution injected into system prompt at ALL spec stages
3. Phase gates checked at Plan transition:
   - Simplicity Gate: ≤3 projects/modules
   - Anti-Abstraction Gate: framework used directly
   - Integration-First Gate: contracts defined before implementation
   - Test-First Gate: tests written before code
4. Gate failures require documented justification in "Complexity Tracking" section

**Files:**
- `internal/tool/spec_constitution.go` — enforce constitution requirement
- `internal/engine/safety/permission_engine.go` — gate at Plan transition
- `internal/engine/permission_session_methods.go` — inject constitution into prompt

### Phase 2: EARS Notation + REQ-XXX.Y.Z Requirement IDs
**Goal:** Structured requirements with traceable IDs.

**Changes:**
1. EARS patterns enforced in spec.md:
   - Ubiquitous: "The system shall..."
   - Event-driven: "WHEN ... THEN ..."
   - State-driven: "WHILE ... THEN ..."
   - Unwanted: "The system shall not..."
   - Optional: "IF ... THEN ..."
2. Each requirement gets REQ-XXX.Y.Z ID
3. Tasks reference REQ IDs
4. Code comments cite REQ IDs

**Files:**
- `internal/spec/validator.go` — EARS validation + REQ ID format
- `internal/tool/spec.go` — task format with REQ IDs
- `internal/tool/spec_analyze.go` — REQ coverage analysis

### Phase 3: NEEDS CLARIFICATION Enforcement
**Goal:** Explicit ambiguity markers prevent AI guessing.

**Changes:**
1. Max 3 unresolved NEEDS CLARIFICATION markers at a time
2. Cannot advance to Plan until all resolved
3. ClarifyTool auto-generates questions for detected ambiguities
4. System prompt instructs AI to use markers instead of guessing

**Files:**
- `internal/spec/validator.go` — enforcement logic
- `internal/tool/spec_clarify.go` — auto-question generation
- `internal/engine/safety/permission_engine.go` — gate at Plan transition

### Phase 4: Orphan REQ Hallucination Detection
**Goal:** Automated detection of code that doesn't trace to spec.

**Changes:**
1. Scan all source files for [REQ-XXX.Y.Z] citations
2. Compare against spec requirements
3. Orphan detection: code cites REQ not in spec → hallucination
4. Missing detection: REQ in spec not cited in code → gap

**Files:**
- `internal/spec/validator.go` — new ScanCodeForRequirements function
- `internal/tool/spec_analyze.go` — orphan/missing detection
- `internal/tool/spec_converge.go` — use detection results

### Phase 5: Subagent Parallelization
**Goal:** Independent tasks executed in parallel.

**Changes:**
1. Task dependency analysis (which tasks depend on which)
2. Parallel group identification
3. Subagent pool execution for independent tasks
4. Merge results after parallel phase completes

**Files:**
- `internal/tool/spec.go` — task dependency parsing
- `internal/engine/workflow/` — parallel execution support

### Phase 6: Auto Convergence Loop
**Goal:** Automatic spec-to-codebase gap detection and repair.

**Changes:**
1. Auto-trigger ConvergeTool after implementation phase
2. Detect drift between spec and codebase
3. Generate convergence tasks automatically
4. Loop until spec ↔ codebase match

**Files:**
- `internal/tool/spec_converge.go` — auto-trigger logic
- `internal/engine/safety/permission_engine.go` — post-implement hook

### Phase 7: Reactive File-Watch Validation
**Goal:** Continuous verification on file changes.

**Changes:**
1. On file save: run related tests
2. On file save: check REQ citations match spec
3. On file save: update task checklist automatically
4. On test failure: notify agent to fix

**Files:**
- `internal/hooks/file_watcher.go` — enhanced with validation
- `internal/hooks/hooks.go` — new hook events for test results

### Phase 8: Verification & Iteration
**Goal:** Compare against target, iterate until perfect.

**Process:**
1. Run full test suite after each phase
2. Compare feature matrix against target
3. Fix any gaps found
4. Iterate until all checks pass
