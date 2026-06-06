# World's Best CLI - Improvement Plan

## Current State Analysis

Hawk is already a sophisticated AI coding agent with:
- 200+ built-in tools
- Bubble Tea TUI with slash commands
- Multi-provider support via eyrie
- Session persistence & recovery
- Plugin system
- Shell completions (bash/zsh/fish/powershell/json)
- Man page generation
- Container sandboxing

## Competitive Analysis: Top OSS AI Coding Agents (2024-2025)

| Agent | Stars | Language | Key Strengths | CLI/UX Patterns |
|-------|-------|----------|---------------|-----------------|
| **Aider** | 45.8k | Python | Git-native, multi-model, voice, IDE integration, /help with rich commands | REPL-first, slash commands, verbose flags, pipe-friendly |
| **OpenHands** | 75.9k | TypeScript/Go | Web UI + headless, agent skills, runtime containers, eval harness | Web-first, headless API, skill marketplace |
| **Continue** | 33.6k | TypeScript | IDE-native (VSCode/JetBrains), custom models, checks/CI | IDE sidebar, slash commands, config-driven |
| **SWE-agent** | 19.4k | Python | Issue-to-fix automation, NeurIPS 2024, cybersecurity focus | Config YAML, single-command runs, docker-native |
| **Open Interpreter** | 63.8k | Python | OS-level control, local execution, vision, voice | REPL, `interpreter -c`, multi-language |

### Common World-Class Patterns Identified:
1. **REPL mode** — `aider`, `open-interpreter` default to REPL, TUI optional
2. **Pipe/JSONL streaming** — All support structured output for scripting
3. **Config-driven** — YAML/TOML configs with full schema + env override
4. **Skill/Plugin marketplace** — OpenHands skills, Continue checks, Aider conventions
5. **Multi-model routing** — Aider's model aliases, Continue's custom models
6. **Voice/TTS integration** — Aider voice-to-code, Open Interpreter voice
7. **Watch/daemon modes** — File-watch auto-re-run, background servers
8. **Git-native workflow** — Aider auto-commits, diff preview, branch mgmt
9. **Rich help system** — Interactive `/help`, command palette (Cmd+K)
10. **Onboarding wizard** — First-run setup, API key config, model picker

## Gaps to World-Class

### Phase 1: Performance & Startup Optimization
- [ ] Sub-100ms cold start (lazy-load heavy deps) ✓ **started**
- [ ] Background catalog/credential warming
- [ ] Binary size optimization (`-ldflags="-s -w"`, trim debug)
- [ ] Startup profiling ✓ **implemented `--startup-profile`**

### Phase 2: Enhanced Discoverability & UX
- [ ] **Interactive onboarding wizard** (`hawk init` — API keys, model picker, sandbox)
- [ ] **Context-aware slash command suggestions** (fuzzy, frequent, project-aware)
- [ ] **Built-in tutorial mode** (`hawk --tutorial` interactive walkthrough)
- [ ] **Better error messages** with actionable fixes (did-you-mean, doc links)
- [ ] **Command palette** (Cmd/Ctrl+K) — fuzzy search all commands/tools/skills
- [ ] **Rich `/help`** with categories, examples, search (like Aider)

### Phase 3: Advanced Shell Integration (HIGHEST IMPACT)
- [ ] **REPL mode** (`hawk -p "prompt"` without TUI, streaming JSONL) — like Aider
- [ ] **Pipe-friendly JSONL streaming** (`hawk -p "fix" --json | jq ...`)
- [ ] **Watch mode** (`hawk watch "fix tests"` auto-re-runs on file changes)
- [ ] **Shell widget/integration** (fzf-style inline picker, atuin-style history sync)
- [ ] **Alias system** for common workflows (`hawk alias fix-tests="test ./..."`)
- [ ] **Daemon/API server** (`hawk serve` — REST/SSE for IDE integrations)

### Phase 4: Extensibility & API
- [ ] **Lua/JS/WASM plugin runtime** (sandboxed, hot-reload)
- [ ] **Tool protocol** for external tools (stdio/HTTP — like MCP but simpler)
- [ ] **Hooks system** (pre/post tool, session events, config change)
- [ ] **Configuration schema** with validation (JSON Schema → Go types)
- [ ] **Skill marketplace** (install from GitHub/registry, versioned)

### Phase 5: Reliability & Data Integrity
- [ ] **CRDT-based session sync** for multi-device
- [ ] **Undo/redo** at session level (tool call granularity)
- [ ] **Automatic backup** before destructive ops (git commit, file snapshot)
- [ ] **Crash recovery** with WAL ✓ **started**

### Phase 6: Accessibility & Polish
- [ ] **Screen reader support** (ARIA in TUI, semantic markup)
- [ ] **High contrast themes** (WCAG AA)
- [ ] **Reduced motion** option
- [ ] **i18n framework** (gettext-style, locale files)

## Priority Order (Updated from Competitive Analysis)
1. **Phase 3: REPL + Watch + JSONL** — highest user impact, matches Aider/OpenInterpreter default UX
2. **Phase 1: Performance** — foundation, sub-100ms target
3. **Phase 2: Onboarding + Help + Command Palette** — critical for adoption
4. **Phase 4: Plugin Runtime + Tool Protocol** — ecosystem growth
5. **Phase 5: Reliability** — trust, production readiness
6. **Phase 6: Accessibility** — inclusion

## Implementation Notes
- **REPL mode** can reuse `runPrint()` with streaming JSONL output
- **Watch mode** uses `fsnotify` + debounce + session resume
- **Command palette** extends existing `CommandPalette` in chat_model.go
- **Plugin runtime** — evaluate `wazero` (WASM) or `go-lua`/`gopher-lua`
- **Config schema** — `go-jsonschema` + codegen for type-safe Settings