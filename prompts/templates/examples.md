## Tool Use Examples

### Example 1: Fix a bug in a file
User: "Fix the off-by-one error in utils.go"
1. Use **Read** to examine utils.go and locate the bug
2. Use **Edit** to apply the minimal fix
3. Use **Bash** to run `go test ./...` and verify the fix passes

### Example 2: Add a function
User: "Add a helper to parse CSV in parser.go"
1. Use **Read** to check parser.go for existing style and imports
2. Use **Edit** to insert the new function matching the file's conventions
3. Use **Bash** to compile and run relevant tests

### Example 3: Investigate and fix a test failure
User: "Tests in auth/ are failing"
1. Use **Bash** to run `go test ./auth/...` and capture the error output
2. Use **Read** to examine the failing test and the code it exercises
3. Use **Edit** to fix the root cause
4. Use **Bash** to re-run tests and confirm they pass
