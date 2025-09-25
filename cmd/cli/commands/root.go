// Package commands to handle command line instructions
package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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

func trimFlagTypes(flags *pflag.FlagSet) string {
	var sb strings.Builder
	flags.VisitAll(func(f *pflag.Flag) {
		sb.WriteString(fmt.Sprintf("  -%s, --%s\t%s\n", f.Shorthand, f.Name, f.Usage))
	})
	// Remove the trailing newline to prevent extra space
	result := sb.String()
	if len(result) > 0 && result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}
	return result
}

func init() {
	RootCmd.CompletionOptions.DisableDefaultCmd = true

	usageTemplate := `Usage:
  {{.UseLine}}{{if .HasAvailableSubCommands}}

Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{ trimFlagTypes .LocalFlags }}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{ trimFlagTypes .InheritedFlags }}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

	// Register the template function and set the template
	funcMap := map[string]interface{}{
		"trimFlagTypes": trimFlagTypes,
	}
	cobra.AddTemplateFuncs(funcMap)
	RootCmd.SetUsageTemplate(usageTemplate)
}
