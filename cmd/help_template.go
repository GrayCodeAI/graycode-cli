package cmd

import (
	"github.com/spf13/cobra"
)

// Modern, theme-aware help output. Section headers render in the brand gold,
// command names in textPrimary, descriptions/flags stay plain so fixed-width
// columns keep their alignment. All color honors ShouldColor() (NO_COLOR,
// --quiet, non-TTY) via auditTint. Pad-then-colorize keeps columns aligned.
func init() {
	cobra.AddTemplateFunc("gcHeader", func(s string) string { return auditTint(s, graycodeColor) })
	cobra.AddTemplateFunc("gcCmd", func(s string) string { return auditTint(s, textPrimary) })
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
  {{gcCmd (rpad .Name .NamePadding)}} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{gcHeader .Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{gcCmd (rpad .Name .NamePadding)}} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

{{gcHeader "Additional Commands:"}}{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{gcCmd (rpad .Name .NamePadding)}} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{gcHeader "Flags:"}}
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

{{gcHeader "Global Flags:"}}
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

{{gcHeader "Additional help topics:"}}{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
