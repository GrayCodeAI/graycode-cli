# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| 0.4.x   | Yes |
| < 0.4   | No  |

## Reporting a Vulnerability

**Do NOT open a public GitHub issue for security vulnerabilities.**

Email: security@graycode.ai

### Response Timeline
- Acknowledgment: 48 hours
- Initial assessment: 5 business days
- Fix: 7-30 days depending on severity

## Security Design

### Sandboxing
- Linux: Landlock filesystem sandboxing + seccomp-bpf syscall filtering
- macOS: sandbox-exec profiles
- Docker: optional container isolation
- All platforms: command allowlists, path restrictions

### Credential Handling
- API keys stored in ~/.hawk/provider.json (0600 permissions)
- Provider keys passed via environment variables, never logged
- Daemon binds to 127.0.0.1 only (localhost)

### Permission System
- All tool executions require user approval based on risk level
- Auto-learning from user decisions (per-project)
- Dangerous operations (rm -rf, git push --force) always prompt

### Network
- eyrie handles all LLM API calls over HTTPS
- No telemetry or data collection without explicit opt-in
- Daemon port (4590) not exposed externally

## Scope

In scope:
- Remote code execution via tool execution
- Sandbox escape
- Permission bypass
- Credential exposure
- Command injection via LLM-generated tool calls
- Path traversal outside allowed directories

Out of scope:
- Issues requiring physical access
- Social engineering
- Denial of service via resource exhaustion
