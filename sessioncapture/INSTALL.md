# trace Integration with hawk

## How it works

When you install hawk via Homebrew, trace is automatically installed as a dependency:

```ruby
# In GrayCodeAI/homebrew-tap/Formula/hawk.rb
class Hawk < Formula
  depends_on "GrayCodeAI/tap/trace"  # ← bundled automatically
end
```

## Automatic behavior

1. **Install hawk** → trace is installed alongside it
2. **Run hawk in a git repo** → trace auto-enables (installs git hooks)
3. **Every commit** → trace captures the session silently
4. **No config needed** — zero setup for the user

## User controls (from within hawk)

```
/trace-enable      Enable session capture for this project
/trace-disable     Disable session capture for this project
/trace-status      Show current capture status
```

Or from CLI:

```bash
hawk config trace.enabled true    # enable
hawk config trace.enabled false   # disable
```

## Architecture

```
brew install hawk
  └── also installs: trace (as dependency)

hawk starts session
  └── sessioncapture.AutoSetup()
        ├── trace installed? YES
        ├── trace enabled? NO → runs "trace enable --agent hawk"
        └── done (hooks installed, recording active)

hawk makes commits
  └── .git/hooks/post-commit (installed by trace)
        └── trace captures session → stores on shadow branch

User wants to disable:
  └── /trace-disable → runs "trace disable" → hooks removed
```

## trace stays standalone

- trace has its own repo, its own binary, its own release cycle
- trace works with ANY agent (Claude Code, Cursor, Codex, hawk)
- hawk just manages trace's lifecycle as a convenience
- Users can also install/manage trace independently
