# Security Policy — hawk

## Supported versions

We support the latest minor version on each `0.x` line, and the latest two
minor versions once `1.x` ships. Older versions receive critical-severity
fixes only on a best-effort basis.

The current canonical version is the contents of the [`VERSION`](./VERSION)
file at the repo root. See [`docs/versioning.md`](https://github.com/GrayCodeAI/hawk/blob/main/docs/versioning.md)
for the eco-wide versioning scheme.

## Reporting a vulnerability

**Do not open a public GitHub issue for security vulnerabilities.** Instead:

1. Open a private [GitHub Security Advisory](https://github.com/GrayCodeAI/hawk/security/advisories/new), **or**
2. Email `security@graycode.ai` with the details below.

Include in your report:

- A description of the vulnerability and the affected component.
- Steps to reproduce, ideally with a minimal proof-of-concept.
- The version (`VERSION` file or git SHA) you tested against.
- The potential impact and any suggested mitigation.

**Response targets:**

- Initial acknowledgement: within **48 hours**.
- Triage and severity assessment: within **5 business days**.
- Coordinated fix and disclosure: within **30 days** for high/critical, **90
  days** for medium/low (per industry-standard responsible disclosure).

## Disclosure policy

We follow [coordinated vulnerability disclosure](https://en.wikipedia.org/wiki/Coordinated_vulnerability_disclosure):

- Reporters receive credit in the advisory and CHANGELOG (unless they opt
  out).
- We request that reporters refrain from public disclosure until a fix has
  been released or the disclosure deadline above has elapsed.
- We will not pursue legal action against good-faith researchers acting
  within this policy.

## Security practices in this repo

- **Dependency monitoring:** vulnerable dependencies are detected by
  `govulncheck`, which runs on every CI build (see "Vulnerability scanning").
- **Static analysis:** `golangci-lint` (including `gosec` rules) and `go vet`
  are enforced in CI.
- **Vulnerability scanning:** `govulncheck` runs on every CI build.
- **Dependency pinning:** `go.sum` is pinned and committed; ecosystem
  submodules are pinned to exact Gitlinks and verified by
  `make submodule-release-parity`.
- **Reproducible builds:** release artefacts ship with SHA-256 checksums via
  goreleaser.
- **No secrets in source:** API keys are configuration, not constants. Pre-
  commit hooks block accidental secret commits.

## Scope

This policy covers the code in this repository and the release artefacts
published from it. It does not cover:

- Third-party dependencies (report to upstream).
- LLM provider services that hawk integrates with (report to the
  provider).
- Local filesystem misuse where an attacker already has shell access (out of
  threat model).

For hawk-specific threat-model notes, see the README and any docs in
this repo.

## Config security model

### Clone-and-load attack defense

Project-level `.hawk/settings.json` can be committed to a git repository.
An attacker who controls a repository could define MCP servers that execute
arbitrary commands when a developer clones and runs hawk in that directory.

**Mitigation:** Project-level MCP servers are blocked by default. They are
only loaded when the user explicitly passes `--allow-project-mcp` on the
command line. Global MCP servers (from `~/.hawk/settings.json`) are always
loaded.

### Security-sensitive fields

The following settings **cannot** be overridden by project-level config:
- MCP servers (blocked by default, require explicit `--allow-project-mcp` flag)
- API keys (never stored in settings.json; use OS secret store via `/config`)

The following settings **can** be overridden by project config:
- `model`, `provider` (convenience, not a security risk)
- `theme`, `auto_allow`, `allowed_tools`, `disallowed_tools`
- `max_budget_usd`, `sandbox`, `autonomy`

### Config merge precedence

1. Global `~/.hawk/settings.json` (lowest priority)
2. Project `.hawk/settings.json` (overrides global)
3. CLI `--settings` flag (overrides both)
4. Environment variables (highest priority for specific keys)

Project-level config CANNOT escalate permissions beyond what global config
allows. The `MergeSettings` function in `internal/config/settings.go`
implements field-by-field merging with explicit precedence rules.

### Credential storage

API keys and secrets are stored in the OS secret store (macOS Keychain,
Linux secret service, Windows Credential Manager) via the `eyrie/credentials`
package. They are never written to `settings.json`, `.env`, or any file
in the repository. The `/config` command manages credential storage
interactively.
