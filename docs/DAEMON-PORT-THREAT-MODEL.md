# Hawk Daemon — Port 4590 Threat Model

The Hawk daemon (`hawk daemon`) binds an HTTP server on port **4590** (default)
of the **loopback interface only** (`127.0.0.1:4590`). This document describes
the threat model, attack surface, and mitigation controls.

---

## What the daemon exposes

| Endpoint | Auth required | Notes |
|----------|--------------|-------|
| `GET /v1/ready` | No | Dependency-aware readiness probe |
| `POST /v1/chat` | Configurable | Creates or resumes a session; SSE streaming |
| `GET /v1/sessions` | Configurable | Lists persisted sessions |
| `GET /v1/sessions/:id` | Configurable | Retrieves a session |
| `DELETE /v1/sessions/:id` | Configurable | Deletes a session |
| `GET /v1/stats` | Configurable | Usage/cost statistics |
| `POST /v1/cancel` | Configurable | Cancels the active generation |

---

## Threat model

### In-scope threats

#### T1 — Local network eavesdropping
The daemon binds only to `127.0.0.1`. Traffic never leaves the loopback
interface. An attacker on the same LAN **cannot** reach the daemon.

#### T2 — Local process CSRF
A malicious local process (malware, compromised script) can send HTTP requests
to `127.0.0.1:4590` without any network privilege.

**Mitigation:**
- Set `HAWK_DAEMON_API_KEY` and configure the daemon with `--api-key` to
  require `Authorization: Bearer <key>` on all non-readiness endpoints.
- The key is validated per-request; the daemon does not issue tokens.

#### T3 — Port scanning / fingerprinting
Any local process can detect that port 4590 is open and identify hawk.

**Mitigation:** Low severity — this is unavoidable for a local HTTP service.
The daemon does not expose version information on unauthenticated endpoints.

#### T4 — Session data exfiltration
Session data (conversation history, tool outputs) is stored in SQLite at
`~/.hawk/sessions/hawk.db`. A local attacker with filesystem read access can
read this file directly regardless of the daemon's auth.

**Mitigation:**
- SQLite file permissions are `0600` (owner-only read/write).
- Use full-disk encryption (FileVault on macOS, LUKS on Linux) for complete
  protection.

#### T5 — Denial of service (resource exhaustion)
A local attacker can flood the daemon with chat requests, consuming LLM API
credits.

**Mitigation:**
- A global concurrency cap bounds in-flight generations (default **4**, tuned
  via `HAWK_DAEMON_MAX_CONCURRENT`). When the cap is hit, new `/v1/chat`
  requests are refused with `503` instead of queuing unboundedly.
- Per-IP token-bucket rate limiting: `/v1/chat` is limited to ~30 req/min
  (burst 6) and other authenticated endpoints to ~10 req/min (burst 4).
  Excess requests get `429`.
- `/v1/cancel` (POST `{ "session_id": ... }`) aborts an in-progress
  generation so a runaway response can be stopped early.
- Consider running the daemon behind an OS-level firewall rule if operating
  in a shared-machine environment.

### Out-of-scope threats

- **Remote network attackers**: the daemon does not bind to `0.0.0.0`; remote
  access is architecturally blocked.
- **Process privilege escalation**: hawk does not run as root and does not use
  `setuid`/`setgid`.

---

## Configuring authentication

Set the daemon API key before starting the daemon:

```bash
# Option 1: environment variable (recommended for CI/automation)
export HAWK_DAEMON_API_KEY="your-random-secret-here"
hawk daemon

# Option 2: CLI flag
hawk daemon --api-key "your-random-secret-here"
```

All clients must then pass:
```
Authorization: Bearer your-random-secret-here
```

The SDK reads this from `HAWK_DAEMON_API_KEY` automatically if the env var is
set.

---

## Changing the port

```bash
hawk daemon --port 9000
```

Or in `~/.hawk/settings.json`:
```json
{
  "daemon": {
    "port": 9000
  }
}
```

---

## Shared machine considerations

If hawk is running on a machine shared by multiple OS users (e.g., a dev
server), set the API key **and** use OS firewall rules to restrict which local
users can connect to port 4590:

```bash
# macOS — pf: allow only current user's processes (requires pf.conf editing)
# Linux — iptables: allow only the hawk-owner UID
sudo iptables -A OUTPUT -p tcp --dport 4590 -m owner ! --uid-owner $UID -j REJECT
```

---

## Related

- [`cmd/daemon.go`](../cmd/daemon.go) — daemon entrypoint and auth middleware
- [`internal/daemon/`](../internal/daemon/) — HTTP handlers
- [`SECURITY.md`](../SECURITY.md) — vulnerability reporting
- [`docs/SECURITY-DEVELOPER.md`](SECURITY-DEVELOPER.md) — credential storage model
