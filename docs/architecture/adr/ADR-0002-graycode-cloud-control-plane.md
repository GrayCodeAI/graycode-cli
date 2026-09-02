# ADR-0002: Hawk Cloud control plane inside GrayCode Platform

- Status: Accepted
- Date: 2026-07-10
- Owners: Hawk maintainers

## Decision

`graycode-platform` is the separate web/platform repository. It contains two
Cloudflare Worker applications:

- `apps/bff` owns browser identity and the authenticated browser gateway;
- `apps/worker`, deployed as `graycode-cloud`, owns the Hawk-specific control
  plane and source of truth for organizations, teams, projects, devices,
  sessions, usage, policies, entitlements, billing, audit, and graph records.

Hawk remains local-first. It may send aggregate usage or explicitly requested
graph data to Hawk Cloud only after the user connects a device. Engines never
import GrayCode Platform or Hawk Cloud code; Hawk uses its isolated HTTP cloud
adapter.

The browser reaches Hawk Cloud only through the BFF's private
`HawkCloudService` Service Binding. The BFF resolves the authenticated
principal; the Worker injects its cloud-only service credential. The browser
never receives that credential.

## Authentication

Interactive CLI authentication uses a browser device-authorization flow:

```text
hawk cloud login
  -> GrayCode Platform BFF/browser authentication
  -> user approves organization/project
  -> Hawk Cloud Worker issues one project-scoped device token
  -> Hawk stores it in the OS secure store
```

Automation uses project-scoped service accounts or API keys. Human credentials
must not be shared with CI systems.

## Consequences

- GrayCode product activity remains separate from Hawk's authoritative usage
  and billing ledger.
- Usage events are versioned and idempotent, with token dimensions and session
  attribution.
- The Worker uses D1 for control-plane state, Queue plus an outbox for durable
  usage delivery, and R2 for large redacted exports.
- Full traces and large exports are opt-in, redacted, encrypted where needed,
  and retention-controlled.
- The no-compile-time-dependency rule in ADR-0001 remains unchanged.
