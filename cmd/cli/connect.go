package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// connectCmd represents the connect command
var connectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Connect to a remote machine",
	Long: `Establishes a connection to a specified machine using the provided user credentials.
You must provide both the username and machine IP address.`,
	Run: func(cmd *cobra.Command, _ []string) {
		user, _ := cmd.Flags().GetString("user")
		machine, _ := cmd.Flags().GetString("machine")
		fmt.Printf("Connecting to %s as %s...\n", machine, user)
	},
}

func init() {
	RootCmd.AddCommand(connectCmd)
	connectCmd.Flags().StringP("user", "u", "", "Username for authentication")
	connectCmd.Flags().StringP("machine", "m", "", "IP address or hostname of the target machine")
	if err := connectCmd.MarkFlagRequired("user"); err != nil {
		fmt.Println(err)
	}
	if err := connectCmd.MarkFlagRequired("machine"); err != nil {
		fmt.Println(err)
	}
}
