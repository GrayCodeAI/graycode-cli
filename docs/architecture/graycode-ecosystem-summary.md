# GrayCode Ecosystem Summary

`graycode-eco` is only a local parent folder. `hawk` is the main CLI and the
only primary Hawk product; the other repositories provide capabilities,
contracts, integrations, tooling, or optional platform services.

## Repository layers

```text
API consumers/extensions
  sparrow   robin   wren   starling
       \      |      |       /
                    hawk
             (main CLI and daemon)
                      |
Support engines
  eyrie   harrier   shrike   swift   kestrel   merlin
                      |
Foundations
                eagle     falcon

Outside the Go runtime graph
  owl                  architecture tooling
  graycode-platform    web, BFF, and Hawk Cloud Worker
```

Product labels map to directories as follows: `harrier`/Harrier, `shrike`/Shrike,
`swift`/Swift, `kestrel`/Kestrel, and `merlin`/Merlin.

## Dependency direction

```text
hawk -> eyrie / harrier / shrike / swift / kestrel / merlin / eagle
engines -> eagle                  # when shared contracts are needed
harrier / kestrel / merlin -> falcon
sparrow / robin / wren -> Hawk daemon API
starling -> Hawk skill/plugin API
```

Forbidden edges are engine-to-engine, engine-to-Hawk-internal, SDK-to-engine,
skills-to-engine, and any Go-module dependency on GrayCode Platform.

## Runtime and hosted plane

```text
Hawk main CLI
  ├── Eyrie provider execution
  ├── Harrier memory
  ├── Shrike token/context management
  ├── Swift swift/provenance
  ├── Kestrel review
  └── Merlin verification

hawk ── optional authenticated HTTP ──> graycode-platform/apps/worker
web ──> graycode-platform/apps/bff ── private Service Binding ──> worker
worker ──> control-plane D1 + usage Queue + R2
```

The Worker is deployed as `graycode-cloud`, but that is an application name,
not a repository. GrayCode Platform remains outside the Hawk Go module graph.
Hawk's cloud usage path is fail-open; graph synchronization is explicit.

## Repository roles

| Repository | Role | Direct dependency rule |
|---|---|---|
| `hawk` | Main CLI, daemon, orchestration, policy | Integrates engines and contracts |
| `eyrie` | Provider runtime | Uses Eagle contracts; exposes `engine` |
| `harrier` | Harrier memory | Uses Eagle/Falcon where required |
| `shrike` | Shrike context engine | Uses Eagle where required |
| `swift` | Swift/provenance | Uses Eagle where required |
| `kestrel` | Kestrel review | Uses Eagle and Falcon |
| `merlin` | Merlin verification | Uses Eagle and Falcon |
| `eagle` | Neutral shared contracts | Leaf module |
| `falcon` | MCP kit | Upstream MCP library only |
| `sparrow` | Go SDK | Hawk API contract |
| `robin` | Python SDK | Hawk API contract |
| `wren` | TypeScript SDK | Hawk API contract |
| `starling` | Skills/extensions | Hawk skill surface |
| `owl` | Architecture explorer | Generated read-only projection |
| `graycode-platform` | Web/BFF/cloud | HTTP and Service Binding only |

## Current status

The repository-level target is implemented: independent Git repositories,
canonical manifest, generated Owl inventory, sibling Go workspace, Eagle parity,
and dependency boundary checks are all present. Remaining work is to publish
the Eagle-compatible Eyrie revision, remove the transitional
`hawk-core-contracts` dependency from the standalone graph, and decide whether
Hawk's graph/projection packages need additional engine-owned facades.
