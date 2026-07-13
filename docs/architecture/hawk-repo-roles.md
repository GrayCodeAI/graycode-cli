# Hawk Repo Roles

## Product repo

### `hawk`
Main product repo.

Owns:

- CLI
- daemon/API surface
- agent loop
- session orchestration
- tool execution flow
- policy and permissions
- engine coordination
- user-facing model/configuration presentation and stable CLI schemas

Hawk is the only primary product surface. Users interact with Hawk, not with six
separate end-user products.

## Support engines

### `eyrie`
Hawk provider engine. Its public host boundary is `eyrie/engine`, which owns
credentials, provider state, catalog discovery, model/deployment selection,
transport, resilience, and normalized generation/streaming. Hawk production
code has zero imports of Eyrie's lower-level packages.

### `yaad`
Hawk memory engine.

### `tok`
Hawk context and token engine.

### `trace`
Hawk audit and replay engine.

### `sight`
Hawk review engine.

### `inspect`
Hawk verification engine.

All six support engines are peers:

- `eyrie`
- `yaad`
- `tok`
- `trace`
- `sight`
- `inspect`

They should stay isolated from each other and depend on Hawk only through
orchestration plus `hawk-core-contracts` where a shared vocabulary is required.

## Ecosystem repos

### `hawk-sdk-go`
Go integration surface for Hawk public APIs/contracts.

### `hawk-sdk-python`
Python integration surface for Hawk public APIs/contracts.

### `hawk-community-skills`
Reusable Hawk skills, recipes, and extension packs.

## Shared foundation

### `hawk-core-contracts`
Shared types, events, findings, policies, and engine request/response contracts.

This repo should stay small, stable, and implementation-free.

### `hawk-mcpkit`
Shared MCP server scaffolding wrapping `mark3labs/mcp-go` — construction,
transports, and handler helpers that MCP-serving engines (`sight`, `inspect`)
would otherwise duplicate.

Like `hawk-core-contracts`, it sits below the engines: it must not import
engines, hawk, or graycode-core.

## Role rules

- Users should feel they are using `hawk`, not six unrelated tools.
- Engines are internal capabilities from a product perspective.
- Engines can stay in separate repos for isolation, testing, and replacement.
- Engines must not import each other.
- SDKs and skills extend Hawk, but should not bypass Hawk to reach engines directly.
