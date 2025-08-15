package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
}

// loginCmd represents the login command
var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with the backend server",
	Long: `Logs in to the backend server using the provided username and password.

Example:
  tunnelR login --user alice --password secret123

The login command requires:
- A username
- A password`,
	Run: func(cmd *cobra.Command, _ []string) {
		user, _ := cmd.Flags().GetString("user")
		password, _ := cmd.Flags().GetString("password")
		server, _ := cmd.Flags().GetString("server")

		// If server flag is not provided, try to load from config
		if server == "" {
			config, err := loadConfig()
			if err != nil {
				fmt.Printf("Error loading config: %v\n", err)
				fmt.Println("Please provide a server address using the --server flag")
				return
			}

			if config.Server == "" {
				fmt.Println("No server found in config. Please provide a server address using the --server flag")
				return
			}

			server = config.Server
			fmt.Printf("Using server from config: %s\n", server)
		}

		// Prepare request body
		reqBody := loginRequest{
			Username: user,
			Password: password,
		}
		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			fmt.Println("Error encoding request:", err)
			os.Exit(1)
		}

		// Make POST request
		url := fmt.Sprintf("https://%s/api/login", server)
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(bodyBytes))
		if err != nil {
			fmt.Println("Error sending request:", err)
			os.Exit(1)
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				fmt.Println("Error closing response body:", err)
			}
		}()

		switch resp.StatusCode {
		case http.StatusOK:
			// Parse token
			var res loginResponse
			if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
				fmt.Println("Error parsing login response:", err)
				os.Exit(1)
			}

			// Create config directory
			homeDir, _ := os.UserHomeDir()
			configDir := filepath.Join(homeDir, ".tunnelr")
			if err := os.MkdirAll(configDir, 0700); err != nil {
				fmt.Println("Error creating config directory:", err)
				os.Exit(1)
			}

			// Load existing config to preserve server if it exists
			config, err := loadConfig()
			if err != nil {
				config = &Config{} // Create new config if loading failed
			}

			// Update token and preserve server
			config.Token = res.Token
			if server != "" && config.Server == "" {
				config.Server = server // Save the server if it wasn't already saved
			}

			// Save updated config
			configPath := filepath.Join(configDir, "config.json")
			configData, _ := json.MarshalIndent(config, "", "  ")
			if err := os.WriteFile(configPath, configData, 0600); err != nil {
				fmt.Println("Error saving token:", err)
				os.Exit(1)
			}

			fmt.Println("Login successful. Token saved to", configPath)

		case http.StatusUnauthorized:
			fmt.Println("Error: Unauthorized. Check your username or password.")
			os.Exit(1)

		case http.StatusInternalServerError:
			fmt.Println("Error: Server encountered an internal error. Please try again later.")
			os.Exit(1)

		default:
			fmt.Printf("Error: Unexpected response (%d)\n", resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			if len(body) > 0 {
				fmt.Println(string(body))
			}
			os.Exit(1)
		}

		// In production, don't print the password (security risk)
		fmt.Printf("Logging in as %s...\n", user)
		// Here you would add your actual authentication logic
	},
}

func init() {
	RootCmd.AddCommand(loginCmd)

	loginCmd.Flags().StringP("user", "u", "", "Username for authentication")
	loginCmd.Flags().StringP("password", "p", "", "Password for authentication")
	loginCmd.Flags().StringP("server", "s", "", "Address or hostname of the backend server")

	if err := loginCmd.MarkFlagRequired("user"); err != nil {
		fmt.Println(err)
	}
	if err := loginCmd.MarkFlagRequired("password"); err != nil {
		fmt.Println(err)
	}
	// Server is not required since it can be loaded from config
}
