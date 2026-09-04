# Spec

Curated spec-driven development resource consolidated for graycode's spec mode.

graycode's spec engine lives in `internal/spec/` (DAG, delta merge, validator, state).
The OpenSpec artifact-workflow schema below is the source graycode's
`DefaultSchema` derives from — everything else in `spec/` is reference material
that was trimmed as dead weight.

## Structure

```
spec/
├── README.md
└── openspec/
    └── schema.yaml        # Artifact workflow schema (embedded by internal/spec/schema.go)
```

Removed (2026-08, dead or duplicated elsewhere):

| Path | Reason removed |
|------|----------------|
| `spec/agent-skills/` | addyosmani/agent-skills vendored copy. 23 SKILL.md files were byte-identical to the runtime-embedded `internal/plugin/bundled_skills/`; no code/script/test referenced it. |
| `spec/spec-kit/` | github/spec-kit vendored docs/commands/templates. Reference-only, no code consumer. |
| `spec/openspec/docs/`, `spec/openspec/examples/`, `spec/openspec/templates/` | Fission-AI/OpenSpec docs and sample artifacts. Only `schema.yaml` is consumed by code. |

## Why OpenSpec schema is kept

`internal/spec/schema.go`'s `DefaultSchema` is derived from
`spec/openspec/schema.yaml` (the `requires:`/delta-spec vocabulary graycode's DAG
parser uses). It is load-bearing and must stay.
