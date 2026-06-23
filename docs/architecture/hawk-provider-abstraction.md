# Hawk Provider Abstraction

## Goal

Hawk must remain model agnostic.

That means Hawk should support multiple providers without leaking provider-specific assumptions across the product.

## Design principle

Provider-specific code lives behind runtime adapters, primarily in `eyrie`.

Hawk decides:

- which provider to use
- whether fallback is allowed
- what capability the task needs

`eyrie` handles:

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
- keep fallback/routing policy inside Hawk orchestration, not inside engines unrelated to runtime

## Future extension

This design allows:

- local models
- hosted APIs
- custom gateways
- enterprise proxies
- model routing by capability or policy
