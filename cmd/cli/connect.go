package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

// Config represents the application configuration
type Config struct {
	Server string `json:"server"`
	Token  string `json:"token,omitempty"`
}

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

		// Check connectivity before proceeding
		fmt.Printf("Checking connectivity to %s...\n", server)
		if !checkServerHealth(server) {
			fmt.Printf("Error: Could not connect to %s. Server might be down or unreachable.\n", server)
			return
		}

		// Save server configuration
		if err := saveServerConfig(server); err != nil {
			fmt.Printf("Warning: Failed to save server configuration: %v\n", err)
		} else {
			fmt.Println("Server configuration saved successfully.")
		}

		fmt.Printf("Connecting to %s as %s...\n", server, user)
	},
}

// checkServerHealth verifies connectivity to the server by calling its /healthz endpoint
func checkServerHealth(server string) bool {
	client := http.Client{
		Timeout: 5 * time.Second,
	}

	url := fmt.Sprintf("http://%s/healthz", server)
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			fmt.Printf("Error closing response body: %v\n", err)
		}
	}()

	return resp.StatusCode == http.StatusOK
}

// saveServerConfig saves the server address to ~/.tunnelr/config.json
func saveServerConfig(server string) error {
	// Load existing config first to preserve other fields like token
	config, err := loadConfig()
	if err != nil {
		config = &Config{}
	}

	// Update the server field
	config.Server = server

	configDir := filepath.Join(os.Getenv("HOME"), ".tunnelr")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "config.json")

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// loadConfig loads the configuration from ~/.tunnelr/config.json
func loadConfig() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".tunnelr", "config.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

func init() {
	RootCmd.AddCommand(connectCmd)
	connectCmd.Flags().StringP("user", "u", "", "Username for authentication")
	connectCmd.Flags().StringP("server", "s", "", "Address or hostname of the backend server")
	if err := connectCmd.MarkFlagRequired("user"); err != nil {
		fmt.Println(err)
	}
	// Remove the required flag for server since we can load it from config
	// if err := connectCmd.MarkFlagRequired("server"); err != nil {
	//     fmt.Println(err)
	// }
}
