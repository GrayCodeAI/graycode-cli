# ADR-0002: Hawk Cloud is the graycode-eco ecosystem control plane

- Status: Accepted
- Date: 2026-07-10
- Owners: Hawk maintainers

## Decision

GrayCode Core is the identity and web-platform layer. Hawk Cloud is the
Hawk-specific control plane and source of truth for organizations, teams,
projects, devices, sessions, usage, policies, entitlements, billing, and audit
records across Hawk, Eyrie, Tok, Yaad, Trace, Sight, and Inspect.

Hawk remains local-first. It may send aggregate usage to Hawk Cloud only after
the user explicitly authenticates and connects a device. Engines never import
GrayCode or Hawk Cloud code; they report through Hawk's isolated cloud adapter.

GrayCode's backend is a browser-facing BFF. It validates GrayCode identity and
forwards scoped requests to Hawk Cloud. Hawk Cloud does not duplicate passwords
or browser sessions. GrayCode-to-Hawk Cloud requests use a signed service
identity; the principal header is not trusted by itself.

## Authentication

Interactive CLI authentication uses a browser device-authorization flow:

```text
hawk login
  -> GrayCode browser authentication
  -> user approves organization/project
  -> Hawk Cloud issues one project-scoped device token
  -> Hawk stores it in the OS secure store
```

Automation uses project-scoped service accounts or API keys. Human credentials
must not be shared with CI systems.

## Consequences

- GrayCode's local usage tables may remain for GrayCode-only product activity,
  but Hawk usage and Hawk billing are owned by Hawk Cloud.
- Usage events are versioned and idempotent, with optional input/output/cache/
  reasoning token dimensions and session attribution.
- Full session traces and large exports are separate from the D1 control-plane
  tables and must be opt-in, redacted, encrypted, and retention-controlled.
- The older telemetry-edge ADR remains valid for the no-compile-time-dependency
  rule; Hawk Cloud is now the preferred destination for Hawk-specific cloud
  accounting.
