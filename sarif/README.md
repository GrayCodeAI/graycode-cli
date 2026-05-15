# sarif

A small, dependency-free SARIF 2.1.0 emitter for the hawk-eco. Used by
`sight` (code review findings) and `inspect` (web-scan findings) to produce
output compatible with GitHub Code Scanning, VS Code SARIF Viewer, and other
SARIF-consuming tools.

## Why this package exists

`sight` and `inspect` were each carrying their own ~250-line copy of the
SARIF 2.1.0 type tree and JSON marshalling. This package collapses them into
a single canonical implementation. Both repos now consume it and only need
to map their domain `Finding` types into the small `sarif.Rule` / `sarif.Result`
shape.

## API

```go
b := sarif.New(sarif.Tool{
    Name:           "mytool",
    Version:        "1.2.3",
    InformationURI: "https://github.com/example/mytool",
})

b.AddRule(sarif.Rule{
    ID:               "mytool/sql-injection",
    Name:             "sql-injection",
    ShortDescription: "Possible SQL injection sink",
    Severity:         sarif.SeverityError,
})

b.AddResult(sarif.Result{
    RuleID:   "mytool/sql-injection",
    Severity: sarif.SeverityError,
    Message:  "concatenated user input into SQL",
    URI:      "src/handlers.go",
    Region:   &sarif.Region{StartLine: 42},
    Taxa:     []sarif.TaxaRef{{ID: "CWE-89", Component: "CWE"}},
})

json, err := b.JSON()
```

The output is canonical SARIF 2.1.0 — see
<https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html>.

## Versioning

Version is read at compile time from the `VERSION` file at the repo root
(see [hawk-eco VERSIONING.md](https://github.com/GrayCodeAI/hawk/blob/main/VERSIONING.md)).

## Status

Local module — until this is published as `github.com/GrayCodeAI/sarif`,
consuming repos use a `replace` directive in their `go.mod` to point at the
local path. Once published:

```bash
# In the consuming repo
go get github.com/GrayCodeAI/sarif@latest
# Then remove the `replace github.com/GrayCodeAI/sarif => ../sarif` line.
```
