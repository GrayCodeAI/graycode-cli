# Hawk Provider Abstraction

## Goal

Hawk must remain model agnostic.

That means Hawk should support multiple providers without leaking provider-specific assumptions across the product.

## Design principle

Provider-specific code lives behind runtime adapters, primarily in `eyrie`.

Hawk decides:

- what capability the task needs
- the semantic intent (`fast`, `balanced`, `reasoning`, `economical`)
- whether an exact user-selected model may fall back

`eyrie` handles:

- capability-to-model resolution
- provider and deployment selection
- health-aware infrastructure routing and fallback
- request translation
- streaming normalization
- tool-call normalization
- retries and backoff
- provider capability differences

## Required capabilities

- chat completion
- streaming
- tool calls
- model metadata
- token usage reporting
- error classification
- timeout/cancellation support

## Hawk-facing abstraction

Hawk should depend on a capability-based interface, not a vendor-specific client.

Example concerns:

- `RunTurn`
- `StreamTurn`
- `SupportsTools`
- `SupportsVision`
- `SupportsLongContext`
- `SupportsJSONMode`

## Rules

- no direct vendor SDK imports in unrelated Hawk packages
- no provider-specific branches inside review/verify logic
- no model-specific assumptions inside session persistence
- keep task-semantic policy inside Hawk orchestration
- keep provider/deployment routing, health, retry, and fallback inside Eyrie
- new Hawk integrations use `github.com/GrayCodeAI/eyrie/engine`; direct lower-level imports are migration-only

## Future extension

This design allows:

- local models
- hosted APIs
- custom gateways
- enterprise proxies
- model routing by capability or policy
