# Graycode Provider Abstraction

## Goal

Graycode must remain model agnostic.

That means Graycode should support multiple providers without leaking provider-specific assumptions across the product.

## Design principle

Provider-specific code lives behind GraycodeRouter's stable `graycode-router/engine` host facade.
Graycode is the face and composition root; GraycodeRouter is the engine.

Graycode decides:

- what capability the task needs
- the semantic intent (`fast`, `balanced`, `reasoning`, `economical`)
- whether an exact user-selected model may fall back

`graycode-router` handles:

- capability-to-model resolution
- provider and deployment selection
- health-aware infrastructure routing and fallback
- request translation
- streaming normalization
- tool-call normalization
- retries and backoff
- provider capability differences
- credential storage, import, and sanitized provider state
- catalog/cache ownership and provider-scoped live discovery

## Required capabilities

- chat completion
- streaming
- tool calls
- model metadata
- token usage reporting
- error classification
- timeout/cancellation support

## Graycode-facing abstraction

Graycode depends on its small `ChatClient` product port and adapts it only to
`graycode-router/engine`, never to a vendor-specific or lower-level GraycodeRouter client.

Example concerns:

- `RunTurn`
- `StreamTurn`
- `SupportsTools`
- `SupportsVision`
- `SupportsLongContext`
- `SupportsJSONMode`

## Rules

- no direct vendor SDK imports in unrelated Graycode packages
- no provider-specific branches inside review/verify logic
- no model-specific assumptions inside session persistence
- keep task-semantic policy inside Graycode orchestration
- keep provider/deployment routing, health, retry, and fallback inside GraycodeRouter
- Graycode production integrations use `github.com/GrayCodeAI/graycode-router/engine`
- direct imports of lower GraycodeRouter packages are forbidden and CI-enforced
- custom gateway settings enter as `engine.Options.CustomGateways` and are
  isolated per Engine instance
- Graycode-owned JSON, session, and conversation schemas are explicit projections;
  they never become aliases of engine DTOs
- local preflight readiness and remote live verification are distinct states

## Future extension

This design allows:

- local models
- hosted APIs
- custom gateways
- enterprise proxies
- model routing by capability or policy
