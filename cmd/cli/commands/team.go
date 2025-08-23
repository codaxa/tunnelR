package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	listTeams  bool
	detailsID  string
	createName string
	deleteID   string
)

type teamListResponse []struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type teamMemberResponse []struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type teamListMembersResponse struct {
	ID    string             `json:"id"`
	Name  string             `json:"name"`
	Users teamMemberResponse `json:"users"`
}

// teamCmd represents the whoami command
var teamCmd = &cobra.Command{
	Use:   "team",
	Short: "Manage teams",
	Long: `A command-line tool to manage teams, including listing, creating, and deleting them.

Examples:
  # List all teams
  tunnelR team -l

  # Displays a team data by its ID (admins only)
  tunnelR team -i 123

  # Create a new team with the name "dev-ops"  (admins only)
  tunnelR team -c dev-ops

  # Delete a team by its ID  (admins only)
  tunnelR team -d 123

This command uses the saved authentication token from previous login.`,

	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		listChanged := cmd.Flags().Lookup("list").Changed
		detailsChanged := cmd.Flags().Lookup("details").Changed
		createChanged := cmd.Flags().Lookup("create").Changed
		deleteChanged := cmd.Flags().Lookup("delete").Changed

		count := 0
		if listChanged {
			count++
		}
		if detailsChanged {
			count++
		}
		if createChanged {
			count++
		}
		if deleteChanged {
			count++
		}

		if count > 1 {
			return fmt.Errorf("only one flag (--list, --details, --create, or --delete) can be used at a time")
		}

		if count == 0 {
			return cmd.Help()
		}
		return nil
	},

	Run: func(cmd *cobra.Command, _ []string) {
		config, err := loadConfig()
		if err != nil {
			fmt.Println("Error loading config:", err)
			fmt.Println("Please login first using the login command")
			os.Exit(1)
		}

		if config.Token == "" {
			fmt.Println("No authentication token found. Please login first using the login command")
			os.Exit(1)
		}

		server := config.Server
		if server == "" {
			fmt.Println("No server configured. Please connect to server first")
			os.Exit(1)
		}

		if !strings.HasPrefix(server, "http://") && !strings.HasPrefix(server, "https://") {
			server = "http://" + server
		}

		// Parse the server URL
		serverURL, err := url.Parse(server)
		if err != nil {
			fmt.Println("Error parsing server URL:", err)
			os.Exit(1)
		}

		// Ensure the scheme is set
		if serverURL.Scheme == "" {
			serverURL.Scheme = "http"
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var path string
		var method string
		var body io.Reader

		if cmd.Flags().Lookup("list").Changed {
			path = "api/v1/teams"
			method = "GET"
		} else if cmd.Flags().Lookup("details").Changed {
			path = fmt.Sprintf("api/v1/teams/%s", detailsID)
			method = "GET"
		} else if cmd.Flags().Lookup("create").Changed {
			path = "api/v1/teams"
			method = "POST"
			payload := map[string]string{"name": createName}
			jsonBytes, err := json.Marshal(payload)
			if err != nil {
				fmt.Println("Error encoding request body:", err)
				os.Exit(1)
			}
			body = bytes.NewBuffer(jsonBytes)
		} else if cmd.Flags().Lookup("delete").Changed {
			path = fmt.Sprintf("api/v1/teams/%s", deleteID)
			method = "DELETE"
		}

		apiURL := serverURL.ResolveReference(&url.URL{Path: path})

		req, err := http.NewRequestWithContext(ctx, method, apiURL.String(), body)
		if err != nil {
			fmt.Println("Error creating request:", err)
			os.Exit(1)
		}

		// Corrected: Removed extra brace.
		if cmd.Flags().Lookup("create").Changed {
			req.Header.Set("Content-Type", "application/json")
		}

		req.Header.Set("Authorization", "Bearer "+config.Token)

		client := &http.Client{
			Timeout: 30 * time.Second,
		}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Println("Error sending request:", err)
			os.Exit(1)
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				fmt.Println("Error closing response body:", err)
			}
		}()

		// Handle response based on the flag
		if resp.StatusCode == http.StatusUnauthorized {
			fmt.Println("❌ Error: You do not have permission.")
			os.Exit(1)
		} else if resp.StatusCode != http.StatusOK && (cmd.Flags().Lookup("list").Changed || cmd.Flags().Lookup("details").Changed) {
			fmt.Printf("Error: Unexpected response (%d)\n", resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			if len(body) > 0 {
				fmt.Println(string(body))
			}
			os.Exit(1)
		} else if resp.StatusCode != http.StatusCreated && cmd.Flags().Lookup("create").Changed {
			fmt.Printf("Error: Unexpected response (%d)\n", resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			if len(body) > 0 {
				fmt.Println(string(body))
			}
			os.Exit(1)
		} else if resp.StatusCode != http.StatusNoContent && cmd.Flags().Lookup("delete").Changed {
			fmt.Printf("Error: Unexpected response (%d)\n", resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			if len(body) > 0 {
				fmt.Println(string(body))
			}
			os.Exit(1)
		}

		if cmd.Flags().Lookup("list").Changed {
			var teams teamListResponse
			if err := json.NewDecoder(resp.Body).Decode(&teams); err != nil {
				fmt.Println("Error parsing response:", err)
				os.Exit(1)
			}
			fmt.Println("Teams:")
			for _, team := range teams {
				fmt.Printf("  ID: %s, Name: %s\n", team.ID, team.Name)
			}
		} else if cmd.Flags().Lookup("details").Changed {
			var team teamListMembersResponse
			if err := json.NewDecoder(resp.Body).Decode(&team); err != nil {
				fmt.Println("Error parsing response:", err)
				os.Exit(1)
			}
			fmt.Println("Team Details:")
			fmt.Printf("  ID: %s, Name: %s\n", team.ID, team.Name)
			fmt.Println("  Users:")
			for _, user := range team.Users {
				fmt.Printf("\tID: %s, Name: %s\n", user.ID, user.Username)
			}
		} else if cmd.Flags().Lookup("create").Changed {
			fmt.Printf("✅ Team '%s' created successfully.\n", createName)
		} else if cmd.Flags().Lookup("delete").Changed {
			fmt.Printf("🗑️ Team with ID %s deleted successfully.\n", deleteID)
		}
	},
}

func init() {
	RootCmd.AddCommand(teamCmd)
	teamCmd.Flags().BoolVarP(&listTeams, "list", "l", false, "List all teams")
	teamCmd.Flags().StringVarP(&detailsID, "details", "i", "", "Display a team's data by its ID")
	teamCmd.Flags().StringVarP(&createName, "create", "c", "", "Create a new team with a given name")
	teamCmd.Flags().StringVarP(&deleteID, "delete", "d", "", "Delete a team by its ID")
}
