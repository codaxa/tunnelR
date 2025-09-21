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
	"path/filepath"
	"strings"
	"time"

	cliutils "github.com/codaxa/tunnelR.git/cmd/cli/utils"
	"github.com/spf13/cobra"
)

var (
	username string
	password string
	server   string
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
	Run: func(_ *cobra.Command, _ []string) {
		// If server flag is not provided, try to load from config
		if server == "" {
			config, err := cliutils.LoadConfig()
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

		if !strings.HasPrefix(server, "http://") && !strings.HasPrefix(server, "https://") {
			server = "http://" + server
		}

		serverURL, err := url.Parse(server)
		if err != nil {
			fmt.Println("Error parsing server URL:", err)
			os.Exit(1)
		}

		if serverURL.Scheme == "" {
			serverURL.Scheme = "http"
		}

		// Prepare request body
		reqBody := loginRequest{
			Username: username,
			Password: password,
		}
		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			fmt.Println("Error encoding request:", err)
			os.Exit(1)
		}

		apiURL := serverURL.ResolveReference(&url.URL{Path: "api/v1/login"})

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "POST", apiURL.String(), bytes.NewBuffer(bodyBytes))
		if err != nil {
			fmt.Println("Error creating request:", err)
			os.Exit(1)
		}

		req.Header.Set("Content-Type", "application/json")

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
			config, err := cliutils.LoadConfig()
			if err != nil {
				config = &cliutils.Config{} // Create new config if loading failed
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

		fmt.Printf("Logging in as %s...\n", username)
	},
}

func init() {
	RootCmd.AddCommand(loginCmd)

	loginCmd.Flags().StringVarP(&username, "user", "u", "", "Username for authentication")
	loginCmd.Flags().StringVarP(&password, "password", "p", "", "Password for authentication")
	loginCmd.Flags().StringVarP(&server, "server", "s", "", "Address or hostname of the backend server")

	if err := loginCmd.MarkFlagRequired("user"); err != nil {
		fmt.Println(err)
	}
	if err := loginCmd.MarkFlagRequired("password"); err != nil {
		fmt.Println(err)
	}
}
