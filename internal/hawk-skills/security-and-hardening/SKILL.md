---
name: security-and-hardening
description: Hardens code against vulnerabilities. Use when handling user input, authentication, data storage, or external integrations. Use when building any feature that accepts untrusted data, manages user sessions, or interacts with third-party services.
version: "1.0.0"
author: "graycode"
license: "MIT"
category: security
tags: ["security", "hardening", "vulnerabilities"]
allowed-tools: ["Read", "Write", "Edit", "Glob", "Grep", "Bash"]
---

# Security and Hardening

## Overview

Security-first development practices for Go applications. Treat every external input as hostile, every secret as sacred, and every authorization check as mandatory. Security isn't a phase — it's a constraint on every line of code that touches user data, authentication, or external systems.

## When to Use

- Building anything that accepts user input
- Implementing authentication or authorization
- Storing or transmitting sensitive data
- Integrating with external APIs or services
- Adding file uploads, webhooks, or callbacks
- Handling payment or PII data

## Process: Threat Model First

Controls bolted on without a threat model are guesses. Before hardening, spend five minutes thinking like an attacker:

1. **Map the trust boundaries.** Where does untrusted data cross into your system? HTTP requests, form fields, file uploads, webhooks, third-party APIs, message queues, and **LLM output**. Every boundary is attack surface.
2. **Name the assets.** What's worth stealing or breaking? Credentials, PII, payment data, admin actions, money movement.
3. **Run STRIDE over each boundary:**

| Threat | Ask | Typical mitigation |
|---|---|---|
| **S**poofing | Can someone impersonate a user/service? | Authentication, signature verification |
| **T**ampering | Can data be altered in transit or at rest? | Integrity checks, parameterized queries, HTTPS |
| **R**epudiation | Can an action be denied later? | Audit logging of security events |
| **I**nformation disclosure | Can data leak? | Encryption, field allowlists, generic errors |
| **D**enial of service | Can it be overwhelmed? | Rate limiting, input size caps, timeouts |
| **E**levation of privilege | Can a user gain rights they shouldn't? | Authorization checks, least privilege |

4. **Write abuse cases next to use cases.** For each feature, ask "how would I misuse this?" — then make that your first test.

## The Three-Tier Boundary System

### Always Do (No Exceptions)

- **Validate all external input** at the system boundary (API routes, form handlers)
- **Parameterize all database queries** — never concatenate user input into SQL
- **Encode output** to prevent XSS (use framework auto-escaping, don't bypass it)
- **Use HTTPS** for all external communication
- **Hash passwords** with bcrypt/scrypt/argon2 (never store plaintext)
- **Set security headers** (CSP, HSTS, X-Frame-Options, X-Content-Type-Options)
- **Use httpOnly, secure, sameSite cookies** for sessions
- **Run `go vet`** and check for vulnerabilities before every release

### Ask First (Requires Human Approval)

- Adding new authentication flows or changing auth logic
- Storing new categories of sensitive data (PII, payment info)
- Adding new external service integrations
- Changing CORS configuration
- Adding file upload handlers
- Modifying rate limiting or throttling
- Granting elevated permissions or roles

### Never Do

- **Never commit secrets** to version control (API keys, passwords, tokens)
- **Never log sensitive data** (passwords, tokens, full credit card numbers)
- **Never trust client-side validation** as a security boundary
- **Never disable security headers** for convenience
- **Never use `exec.Command`** with user-provided data without sanitization
- **Never store sessions in client-accessible storage** (localStorage for auth tokens)
- **Never expose stack traces** or internal error details to users

## OWASP Prevention Patterns (Go)

### Injection (SQL, NoSQL, OS Command)

```go
// BAD: SQL injection via string concatenation
query := fmt.Sprintf("SELECT * FROM users WHERE id = '%s'", userID)

// GOOD: Parameterized query
row := db.QueryRow("SELECT * FROM users WHERE id = $1", userID)

// GOOD: Using an ORM with parameterized input
user, err := db.User.FindUnique(userID)
```

### Broken Authentication

```go
// Password hashing
import "golang.org/x/crypto/bcrypt"

const hashCost = 12
hashed, err := bcrypt.GenerateFromPassword([]byte(plaintext), hashCost)
err = bcrypt.CompareHashAndPassword(hashed, []byte(plaintext))

// Session management — use secure cookie configuration
http.SetCookie(w, &http.Cookie{
    Name:     "session",
    Value:    sessionToken,
    HttpOnly: true,
    Secure:   true,
    SameSite: http.SameSiteLaxMode,
    MaxAge:   86400,
})
```

### Cross-Site Scripting (XSS)

```go
// BAD: Rendering user input as raw HTML
w.Write([]byte(userInput))

// GOOD: Use html/template auto-escaping
tmpl.Execute(w, data) // template auto-escapes by default

// GOOD: Explicit escaping
import "html"
safe := html.EscapeString(userInput)
```

### Broken Access Control

```go
// Always check authorization, not just authentication
func UpdateTaskHandler(w http.ResponseWriter, r *http.Request) {
    task, err := taskService.FindByID(r.URL.Query().Get("id"))
    if err != nil {
        http.Error(w, "Not found", http.StatusNotFound)
        return
    }

    // Check that the authenticated user owns this resource
    if task.OwnerID != auth.UserID(r) {
        http.Error(w, "Forbidden", http.StatusForbidden)
        return
    }

    // Proceed with update
}
```

### Server-Side Request Forgery (SSRF)

```go
// BAD: fetch whatever the user gives you
resp, err := http.Get(r.FormValue("url"))

// GOOD: allowlist scheme + host, reject private IPs
import "net/url"

var allowedHosts = map[string]bool{"hooks.example.com": true}

func assertSafeURL(raw string) (*url.URL, error) {
    parsed, err := url.Parse(raw)
    if err != nil {
        return nil, err
    }
    if parsed.Scheme != "https" {
        return nil, errors.New("https only")
    }
    if !allowedHosts[parsed.Hostname()] {
        return nil, errors.New("host not allowed")
    }
    // Additional: resolve DNS and check for private/reserved IPs
    return parsed, nil
}
```

## Input Validation Patterns

### Validation at Boundaries

```go
// Validate incoming data before processing
type CreateTaskRequest struct {
    Title       string `json:"title" validate:"required,min=1,max=200"`
    Description string `json:"description" validate:"max=2000"`
    Priority    string `json:"priority" validate:"required,oneof=low medium high"`
}

func CreateTaskHandler(w http.ResponseWriter, r *http.Request) {
    var req CreateTaskRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }
    if err := validate.Struct(req); err != nil {
        http.Error(w, "Validation failed", http.StatusUnprocessableEntity)
        return
    }
    // Process validated data
}
```

### File Upload Safety

```go
// Restrict file types and sizes
var allowedTypes = map[string]bool{
    "image/jpeg": true,
    "image/png":  true,
    "image/webp": true,
}
const maxSize = 5 * 1024 * 1024 // 5MB

func ValidateUpload(file io.Reader, header *multipart.FileHeader) error {
    if !allowedTypes[header.Header.Get("Content-Type")] {
        return errors.New("file type not allowed")
    }
    if header.Size > maxSize {
        return errors.New("file too large (max 5MB)")
    }
    return nil
}
```

## Triaging Dependency Vulnerabilities

```
Vulnerability reported in a dependency:
|-- Severity: critical or high
|   |-- Is the vulnerable code reachable in your app?
|   |   |-- YES --> Fix immediately (update, patch, or replace)
|   |   |-- NO (dev-only dep, unused code path) --> Fix soon, not a blocker
|   |-- Is a fix available?
|       |-- YES --> Update to the patched version
|       |-- NO --> Check for workarounds, consider replacing the dependency
|-- Severity: moderate
|   |-- Reachable in production? --> Fix in the next release cycle
|   |-- Dev-only? --> Fix when convenient
|-- Severity: low
    --> Track and fix during regular dependency updates
```

**Key questions:**
- Is the vulnerable function actually called in your code path?
- Is the dependency a runtime dependency or dev-only?
- Is the vulnerability exploitable given your deployment context?

### Supply-Chain Hygiene

- **Commit the lockfile** and install with `go mod vendor` or pin versions in CI — reproducible builds, no silent version drift.
- **Review new dependencies before adding them** — maintenance, download counts, and whether they truly earn their place.
- **Be wary of `init()` functions** in unfamiliar packages — they run arbitrary code at import time.
- **Watch for typosquats** — similar package names that inject malicious code.

## Rate Limiting

```go
import "golang.org/x/time/rate"

// General API rate limiter
var apiLimiter = rate.NewLimiter(rate.Every(time.Second/10), 20) // 10 req/s, burst 20

func RateLimitMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !apiLimiter.Allow() {
            http.Error(w, "Rate limited", http.StatusTooManyRequests)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

## Secrets Management

```
.env files:
  +-- .env.example  -> Committed (template with placeholder values)
  +-- .env          -> NOT committed (contains real secrets)
  +-- .env.local    -> NOT committed (local overrides)

.gitignore must include:
  .env
  .env.local
  .env.*.local
  *.pem
  *.key
```

**Always check before committing:**
```bash
# Check for accidentally staged secrets
git diff --cached | grep -i "password\|secret\|api_key\|token"
```

**If a secret is ever committed, rotate it.** Deleting the line or rewriting history is not enough — assume it's compromised the moment it reaches a remote.

## Securing AI / LLM Features

If your app calls an LLM, it inherits a new attack surface:

- **Treat all model output as untrusted input.** Never pass LLM output straight into `exec.Command`, a database query, or `template.HTML`.
- **Assume prompts can be hijacked.** The system prompt is not a security boundary; enforce permissions in code, not in the prompt.
- **Keep secrets and other users' data out of prompts.** Anything in the context can be echoed back.
- **Constrain tool and agent permissions.** Scope tools to the minimum, require confirmation for destructive actions.
- **Bound consumption.** Cap tokens, request rate, and loop depth.

```go
// BAD: trusting model output as a command
cmd := exec.Command("sh", "-c", llmOutput)
cmd.Run()

// GOOD: model output is data, validate before use
var intent Action
if err := json.Unmarshal([]byte(llmOutput), &intent); err != nil {
    return fmt.Errorf("unexpected model output: %w", err)
}
if err := validateAction(intent); err != nil {
    return fmt.Errorf("invalid action: %w", err)
}
runAllowlistedAction(intent)
```

## Security Review Checklist

```markdown
### Authentication
- [ ] Passwords hashed with bcrypt/scrypt/argon2 (cost >= 12)
- [ ] Session tokens are httpOnly, secure, sameSite
- [ ] Login has rate limiting
- [ ] Password reset tokens expire

### Authorization
- [ ] Every endpoint checks user permissions
- [ ] Users can only access their own resources
- [ ] Admin actions require admin role verification

### Input
- [ ] All user input validated at the boundary
- [ ] SQL queries are parameterized
- [ ] HTML output is encoded/escaped
- [ ] Server-side URL fetches are allowlisted (no SSRF)

### Data
- [ ] No secrets in code or version control
- [ ] Sensitive fields excluded from API responses
- [ ] PII encrypted at rest (if applicable)

### Infrastructure
- [ ] Security headers configured (CSP, HSTS, etc.)
- [ ] CORS restricted to known origins
- [ ] Dependencies audited for vulnerabilities
- [ ] Error messages don't expose internals

### AI / LLM (if used)
- [ ] Model output treated as untrusted (no exec/SQL/innerHTML)
- [ ] Secrets and other users' data kept out of prompts
- [ ] Tool/agent permissions scoped; destructive actions require confirmation
```

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "This is an internal tool, security doesn't matter" | Internal tools get compromised. Attackers target the weakest link. |
| "We'll add security later" | Security retrofitting is 10x harder than building it in. Add it now. |
| "No one would try to exploit this" | Automated scanners will find it. Security by obscurity is not security. |
| "The framework handles security" | Frameworks provide tools, not guarantees. You still need to use them correctly. |
| "It's just a prototype" | Prototypes become production. Security habits from day one. |
| "Threat modeling is overkill here" | Five minutes of "how would I attack this?" prevents the design flaws no control can patch later. |
| "It's just LLM output, it's only text" | That "text" can be a SQL statement, a script tag, or a shell command. Treat it like any untrusted input. |

## Red Flags

- User input passed directly to database queries, shell commands, or HTML rendering
- Secrets in source code or commit history
- API endpoints without authentication or authorization checks
- Missing CORS configuration or wildcard (`*`) origins
- No rate limiting on authentication endpoints
- Stack traces or internal errors exposed to users
- Dependencies with known critical vulnerabilities
- Server fetches user-supplied URLs without an allowlist (SSRF)
- LLM/model output passed into a query, the DOM, a shell, or `exec.Command`
- Secrets, PII, or the full system prompt placed inside an LLM context window

## Verification

After implementing security-relevant code:

- [ ] No secrets in source code or git history
- [ ] All user input validated at system boundaries
- [ ] Authentication and authorization checked on every protected endpoint
- [ ] Security headers present in response
- [ ] Error responses don't expose internal details
- [ ] Rate limiting active on auth endpoints
- [ ] Server-side URL fetches validated against an allowlist (no SSRF)
- [ ] LLM/model output validated and encoded before use (if AI features present)
