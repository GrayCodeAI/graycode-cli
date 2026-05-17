---
name: security-scan
description: Scans code for injection, auth flaws, secrets, and dependency vulnerabilities
version: "1.0.0"
author: graycode
license: MIT
category: security
tags: ["security", "scan", "vulnerabilities"]
allowed-tools: Read Grep Bash Glob
---

# Security Scan

## When to Use
- Before merging PRs with auth or data handling changes
- Auditing a new codebase
- Checking for leaked secrets or credentials

## Workflow
1. **Secrets scan**: grep for API keys, tokens, passwords in source
2. **Injection**: check for unsanitized user input in SQL, shell, HTML
3. **Auth**: verify authentication on all sensitive endpoints
4. **Dependencies**: check for known vulnerabilities (`go vuln`, `npm audit`)
5. **Permissions**: verify least-privilege access patterns
6. **Logging**: ensure no PII or secrets in log output

## Patterns to Flag

### Hardcoded secrets
```
# Flag these patterns
password = "..."
api_key = "sk-..."
token = "ghp_..."
AWS_SECRET_ACCESS_KEY=...
```

### SQL injection
```go
// Bad
db.Query("SELECT * FROM users WHERE id = " + userInput)

// Good
db.Query("SELECT * FROM users WHERE id = $1", userInput)
```

### Missing auth check
```go
// Flag: no auth middleware on sensitive route
router.POST("/admin/delete-user", deleteUserHandler)
```

## Verification
- No hardcoded secrets in source (use env vars or secret managers)
- All user input is validated and sanitized
- SQL queries use parameterized statements
- Dependencies have no known critical CVEs
