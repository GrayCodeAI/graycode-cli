package cmd

import (
	"github.com/spf13/cobra"
)

// Modern, theme-aware help output. Section headers render in the brand gold,
// command names in textPrimary, command descriptions in muted. Flags stay
// plain. Pad-then-colorize keeps the name column aligned; descriptions are the
// last column so coloring them (zero-width ANSI) cannot break alignment. All
// color honors ShouldColor() (NO_COLOR, --quiet, non-TTY) via auditTint.
func init() {
	cobra.AddTemplateFunc("gcHeader", func(s string) string { return auditTint(s, hawkColor) })
	cobra.AddTemplateFunc("gcCmd", func(s string) string { return auditTint(s, textPrimary) })
	cobra.AddTemplateFunc("gcDesc", func(s string) string { return auditTint(s, textMuted) })
	rootCmd.SetUsageTemplate(modernUsageTemplate)
}

const modernUsageTemplate = `{{gcHeader "Usage:"}}{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

{{gcHeader "Aliases:"}}
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

{{gcHeader "Examples:"}}
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

{{gcHeader "Available Commands:"}}{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{gcCmd (rpad .Name .NamePadding)}} {{gcDesc .Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{gcHeader .Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{gcCmd (rpad .Name .NamePadding)}} {{gcDesc .Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

{{gcHeader "Additional Commands:"}}{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{gcCmd (rpad .Name .NamePadding)}} {{gcDesc .Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{gcHeader "Flags:"}}
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

{{gcHeader "Global Flags:"}}
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

{{gcHeader "Additional help topics:"}}{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
