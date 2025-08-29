package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/codaxa/tunnelR.git/cmd/cli/shared"
	"github.com/codaxa/tunnelR.git/cmd/cli/utils"
	"github.com/spf13/cobra"
)

var userID string

// teamCmd represents the whoami command
var teamCmd = &cobra.Command{
	Use:   "team",
	Short: "Manage teams in the system",
	Long: `A command-line tool to manage teams, including listing, creating, deleting them, linking, unlinking users to them and display all machines linked to a team.

This command uses the saved authentication token from previous login.`,
}

// teamCmd represents the whoami command
var teamAddCmd = &cobra.Command{
	Use:   "add [team-name]",
	Short: "Add a new team to the system",
	Long: `Register a new team in the system with authentication credentials.

Examples:
  # Create a new team with the name "dev-ops"  (admins only)
  tunnelR team add dev-ops

This command uses the saved authentication token from previous login.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		if name == "" {
			fmt.Println("Error: You must provide team name")
			os.Exit(1)
		}

		payload := map[string]string{
			"name": name,
		}

		resp, err := utils.MakeAuthenticatedRequest("POST", "/api/v1/teams", payload)
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
				fmt.Println("Error closing response body:", err)
			}
		}()

		// Handle response based on the flag
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			fmt.Println("❌ Error: You do not have permission.")
			os.Exit(1)

		case http.StatusCreated:
			fmt.Printf("✅ Team '%s' created successfully.\n", name)
		default:
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Error: Received status code %d. Response: %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}
	},
}

// teamCmd represents the whoami command
var teamRmCmd = &cobra.Command{
	Use:   "rm [team-id]",
	Short: "Remove a team from the system",
	Long: `Remove an existing team from the system with authentication credentials.

Examples:
  # Remove an existing team with the ID 1234-456-789  (admins only)
  tunnelR team rm 1234-456-789

This command uses the saved authentication token from previous login.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		if id == "" {
			fmt.Println("Error: You must provide team id")
			os.Exit(1)
		}

		path := "/api/v1/teams/" + id

		resp, err := utils.MakeAuthenticatedRequest("DELETE", path, nil)
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
				fmt.Println("Error closing response body:", err)
			}
		}()

		// Handle response based on the flag
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			fmt.Println("❌ Error: You do not have permission.")
			os.Exit(1)
		case http.StatusNoContent:
			fmt.Printf("🗑️ Team with ID %s deleted successfully.\n", id)
		default:
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Error: Received status code %d. Response: %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}
	},
}

// teamCmd represents the whoami command
var teamGetCmd = &cobra.Command{
	Use:   "get [team-id]",
	Short: "Get detailed team information",
	Long: `Retrieve comprehensive information about a specific team by its ID.

Examples:
  # Get an existing team with the ID 1234-456-789 information  (admins only)
  tunnelR team get 1234-456-789

This command uses the saved authentication token from previous login.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		if id == "" {
			fmt.Println("Error: You must provide team id")
			os.Exit(1)
		}

		path := "/api/v1/teams/" + id

		resp, err := utils.MakeAuthenticatedRequest("GET", path, nil)
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
				fmt.Println("Error closing response body:", err)
			}
		}()

		// Handle response based on the flag
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			fmt.Println("❌ Error: You do not have permission.")
			os.Exit(1)
		case http.StatusOK:
			var team shared.TeamListMembersResponse
			if err := json.NewDecoder(resp.Body).Decode(&team); err != nil {
				fmt.Println("Error parsing response:", err)
				os.Exit(1)
			}
			fmt.Println("Team Details:")
			fmt.Printf("  ID: %s, Name: %s\n", team.ID, team.Name)
			users := team.Users
			fmt.Println("  Members:")
			utils.PrintUsersTable(users)
		default:
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Error: Received status code %d. Response: %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}
	},
}

// teamCmd represents the whoami command
var teamListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all authenticated user teams",
	Long: `Display a comprehensive list of all teams you have access to in the system.

Examples:
  # List all associated teams to the user
  tunnelR team list

This command uses the saved authentication token from previous login.`,
	Run: func(cmd *cobra.Command, _ []string) {
		path := "/api/v1/teams"

		resp, err := utils.MakeAuthenticatedRequest("GET", path, nil)
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
				fmt.Println("Error closing response body:", err)
			}
		}()

		// Handle response based on the flag
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			fmt.Println("❌ Error: You do not have permission.")
			os.Exit(1)
		case http.StatusOK:
			var teams []shared.Team
			if err := json.NewDecoder(resp.Body).Decode(&teams); err != nil {
				fmt.Println("Error parsing response:", err)
				os.Exit(1)
			}
			fmt.Println("Teams:")
			utils.PrintTeamssTable(teams)
		default:
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Error: Received status code %d. Response: %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}
	},
}

// teamCmd represents the whoami command
var teamAddUserCmd = &cobra.Command{
	Use:   "add-to-team [team-id]",
	Short: "Associate a user with a team",
	Long: `Add an existing user to a team.

Required flags:
  --user-id, -u    ID of the user to be added to the team

Examples:
  # Add an existing user with ID ABCD-EFG-HIJ to a team with the ID 1234-456-789  (admins only)
  tunnelR team add-to-team 1234-456-789 -u ABCD-EFG-HIJ

This command uses the saved authentication token from previous login.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		teamID := args[0]
		if teamID == "" {
			fmt.Println("Error: You must provide team id")
			os.Exit(1)
		}

		path := "/api/v1/teams/" + teamID + "/users/" + userID

		resp, err := utils.MakeAuthenticatedRequest("POST", path, nil)
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
				fmt.Println("Error closing response body:", err)
			}
		}()

		// Handle response based on the flag
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			fmt.Println("❌ Error: You do not have permission.")
			os.Exit(1)
		case http.StatusNoContent:
			fmt.Printf("✅ User with ID %s added successfully to team with ID %s.\n", userID, teamID)
		default:
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Error: Received status code %d. Response: %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}
	},
}

// teamCmd represents the whoami command
var teamRmUserCmd = &cobra.Command{
	Use:   "remove-from-team [team-id]",
	Short: "Disassociate a user from a team",
	Long: `Remove a user from a team.

Required flags:
  --user-id, -u    ID of the user to be removed from the team

Examples:
  # Remove an associated user with ID ABCD-EFG-HIJ from team with the ID 1234-456-789  (admins only)
  tunnelR team remove-from-team 1234-456-789 -u ABCD-EFG-HIJ

This command uses the saved authentication token from previous login.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		teamID := args[0]
		if teamID == "" {
			fmt.Println("Error: You must provide team id")
			os.Exit(1)
		}

		path := "/api/v1/teams/" + teamID + "/users/" + userID

		resp, err := utils.MakeAuthenticatedRequest("DELETE", path, nil)
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
				fmt.Println("Error closing response body:", err)
			}
		}()

		// Handle response based on the flag
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			fmt.Println("❌ Error: You do not have permission.")
			os.Exit(1)
		case http.StatusNoContent:
			fmt.Printf("🗑️ User with ID %s removed successfully from team with ID %s.\n", userID, teamID)
		default:
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Error: Received status code %d. Response: %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}
	},
}

// teamCmd represents the whoami command
var teamMachinesCmd = &cobra.Command{
	Use:   "machines [team-id]",
	Short: "List all associated machines to a team",
	Long: `Display a comprehensive list of all machines a team has access to in the system.

Examples:
  # List all associated machines to a team with ID 1234-456-789 (admins only)
  tunnelR team machines 1234-456-789

This command uses the saved authentication token from previous login.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		teamID := args[0]
		if teamID == "" {
			fmt.Println("Error: You must provide team id")
			os.Exit(1)
		}

		path := "/api/v1/teams/" + teamID + "/machines"

		resp, err := utils.MakeAuthenticatedRequest("GET", path, nil)
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
				fmt.Println("Error closing response body:", err)
			}
		}()

		// Handle response based on the flag
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			fmt.Println("❌ Error: You do not have permission.")
			os.Exit(1)
		case http.StatusOK:
			var team shared.TeamListMachinesResponse
			if err := json.NewDecoder(resp.Body).Decode(&team); err != nil {
				fmt.Println("Error parsing response:", err)
				os.Exit(1)
			}
			fmt.Println("Team Details:")
			fmt.Printf("  ID: %s, Name: %s\n", team.ID, team.Name)
			machines := team.Machines
			fmt.Println("  Machines:")
			utils.PrintMachinesTable(machines)

		default:
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Error: Received status code %d. Response: %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}
	},
}

func init() {
	RootCmd.AddCommand(teamCmd)
	teamCmd.AddCommand(teamAddCmd, teamRmCmd, teamGetCmd, teamListCmd, teamAddUserCmd, teamRmUserCmd, teamMachinesCmd)
	teamAddUserCmd.Flags().StringVarP(&userID, "user-id", "u", "", "ID of the user to add to the team")
	if err := teamAddUserCmd.MarkFlagRequired("user-id"); err != nil {
		fmt.Println(err)
	}

	teamRmUserCmd.Flags().StringVarP(&userID, "user-id", "u", "", "ID of the user to add to the team")
	if err := teamRmUserCmd.MarkFlagRequired("user-id"); err != nil {
		fmt.Println(err)
	}
}
