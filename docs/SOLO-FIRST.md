# Solo-first product direction

hawk is built **for individual developers** — one person, one machine, one workflow. Team and enterprise features are explicitly **later**, not now.

## Who this is for

| In scope (now) | Out of scope (later) |
|----------------|----------------------|
| One developer on macOS or Linux | Org-wide admin, SSO, RBAC |
| Local config at `~/.hawk/` | Shared team config servers |
| API keys in OS keychain | Vault, proxy gateways, seat licensing |
| Local yaad memory (`~/.yaad/data/`) | Team memory sync, cloud graph |
| Docker/bash isolation on your laptop | Fleet sandbox orchestration |
| `hawk solo` / `/config` first-run | IT-managed deployment packs |

## Design principles

1. **Zero trust in env files** — paste keys in `/config`; never document `export ANTHROPIC_API_KEY` as the happy path.
2. **Graceful optional layers** — yaad, Docker, MCP are enhancements; core chat works without them.
3. **Honest diagnostics** — `hawk solo` tells you exactly what is missing (key, model, Docker, yaad).
4. **Local-first privacy** — code stays on your machine except to the LLM provider you choose.
5. **No co-author theater** — commits list the human author only (see CONTRIBUTING).

## Solo developer journey

```
Install hawk
    → hawk (TUI opens /config on first run)
    → Paste API key → OS keychain
    → Pick model from eyrie catalog
    → hawk solo  (READY)
    → Chat with tools (Docker bash when available)
    → yaad remembers conventions across sessions (optional)
```

## Verify

```bash
hawk solo              # unified readiness report
hawk preflight         # eyrie chat readiness
hawk credentials status
./scripts/verify-solo-path.sh
```

## Security model

See [SECURITY-SOLO.md](SECURITY-SOLO.md) for keychain-only credentials, Read-tool path blocks, and container isolation.

## Architecture reference

- [ecosystem-message-flow.md](ecosystem-message-flow.md) — eyrie · yaad · tok in one chat turn
- [../plans/SOLO-DEVELOPER-PATH.md](../plans/SOLO-DEVELOPER-PATH.md) — research, gaps, implementation plan

## What stays in the repo but is not “solo core”

These exist for power users and future team modes; they are not required for the solo path:

- `hawk mission` — parallel git worktrees
- Daemon mode — HTTP API for integrations
- Multi-agent personas — optional `/agents`

When adding features, ask: **does this help one developer on their laptop?** If not, defer or gate behind explicit opt-in.
