# Hawk Review and Verify Lifecycle

## Goal

Review and verification should be standard parts of Hawk's workflow, not optional bolt-ons.

## Roles

### `sight`
Answers:

- what looks wrong
- what is risky
- what likely regressed

### `inspect`
Answers:

- did the output pass checks
- did the result actually work
- did required validation complete

## Suggested lifecycle

### Before changes
Optional review pass for:

- risky repo areas
- policy-sensitive files
- planning guidance

### During execution
Trace every major action:

- prompt turn
- tool call
- file edit
- command execution
- provider invocation

### After changes
Run `sight` for:

- code review
- risk detection
- regression hints

Run `inspect` for:

- test execution summaries
- assertion results
- build/lint/check results
- final validation status

## Decision policy

Hawk should define when review and verification are:

- required
- recommended
- skipped

Example factors:

- file sensitivity
- command risk
- user mode
- task type
- presence of tests/checks

## Output model

Normalized outputs should include:

- status
- findings
- severity
- evidence
- recommended next action

These outputs should become contract types in `hawk-core-contracts`.
