---
name: deep-interview
description: Structured interview workflow for requirements gathering before coding
version: "1.0.0"
author: graycode
license: MIT
category: workflow
tags: ["requirements", "interview", "planning", "specification", "clarification"]
allowed-tools: Read Grep Glob Bash
---

# Deep Interview

A structured Socratic questioning workflow that gathers requirements before implementation begins. Prevents wasted effort from building the wrong thing.

## When to Use

- Starting a new feature with ambiguous or incomplete requirements
- User request is vague ("make it better", "fix the performance", "add auth")
- Multiple valid interpretations exist for the task
- High-risk changes where misunderstanding the goal is costly
- Before committing to a multi-day implementation plan

## Workflow

### Phase 1: Context Gathering

Read the relevant code, configs, and documentation to understand the current state.

```
1. Identify the affected files and modules
2. Read existing tests to understand expected behavior
3. Check git history for recent related changes
4. Review any existing specs or ADRs
```

### Phase 2: Stakeholder Questions

Ask targeted questions to clarify intent. Group questions by concern:

**Goal questions:**
- What specific problem does this solve?
- What does success look like? How will we know it's done?
- Is this a new behavior or a modification of existing behavior?

**Scope questions:**
- What should this NOT do? (explicit non-goals)
- Are there edge cases you're already aware of?
- Should this affect existing behavior or be additive only?

**Constraint questions:**
- Are there performance requirements (latency, throughput, memory)?
- What's the security surface? (auth, input validation, data sensitivity)
- Are there compatibility requirements (API versions, browsers, OS)?

### Phase 3: Constraint Identification

Synthesize answers into explicit constraints:

```
## Constraints
- MUST: [hard requirements from stakeholder answers]
- SHOULD: [strong preferences, deviate only with justification]
- MUST NOT: [explicit non-goals and boundaries]
- ASSUMPTIONS: [things we're taking as given, verify if wrong]
```

### Phase 4: Scope Agreement

Present a concise scope summary for confirmation:

```
## Proposed Scope
**Goal:** [one sentence]
**Changes:** [list of files/modules affected]
**Out of scope:** [explicit exclusions]
**Risks:** [identified risks and mitigations]
**Estimated effort:** [rough sizing]
```

Ask: "Does this match your intent? Anything to add or remove?"

### Phase 5: Spec Generation

Generate a machine-readable spec that can guide implementation:

```
## Acceptance Criteria
- [ ] Criterion 1: [testable statement]
- [ ] Criterion 2: [testable statement]

## Technical Approach
- Implementation strategy in 2-3 sentences
- Key interfaces or types to create/modify
- Test strategy (what to test, what to mock)

## Dependencies
- External packages or APIs needed
- Internal modules affected
- Ordering constraints
```

## Patterns

### Pattern: Vague Request

When the user says "optimize this":
1. Ask: "What metric are we optimizing? Latency, throughput, memory, readability?"
2. Ask: "What's the current baseline? Do you have measurements?"
3. Ask: "What's the target? What improvement would be meaningful?"

### Pattern: Feature Request

When the user says "add X":
1. Ask: "Where should X live? New module, existing module, external package?"
2. Ask: "Should X be opt-in or default? Configurable?"
3. Ask: "How should X fail? Graceful degradation or hard error?"

### Pattern: Bug Report

When the user says "fix Y":
1. Ask: "What's the expected behavior vs actual?"
2. Ask: "Can you reproduce it consistently? What are the steps?"
3. Ask: "When did this start? Was there a recent change?"

## Verification

Before proceeding to implementation, confirm:

- [ ] All acceptance criteria are testable
- [ ] Non-goals are explicitly documented
- [ ] Constraints are categorized (MUST/SHOULD/MUST NOT)
- [ ] Scope summary was confirmed by the user
- [ ] Risks have mitigation strategies
- [ ] Dependencies are identified and available
