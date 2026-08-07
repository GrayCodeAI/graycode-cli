# Hawk Permission Model — Improvements (2026-08-07)

This document describes the improvements made to hawk's permission, isolation,
and autonomy systems. All changes preserve the existing fail-closed architecture
and are backward-compatible.

## Summary of Changes

| # | Improvement | Files | Risk |
|---|-------------|-------|------|
| 1 | Unified grant store | `permissions/grants.go`, `permission.go`, `advanced.go`, `approval.go` | Low |
| 2 | Per-tool autonomy profiles | `safety/profile.go`, `autonomy.go`, `settings.go` | Low |
| 3 | Time-bound scoped bypass | `advanced.go`, `permission_engine.go` | Low |
| 4 | Personal hard ceiling (never rules) | `settings.go`, `permission_engine.go` | Low |
| 5 | Permission metrics + audit log | `permission_metrics.go`, `permission_engine.go`, `permission.go` | Low |
| 6 | Container network isolation | `container.go`, `settings.go` | Low |
| 7 | Egress enforcement via sandbox | `permission_engine.go` | Low |
| 8 | CodeVerifier language expansion | `code_verifier.go`, `config_toml.go` | Low |
| 9 | Spec-stage test allowance | `permission_engine.go`, `settings.go` | Low |
| 10 | Approve-for-N | `approval_gate.go`, `multiagent/approval.go` | Low |
| 11 | Grants CLI surface | `permissions_center.go` | Low |
| 12 | Autonomy picker UX + Supervised guard | `autonomy_tiers.go`, `chat_update.go`, `chat_model.go` | Low |
| 13 | Seatbelt crash cleanup | `seatbelt.go` | Low |
| 14 | Credential access approval | `sandbox/credentials.go`, `tool/credential_gate.go`, `cmd/credential_gate.go` | Medium |
| 15 | Full Dockerfile toolkit | `container/Dockerfile` | Low |
| 16 | OSV live database | `osv_checker.go` | Medium |
| 17 | Transcript resume | `worker_transcript.go`, `worker.go` | Medium |
| 18 | go vet copylocks fix | `permission_engine.go` | Low |

---

## 1. Unified Grant Store

**Problem:** Three separate allow/deny systems (`PermissionMemory`, `AutoModeState`,
`ApprovalStore`) with different scopes, persistence, and matching semantics.

**Solution:** `permissions/grants.go` introduces a `GrantStore` interface and
`UnifiedGrants` that merges all three backends into one precedence-ordered view.

**Precedence:** deny > allow, then higher source priority (governance > hook >
user-deny > user-allow > auto-learned), then more specific pattern.

**Key types:**
```go
type Grant struct {
    Tool, Pattern string
    Allow         bool
    Source        GrantSource
    Scope         string
    Expires       *time.Time
}

type GrantStore interface { Grants() []Grant }

type UnifiedGrants struct { stores []GrantStore }
func (u *UnifiedGrants) Check(tool, summary string, time.Time) (bool, bool)
```

---

## 2. Per-Tool Autonomy Profiles

**Problem:** Autonomy is a flat 5-level int. Can't say "Full but still ask for
network" without writing rules.

**Solution:** `safety/profile.go` introduces `AutonomyProfile` with per-flag
overrides (`AutoExecuteBash`, `AutoNetwork`, etc.) at any tier.

**Settings:** `settings.AutonomyOverrides map[string]bool` persists per-flag tweaks.

**CLI:** `/autonomy profile auto_execute_bash=off`

---

## 3. Time-Bound Scoped Bypass

**Problem:** `BypassKillswitch` is global and permanent once enabled.

**Solution:** `advanced.go` replaces the bool with `BypassGrant` (scope + expiry +
reason). `permissions.ToolCategory()` maps tools to categories (bash/network/filesystem).

**CLI:** `/autonomy bypass on --scope=bash --for=5m --reason="debugging"`

---

## 4. Personal Hard Ceiling (Never Rules)

**Problem:** No user-set ceiling that even YOLO can't override.

**Solution:** `settings.NeverAllow []string` evaluated after governance but before
autonomy and bypass. Even YOLO + bypass cannot override a never-rule.

**CLI:** `/autonomy never Write(*.env)` / `/autonomy never clear`

---

## 5. Permission Metrics + Audit Log

**Problem:** No telemetry on permission decisions; bypass is only `slog.Warn`.

**Solution:** `permission_metrics.go` provides atomic counters (decisions by
outcome/reason/tool, bypass by scope, governance denials by tool, autonomy level
gauge). `permission.go` adds a ring-buffer audit log (256 entries).

**CLI:** `/autonomy audit` (recent decisions), `/autonomy metrics` (counters)

---

## 6. Container Network Isolation

**Problem:** Default `bridge` network lets concurrent containers probe each other.

**Solution:** `settings.ContainerNetwork` (`none`/`bridge`/`isolated`). When
`isolated`, a per-container Docker network is created at Start and removed at Stop.

**CLI:** `/autonomy isolation <none|bridge|isolated>`

---

## 7. Egress Enforcement via Sandbox

**Problem:** `EgressInspector` uses brittle regex; real enforcement should be the
sandbox network mode.

**Solution:** `permission_engine.go` denies WebFetch/WebSearch at `TierStrict`
regardless of autonomy or egress regex. Clear error: "network access denied by
sandbox strict mode".

---

## 8. CodeVerifier Language Expansion

**Problem:** Only Python/Go/Bash patterns. Node.js `child_process` not caught.

**Solution:** `code_verifier.go` adds JS/TypeScript (`child_process`, `vm`, `Function`,
`fs.unlinkSync`/`rmSync`) and Ruby (`Kernel#system`/`exec`, `FileUtils.rm_rf`)
patterns. Configurable via `sandbox.toml` `[profiles.custom.code_verifier]`.

---

## 9. Spec-Stage Test Allowance

**Problem:** Spec gate blocks everything except spec tools + reads. Can't run tests.

**Solution:** `settings.SpecAllowTests` enables safe test commands (`go test`,
`npm test`, `pytest`, `cargo test`, etc.) during the spec workflow.

**CLI:** `/autonomy spec-tests <on|off>`

---

## 10. Approve-for-N

**Problem:** No middle ground between "allow once" and "allow for session."

**Solution:** `ApprovalApproveForN` in `approval_gate.go` and `ResponseApproveForN`
in `multiagent/approval.go`. Both engine and multiagent gates support N-count
approvals with auto-expiry.

**CLI:** `/autonomy allow Bash(go test*) --for=10`

---

## 11. Grants CLI Surface

**Problem:** `sandbox.grants.jsonc` exists but no CLI to manage it.

**Solution:** `/autonomy rules` now shows unified grants with source labels
(`[memory]`, `[auto]`, `[grant]`). `/autonomy grants cleanup` rebuilds rules from
settings.

---

## 12. Autonomy Picker UX + Supervised Guard

**Problem:** Supervised excluded from Ctrl+L quick-cycle; no guard against accidental
max-friction.

**Solution:** `containerAutonomyTiers` now includes all 5 levels. Regular cycle skips
Supervised (YOLO→Basic wrap). At YOLO, first Ctrl+L shows confirmation prompt;
second within 1.5s lands on Supervised. Prompt auto-expires.

---

## 13. Seatbelt Crash Cleanup

**Problem:** Crash could leave `sandbox-*.sb` temp files behind.

**Solution:** `seatbelt.go` `init()` removes orphaned `hawk-seatbelt-*.sb` files from
`os.TempDir()` at process startup.

---

## 14. Credential Access Approval

**Problem:** Container has no access to host credentials (SSH keys, git config, kube
config, etc.). Either everything is auto-forwarded (insecure) or nothing is (broken).

**Solution:** Approval-gated credential forwarding. Container starts with credentials
mounted read-only into `/_credentials/staging/`. Expected paths (`~/.kube/config`)
are symlinks to a "denied" placeholder. AI calls `RequestCredential` tool → user
approves/denies → symlink flips to staging copy.

**Registry:** gitconfig, kube, aws, gh, docker, gnupg, terraform.

**Files:** `sandbox/credentials.go`, `tool/credential_gate.go`, `cmd/credential_gate.go`.

---

## 15. Full Dockerfile Toolkit

**Problem:** Sandbox image had only minimal tooling (git, curl, python, node, Go).

**Solution:** `container/Dockerfile` extended with: gh, docker CLI, terraform, kubectl,
helm, Java 21, Ruby, Rust, .NET 8, vim, nano, gpg, psql, mysql, redis-cli, sqlcmd, bat,
delta, zoxide, direnv, starship, zsh.

---

## 16. OSV Live Database

**Problem:** `OSVChecker.RefreshDatabase()` was a stub; embedded-only database.

**Solution:** Live OSV API integration via `api.osv.dev/v1/querybatch`. Rate-limited
(1 req/sec), background refresh, malware-only filtering, cache invalidation.

**API:** `osv_checker.go` — `EnableNetworkRefresh(interval)`,
`StartBackgroundRefresh()`, `Stop()`, `RefreshDatabase()`.

---

## 17. Transcript Resume

**Problem:** Multi-agent workers are fire-and-forget. Crash loses all progress.

**Solution:** Worker transcripts persisted to JSONL (`missionDir/workers/<id>.jsonl`).
On resume: completed → reuse handoff; incomplete → load messages and continue.

**Files:** `worker_transcript.go` (writer/reader), `worker.go` (EngineWorker changes).

---

## 18. go vet copylocks Fix

**Problem:** `CheckToolSnapshot` and `EvaluateTool` copied `PermissionEngine` including
its `sync.RWMutex`, triggering vet warnings.

**Solution:** Both functions now construct a fresh `PermissionEngine` with only the
needed fields, avoiding the mutex copy.

---

## Architecture After Changes

```
Tool call
    │
    ▼
PermissionEngine.evaluateToolDecision()
    │
    1. DryRun                 → deny
    2. Governance ceiling     → deny (admin POLICY ∩ PROFILE)
    3. NeverAllow (NEW)       → deny (personal ceiling)
    4. PreToolUse hooks       → deny
    5. Sandbox mode           → deny (read-only strict)
    6. Network egress (NEW)   → deny (strict blocks WebFetch)
    7. Spec-stage gate        → deny (or allow tests if on)
    8. Destructive hard-deny  → deny
    9. UnifiedGrants (NEW)    → allow/deny (deny>allow, specificity)
    10. Profile (NEW)         → allow (per-flag autonomy + overrides)
    11. Scoped bypass (NEW)    → allow (time/category-limited)
    12. Classifier            → allow (safe bash)
    13. ApprovalGate          → human confirm (once/session/N)
    14. Prompt user           → allow once / always / deny
    │
    ├── Metrics recorded (NEW)
    └── Audit logged (NEW)
```

## Verification

- `go build ./...` — clean
- `go vet ./...` — clean
- `go test ./...` — 127 packages pass
- Credential approval tested end-to-end in Docker (staging mounts, symlink gating,
  read-only enforcement all verified)
