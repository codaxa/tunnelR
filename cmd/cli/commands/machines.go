package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// machinesCmd represents the machines command
var machinesCmd = &cobra.Command{
	Use:   "machines",
	Short: "Manage machines in the system",
	Long:  `Add, list, and remove machines from the system.`,
}

// machinesAddCmd represents the add command
var machinesAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new machine",
	Long:  `Add a new machine to the system.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Load config to get token first
		config, err := loadConfig()
		if err != nil {
			fmt.Println("Error loading config:", err)
			os.Exit(1)
		}

		// Check if token exists
		if config.Token == "" {
			fmt.Println("Please log in first using the login command")
			os.Exit(1)
		}

		// Get flags
		hostname, _ := cmd.Flags().GetString("hostname")
		ip, _ := cmd.Flags().GetString("ip")
		keyFile, _ := cmd.Flags().GetString("key-file")
		password, _ := cmd.Flags().GetString("password")

		// Ensure at least one authentication method is provided
		if keyFile == "" && password == "" {
			fmt.Println("Error: You must provide either a key file or password or both")
			os.Exit(1)
		}

		// Prepare request payload
		payload := map[string]string{
			"hostname": hostname,
			"ip":       ip,
		}

		// Add authentication methods to payload
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
			payload["key_data"] = string(keyData)
		}

		// Make the request
		resp, err := makeAuthenticatedRequest("POST", "/api/v1/machines", payload)
		if err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		// Handle response
		switch resp.StatusCode {
		case http.StatusCreated:
			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				fmt.Println("Error parsing response:", err)
				os.Exit(1)
			}
			fmt.Printf("Machine added successfully. ID: %s\n", result["id"])
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
	Short: "List all machines",
	Long:  `Display a list of all machines in the system.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Make the request
		resp, err := makeAuthenticatedRequest("GET", "/api/v1/machines", nil)
		if err != nil {
			if strings.Contains(err.Error(), "no authentication token found") {
				fmt.Println("Please log in first using the login command")
			} else {
				fmt.Println("Error:", err)
			}
			os.Exit(1)
		}
		defer resp.Body.Close()

		// Handle response
		switch resp.StatusCode {
		case http.StatusOK:
			var machines []struct {
				ID        string    `json:"id"`
				Hostname  string    `json:"hostname"`
				IP        string    `json:"ip"`
				CreatedAt time.Time `json:"created_at"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&machines); err != nil {
				fmt.Println("Error parsing response:", err)
				os.Exit(1)
			}

			// Format output as a table
			// Create a styled table with better spacing
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', tabwriter.TabIndent)

			// Print styled header
			fmt.Println("\n┌─────────────────────── MACHINES ───────────────────────┐")
			fmt.Fprintln(w, "\033[1mID\tHOSTNAME\tIP ADDRESS\tCREATED AT\033[0m")

			// Print separator
			fmt.Fprintln(w, "────────\t────────\t─────────\t───────\t──────────")

			// Print data rows
			for _, m := range machines {

				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					m.ID,
					m.Hostname,
					m.IP,
					m.CreatedAt.Format("Jan 02, 2006 15:04"))
			}
			fmt.Println("└──────────────────────────────────────────────────────────┘")
			w.Flush()

			fmt.Printf("\nTotal: %d machines\n", len(machines))
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

// machinesRmCmd represents the rm command
var machinesRmCmd = &cobra.Command{
	Use:   "rm [machine-id]",
	Short: "Remove a machine",
	Long:  `Remove a machine from the system by its ID.`,
	Args:  cobra.ExactArgs(1), // Require exactly one argument (the machine ID)
	Run: func(cmd *cobra.Command, args []string) {
		// Get machine ID from args
		machineID := args[0]

		// Make the request
		resp, err := makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/machines/%s", machineID), nil)
		if err != nil {
			if strings.Contains(err.Error(), "no authentication token found") {
				fmt.Println("Please log in first using the login command")
			} else {
				fmt.Println("Error:", err)
			}
			os.Exit(1)
		}
		defer resp.Body.Close()

		// Handle response
		switch resp.StatusCode {
		case http.StatusNoContent:
			fmt.Println("Machine removed successfully.")
		case http.StatusUnauthorized:
			fmt.Println("Unauthorized. Please log in again.")
			os.Exit(1)
		case http.StatusForbidden:
			fmt.Println("You don't have permission to remove machines.")
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

// machinesUpdateCmd represents the update command
var machinesUpdateCmd = &cobra.Command{
	Use:   "update [machine-id]",
	Short: "Update a machine",
	Long:  `Update properties of an existing machine in the system.`,
	Args:  cobra.ExactArgs(1), // Require exactly one argument (the machine ID)
	Run: func(cmd *cobra.Command, args []string) {
		// Get machine ID from args
		machineID := args[0]

		// Get flags
		hostname, _ := cmd.Flags().GetString("hostname")
		ip, _ := cmd.Flags().GetString("ip")
		keyFile, _ := cmd.Flags().GetString("key-file")
		password, _ := cmd.Flags().GetString("password")

		// Prepare request payload - only include fields that were specified
		payload := map[string]string{}
		if hostname != "" {
			payload["hostname"] = hostname
		}
		if ip != "" {
			payload["ip"] = ip
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
			payload["key_data"] = string(keyData)
		}

		// Check if there's anything to update
		if len(payload) == 0 {
			fmt.Println("No update parameters provided. Use --hostname, --ip, --password, --key-file, or --team-id flags.")
			os.Exit(1)
		}

		// Make the request
		resp, err := makeAuthenticatedRequest("PUT", fmt.Sprintf("/api/v1/machines/%s", machineID), payload)
		if err != nil {
			if strings.Contains(err.Error(), "no authentication token found") {
				fmt.Println("Please log in first using the login command")
			} else {
				fmt.Println("Error:", err)
			}
			os.Exit(1)
		}
		defer resp.Body.Close()

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

// makeAuthenticatedRequest makes an HTTP request with authentication
func makeAuthenticatedRequest(method, path string, payload interface{}) (*http.Response, error) {
	// Load config to get server and token
	config, err := loadConfig()
	if err != nil {
		return nil, fmt.Errorf("error loading config: %w", err)
	}

	if config.Token == "" {
		return nil, fmt.Errorf("no authentication token found")
	}

	var body io.Reader
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("error encoding request: %w", err)
		}
		body = bytes.NewBuffer(jsonData)
	}

	// Construct the full URL
	url := fmt.Sprintf("http://%s%s", config.Server, path)

	// Create the request
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Add headers
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+config.Token)

	// Send the request
	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
}

func init() {
	RootCmd.AddCommand(machinesCmd)

	// Add subcommands
	machinesCmd.AddCommand(machinesAddCmd)
	machinesCmd.AddCommand(machinesListCmd)
	machinesCmd.AddCommand(machinesRmCmd)
	machinesCmd.AddCommand(machinesUpdateCmd)

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
	machinesUpdateCmd.Flags().StringP("team-id", "t", "", "New team ID to associate the machine with")
}
