package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// connectCmd represents the connect command
var connectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Connect to a backend server",
	Long: `Initiates a connection to the specified backend server using the provided username.

This command requires both:
- A username for authentication.
- The server address where the backend is running.

Example:
  tunnelR connect --user alice --server backend.example.com`,
	Run: func(cmd *cobra.Command, _ []string) {
		user, _ := cmd.Flags().GetString("user")
		server, _ := cmd.Flags().GetString("server")
		fmt.Printf("Connecting to %s as %s...\n", server, user)
	},
}

func init() {
	RootCmd.AddCommand(connectCmd)
	connectCmd.Flags().StringP("user", "u", "", "Username for authentication")
	connectCmd.Flags().StringP("server", "s", "", "Address or hostname of the backend server")
	if err := connectCmd.MarkFlagRequired("user"); err != nil {
		fmt.Println(err)
	}
	if err := connectCmd.MarkFlagRequired("server"); err != nil {
		fmt.Println(err)
	}
}
