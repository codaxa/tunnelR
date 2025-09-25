package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	cliutils "github.com/codaxa/tunnelR.git/cmd/cli/utils"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

type wsMessage struct {
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
	Data    string `json:"data,omitempty"`
}

var accessCmd = &cobra.Command{
	Use:   "access <machine-id>",
	Short: "Connect to a machine via WebSocket SSH",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		config, err := cliutils.LoadConfig()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			fmt.Println("Please provide a server address using the --server flag")
			return
		}

		// Build WebSocket URL
		server := config.Server
		if strings.HasPrefix(server, "http://") {
			server = "ws://" + strings.TrimPrefix(server, "http://")
		} else if strings.HasPrefix(server, "https://") {
			server = "wss://" + strings.TrimPrefix(server, "https://")
		} else if server != "" {
			server = "ws://" + server
		}
		machineID := args[0]
		wsURL := fmt.Sprintf("%s/api/v1/access/shell?machine_id=%s", server, machineID)
		header := map[string][]string{
			"Authorization": {"Bearer " + config.Token},
		}

		// Connect to WebSocket
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
		if err != nil {
			fmt.Printf("Error connecting to WebSocket: %v\n", err)
			os.Exit(1)
		}
		defer func() {
			if err := conn.Close(); err != nil {
				fmt.Printf("Error closing WebSocket connection: %v\n", err)
			}
		}()

		// Goroutine for server -> client
		go func() {
			for {
				_, msgBytes, err := conn.ReadMessage()
				if err != nil {
					// Handle connection closures
					if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						fmt.Println("\nConnection closed by server")
						os.Exit(0)
					}
					if strings.Contains(err.Error(), "broken pipe") ||
						strings.Contains(err.Error(), "connection reset") ||
						strings.Contains(err.Error(), "use of closed network connection") {
						fmt.Println("\nConnection lost")
						os.Exit(1)
					}
					fmt.Printf("\nConnection error: %v\n", err)
					os.Exit(1)
				}

				var msg wsMessage
				if err := json.Unmarshal(msgBytes, &msg); err != nil {
					continue
				}

				switch msg.Type {
				case "stdout":
					if msg.Data != "" {
						fmt.Print(msg.Data)
					}
				case "stderr":
					if msg.Data != "" {
						fmt.Fprint(os.Stderr, msg.Data)
					}
				case "system":
					fmt.Println(msg.Message)
				case "exit":
					fmt.Printf("\nSession ended (exit code: %v)\n", msg.Message)
					os.Exit(0)
				case "error":
					fmt.Fprintf(os.Stderr, "Error: %s\n", msg.Message)
					os.Exit(1)
				case "close":
					fmt.Println("\nServer requested connection close")
					os.Exit(0)
				}
			}
		}()

		// Handle Ctrl+C and Ctrl+Z signals
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTSTP)
		go func() {
			for sig := range sigCh {
				var ctrl string
				switch sig {
				case syscall.SIGINT:
					ctrl = "\x03" // Ctrl+C
				case syscall.SIGTSTP:
					ctrl = "\x1a" // Ctrl+Z
				}
				if err := conn.WriteJSON(wsMessage{Type: "stdin", Data: ctrl}); err != nil {
					fmt.Printf("Error writing message to WebSocket: %v\n", err)
					os.Exit(1)
				}
			}
		}()

		// Read user input line by line
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {

			input := scanner.Text()
			if input == "exit" {
				break
			}

			if input == "clear" {
				fmt.Print("\033[2J\033[H")
				sendMsg := wsMessage{Type: "stdin", Data: input + "\n"}
				if err := conn.WriteJSON(sendMsg); err != nil {
					fmt.Printf("Error sending command: %v\n", err)
					break
				}
				continue
			}

			sendMsg := wsMessage{Type: "stdin", Data: input + "\n"}
			if err := conn.WriteJSON(sendMsg); err != nil {
				fmt.Printf("Error sending command: %v\n", err)
				break
			}
		}
	},
}

func init() {
	RootCmd.AddCommand(accessCmd)
}
