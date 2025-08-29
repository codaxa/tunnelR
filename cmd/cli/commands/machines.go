package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/codaxa/tunnelR.git/cmd/cli/shared"
	"github.com/codaxa/tunnelR.git/cmd/cli/utils"
	"github.com/spf13/cobra"
)

// machinesCmd represents the machines command
var machinesCmd = &cobra.Command{
	Use:   "machines",
	Short: "Manage machines in the system",
	Long: `Comprehensive machine management - add, list, update, delete, and associate machines with teams. 
Machines represent physical or virtual servers that can be accessed through the system.
Use subcommands to perform specific operations on machines.`,
}

// machinesAddCmd represents the add command
var machinesAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new machine to the system",
	Long: `Register a new machine in the system with authentication credentials.
Required flags:
  --hostname, -n    Hostname or descriptive name for the machine
  --ip, -i          IP address for connecting to the machine

Authentication (at least one required):
  --password, -p    SSH password for password-based authentication
  --key-file, -k    Path to SSH private key file for key-based authentication

The system supports three authentication methods:
- Password authentication (provide --password)
- Key-based authentication (provide --key-file)
- Both methods combined (provide both flags)`,

	Run: func(cmd *cobra.Command, _ []string) {
		// Get flags
		hostname, _ := cmd.Flags().GetString("hostname")
		ip, _ := cmd.Flags().GetString("ip")
		keyFile, _ := cmd.Flags().GetString("key-file")
		password, _ := cmd.Flags().GetString("password")

		// Ensure at least one authentication method is provided
		if keyFile == "" && password == "" {
			fmt.Println("Error: You must provide either a key file or password")
			os.Exit(1)
		}

		// Prepare request payload
		payload := map[string]string{
			"hostname":   hostname,
			"ip_address": ip,
		}

		// Set auth method and add authentication data to payload
		if password != "" && keyFile != "" {
			payload["auth_method"] = "both"
			payload["password"] = password

			// Read key file content
			keyData, err := os.ReadFile(keyFile)
			if err != nil {
				fmt.Printf("Error reading key file: %s\n", err)
				os.Exit(1)
			}
			payload["key"] = string(keyData)

		} else if password != "" {
			payload["auth_method"] = "password"
			payload["password"] = password
		} else if keyFile != "" {
			payload["auth_method"] = "key"
			// Read key file content
			keyData, err := os.ReadFile(keyFile)
			if err != nil {
				fmt.Printf("Error reading key file: %s\n", err)
				os.Exit(1)
			}
			payload["key"] = string(keyData)
		}

		// Make the request
		resp, err := utils.MakeAuthenticatedRequest("POST", "/api/v1/machines", payload)
		if err != nil {
			if strings.Contains(err.Error(), "no authentication token found") {
				fmt.Println("Please log in first using the login command")
			} else {
				fmt.Println("Error:", err)
			}
			os.Exit(1)
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Printf("Error closing response body: %v", err)
			}
		}()

		// Handle response
		switch resp.StatusCode {
		case http.StatusCreated:
			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				fmt.Println("Error parsing response:", err)
				os.Exit(1)
			}
			fmt.Printf("Machine added successfully. ID: %s\n", result["machine_id"])
		case http.StatusUnauthorized:
			fmt.Println("Unauthorized. Please log in again.")
			os.Exit(1)
		case http.StatusForbidden:
			fmt.Println("You don't have permission to add machines.")
			os.Exit(1)
		default:
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Error: Received status code %d. Response: %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}
	},
}

// machinesListCmd represents the list command
var machinesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all accessible machines",
	Long: `Display a comprehensive list of all machines you have access to in the system.
The output includes each machine's:
- Unique identifier (ID)
- Hostname
- IP address
- Creation timestamp

Results are formatted in a tabular layout for easy reading.
This command requires authentication and will only show machines you have permission to view.`,
	Run: func(_ *cobra.Command, _ []string) {
		// Make the request
		resp, err := utils.MakeAuthenticatedRequest("GET", "/api/v1/user/machines", nil)
		if err != nil {
			if strings.Contains(err.Error(), "no authentication token found") {
				fmt.Println("Please log in first using the login command")
			} else {
				fmt.Println("Error:", err)
			}
			os.Exit(1)
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Printf("Error closing response body: %v", err)
			}
		}()

		// Handle response
		switch resp.StatusCode {
		case http.StatusOK:
			var machines []shared.Machine
			if err := json.NewDecoder(resp.Body).Decode(&machines); err != nil {
				fmt.Println("Error parsing response:", err)
				os.Exit(1)
			}
			utils.PrintMachinesTable(machines)
		case http.StatusUnauthorized:
			fmt.Println("Unauthorized. Please log in again.")
			os.Exit(1)
		default:
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Error: Received status code %d. Response: %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}
	},
}

// machineRemoveFunction is a shared function to remove/delete a machine
// This eliminates code duplication between rm and delete commands
func machineRemoveFunction(_ *cobra.Command, args []string) {
	// Get machine ID from args
	machineID := args[0]

	// Make the request
	resp, err := utils.MakeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/machines/%s", url.PathEscape(machineID)), nil)
	if err != nil {
		if strings.Contains(err.Error(), "no authentication token found") {
			fmt.Println("Please log in first using the login command")
		} else {
			fmt.Println("Error:", err)
		}
		return // Already changed from os.Exit(1)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	// Handle response
	switch resp.StatusCode {
	case http.StatusNoContent:
		fmt.Println("Machine removed successfully.")
	case http.StatusUnauthorized:
		fmt.Println("Unauthorized. Please log in again.")
		return // Changed from os.Exit(1)
	case http.StatusForbidden:
		fmt.Println("You don't have permission to remove machines.")
		return // Changed from os.Exit(1)
	case http.StatusNotFound:
		fmt.Println("Machine not found.")
		return // Changed from os.Exit(1)
	default:
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Error: Received status code %d. Response: %s\n", resp.StatusCode, string(body))
		return // Changed from os.Exit(1)
	}
}

// machinesRmCmd represents the rm command
var machinesRmCmd = &cobra.Command{
	Use:     "rm [machine-id]",
	Aliases: []string{"delete"}, // Add delete as an alias
	Short:   "Remove a machine from the system",
	Long: `Permanently remove a machine from the system by its unique identifier.
This operation cannot be undone and will remove all associations with teams.

Arguments:
  machine-id    The unique identifier of the machine to remove

This command requires administrative privileges or ownership of the machine.
Use with caution as all access configurations for this machine will be deleted.

Aliases:
  rm, delete    Both names perform the same operation.`,
	Args: cobra.ExactArgs(1), // Require exactly one argument (the machine ID)
	Run:  machineRemoveFunction,
}

// machinesUpdateCmd represents the update command
var machinesUpdateCmd = &cobra.Command{
	Use:   "update [machine-id]",
	Short: "Update machine properties",
	Long: `Modify properties of an existing machine in the system.
You can update hostname, IP address, and authentication methods.

Arguments:
  machine-id    The unique identifier of the machine to update

Available flags:
  --hostname, -n       New hostname for the machine
  --ip, -i             New IP address for the machine
  --auth-method, -a    Authentication method (password, key, or both)
  --password, -p       New SSH password for the machine
  --key-file, -k       Path to new SSH private key file

At least one update parameter must be provided. Authentication credentials
will only be updated if explicitly specified.`,
	Args: cobra.ExactArgs(1), // Require exactly one argument (the machine ID)
	Run: func(cmd *cobra.Command, args []string) {
		// Get machine ID from args
		machineID := args[0]

		// Get flags
		hostname, _ := cmd.Flags().GetString("hostname")
		ip, _ := cmd.Flags().GetString("ip")
		keyFile, _ := cmd.Flags().GetString("key-file")
		password, _ := cmd.Flags().GetString("password")
		authMethod, _ := cmd.Flags().GetString("auth-method")

		// Prepare request payload
		payload := map[string]string{
			"id": machineID,
		}

		// Add other fields if they were provided
		if hostname != "" {
			payload["hostname"] = hostname
		}
		if ip != "" {
			payload["ip"] = ip
		}

		// Set auth_method based on explicit flag or infer from credentials
		if authMethod != "" {
			payload["auth_method"] = authMethod
		} else if password != "" && keyFile != "" {
			payload["auth_method"] = "both"
		} else if password != "" {
			payload["auth_method"] = "password"
		} else if keyFile != "" {
			payload["auth_method"] = "key"
		}

		if password != "" {
			payload["password"] = password
		}

		if keyFile != "" {
			// Read key file content
			keyData, err := os.ReadFile(keyFile)
			if err != nil {
				fmt.Printf("Error reading key file: %s\n", err)
				os.Exit(1)
			}
			payload["key"] = string(keyData)
		}

		// Check if there's anything to update besides the ID
		if len(payload) <= 1 {
			fmt.Println("No update parameters provided. Use --hostname, --ip, --auth-method, --password, or --key-file flags.")
			os.Exit(1)
		}

		// Make the request
		resp, err := utils.MakeAuthenticatedRequest("PUT", "/api/v1/machines", payload)
		if err != nil {
			if strings.Contains(err.Error(), "no authentication token found") {
				fmt.Println("Please log in first using the login command")
			} else {
				fmt.Println("Error:", err)
			}
			os.Exit(1)
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Printf("Error closing response body: %v", err)
			}
		}()

		// Handle response
		switch resp.StatusCode {
		case http.StatusOK:
			fmt.Println("Machine updated successfully.")
		case http.StatusUnauthorized:
			fmt.Println("Unauthorized. Please log in again.")
			os.Exit(1)
		case http.StatusForbidden:
			fmt.Println("You don't have permission to update machines.")
			os.Exit(1)
		case http.StatusNotFound:
			fmt.Println("Machine not found.")
			os.Exit(1)
		default:
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Error: Received status code %d. Response: %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}
	},
}

// machinesAddToTeamCmd represents the command to add a machine to a team
var machinesAddToTeamCmd = &cobra.Command{
	Use:   "add-to-team [machine-id]",
	Short: "Associate a machine with a team",
	Long: `Add an existing machine to a team to grant team members access to the machine.

Arguments:
  machine-id    The unique identifier of the machine to add to a team

Required flags:
  --team-id, -t    ID of the team to add the machine to

This operation requires appropriate permissions for both the machine and the team.
Team members will gain access according to team permission policies.`,
	Args: cobra.ExactArgs(1), // Require exactly one argument (the machine ID)
	Run: func(cmd *cobra.Command, args []string) {
		// Get machine ID from args
		machineID := args[0]

		// Get team ID from flag
		teamID, _ := cmd.Flags().GetString("team-id")

		// Check if team ID was provided
		if teamID == "" {
			fmt.Println("Error: team-id is required")
			os.Exit(1)
		}

		// Prepare request payload
		payload := map[string]string{
			"machine_id": machineID,
			"team_id":    teamID,
		}

		// Make the request
		resp, err := utils.MakeAuthenticatedRequest("POST", "/api/v1/machines/teams", payload)
		if err != nil {
			if strings.Contains(err.Error(), "no authentication token found") {
				fmt.Println("Please log in first using the login command")
			} else {
				fmt.Println("Error:", err)
			}
			os.Exit(1)
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Printf("Error closing response body: %v", err)
			}
		}()

		// Handle response
		switch resp.StatusCode {
		case http.StatusNoContent:
			fmt.Println("Machine successfully added to team.")
		case http.StatusUnauthorized:
			fmt.Println("Unauthorized. Please log in again.")
			os.Exit(1)
		case http.StatusForbidden:
			fmt.Println("You don't have permission to add machines to teams.")
			os.Exit(1)
		case http.StatusNotFound:
			fmt.Println("Machine or team not found.")
			os.Exit(1)
		default:
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Error: Received status code %d. Response: %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}
	},
}

// machinesRemoveFromTeamCmd represents the command to remove a machine from a team
var machinesRemoveFromTeamCmd = &cobra.Command{
	Use:   "remove-from-team [machine-id]",
	Short: "Disassociate a machine from a team",
	Long: `Remove a machine from a team, revoking access for team members.

Arguments:
  machine-id    The unique identifier of the machine to remove from a team

Required flags:
  --team-id, -t    ID of the team to remove the machine from

This operation requires administrative privileges for the team.
Team members will immediately lose access to the machine unless they
have access through other teams or direct permissions.`,
	Args: cobra.ExactArgs(1), // Require exactly one argument (the machine ID)
	Run: func(cmd *cobra.Command, args []string) {
		// Get machine ID from args
		machineID := args[0]

		// Get team ID from flag
		teamID, _ := cmd.Flags().GetString("team-id")

		// Check if team ID was provided
		if teamID == "" {
			fmt.Println("Error: team-id is required")
			os.Exit(1)
		}

		// Prepare request payload
		payload := map[string]string{
			"machine_id": machineID,
			"team_id":    teamID,
		}

		// Make the request
		resp, err := utils.MakeAuthenticatedRequest("DELETE", "/api/v1/machines/teams", payload)
		if err != nil {
			if strings.Contains(err.Error(), "no authentication token found") {
				fmt.Println("Please log in first using the login command")
			} else {
				fmt.Println("Error:", err)
			}
			os.Exit(1)
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Printf("Error closing response body: %v", err)
			}
		}()

		// Handle response
		switch resp.StatusCode {
		case http.StatusOK, http.StatusNoContent:
			fmt.Println("Machine successfully removed from team.")
		case http.StatusUnauthorized:
			fmt.Println("Unauthorized. Please log in again.")
			os.Exit(1)
		case http.StatusForbidden:
			fmt.Println("You don't have permission to remove machines from teams.")
			os.Exit(1)
		case http.StatusNotFound:
			fmt.Println("Machine or team not found, or machine is not in the specified team.")
			os.Exit(1)
		default:
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Error: Received status code %d. Response: %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}
	},
}

// displayMachineDetails is a shared function to display machine information
// This eliminates code duplication between get and get-by-ip commands
func displayMachineDetails(resp *http.Response, resourceName string) {
	switch resp.StatusCode {
	case http.StatusOK:
		var machine struct {
			ID         string    `json:"id"`
			Hostname   string    `json:"hostname"`
			IP         string    `json:"ip_address"`
			AuthMethod string    `json:"auth_method"`
			CreatedAt  time.Time `json:"created_at"`
			UpdatedAt  time.Time `json:"updated_at"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&machine); err != nil {
			fmt.Println("Error parsing response:", err)
			return
		}

		// Display machine details
		fmt.Println("\n┌─────────────────── MACHINE DETAILS ───────────────────┐")
		fmt.Printf("  ID:            %s\n", machine.ID)
		fmt.Printf("  Hostname:      %s\n", machine.Hostname)
		fmt.Printf("  IP Address:    %s\n", machine.IP)
		fmt.Printf("  Auth Method:   %s\n", machine.AuthMethod)
		fmt.Printf("  Created At:    %s\n", machine.CreatedAt.Format("Jan 02, 2006 15:04:05"))
		fmt.Printf("  Updated At:    %s\n", machine.UpdatedAt.Format("Jan 02, 2006 15:04:05"))
		fmt.Println("└──────────────────────────────────────────────────────────┘")
	case http.StatusUnauthorized:
		fmt.Println("Unauthorized. Please log in again.")
	case http.StatusForbidden:
		fmt.Println("You don't have permission to view this machine.")
	case http.StatusNotFound:
		fmt.Printf("No machine found with the specified %s.\n", resourceName)
	default:
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Error: Received status code %d. Response: %s\n", resp.StatusCode, string(body))
	}
}

// machinesGetCmd represents the get command
var machinesGetCmd = &cobra.Command{
	Use:   "get [machine-id]",
	Short: "Get detailed machine information",
	Long: `Retrieve comprehensive information about a specific machine by its ID.

Arguments:
  machine-id    The unique identifier of the machine to retrieve

The command displays detailed information including:
- Machine ID and hostname
- IP address configuration
- Authentication method in use
- Creation and last update timestamps

This provides more detailed information than the 'list' command
for a single machine record.`,
	Args: cobra.ExactArgs(1), // Require exactly one argument (the machine ID)
	Run: func(_ *cobra.Command, args []string) {
		// Get machine ID from args
		machineID := args[0]

		// Make the request
		resp, err := utils.MakeAuthenticatedRequest("GET", fmt.Sprintf("/api/v1/machines/id/%s", url.PathEscape(machineID)), nil)
		if err != nil {
			if strings.Contains(err.Error(), "no authentication token found") {
				fmt.Println("Please log in first using the login command")
			} else {
				fmt.Println("Error:", err)
			}
			return
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Printf("Error closing response body: %v", err)
			}
		}()

		displayMachineDetails(resp, "ID")
	},
}

// machinesGetByIPCmd represents the command to get a machine by IP address
var machinesGetByIPCmd = &cobra.Command{
	Use:   "get-by-ip [ip-address]",
	Short: "Find machine by IP address",
	Long: `Locate and display machine details using its IP address instead of ID.

Arguments:
  ip-address    The IP address of the machine to find

This command is useful when you know a machine's IP address but not its ID.
It returns the same detailed information as the 'get' command if a match is found.
If multiple machines share the same IP (unusual), only the first match is returned.`,
	Args: cobra.ExactArgs(1), // Require exactly one argument (the IP address)
	Run: func(_ *cobra.Command, args []string) {
		// Get IP address from args
		ipAddress := args[0]

		// Make the request
		resp, err := utils.MakeAuthenticatedRequest("GET", fmt.Sprintf("/api/v1/machines/ip/%s", url.PathEscape(ipAddress)), nil)
		if err != nil {
			if strings.Contains(err.Error(), "no authentication token found") {
				fmt.Println("Please log in first using the login command")
			} else {
				fmt.Println("Error:", err)
			}
			return
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Printf("Error closing response body: %v", err)
			}
		}()

		displayMachineDetails(resp, "IP address")
	},
}

func init() {
	RootCmd.AddCommand(machinesCmd)

	// Add subcommands
	machinesCmd.AddCommand(machinesAddCmd)
	machinesCmd.AddCommand(machinesListCmd)
	machinesCmd.AddCommand(machinesRmCmd)
	machinesCmd.AddCommand(machinesUpdateCmd)
	machinesCmd.AddCommand(machinesAddToTeamCmd)
	machinesCmd.AddCommand(machinesRemoveFromTeamCmd)
	machinesCmd.AddCommand(machinesGetCmd)     // Add the new get command
	machinesCmd.AddCommand(machinesGetByIPCmd) // Add the new get-by-ip command

	// Add flags to the add command
	machinesAddCmd.Flags().StringP("hostname", "n", "", "Hostname of the machine")
	machinesAddCmd.Flags().StringP("ip", "i", "", "IP address of the machine")
	machinesAddCmd.Flags().StringP("password", "p", "", "SSH password for the machine")
	machinesAddCmd.Flags().StringP("key-file", "k", "", "Path to SSH private key file")

	// Mark required flags
	if err := machinesAddCmd.MarkFlagRequired("hostname"); err != nil {
		fmt.Println(err)
	}
	if err := machinesAddCmd.MarkFlagRequired("ip"); err != nil {
		fmt.Println(err)
	}

	// Add flags to the update command
	machinesUpdateCmd.Flags().StringP("hostname", "n", "", "New hostname for the machine")
	machinesUpdateCmd.Flags().StringP("ip", "i", "", "New IP address for the machine")
	machinesUpdateCmd.Flags().StringP("password", "p", "", "SSH password for the machine")
	machinesUpdateCmd.Flags().StringP("key-file", "k", "", "Path to SSH private key file")
	machinesUpdateCmd.Flags().StringP("auth-method", "a", "", "Authentication method (password, key, or both)")

	// Add flags to the add-to-team command
	machinesAddToTeamCmd.Flags().StringP("team-id", "t", "", "ID of the team to add the machine to")

	if err := machinesAddToTeamCmd.MarkFlagRequired("team-id"); err != nil {
		fmt.Println(err)
	}

	// Add flags to the remove-from-team command
	machinesRemoveFromTeamCmd.Flags().StringP("team-id", "t", "", "ID of the team to remove the machine from")
	if err := machinesRemoveFromTeamCmd.MarkFlagRequired("team-id"); err != nil {
		fmt.Println(err)
	}
}
