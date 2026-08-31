# Design Doc: graycode-eco Extension Marketplace + Unified Documentation Site

**Status:** Draft
**Author:** Ecosystem / DX working group
**Last updated:** 2026-06-06
**Scope:** Multi-month effort spanning all 5 repos (hawk, eyrie, harrier, shrike, swift) + a new gallery web property + a unified docs site.

> This is a design a team executes against, not a code-session deliverable. It grounds every claim in the actual graycode-eco codebase (cited as `path:line`) and reuses what already exists rather than greenfielding.

---

## 1. Overview & Competitive Context

Two adjacent gaps from `TOP20_COMPARISON.md` are addressed together because they share infrastructure (a registry index, a content format, a web property):

1. **Centralized extension gallery / plugin marketplace** — `TOP20_COMPARISON.md:65` (hawk P1) and `TOP20_COMPARISON.md:227` (cross-cutting P1).
2. **Documentation site (Docusaurus/Mintlify) for the graycode-eco ecosystem** — `TOP20_COMPARISON.md:232` (cross-cutting P1).

### Who ships this in the Top 20

| Capability | Prior art (from comparison doc) | Citation |
|---|---|---|
| Browsable extension gallery bundling MCP servers, slash commands, prompts, hooks, themes, sub-agents, skills | **Gemini CLI** — `geminicli.com/extensions/browse/`, `/ide`-style install from inside the CLI | `TOP20_COMPARISON.md:65`, `:227` |
| Marketplace checked at runtime for installable assets | **Continue** — "checks marketplace" | `TOP20_COMPARISON.md:65` |
| Skills marketplace | **CrewAI skills** | `TOP20_COMPARISON.md:65` |
| Unified docs site (getting-started, API refs, architecture, comparison tables) | **OpenHands / LiteLLM / Mem0** all run Docusaurus/Mintlify-class doc portals | `TOP20_COMPARISON.md:232` |

### Where graycode-eco is today (the strong starting position)

graycode-eco is **not** starting from zero. A large fraction of the marketplace already exists as working Go and a populated registry:

- **A populated registry already ships.** `starling/registry.json` is a 4.3 MB JSON array of skill entries with `name`, `description`, `category`, `tags`, `path`, `file_count`, `has_scripts` (see file head). It is served raw from GitHub and consumed by hawk at `hawk/internal/plugin/registry.go:18` (`defaultIndexURL = https://raw.githubusercontent.com/GrayCodeAI/starling/main/registry.json`).
- **A registry client already works.** `hawk/internal/plugin/registry.go` defines `SkillEntry`/`SkillIndex` (`:21`, `:36`), `FetchIndex` with a 1-hour cache (`:60`), and `Install/Remove/InstalledSkillInfo` (`:196`, `:289`, `:309`).
- **CLI surface already exists.** `hawk skills {list,search,install,remove,info,trending,audit}` is wired in `hawk/cmd/skills_cmd.go:16-211`.
- **The extension format is partially standardized.** `SKILL.md` files carry YAML frontmatter (`name`, `description`, `license`, `tags`, `version`) — see `starling/api/openapi.yaml` `x-skill-format` and `categories/testing/ab-test-setup/SKILL.md` frontmatter.
- **A V2 manifest already supports MCP-adjacent bundles.** `hawk/internal/plugin/manifest_v2.go:11` (`ManifestV2`) carries `Tools`, `Permissions`, `Hooks` (`:32` `ManifestHook` with `Event`/`Command`/`Priority`), `Config`, `Dependencies`, `Mode` (subprocess/daemon).
- **Trust scaffolding already exists.** `hawk/internal/plugin/malware_check.go:19` blocks `eval()`, pipe-to-shell, reverse shells; `hawk/internal/plugin/audit.go` flags hidden-Unicode / homoglyph attacks (`hawk skills audit`).

The gap is therefore **consolidation and elevation**, not invention: (a) a *standardized, multi-asset* extension format (today's registry is skills-only); (b) a *browsable web gallery*; (c) a *trust/signing* story beyond static malware regexes; and (d) a *unified docs site* that today is five disconnected `README.md`/`ARCHITECTURE.md` trees (`hawk/docs/`, `eyrie/docs/`, `harrier/`, `shrike/docs/`, `swift/docs/`).

---

## 2. Goals / Non-Goals

### Goals

- **G1.** A single standardized **Hawk Extension** format: a directory with `extension.yaml` (or extended `SKILL.md` frontmatter) that can bundle any subset of: MCP servers, slash commands, prompts, hooks, themes, sub-agents (personas), and skills.
- **G2.** A **browsable gallery** (hawkskills.dev-style) generated from the registry index, with search, categories, per-extension detail pages, and copy-paste install commands.
- **G3.** A versioned, machine-readable **registry index format** (v2) that is a superset of today's `registry.json` and remains backward compatible with `hawk/internal/plugin/registry.go`.
- **G4.** An **install / update / remove flow** with **signing & trust verification** (provenance + signature check, not just regex malware scan).
- **G5.** A **unified documentation site** spanning all 5 repos: getting-started, per-repo API reference, architecture diagrams, cookbook recipes, and the competitor comparison tables — with the gallery either embedded in or cross-linked from it.
- **G6.** Privacy-first throughout: no telemetry-by-default, no account required to browse/install, install is auditable and offline-capable.

### Non-Goals

- **NG1.** A hosted, multi-tenant SaaS backend with accounts, billing, or org RBAC (those are separate P0/P2 gaps at `TOP20_COMPARISON.md:33`, `:72`). The gallery is a **static, build-time-generated site backed by a git repo**, not a dynamic app server.
- **NG2.** Paid/commercial extensions, license-key enforcement, or DRM.
- **NG3.** Replacing per-repo deep-dive docs; the unified site **aggregates and cross-links**, it does not fork content.
- **NG4.** A new package manager. We reuse `git clone`-based install (`registry.go:215`) and the `~/.hawk/skills` / `.hawk/skills` layout (`registry.go:201-203`).
- **NG5.** Runtime arbitrary-code execution in the browser gallery (it renders metadata only; nothing executes client-side).

---

## 3. Architecture

### 3.1 Components

```
┌─────────────────────────────────────────────────────────────────────┐
│  starling  (git repo = source of truth)                  │
│  ├── categories/<cat>/<ext>/extension.yaml   (NEW: multi-asset)       │
│  │       + SKILL.md / commands/ / mcp/ / hooks/ / themes/ / agents/   │
│  ├── registry.json            (v1, kept)                              │
│  ├── registry.v2.json         (NEW: superset index)                  │
│  ├── signatures/<ext>.sig     (NEW: minisign/cosign detached sigs)   │
│  └── tools/*.py               (existing validators, extended)        │
└───────────────┬─────────────────────────────────────┬───────────────┘
                │ raw.githubusercontent (index + sigs)  │ build inputs
                ▼                                       ▼
┌───────────────────────────────┐      ┌──────────────────────────────┐
│  hawk CLI (Go)                │      │  Gallery + Docs site (static) │
│  internal/plugin/registry.go  │      │  Docusaurus or Mintlify       │
│  + new: trust.go, extman.go   │      │  - /extensions (gallery)      │
│  cmd/skills_cmd.go (+ ext cmd)│      │  - /docs/{hawk,eyrie,...}     │
│  internal/plugin/manifest_v2  │      │  - generated from registry.v2 │
└───────────────────────────────┘      └──────────────────────────────┘
```

### 3.2 Data model — the Extension format (G1)

Two interoperable representations, chosen so we **do not break** today's skills-only registry:

**(a) Minimal — extended `SKILL.md` frontmatter** (for single-asset skills, unchanged path):
Already parsed at `hawk/internal/plugin/skill_loader.go:51` (`parseSkillFrontMatter`) and discovered at `hawk/internal/tool/skill.go:62` (`discoverSkills`). We keep this working verbatim.

**(b) Full — `extension.yaml`** (new, for multi-asset bundles):

```yaml
apiVersion: hawk.extension/v1
kind: Extension
name: terraform-pro
version: 1.4.0
description: Terraform authoring — skills, an MCP server, slash commands, and a theme.
license: MIT
author: { name: GrayCode AI, url: https://github.com/GrayCodeAI }
homepage: https://hawkskills.dev/extensions/terraform-pro
keywords: [terraform, iac, devops]
compat:
  minHawkVersion: "1.2.0"          # maps to ManifestV2.MinHawkVersion (manifest_v2.go:20)
provides:
  skills:    [ ./SKILL.md ]
  commands:  [ ./commands/plan.md, ./commands/apply.md ]   # slash commands
  prompts:   [ ./prompts/review.md ]
  hooks:                                                    # maps to ManifestHook (manifest_v2.go:32)
    - { event: pre_tool, command: ./hooks/tf-guard.sh, priority: 50 }
  mcpServers:                                               # maps to internal/mcp/mcp.go
    - name: terraform-mcp
      transport: stdio                                      # stdio | http | sse
      command: ./mcp/server
  themes:    [ ./themes/desert.toml ]
  subagents: [ ./agents/iac-reviewer.md ]                   # personas (internal/multiagent)
permissions: [ run_shell, network ]                         # ManifestV2.Permissions
signing:
  publisher: "graycode"
  signature: "signatures/terraform-pro.sig"
```

The `provides` block is intentionally a **strict superset of the existing assets hawk already loads**: skills (`tool/skill.go`), hooks (`manifest_v2.go:32`), MCP servers (`internal/mcp/mcp.go`), and personas/sub-agents (`internal/multiagent/agents/`, cited at `TOP20_COMPARISON.md:62`). No new runtime concept is invented — `extension.yaml` is a *bundling manifest* over existing loaders.

### 3.3 Registry index format v2 (G3)

Today's `SkillEntry` (`registry.go:21`) already has most fields. v2 adds a `kind` discriminator and a `provides` summary so the gallery can show capability badges without cloning each repo:

```jsonc
{
  "version": 2,
  "updated_at": "2026-06-06T00:00:00Z",
  "extensions": [
    {
      "name": "terraform-pro",
      "kind": "extension",            // "skill" | "extension"
      "description": "...",
      "category": "devops",
      "tags": ["terraform","iac"],
      "path": "categories/devops/terraform-pro",
      "version": "1.4.0",
      "license": "MIT",
      "author": "GrayCode AI",
      "repo": "GrayCodeAI/starling",
      "provides": { "skills":1, "commands":2, "hooks":1, "mcpServers":1, "themes":1, "subagents":1 },
      "signed": true,
      "publisher": "graycode",
      "installs": 0,
      "updated_at": "..."
    }
  ]
}
```

Backward compatibility: `registry.go`'s `SkillIndex.Skills` (`:36`) keeps reading `registry.json`. A small shim maps v2 `extensions[]` → `SkillEntry` so older hawk binaries keep working; new binaries prefer `registry.v2.json` and fall back. The existing `tools/update_registry.py` / `tools/registry_schema.py` generators (in `starling/tools/`) are extended to emit both.

### 3.4 API surface

The marketplace has **no HTTP API server** (per NG1). Its "API" is:

1. **The static index files** over `raw.githubusercontent.com` (already the contract — `registry.go:18`).
2. **The hawk CLI**, extended:
   - Existing: `hawk skills {list,search,install,remove,info,trending,audit}` (`cmd/skills_cmd.go`).
   - New: `hawk ext {search,install,update,verify,info,publish}` for multi-asset extensions, plus `hawk ext install --verify` (signature-checked by default).
3. **The gallery's generated JSON** (`/extensions/index.json`) for the site's client-side search (build-time, no server).

### 3.5 Key flows (sequences)

**Flow A — Install with trust verification**

```
user: hawk ext install GrayCodeAI/starling terraform-pro
  → RegistryClient.FetchIndex()           registry.go:60 (cache <1h)
  → resolve entry, get repo+path+publisher+signature
  → git clone --depth 1 (existing)        registry.go:215
  → trust.go: fetch signatures/<ext>.sig + publisher pubkey
  → trust.go: verify detached signature over the extension tree hash
       fail → abort with provenance error (unless --allow-unsigned)
  → audit.go (hidden-unicode) + malware_check.go (regex) on every file
       blocked pattern → abort                malware_check.go:19
  → parse extension.yaml → ManifestV2       manifest_v2.go:42
  → copy assets to ~/.hawk/skills/<name> (and commands/hooks/mcp dirs)
       scope user|project                    registry.go:201-203
  → inject source-tracking metadata         registry.go (injectSourceMetadata)
  → print: installed N assets (skills, 1 mcp, 1 theme, ...)
```

**Flow B — Gallery browse & install (no account, no server)**
```
browser → hawkskills.dev/extensions  (static Docusaurus/Mintlify page)
  → client loads /extensions/index.json (generated from registry.v2.json)
  → user filters by category/tag/capability badge (skills, MCP, hooks, theme)
  → detail page renders extension.yaml metadata + README + capability list
  → "Install" shows: hawk ext install <repo> <name>   (copy button)
  → nothing executes in browser; install happens locally via CLI
```

**Flow C — Docs build (unified site, all 5 repos)**
```
CI (docs repo) →
  pull README.md / ARCHITECTURE.md / docs/** from each of hawk, eyrie,
     harrier (Harrier), shrike (Shrike), swift (Swift)  (independent checkouts or sparse checkout)
  → transform: inject sidebars, rewrite intra-repo links
  → generate /extensions/* from registry.v2.json
  → render comparison tables from TOP20_COMPARISON.md
  → static build → deploy (GitHub Pages / Cloudflare Pages)
```

---

## 4. Integration with Existing graycode-eco Code

Concrete reuse map — what is *already there* and what each piece becomes:

| New capability | Reuse today | File:line |
|---|---|---|
| Registry fetch + 1h cache | `RegistryClient.FetchIndex` | `hawk/internal/plugin/registry.go:60` |
| Index URL contract | `defaultIndexURL` | `registry.go:18` |
| Entry/index schema | `SkillEntry`, `SkillIndex` | `registry.go:21`, `:36` |
| git-clone install + scopes | `RegistryClient.Install` | `registry.go:196-215`, `:201-203` |
| Remove / info | `Remove`, `InstalledSkillInfo` | `registry.go:289`, `:309` |
| CLI command tree | `skillsCmd` + subcommands | `hawk/cmd/skills_cmd.go:16-211` |
| Skill discovery roots | `discoverSkills`, `skillRoots` | `hawk/internal/tool/skill.go:62`, `:91` |
| Frontmatter parsing | `parseSkillFrontMatter` | `hawk/internal/plugin/skill_loader.go:51` |
| Multi-asset manifest | `ManifestV2` (+`Hooks`,`Config`,`Dependencies`,`Mode`) | `hawk/internal/plugin/manifest_v2.go:11`, `:32` |
| Hook bundling | `ManifestHook{Event,Command,Priority}` | `manifest_v2.go:32` |
| MCP server bundling | MCP client/loader | `hawk/internal/mcp/mcp.go`, `hawk/internal/tool/mcp_tool.go` |
| Sub-agent bundling | Persona system (YAML frontmatter MD) | `hawk/internal/multiagent/agents/` (cf. `TOP20_COMPARISON.md:62`) |
| Rule/skill discovery precedence | `DefaultRuleSources`, `RuleDiscoverer` | `hawk/internal/context/rules.go:20`, `:46` |
| Trust — static scan | `malware_check.go` (blocked/suspicious regex) | `hawk/internal/plugin/malware_check.go:19` |
| Trust — unicode/homoglyph | `audit.go` (`AuditFinding`, severities) | `hawk/internal/plugin/audit.go:12-22` |
| Registry generators/validators | Python tooling | `starling/tools/{update_registry,registry_schema,validate_skill,package_skill,sync_marketplace}.py` |
| Format reference | OpenAPI skill-format spec | `starling/api/openapi.yaml` (`x-skill-format`) |
| IDE/agent plugin manifests | `.claude-plugin/{plugin,marketplace}.json` | `starling/.claude-plugin/` |

**Net-new Go (small, additive):**
- `hawk/internal/plugin/trust.go` — signature verification (Flow A) and a `trustdb` of publisher public keys.
- `hawk/internal/plugin/extension.go` — parse `extension.yaml`, expand `provides` into the existing loaders.
- `hawk/cmd/ext_cmd.go` — `hawk ext` command tree mirroring `skills_cmd.go`.
- Extend `manifest_v2.go` with a `Signing` struct and a `Provides` map (or a thin adapter from `extension.yaml`).

**Net-new outside Go:**
- `registry.v2.json` generator (extend `tools/update_registry.py`).
- The gallery + docs site (new repo, e.g. `hawk-docs`).

The discovery-precedence engine (`rules.go:20` `DefaultRuleSources`, walk-up + local/global/distance/priority sort at `rules.go:97-108`) is reused unchanged for *where* installed extension assets are found — installed extensions land in `.hawk/skills` / `~/.hawk/skills` (project then user), exactly the precedence `RuleDiscoverer` already honors.

---

## 5. Phased Rollout

### P0 — Foundation (format + index + trust core)
- **M0.1** Specify and freeze the `extension.yaml` schema; publish as `x-extension-format` alongside the existing `x-skill-format` in `starling/api/openapi.yaml`.
- **M0.2** `registry.v2.json` generator in `tools/update_registry.py`; emit both v1 and v2; v1 stays the default `defaultIndexURL` consumer for older binaries.
- **M0.3** `extension.go` parser + adapter into `ManifestV2`; `hawk ext install` reuses `registry.go:196` clone path; multi-asset copy (skills/commands/hooks/mcp/themes/subagents).
- **M0.4** Trust v1: detached **minisign/cosign** signatures (`signatures/<ext>.sig`), publisher pubkey bundled in hawk; `--allow-unsigned` escape hatch; wire `malware_check.go` + `audit.go` into the install path as hard gates.
- **Exit:** `hawk ext install` works for at least 5 multi-asset extensions, signature-verified, on a v2 index.

### P1 — Gallery + Docs site (the visible surface)
- **M1.1** Stand up the static site (decision in §6). Pages: `/extensions` (gallery), per-extension detail, `/docs/{hawk,eyrie,harrier,shrike,swift}` (getting-started + API ref + architecture), `/comparison` (rendered from `TOP20_COMPARISON.md`).
- **M1.2** Build-time generation of `/extensions/index.json` from `registry.v2.json`; client-side fuzzy search, category + capability-badge filters.
- **M1.3** Aggregate the 5 independent repositories' docs via checkout or sparse checkout in CI (Flow C); link-rewriting + unified sidebar.
- **M1.4** `hawk ext update` (diff installed version vs index, re-verify, atomic replace) and `hawk ext verify <name>` (re-check signature/audit of an installed extension).
- **Exit:** hawkskills.dev (or chosen domain) live; gallery browseable; one-line install copy works; docs cover all 5 repos with getting-started + API ref.

### P2 — Ecosystem maturity & publisher self-service
- **M2.1** `hawk ext publish` — scaffolds `extension.yaml`, runs validators (`tools/validate_skill.py` extended), signs locally, opens a PR to `starling`.
- **M2.2** Publisher trust tiers: verified-publisher badge (key in hawk's bundled trust DB) vs community (signed but unverified) vs unsigned.
- **M2.3** Trending/installs analytics — privacy-preserving (aggregate counts from PR-based opt-in pings or GitHub stars only; **no per-user tracking**, see §7).
- **M2.4** Cross-repo extension kinds: eyrie provider plug-ins, shrike compression profiles (`TOP20_COMPARISON.md:181` team profiles), swift exporters — registered through the same `extension.yaml` `provides` mechanism with new `kind`s.
- **Exit:** External contributors can publish a verified, multi-asset extension end-to-end; gallery shows mixed-kind extensions across the ecosystem.

---

## 6. Build-vs-Buy & Dependencies

### Gallery + Docs site framework

| Option | Pros | Cons | License |
|---|---|---|---|
| **Docusaurus** (recommended) | OSS, React, self-hosted on GitHub/Cloudflare Pages, full control over the custom `/extensions` gallery component, MDX, versioned docs, large ecosystem | More config than hosted Mintlify; we own the build | **MIT** — no vendor lock-in, aligns with the MIT skills repo (`starling/LICENSE`) |
| **Mintlify** | Beautiful defaults, fast, hosted | Hosted SaaS dependency, less freedom for a custom gallery widget, potential cost, **data leaves our control** (conflicts with privacy-first) | Proprietary SaaS |

**Recommendation: Docusaurus**, self-hosted on GitHub Pages or Cloudflare Pages. It is MIT, keeps the project's privacy-first / no-third-party-data posture (§7), and lets us build the gallery as a first-class React route fed by a static `index.json`. Mintlify's hosted model conflicts with NG1/G6.

### Signing / trust

| Need | Build vs Buy | Choice | License |
|---|---|---|---|
| Detached signatures | Buy (library) | **minisign** (Ed25519, tiny) or **cosign/sigstore** if we want keyless OIDC provenance later | minisign: ISC; cosign: Apache-2.0 |
| Static malware/unicode scan | Already built | reuse `malware_check.go`, `audit.go` | in-repo |

minisign is the lighter P0 choice (single keypair, no external infra). cosign/sigstore is the P2 upgrade if we want keyless, transparency-log-backed provenance (which also dovetails with the SBOM/Cosign work at `TOP20_COMPARISON.md:240`).

### New runtime dependencies for hawk
- A minisign/Ed25519 verify path: Go stdlib `crypto/ed25519` is sufficient — **no new dependency** for verification; only key management is new.
- No new dependency for install: `git` is already required (`registry.go:215`).

### Licensing implications
- Every extension's `extension.yaml` **must** carry a `license` field (already required for skills — `openapi.yaml` `required_frontmatter`). The gallery surfaces it; the validator rejects missing/unknown licenses.
- The community repo is **MIT** (`starling/LICENSE`). Contributed extensions retain their own license but must be OSI-approved; a CI check (extend `tools/validate_skill.py`) flags GPL/AGPL bundled binaries that would conflict with hawk's distribution model (mirrors `TOP20_COMPARISON.md:241`).
- Docusaurus (MIT) and minisign (ISC) impose no copyleft obligations.

---

## 7. Security & Privacy Considerations

These repos are privacy-first; the marketplace must not regress that.

- **No account, no tracking to browse or install (G6).** The gallery is static; install is a local `git clone` + file copy. There is no server logging user identity. This matches the current design where the registry is just a raw JSON file (`registry.go:18`).
- **Supply-chain trust (the core new risk).** Installing an extension means importing executable assets (hooks `manifest_v2.go:32`, MCP server binaries, scripts). Mitigations, in order of enforcement at install time (Flow A):
  1. **Signature verification** (`trust.go`): detached signature over the extension tree hash, verified against a bundled/known publisher key. Unsigned → blocked unless `--allow-unsigned`.
  2. **Static malware scan** (`malware_check.go:19`): blocks `eval(`, pipe-to-shell, base64-to-shell, reverse shells, netcat `-e`.
  3. **Hidden-Unicode / homoglyph audit** (`audit.go`): catches prompt-injection-via-invisible-characters in `SKILL.md`/prompts (already `hawk skills audit`).
  4. **Permission disclosure:** `extension.yaml` `permissions` (mapped to `ManifestV2.Permissions`, `manifest_v2.go:18`) are shown to the user before install; `run_shell`/`network` require explicit confirmation.
- **MCP server execution is the highest-risk asset.** Bundled MCP servers run as subprocesses. P0 ships them as **opt-in** (the extension installs the *definition*; the user must explicitly enable the server), and prefers `stdio` transport with no inbound network surface.
- **Prompt-injection through extension content.** Skills/prompts are injected into the system prompt via the same path as rules (`rules.go`). The audit (3) plus the existing HTML-comment-stripping concern (`TOP20_COMPARISON.md:74`) apply; the validator strips/flags suspicious frontmatter.
- **Offline/air-gapped install.** Because install is `git clone` + local copy with local signature verification, an org can mirror `starling` internally and point `defaultIndexURL`/`--index` at it — no call to GitHub required.
- **No third-party doc analytics.** Self-hosted Docusaurus (§6) avoids shipping user reading data to a SaaS. Any "installs" metric (P2) is aggregate-only and opt-in.
- **Signing key custody.** Verified-publisher keys ship in hawk's binary; rotation requires a hawk release. P2 cosign/sigstore would move trust to a transparency log, reducing reliance on bundled keys.

---

## 8. Open Questions

1. **Signing backend:** minisign (simple, key-in-binary) for P0, or go straight to sigstore/cosign keyless (transparency log, no key custody) and align with the SBOM effort (`TOP20_COMPARISON.md:240`)? Tradeoff: operational simplicity now vs. provenance rigor later.
2. **One repo or two?** Keep gallery + docs in `starling`, or split docs into a new `hawk-docs` repo with the 5 repos as submodules? (Submodules complicate contributor flow but isolate doc build from skill content.)
3. **Cross-repo extension kinds (P2):** do eyrie/shrike/swift assets (provider plugins, compression profiles, exporters) belong in the *same* `starling` registry, or a per-repo registry federated into one gallery index?
4. **Versioning & compat matrix:** `compat.minHawkVersion` exists (`manifest_v2.go:20`), but do we also need per-asset compat (e.g., an MCP server needing a specific transport hawk supports)? Tie-in to the tri-modal MCP transport gap (`TOP20_COMPARISON.md:83`).
5. **Install integrity for non-skill assets:** today install only copies `SKILL.md` trees (`registry.go` discovers `SKILL.md`); multi-asset copy (commands/hooks/mcp/themes/subagents) needs a defined on-disk layout under `~/.hawk/`. What are the canonical install dirs for each asset kind?
6. **Domain & hosting:** confirm `hawkskills.dev` (referenced `TOP20_COMPARISON.md:65`) ownership and Pages target (GitHub vs Cloudflare).
7. **Docs source of truth:** auto-aggregate from each repo's `README.md`/`ARCHITECTURE.md` (drift-free but messy formatting) vs. hand-curated landing pages that link into per-repo docs (cleaner but duplicative)?

---

## 9. Effort Estimate (rough, eng-weeks)

| Workstream | Scope | Est. |
|---|---|---|
| **P0 — format + index v2** | `extension.yaml` spec, v2 generator in `tools/`, `extension.go` parser + `ManifestV2` adapter, multi-asset install layout, `hawk ext` command tree | **4–5 ew** |
| **P0 — trust core** | `trust.go` (Ed25519/minisign verify), publisher key bundling, wire `malware_check`/`audit` as install gates, permission-disclosure UX | **3–4 ew** |
| **P1 — gallery** | Docusaurus setup, `/extensions` React route, build-time `index.json`, search + filters + detail pages | **4–5 ew** |
| **P1 — unified docs** | Aggregate 5 independent repositories (checkout/sparse-checkout CI), sidebars + link rewrite, getting-started + API-ref scaffolding per repo, comparison tables from `TOP20_COMPARISON.md` | **5–7 ew** |
| **P1 — CLI update/verify** | `hawk ext update`, `hawk ext verify`, atomic replace | **2 ew** |
| **P2 — publisher self-service** | `hawk ext publish` scaffolder + validators, PR automation | **3 ew** |
| **P2 — trust tiers + sigstore upgrade** | verified-publisher badges, optional cosign/sigstore keyless + transparency log | **3–4 ew** |
| **P2 — cross-repo kinds** | eyrie/shrike/swift extension kinds + federated index | **3–4 ew** |
| **Cross-cutting** | docs writing (content, not framework), security review, CI, design iteration | **4–6 ew** |
| **Total** | | **≈ 31–44 eng-weeks** (~7.5–11 eng-months; 2 engineers ~4–5 calendar months) |

The estimate is bounded on the low side because the hardest plumbing — the registry client, install flow, CLI tree, V2 manifest, and trust-scan primitives — **already exists and is cited above**; most P0 work is *elevation and bundling*, not greenfield. The largest single line item is writing real documentation content for five repos (not the framework), which dominates P1.

---

### Appendix: grounding index (real files cited)

- `starling/registry.json`, `registry.json` head (schema), `tools/*.py`, `api/openapi.yaml`, `.claude-plugin/{plugin,marketplace}.json`, `categories/testing/ab-test-setup/SKILL.md`, `LICENSE`
- `hawk/internal/plugin/registry.go:18,21,36,60,196,201,215,289,309`
- `hawk/internal/plugin/manifest_v2.go:11,18,20,32,42`
- `hawk/internal/plugin/skill_loader.go:51`
- `hawk/internal/plugin/malware_check.go:19`
- `hawk/internal/plugin/audit.go:12`
- `hawk/internal/tool/skill.go:62,91`
- `hawk/internal/context/rules.go:20,46,97`
- `hawk/cmd/skills_cmd.go:16-211`
- `hawk/internal/mcp/mcp.go`, `hawk/internal/tool/mcp_tool.go`
- `TOP20_COMPARISON.md:65,74,83,181,227,232,240,241`
