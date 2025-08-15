/*
Copyright © 2025 CODAXA
*/

// Package cli to handle command line instructions
package cli

import (
	"github.com/spf13/cobra"
	"os"
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "tunnelR",
	Short: "Securely manage temporary SSH access to Linux servers with Zero Trust controls.",
	Long: `tunnelR is a cross-platform CLI for creating, managing, and revoking time-limited SSH access to Linux servers.

Designed with Zero Trust and Just-in-Time security principles, tunnelR ensures users get the minimum required access for the shortest necessary time.

Key capabilities:
- Grant and revoke SSH access in seconds via the CLI.
- Automatically create and remove temporary Linux user accounts.
- Assign access based on teams, roles, and policies through a centralized backend API.
- Audit and log all SSH events for compliance and troubleshooting.
- Secure backend communication via HTTPS with PostgreSQL storage.

Ideal for DevOps teams, cloud infrastructure admins, and organizations adopting JIT and Zero Trust access models.`,

	Run: func(_ *cobra.Command, _ []string) {},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := RootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	RootCmd.CompletionOptions.DisableDefaultCmd = true
	var customUsageTemplate = `Usage:
  {{.UseLine}}{{if .HasAvailableSubCommands}}

Available Commands:
{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}  {{rpad .Name .NamePadding }} {{.Short}}
{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:
{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}
{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

	RootCmd.SetUsageTemplate(customUsageTemplate)
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.tunnelR.git.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	// RootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
