package config

import "strings"

// ZshCompletion returns a zsh completion script for hawk.
func ZshCompletion() string {
	return `#compdef hawk

_hawk() {
  local -a commands
  commands=(
    'chat:Start interactive chat (default)'
    'config:View or modify configuration'
    'doctor:Run diagnostics'
    'mcp:Manage MCP servers'
    'sessions:List saved sessions'
    'tools:List available tools'
    'version:Show version'
  )

  local -a flags
  flags=(
    '-p[Print mode - non-interactive]:prompt'
    '--print[Print mode - non-interactive]:prompt'
    '--model[Model to use]:model'
    '--provider[Provider to use]:provider'
    '--system-prompt[System prompt]:prompt'
    '--max-turns[Max conversation turns]:number'
    '--max-budget-usd[Max cost budget]:amount'
    '--continue[Resume last session]'
    '--session-id[Resume specific session]:session_id'
    '--output-format[Output format (text/json/stream-json)]:format:(text json stream-json)'
    '--permission-mode[Permission mode]:mode:(default acceptEdits bypassPermissions plan)'
    '--tools[Comma-separated tool list]:tools'
    '--add-dir[Additional allowed directory]:directory:_directories'
    '--verbose[Enable verbose logging]'
    '--no-color[Disable colors]'
    '--help[Show help]'
    '--version[Show version]'
  )

  _arguments -s $flags

  if (( CURRENT == 2 )); then
    _describe 'command' commands
  fi
}

_hawk "$@"
`
}

// BashCompletion returns a bash completion script for hawk.
func BashCompletion() string {
	return `_hawk_completions() {
  local cur prev opts
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"

  opts="chat config doctor mcp sessions tools version -p --print --model --provider --system-prompt --max-turns --continue --session-id --output-format --permission-mode --tools --add-dir --verbose --no-color --help --version"

  case "$prev" in
    --model)
      COMPREPLY=($(compgen -W "claude-sonnet-4-20250514 claude-opus-4-20250514 gpt-4o gemini-2.5-flash" -- "$cur"))
      return 0
      ;;
    --provider)
      COMPREPLY=($(compgen -W "anthropic openai gemini openrouter groq deepseek mistral ollama" -- "$cur"))
      return 0
      ;;
    --output-format)
      COMPREPLY=($(compgen -W "text json stream-json" -- "$cur"))
      return 0
      ;;
    --permission-mode)
      COMPREPLY=($(compgen -W "default acceptEdits bypassPermissions plan" -- "$cur"))
      return 0
      ;;
    --add-dir)
      COMPREPLY=($(compgen -d -- "$cur"))
      return 0
      ;;
  esac

  COMPREPLY=($(compgen -W "$opts" -- "$cur"))
}

complete -F _hawk_completions hawk
`
}

// FishCompletion returns a fish completion script for hawk.
func FishCompletion() string {
	return `complete -c hawk -f

# Commands
complete -c hawk -n '__fish_use_subcommand' -a chat -d 'Start interactive chat'
complete -c hawk -n '__fish_use_subcommand' -a config -d 'View or modify configuration'
complete -c hawk -n '__fish_use_subcommand' -a doctor -d 'Run diagnostics'
complete -c hawk -n '__fish_use_subcommand' -a mcp -d 'Manage MCP servers'
complete -c hawk -n '__fish_use_subcommand' -a sessions -d 'List saved sessions'
complete -c hawk -n '__fish_use_subcommand' -a tools -d 'List available tools'
complete -c hawk -n '__fish_use_subcommand' -a version -d 'Show version'

# Flags
complete -c hawk -s p -l print -d 'Print mode (non-interactive)' -x
complete -c hawk -l model -d 'Model to use' -x -a 'claude-sonnet-4-20250514 claude-opus-4-20250514 gpt-4o gemini-2.5-flash'
complete -c hawk -l provider -d 'Provider' -x -a 'anthropic openai gemini openrouter groq deepseek mistral ollama'
complete -c hawk -l continue -d 'Resume last session'
complete -c hawk -l output-format -d 'Output format' -x -a 'text json stream-json'
complete -c hawk -l permission-mode -d 'Permission mode' -x -a 'default acceptEdits bypassPermissions plan'
complete -c hawk -l verbose -d 'Enable verbose logging'
complete -c hawk -l no-color -d 'Disable colors'
`
}

// InstallCompletions returns instructions for installing shell completions.
func InstallCompletions(shell string) string {
	switch strings.ToLower(shell) {
	case "zsh":
		return `# Add to ~/.zshrc:
eval "$(hawk completions zsh)"

# Or save to a file:
hawk completions zsh > ~/.zsh/completions/_hawk`
	case "bash":
		return `# Add to ~/.bashrc:
eval "$(hawk completions bash)"

# Or save to a file:
hawk completions bash > /etc/bash_completion.d/hawk`
	case "fish":
		return `# Save to fish completions dir:
hawk completions fish > ~/.config/fish/completions/hawk.fish`
	default:
		return "Supported shells: zsh, bash, fish"
	}
}
