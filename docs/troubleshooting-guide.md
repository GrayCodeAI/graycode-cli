# Troubleshooting Guide

A practical guide for diagnosing common graycode daemon and CLI issues.

## Table of Contents

- [Daemon Won't Start](#daemon-wont-start)
- [Health/Readiness Failures](#healthreadiness-failures)
- [API Key / Authentication Issues](#api-key--authentication-issues)
- [Rate Limiting (429)](#rate-limiting-429)
- [CORS Errors](#cors-errors)
- [Chat Returns 503](#chat-returns-503)
- [Metrics Endpoint](#metrics-endpoint)
- [Telemetry / Tracing Not Working](#telemetry--tracing-not-working)
- [Audit Log Issues](#audit-log-issues)
- [Tool Execution Fails](#tool-execution-fails)
- [Performance Issues](#performance-issues)
- [Docker Issues](#docker-issues)
- [Systemd Issues](#systemd-issues)

---

## Daemon Won't Start

### "apiKey is empty and bind address is not loopback"

The daemon refuses to start when bound to `0.0.0.0` without an API key,
because the auth middleware would be open to the network.

**Fix:** Set an API key:

```bash
export GRAYCODE_DAEMON_API_KEY=$(openssl rand -base64 32)
graycode daemon start --host 0.0.0.0 --port 4590
```

Or bind to loopback only (no API key required, but not remotely accessible):

```bash
graycode daemon start --host 127.0.0.1 --port 4590
```

### "permission denied" on state directory

The daemon writes logs, PID files, and the audit log to `~/.graycode/state/`.

**Fix:**

```bash
mkdir -p ~/.graycode/state
chmod 750 ~/.graycode/state
# If running under systemd as user 'graycode', ensure ownership:
chown -R graycode:graycode ~/.graycode
```

### "port already in use"

Another process is using port 4590.

**Fix:**

```bash
# Find the process
lsof -i :4590

# Or use a different port
graycode daemon start --port 4591
```

---

## Health/Readiness Failures

### `GET /v1/ready` returns 503

The readiness probe fails when Eyrie's local preflight doesn't pass. This
checks provider state, catalog, credentials, and model selection.

**Diagnosis:**

```bash
curl -v http://localhost:4590/v1/ready
```

Check the response body for the specific failed check. Common causes:

- No model configured — set `GRAYCODE_MODEL` or provider credentials.
- Eyrie catalog not initialized — the nine Go modules are independent sibling
  repositories; from a full parent workspace, run `make setup` in graycode to
  regenerate the parent `go.work`:
  ```bash
  make setup
  go work sync
  ```

### `GET /v1/health` returns 503

The health endpoint should always return 200 when the daemon is running.
If it returns 503, the daemon process may have crashed or the server
failed to start.

**Diagnosis:**

```bash
# Check if the process is running
ps aux | grep graycode

# Check logs
journalctl -u graycode-daemon -n 50
tail -50 ~/.graycode/state/daemon.log
```

---

## API Key / Authentication Issues

### "401 Unauthorized" on all requests

The daemon requires `Authorization: Bearer <key>` or `X-API-Key: <key>`.

**Fix:**

```bash
curl -H "X-API-Key: $GRAYCODE_DAEMON_API_KEY" http://localhost:4590/v1/stats
```

### "constant time comparison" errors in logs

These are informational — the daemon performs a constant-time comparison
even when no API key is set (to avoid timing-based information leakage).
No action needed.

### Forgotten API key

The API key is written to `~/.graycode/state/daemon.key` (permissions 0600).

```bash
cat ~/.graycode/state/daemon.key
```

**Security note:** Remove this file in production after initial testing:

```bash
rm ~/.graycode/state/daemon.key
```

---

## Rate Limiting (429)

If you receive `429 Too Many Requests`, the per-IP rate limiter has rejected
your request. The default limits are:

- **General API**: 10 req/min, burst 4
- **Chat**: 30 req/min, burst 6
- **Concurrent chat sessions**: 4 (configurable via `GRAYCODE_DAEMON_MAX_CONCURRENT`)

**Fix:**

- Reduce request frequency
- Increase rate limits in the daemon config (requires code change — see
  `defaultAPIRatePerMin` and `defaultChatRatePerMin` in `internal/daemon/daemon.go`)
- Add `Retry-After` header handling on the client

---

## CORS Errors

### "No 'Access-Control-Allow-Origin' header" in browser console

CORS is **disabled by default**. Enable it when serving browser-based clients:

```bash
graycode daemon start --cors https://app.example.com --cors https://admin.example.com
```

Use `--cors '*'` only for development — it allows any origin.

### Pre-flight (OPTIONS) returns 405 Method Not Allowed

The CORS middleware handles OPTIONS preflight automatically when the `cors`
feature flag is enabled. If you're getting 405, ensure CORS is enabled:

```bash
export GRAYCODE_FEATURE_CORS=1
```

---

## Chat Returns 503

The chat endpoint returns 503 when no session factory is configured (CLI
mode) or when the engine is not ready (daemon mode).

**Diagnosis:**

```bash
curl http://localhost:4590/v1/ready
```

If readiness fails, the session factory exists but Eyrie preflight is not
satisfied (missing model, credentials, etc.).

---

## Metrics Endpoint

### `GET /v1/metrics` returns 401

The metrics endpoint is protected by the same API key authentication as other
endpoints.

```bash
curl -H "X-API-Key: $GRAYCODE_DAEMON_API_KEY" http://localhost:4590/v1/metrics
```

### Metrics output is empty

The daemon hasn't received any requests yet. Make a request first, then
check metrics again.

### Prometheus can't parse the output

The daemon uses Prometheus text exposition format 0.0.4. Ensure your
Prometheus version supports this format (Prometheus 2.20+).

For troubleshooting, try the JSON format:

```bash
curl -H "X-API-Key: $GRAYCODE_DAEMON_API_KEY" \
  "http://localhost:4590/v1/metrics?format=json"
```

---

## Telemetry / Tracing Not Working

### Traces are not being sent to the collector

Telemetry is **opt-in**. You must explicitly enable it:

```bash
export GRAYCODE_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
graycode daemon start
```

**Important**: Setting `OTEL_EXPORTER_OTLP_ENDPOINT` alone does **not**
enable telemetry. The `GRAYCODE_ENABLE_TELEMETRY=1` flag is required.

### "telemetry initialization failed" warning in logs

The OTel SDK couldn't connect to the configured endpoint. Check:

1. The collector is running and reachable
2. The endpoint URL is correct (include port and path if needed)
3. The protocol matches (`http/protobuf` is the default)

```bash
# Verify the collector is reachable
curl -v http://localhost:4318/v1/traces
```

### Spans are created but not exported

The OTel SDK uses a batch span processor with a 5-second flush interval.
On shutdown, the daemon waits up to 2 seconds (configurable via
`GRAYCODE_OTEL_SHUTDOWN_TIMEOUT_MS`) to flush pending spans.

To force a flush, send SIGTERM to the daemon — it will flush telemetry
before shutting down.

---

## Audit Log Issues

### Security log won't open

The audit log is stored in `~/.graycode/state/securitylog/`. If the directory
doesn't exist or isn't writable:

```bash
mkdir -p ~/.graycode/state/securitylog
chmod 700 ~/.graycode/state/securitylog
```

### "log tail does not match head pointer (truncated or tampered)"

This error means the security log has been modified or truncated. The
tamper-evident design detected an inconsistency. Restore from a backup
of the `~/.graycode/state/securitylog/` directory.

### Lost the HMAC key

The HMAC key (`sel.key`) is required to verify the audit log. If it's
lost, all entries become unverifiable. **Always back up the entire
`~/.graycode/state/securitylog/` directory.**

---

## Tool Execution Fails

### "Container not ready — tools are disabled"

Tools that require sandboxing are disabled until the sandbox container
is running. Check the sandbox status:

```bash
graycode sandbox status
```

Start the sandbox:

```bash
graycode sandbox start
```

### Permission denied for a tool

The permission service may have denied the tool execution. Check the
audit log for the `denied` event type.

---

## Performance Issues

### High latency on /v1/chat

1. Check the metrics endpoint for request duration:
   ```bash
   curl -H "X-API-Key: $GRAYCODE_DAEMON_API_KEY" \
     "http://localhost:4590/v1/metrics?format=json"
   ```
2. Check concurrent sessions: `graycode_daemon_chat_concurrency_used`
3. If at capacity, increase the concurrency limit:
   ```bash
   export GRAYCODE_DAEMON_MAX_CONCURRENT=8
   ```

### High memory usage

The daemon retains session state in memory. Long-running sessions with
large contexts can consume significant memory. Consider:

- Periodic session cleanup
- Limiting `max_turns` per session
- Using the `/v1/sessions` endpoint to monitor active sessions

---

## Docker Issues

### Daemon exits immediately

The default entrypoint runs `graycode daemon start --host 0.0.0.0 --port 4590`.
Non-loopback binds require both an API key and native TLS; an API key alone
does not protect credentials or conversation data from plaintext interception.

**Fix:**

```bash
docker run -p 4590:4590 \
  -e GRAYCODE_DAEMON_API_KEY=$(openssl rand -base64 32) \
  -v "$PWD/certs:/certs:ro" \
  ghcr.io/graycodeai/graycode-daemon:latest \
  --tls-cert /certs/server.crt --tls-key /certs/server.key
```

### Health check fails in container

The container's `HEALTHCHECK` probes `http://127.0.0.1:4590/v1/health`.
If the daemon is still starting up, the health check may fail before the
`start-period` expires (10 seconds). Increase the health check interval
or start-period if needed.

---

## Systemd Issues

### "Failed to start graycode-daemon.service: Unit not found"

Install the unit file:

```bash
sudo cp packaging/systemd/graycode-daemon.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now graycode-daemon
```

### "Permission denied" accessing state directory

The systemd unit runs as user `graycode` with `ProtectSystem=strict` and
`ReadWritePaths=%h/.graycode/state`. Ensure the user's home directory has
the correct state:

```bash
sudo -u graycode mkdir -p /home/graycode/.graycode/state
sudo -u graycode chmod 750 /home/graycode/.graycode/state
```

### Daemon not logging to journald

If using the systemd unit, logs go to both journald (stdout/stderr) and
`~/.graycode/state/daemon.log`. Check:

```bash
journalctl -u graycode-daemon -f
tail -f ~/.graycode/state/daemon.log
```

If journald logs are missing, verify that stdout/stderr are not being
redirected in the unit file's `ExecStart`.
