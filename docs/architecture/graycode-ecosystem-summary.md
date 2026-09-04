# GrayCode Ecosystem Summary

`graycode-eco` is only a local parent folder. `graycode` is the main CLI and the
only primary Graycode product; the other repositories provide capabilities,
contracts, integrations, tooling, or optional platform services.

## Repository layers

```text
API consumers/extensions
  sparrow   robin   wren   starling
       \      |      |       /
                    graycode
             (main CLI and daemon)
                      |
Support engines
  graycode-router   harrier   shrike   swift   kestrel   merlin
                      |
Foundations
                eagle     falcon

Outside the Go runtime graph
  owl                  architecture tooling
  graycode-platform    web, BFF, and Graycode Cloud Worker
```

Product labels map to directories as follows: `harrier`/Harrier, `shrike`/Shrike,
`swift`/Swift, `kestrel`/Kestrel, and `merlin`/Merlin.

## Dependency direction

```text
graycode -> graycode-router / harrier / shrike / swift / kestrel / merlin / eagle
engines -> eagle                  # when shared contracts are needed
harrier / kestrel / merlin -> falcon
sparrow / robin / wren -> Graycode daemon API
starling -> Graycode skill/plugin API
```

Forbidden edges are engine-to-engine, engine-to-Graycode-internal, SDK-to-engine,
skills-to-engine, and any Go-module dependency on GrayCode Platform.

## Runtime and hosted plane

```text
Graycode main CLI
  ├── GraycodeRouter provider execution
  ├── Harrier memory
  ├── Shrike token/context management
  ├── Swift swift/provenance
  ├── Kestrel review
  └── Merlin verification

graycode ── optional authenticated HTTP ──> graycode-platform/apps/worker
web ──> graycode-platform/apps/bff ── private Service Binding ──> worker
worker ──> control-plane D1 + usage Queue + R2
```

The Worker is deployed as `graycode-cloud`, but that is an application name,
not a repository. GrayCode Platform remains outside the Graycode Go module graph.
Graycode's cloud usage path is fail-open; graph synchronization is explicit.

## Repository roles

| Repository | Role | Direct dependency rule |
|---|---|---|
| `graycode` | Main CLI, daemon, orchestration, policy | Integrates engines and contracts |
| `graycode-router` | Provider runtime | Uses Eagle contracts; exposes `engine` |
| `harrier` | Harrier memory | Uses Eagle/Falcon where required |
| `shrike` | Shrike context engine | Uses Eagle where required |
| `swift` | Swift/provenance | Uses Eagle where required |
| `kestrel` | Kestrel review | Uses Eagle and Falcon |
| `merlin` | Merlin verification | Uses Eagle and Falcon |
| `eagle` | Neutral shared contracts | Leaf module |
| `falcon` | MCP kit | Upstream MCP library only |
| `sparrow` | Go SDK | Graycode API contract |
| `robin` | Python SDK | Graycode API contract |
| `wren` | TypeScript SDK | Graycode API contract |
| `starling` | Skills/extensions | Graycode skill surface |
| `owl` | Architecture explorer | Generated read-only projection |
| `graycode-platform` | Web/BFF/cloud | HTTP and Service Binding only |

## Current status

The repository-level target is implemented: independent Git repositories,
canonical manifest, generated Owl inventory, sibling Go workspace, Eagle parity,
and dependency boundary checks are all present. Remaining work is to publish
the Eagle-compatible GraycodeRouter revision, remove the transitional
`graycode-core-contracts` dependency from the standalone graph, and decide whether
Graycode's graph/projection packages need additional engine-owned facades.
