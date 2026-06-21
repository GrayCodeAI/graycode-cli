# Hawk Dependency Rules

## Required graph

```text
hawk -> eyrie
hawk -> yaad
hawk -> tok
hawk -> trace
hawk -> sight
hawk -> inspect
hawk -> hawk-core-contracts

eyrie -> hawk-core-contracts
yaad  -> hawk-core-contracts
tok   -> hawk-core-contracts
trace -> hawk-core-contracts
sight -> hawk-core-contracts
inspect -> hawk-core-contracts

hawk-sdk-go -> hawk public API/contracts
hawk-sdk-python -> hawk public API/contracts
hawk-community-skills -> hawk plugin/skill API
```

## Forbidden graph

```text
engine -> engine
engine -> hawk/internal/*
engine -> hawk/shared/* as a long-term public dependency
sdk -> engine
skills -> engine
```

## Rules

### 1. Hawk is the orchestrator
Only Hawk coordinates the support engines.

### 2. Engines are peers
Engines may share concepts through contracts, but not by importing each other.

### 3. Shared types belong in contracts
Anything used across repos must move to `hawk-core-contracts`.

### 4. Public integrations go through Hawk
SDKs and skills must use Hawk public APIs, contracts, or plugin surfaces.

### 5. Provider logic stays behind runtime boundaries
Provider-specific code should not leak into memory, review, verify, or trace engines.

## Current cleanup targets

Based on current local structure:

- `sight -> hawk/shared/types` removed
- `inspect -> hawk/shared/types` removed
- review any `eyrie` or `yaad` dependency on `tok` and reduce it to contracts or Hawk orchestration where possible

## Enforcement ideas

- document allowed import boundaries in each repo README
- add CI checks for forbidden import paths
- keep `hawk-core-contracts` versioned and minimal
