package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

type whoamiResponse struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

// whoamiCmd represents the whoami command
var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Display current user information",
	Long: `Shows details about the currently authenticated user, including username and role.

Example:
  tunnelR whoami

This command uses the saved authentication token from previous login.`,
	Run: func(_ *cobra.Command, _ []string) {
		// Load config to get token and server
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

		// Create HTTP request
		url := fmt.Sprintf("http://%s/api/whoami", server)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			fmt.Println("Error creating request:", err)
			os.Exit(1)
		}

		// Add auth token to header
		req.Header.Set("Authorization", "Bearer "+config.Token)

		// Make the request
		client := &http.Client{}
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

		// Handle response
		switch resp.StatusCode {
		case http.StatusOK:
			var userInfo whoamiResponse
			if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
				fmt.Println("Error parsing response:", err)
				os.Exit(1)
			}

			fmt.Println("Current user information:")
			fmt.Printf("Username: %s\n", userInfo.Username)
			fmt.Printf("Role: %s\n", userInfo.Role)

		case http.StatusUnauthorized:
			fmt.Println("Error: Your session has expired or is invalid. Please login again.")
			os.Exit(1)

		default:
			fmt.Printf("Error: Unexpected response (%d)\n", resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			if len(body) > 0 {
				fmt.Println(string(body))
			}
			os.Exit(1)
		}
	},
}

func init() {
	RootCmd.AddCommand(whoamiCmd)
}
