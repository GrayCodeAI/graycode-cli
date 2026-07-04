# Spec

Curated spec-driven development resources from three external projects, consolidated for hawk's spec mode. These are **reference materials** — hawk's actual spec engine is in `internal/engine/spec/`.

## Sources

| Directory | Source | Stars | Description |
|-----------|--------|-------|-------------|
| `openspec/` | [Fission-AI/OpenSpec](https://github.com/Fission-AI/OpenSpec) | ~5k | Schema-driven artifact workflow engine with delta specs. Full TypeScript CLI tool for change management. |
| `spec-kit/` | [github/spec-kit](https://github.com/github/spec-kit) | ~117k | GitHub's Spec-Driven Development toolkit with 10 SDD commands, 5 artifact templates, extension system. Python CLI. |
| `agent-skills/` | [addyosmani/agent-skills](https://github.com/addyosmani/agent-skills) | ~20k | Production-grade engineering skills for AI coding agents. 25+ skills, 8 commands, references, hooks, agents. |

## Structure

```
spec/
├── README.md
│
├── openspec/                              # Fission-AI/OpenSpec
│   ├── schema.yaml                        # Artifact workflow schema
│   ├── templates/                         # Artifact templates
│   │   ├── proposal.md
│   │   ├── spec.md                        # Delta spec format
│   │   ├── design.md
│   │   └── tasks.md
│   ├── docs/                              # 25 doc pages (concepts, CLI, workflows, FAQ)
│   └── examples/                          # Real-world change artifacts
│       ├── add-global-install-scope/      # Full proposal + 6 specs + design + tasks
│       ├── add-tool-command-surface-capabilities/
│       └── add-qa-smoke-harness/
│
├── spec-kit/                              # github/spec-kit
│   ├── spec-driven.md                     # Full SDD methodology doc
│   ├── AGENTS.md                          # Agent instructions
│   ├── DEVELOPMENT.md                     # Developer onboarding
│   ├── commands/                          # 10 SDD command workflows
│   │   ├── specify.md                     # Write the spec
│   │   ├── clarify.md                     # Ask clarifying questions [NEEDS CLARIFICATION]
│   │   ├── plan.md                        # Write the plan
│   │   ├── tasks.md                       # Write task breakdown
│   │   ├── checklist.md                   # Quality checklist review
│   │   ├── implement.md                   # Execute implementation
│   │   ├── converge.md                    # Gap analysis
│   │   ├── analyze.md                     # Codebase analysis before spec
│   │   ├── constitution.md                # Project constitution setup
│   │   └── taskstoissues.md              # Convert tasks to issues
│   └── templates/
│       ├── spec-template.md
│       ├── plan-template.md
│       ├── tasks-template.md
│       ├── constitution-template.md
│       ├── checklist-template.md
│       └── vscode-settings.json
│
└── agent-skills/                          # addyosmani/agent-skills
    ├── spec-driven-development.md         # Gated 4-phase SDD skill
    ├── definition-of-done.md              # Standing quality checklist
    ├── AGENTS.md                          # Agent definitions
    ├── CLAUDE.md                          # Claude project config
    ├── commands/                          # 8 .toml command definitions
    ├── skills/                            # 25+ engineering skills
    │   ├── spec-driven-development/       # SDD skill (v2)
    │   ├── interview-me/                  # Asks clarifying questions
    │   ├── idea-refine/                   # Refines vague ideas into specs
    │   ├── planning-and-task-breakdown/
    │   ├── incremental-implementation/
    │   ├── test-driven-development/
    │   ├── code-review-and-quality/
    │   ├── code-simplification/
    │   ├── context-engineering/
    │   ├── doubt-driven-development/
    │   ├── debugging-and-error-recovery/
    │   ├── git-workflow-and-versioning/
    │   ├── documentation-and-adrs/
    │   ├── security-and-hardening/
    │   ├── performance-optimization/
    │   ├── observability-and-instrumentation/
    │   ├── shipping-and-launch/
    │   ├── ci-cd-and-automation/
    │   ├── api-and-interface-design/
    │   ├── browser-testing-with-devtools/
    │   ├── frontend-ui-engineering/
    │   ├── deprecation-and-migration/
    │   ├── source-driven-development/
    │   ├── using-agent-skills/
    │   └── ... (more)
    ├── docs/                              # Setup guides for all major AI tools
    ├── references/                        # 7 checklists and reference docs
    ├── hooks/                             # Session hooks (session-start, sdd-cache)
    └── agents/                            # Pre-built agent personas
```

## Detailed Comparison

### OpenSpec vs spec-kit vs agent-skills vs hawk's spec

| Capability | OpenSpec | spec-kit | agent-skills | hawk spec |
|-----------|----------|----------|--------------|-----------|
| **Artifact DAG** | `requires:` deps, schema-driven | Phase gating | 4-phase gating | `Graph` with Kahn's algo |
| **Delta specs** | `## ADDED/MODIFIED/REMOVED/RENAMED` | Same format | - | `ParseDeltaSpec` + `ApplyDelta` |
| **Quality validation** | Zod validation rules | Checklist + 3x re-validate | Definition of Done | `ValidateSpec/Plan/Tasks` |
| **Clarify questions** | - | `clarify` cmd + `[NEEDS CLARIFICATION]` markers | `interview-me` skill + assumption surfacing | Prompt-driven (model uses `AskUser`) |
| **Constitution** | - | `constitution-template.md` + `constitution` cmd | - | - |
| **Idea refinement** | - | - | `idea-refine` skill (frameworks, examples, refinement criteria) | - |
| **Extensibility** | 27 tool adapters, profiles | Extension system (git, agent-context, bug), presets, bundles, workflows | Hooks system (session-start, sdd-cache) | `tool.Tool` interface |
| **Archive** | `archive` command (moves dirs) | `archive` concept | - | `Archive` function |
| **Convergence** | `verify` command | `converge` command + gap analysis | Definition of Done | `AssessConvergence` |
| **Task tracking** | tasks.md with checkboxes | `- [ ]` format + `taskstoissues` | Task breakdown template | Checkbox regex + phase tracking |
| **Stores/remotes** | Git-based store system | - | - | - |
| **CLI tool** | TypeScript, pnpm | Python, pipx/uv | - | Go, single binary |
| **AI tool support** | 27+ adapters | 30+ integrations | - | Tool registry + MCP |

### Key patterns hawk should adopt

| Pattern | Source | Why |
|---------|--------|-----|
| `clarify` command flow | spec-kit | Structured question asking before spec writing |
| `idea-refine` skill | agent-skills | Refining vague user ideas into actionable specs |
| `constitution` template | spec-kit | Documenting project governance rules |
| `converge` gap analysis | spec-kit | Checking implementation matches spec |
| `analyze` codebase scan | spec-kit | Pre-spec codebase analysis |
| Session hooks | agent-skills | Pre/post session lifecycle hooks |
| Definition of Done | agent-skills | Quality bar checklist applied to every change |
| `[NEEDS CLARIFICATION]` markers | spec-kit | Inline markers for unresolved questions (max 3) |
| `taskstoissues` | spec-kit | Converting task checkboxes to organized issues |
| Extension system | spec-kit | Pluggable git, agent-context, bug triage workflows |
| Archive + real-world examples | OpenSpec | Real change artifacts showing the full workflow |
