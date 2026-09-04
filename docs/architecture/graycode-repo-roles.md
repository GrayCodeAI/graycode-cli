# Graycode Repo Roles

## Product repo

### `graycode`
Main CLI and product repository. It is the orchestration root; `graycode-eco`
is only the local parent folder for the independent repositories.

Owns:

- CLI
- daemon/API surface
- agent loop
- session orchestration
- tool execution flow
- policy and permissions
- engine coordination
- user-facing model/configuration presentation and stable CLI schemas

Graycode is the only primary product surface. Users interact with Graycode, not with six
separate end-user products.

## Support engines

### `eyrie`
Graycode provider engine. Its public host boundary is `eyrie/engine`, which owns
credentials, provider state, catalog discovery, model/deployment selection,
transport, resilience, and normalized generation/streaming. Graycode production
code has zero imports of Eyrie's lower-level packages.

### `harrier` (Harrier)
Graycode memory engine.

### `shrike` (Shrike)
Graycode context and token engine.

### `swift` (Swift)
Graycode audit and replay engine.

### `kestrel` (Kestrel)
Graycode review engine.

### `merlin` (Merlin)
Graycode verification engine.

All six support engines are peers:

- `eyrie`
- `harrier` (Harrier)
- `shrike` (Shrike)
- `swift` (Swift)
- `kestrel` (Kestrel)
- `merlin` (Merlin)

They should stay isolated from each other and are coordinated by Graycode;
they may depend on `eagle` where a shared vocabulary is required.

## Ecosystem repos

### `sparrow`
Go integration surface for Graycode public APIs/contracts.

### `robin`
Python integration surface for Graycode public APIs/contracts.

### `wren`
TypeScript integration surface for Graycode public APIs/contracts.

### `starling`
Reusable Graycode skills, recipes, and extension packs.

## Shared foundation

### `eagle`
Shared types, events, findings, policies, and engine request/response contracts.

This repo should stay small, stable, and implementation-free.

### `falcon`
Shared MCP server scaffolding wrapping `mark3labs/mcp-go` — construction,
transports, and handler helpers that MCP-serving engines (`kestrel`, `merlin`)
would otherwise duplicate.

Like `eagle`, it sits below the engines: it must not import
engines, graycode, or graycode-platform.

## Tooling and platform

### `owl`
Read-only architecture visualization tooling. It consumes the generated
projection of `graycode/ecosystem.yaml`; it is not a runtime dependency.

### `graycode-platform`
Separate web, browser-BFF, and Graycode Cloud repository. Its Worker deployment is
named `graycode-cloud`, but it connects to Graycode only through authenticated
runtime HTTP/Service Binding and is never a Go dependency.

## Role rules

- Users should feel they are using `graycode`, not six unrelated tools.
- Engines are internal capabilities from a product perspective.
- Engines can stay in separate repos for isolation, testing, and replacement.
- Engines must not import each other.
- SDKs and skills extend Graycode, but should not bypass Graycode to reach engines directly.
