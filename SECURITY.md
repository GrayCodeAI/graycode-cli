# Security Policy — hawk

## Supported versions

We support the latest minor version on each `0.x` line, and the latest two
minor versions once `1.x` ships. Older versions receive critical-severity
fixes only on a best-effort basis.

The current canonical version is the contents of the [`VERSION`](./VERSION)
file at the repo root. See [`VERSIONING.md`](https://github.com/GrayCodeAI/hawk-eco/blob/main/VERSIONING.md)
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

- **Dependency monitoring:** automated via Dependabot (see
  `.github/dependabot.yml`).
- **Static analysis:** `golangci-lint` / `ruff` / `mypy` enforced in CI.
- **Vulnerability scanning:** `govulncheck` (Go) / `pip-audit` (Python) run
  on every CI build.
- **Lockfiles:** `go.sum` / `pnpm-lock.yaml` / `pyproject.toml` are pinned
  and committed.
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
