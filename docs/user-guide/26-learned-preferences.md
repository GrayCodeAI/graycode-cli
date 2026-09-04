# Learned Preferences

Graycode can learn coding-style tendencies from feedback through its taste system.
These preferences are advisory context, not policy.

## Policy Boundaries

- Explicit user instructions override learned preferences.
- Project rules and `AGENTS.md` remain authoritative.
- Permission, sandbox, and security controls cannot be weakened by taste.
- Review severity and correctness findings cannot be hidden by preferences.
- Skills provide reusable procedures; preferences describe tendencies.
- Harrier stores durable facts, conventions, and decisions separately.

Examples of suitable preferences include table-driven tests, preferred error
wrapping style, naming conventions, and project-specific abstraction habits.

## Evidence

Preference confidence is based on repeated observations such as accepted edits,
corrections, and explicit feedback. A single interaction should not silently
become a project rule. Low-confidence signals are not injected into prompts.

## Review Usage

Preference-aware review should supplement objective checks:

```text
Objective: security, correctness, regression risk, missing tests
Preference: naming, test organization, error-handling convention
```

The objective layer always wins. Use `/taste` and `/learn` to merlin or teach
preferences explicitly rather than relying on opaque model behavior.
