# Graycode Harness

Graycode treats the agent runtime as a product boundary around provider output.

## Tool-Call Path

1. Eyrie normalizes provider protocol responses.
2. Graycode validates and resolves tool metadata.
3. Permission and sandbox policy runs before execution.
4. Tool execution observes timeouts, cancellation, and path boundaries.
5. Results are redacted, persisted, and returned to the model.
6. Stream retry and reasoning-only recovery handle provider failures.
7. Session cleanup removes dangling tool-use/result turns after cancellation.

## Context Integrity

Compaction preserves API invariants by keeping tool-use and tool-result pairs
valid while clearing or reducing old content according to policy. Token,
turn, cost, and time limits are separate controls and should not be treated as
interchangeable.

## Verification

The harness is testable without live providers through scripted providers and
recorded interactions. Provider protocol and behavioral conformance belongs in
Eyrie's verification package; Graycode owns host-level UX, permissions, persistence,
and review contracts.

## Security Rule

Personalized preferences may influence ranking or suggestions, but they cannot
disable objective security, correctness, permission, or audit behavior.
