## Summary

Ship the post-audit hardening batch: Charm v2-only TUI stack, tighter binary
size gate, Hawk Cloud CLI integration, engine pin hygiene, release Gitlink
strictness, and Yaad re-pin after the demo TUI nested-module split.

## Why

- Dual Charm v1/v2 inflated the binary and dependency graph
- Releases must never silently fall back to engine `main` when a Gitlink is
  missing or unreachable
- Yaad’s library graph must stay free of Bubble Tea so Hawk does not pay for a
  demo TUI

## Highlights

| Area | Change |
|------|--------|
| TUI | Migrate to `charm.land/*/v2`; fix remaining API incompatibilities |
| Size | `make size-check` / CI threshold **110MB → 80MB** (~75MB verified) |
| Cloud | CLI login, usage reporting, delivery-context wiring |
| Pins | Submodule updates + `scripts/check-submodule-release-parity.sh` |
| Layers | `scripts/check-internal-layer-imports.sh` |
| Release | `checkout-eyrie` fails closed without Gitlinks; release job verifies pins |
| Yaad | Re-pin to nested-module TUI split (`b7ee281`) |
| Docs | Remediation plans updated with acceptance evidence |

## Depends on

1. Merge/push **yaad** PR first (`b7ee281` must be reachable on origin)
2. Then this PR (or push) so public-module CI can resolve the new pseudo-version

## Test plan

- [x] `make size-check` → ~75 MB
- [x] `go list -m all` has no `charmbracelet/{bubbles,bubbletea,lipgloss}` v1 stack
- [x] `go test ./internal/intelligence/memory/ ./internal/platform/cloud/ ./cmd`
- [x] `make internal-layers-guard`
- [ ] CI green after yaad is published (`public-modules`, `submodule-release-parity`)
- [ ] Manual smoke: REPL, `/config`, `/autonomy` pickers (Charm v2)

## Rollout

```bash
# 1) yaad
cd yaad && git push origin main   # or open PR from docs/PR_BODY.md

# 2) hawk
cd hawk && git push origin main   # or open PR from this body
# if needed after publish:
go mod tidy && git add go.sum && git commit -m "chore: refresh go.sum after yaad publish"
```
