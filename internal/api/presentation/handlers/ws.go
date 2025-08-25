package handlers

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"crypto/rsa"
	"encoding/pem"

	"github.com/codaxa/tunnelR.git/internal/api/app/service"
	"github.com/codaxa/tunnelR.git/internal/api/core/model"
	authutils "github.com/codaxa/tunnelR.git/internal/api/presentation/utils"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

// ShellHandler handles WebSocket connections for shell access
type ShellHandler struct {
	machineService   *service.MachineService
	accessLogService *service.AccessLogService
	upgrader         websocket.Upgrader
	maxSessions      int
	defaultTTLHours  int // Default time-to-live for ephemeral accounts in hours
	maxTTLHours      int // Maximum allowed TTL for ephemeral accounts in hours
}

// NewShellHandler creates a new ShellHandler
func NewShellHandler(machineService *service.MachineService,
	accessLogService *service.AccessLogService,
	maxSessions int) *ShellHandler {
	log.Printf("Creating new ShellHandler with maxSessions=%d", maxSessions)
	return &ShellHandler{
		machineService:   machineService,
		accessLogService: accessLogService,
		upgrader: websocket.Upgrader{
			ReadBufferSize:   1024,
			WriteBufferSize:  1024,
			HandshakeTimeout: 5 * time.Second,
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				log.Printf("WebSocket origin check: %s", origin)
				return true // You may want to restrict this in production
			},
		},
		maxSessions:     maxSessions,
		defaultTTLHours: 8,
		maxTTLHours:     24,
	}
}

// Message types for WebSocket communication
type wsMessageType string

const (
	msgTypeStdin  wsMessageType = "stdin"
	msgTypeStdout wsMessageType = "stdout"
	msgTypeStderr wsMessageType = "stderr"
	msgTypeResize wsMessageType = "resize"
	msgTypeSystem wsMessageType = "system"
	msgTypeExit   wsMessageType = "exit"
	msgTypePing   wsMessageType = "ping"
	msgTypePong   wsMessageType = "pong"
	msgTypeError  wsMessageType = "error"
	msgTypeClose  wsMessageType = "close"
)

// Message structures
type wsMessage struct {
	Type    wsMessageType `json:"type"`
	Data    string        `json:"data,omitempty"`
	Cols    int           `json:"cols,omitempty"`
	Rows    int           `json:"rows,omitempty"`
	TS      int64         `json:"ts,omitempty"`
	Code    interface{}   `json:"code,omitempty"`
	Message string        `json:"message,omitempty"`
}

// ShellAccess handles WebSocket connections for shell access
func (h *ShellHandler) ShellAccess(w http.ResponseWriter, r *http.Request) {

	log.Printf("[WS] Connection attempt started from %s", r.RemoteAddr)

	// Get machine ID from query parameter
	machineID := r.URL.Query().Get("machine_id")
	if machineID == "" {
		log.Printf("[WS] Missing machine_id parameter from %s", r.RemoteAddr)
		http.Error(w, "Missing machine_id parameter", http.StatusBadRequest)
		return
	}

	// Get optional TTL from query parameter (in hours)
	ttlHours := h.defaultTTLHours
	if ttlParam := r.URL.Query().Get("ttl"); ttlParam != "" {
		if parsed, err := strconv.Atoi(ttlParam); err == nil {
			if parsed > 0 && parsed <= h.maxTTLHours {
				ttlHours = parsed
			}
		}
	}

	// Extract user ID from JWT token
	userID, err := authutils.ExtractUserID(r)
	if err != nil {
		log.Printf("[WS] Error extracting user ID from %s: %v", r.RemoteAddr, err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get user info for username
	username, err := authutils.ExtractUsername(r)
	if err != nil {
		log.Printf("[WS] Error extracting username from %s: %v", r.RemoteAddr, err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check if user has access to the machine
	machine, err := h.machineService.GetMachineByID(r.Context(), machineID, userID)
	if err != nil {
		if err == service.ErrMachineNotFound {
			log.Printf("[WS] Machine %s not found for user %s", machineID, userID)
			http.Error(w, "Machine not found", http.StatusNotFound)
			return
		}
		if err == service.ErrUserCanNotUseMachine {
			log.Printf("[WS] User %s does not have access to machine %s", userID, machineID)
			http.Error(w, "You do not have access to this machine", http.StatusForbidden)
			return
		}
		log.Printf("[WS] Error checking machine access: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] Error upgrading to WebSocket for user %s, machine %s: %v",
			userID, machineID, err)
		return
	}

	// After successful upgrade, handle the ephemeral account creation in a separate goroutine
	// to avoid blocking the response
	go func() {
		// Create a new background context instead of using the request context
		newCtx := context.Background()

		// Create temporary username
		tempUsername, err := h.createEphemeralAccount(newCtx, machine, username, userID, ttlHours)
		if err != nil {
			// Send error message over WebSocket
			errMsg := wsMessage{
				Type:    msgTypeError,
				Code:    "PROVISION_FAILED",
				Message: "Failed to create temporary user account",
			}
			conn.WriteJSON(errMsg)
			conn.Close()
			return
		}

		// Start WebSocket session with the ephemeral account
		h.handleWebSocketSession(newCtx, conn, machine, userID, tempUsername, ttlHours)
	}()
}

// handleWebSocketSession manages the WebSocket connection for shell access
func (h *ShellHandler) handleWebSocketSession(ctx context.Context, conn *websocket.Conn, machine *model.Machine, userID, tempUsername string, ttlHours int) {
	sessionStart := time.Now()
	log.Printf("[WS-Session] Starting WebSocket session for user %s, machine %s, temp user %s",
		userID, machine.ID, tempUsername)

	// Set read/write deadlines
	readDeadline := 11 * time.Minute
	writeDeadline := 10 * time.Second
	log.Printf("[WS-Session] Using read deadline: %v, write deadline: %v", readDeadline, writeDeadline)

	// Create channels for communication
	inputCh := make(chan wsMessage, 100)
	outputCh := make(chan wsMessage, 100)
	errorCh := make(chan error, 10)
	doneCh := make(chan struct{})

	// Set initial read deadline
	if err := conn.SetReadDeadline(time.Now().Add(readDeadline)); err != nil {
		log.Printf("[WS-Session] Error setting initial read deadline: %v", err)
		return
	}

	// Set up ping/pong handlers for WebSocket control frames
	conn.SetPingHandler(func(data string) error {
		log.Printf("[WS-Session] Received WebSocket ping frame")
		err := conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(writeDeadline))
		if err != nil {
			log.Printf("[WS-Session] Error sending pong frame: %v", err)
			return err
		}
		return nil
	})

	conn.SetPongHandler(func(data string) error {
		log.Printf("[WS-Session] Received WebSocket pong frame")
		// Reset read deadline when we get a pong response
		if err := conn.SetReadDeadline(time.Now().Add(readDeadline)); err != nil {
			log.Printf("[WS-Session] Error resetting read deadline after pong: %v", err)
			return err
		}
		return nil
	})

	// Start heartbeat goroutine to keep connection alive
	pingInterval := 5 * time.Minute // Ping every 5 minutes
	log.Printf("[WS-Session] Starting heartbeat goroutine with interval: %v", pingInterval)

	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Send WebSocket ping frame
				log.Printf("[WS-Session] Sending WebSocket ping frame")
				if err := conn.WriteControl(
					websocket.PingMessage,
					[]byte{},
					time.Now().Add(writeDeadline),
				); err != nil {
					log.Printf("[WS-Session] Error sending ping frame: %v", err)
					errorCh <- fmt.Errorf("ping error: %w", err)
					return
				}

				// Also send application-level ping message
				pingTS := time.Now().Unix()
				log.Printf("[WS-Session] Sending application ping with ts: %d", pingTS)
				pingMsg := wsMessage{
					Type: msgTypePing,
					TS:   pingTS,
				}
				outputCh <- pingMsg
			case <-doneCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	// Start reader goroutine
	go func() {
		defer close(inputCh)
		messageCount := 0
		for {
			startRead := time.Now()
			_, messageBytes, err := conn.ReadMessage()
			readDuration := time.Since(startRead)

			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					log.Printf("[WS-Reader] WebSocket read error after %d messages: %v", messageCount, err)
					errorCh <- err
				} else {
					log.Printf("[WS-Reader] WebSocket closed by client after %d messages", messageCount)
				}
				return
			}
			messageCount++

			// Log message details
			log.Printf("[WS-Reader] Read message #%d (%d bytes) in %v",
				messageCount, len(messageBytes), readDuration)

			// Reset read deadline
			if err := conn.SetReadDeadline(time.Now().Add(readDeadline)); err != nil {
				log.Printf("[WS-Reader] Error resetting read deadline: %v", err)
				errorCh <- err
				return
			}

			// Parse message
			var message wsMessage
			if err := json.Unmarshal(messageBytes, &message); err != nil {
				log.Printf("[WS-Reader] Error parsing WebSocket message: %v", err)
				continue
			}
			log.Printf("[WS-Reader] Received message type: %s", message.Type)

			// Handle message based on type
			switch message.Type {
			case msgTypePing:
				log.Printf("[WS-Reader] Received ping with timestamp: %d", message.TS)
				// Respond with pong
				outputCh <- wsMessage{
					Type: msgTypePong,
					TS:   message.TS,
				}
			case msgTypeClose:
				log.Printf("[WS-Reader] Received close request from client")
				// Client requested close
				close(doneCh)
				return
			default:
				// Pass other messages to input channel
				log.Printf("[WS-Reader] Forwarding message of type '%s' to input channel", message.Type)
				inputCh <- message
			}
		}
	}()

	// Start writer goroutine
	go func() {
		messageCount := 0
		for {
			select {
			case message, ok := <-outputCh:
				if !ok {
					log.Printf("[WS-Writer] Output channel closed after %d messages", messageCount)
					return
				}
				messageCount++
				log.Printf("[WS-Writer] Preparing to send message #%d of type: %s", messageCount, message.Type)

				// Set write deadline
				writeStart := time.Now()
				if err := conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
					log.Printf("[WS-Writer] Error setting write deadline: %v", err)
					errorCh <- err
					return
				}

				// Write message
				if err := conn.WriteJSON(message); err != nil {
					log.Printf("[WS-Writer] WebSocket write error: %v", err)
					errorCh <- err
					return
				}
				log.Printf("[WS-Writer] Sent message #%d of type '%s' in %v",
					messageCount, message.Type, time.Since(writeStart))
			case <-doneCh:
				log.Printf("[WS-Writer] Done channel signaled, exiting after %d messages", messageCount)
				return
			}
		}
	}()

	// Create SSH client as root (for system access)
	sshClient, err := h.createSSHClient(machine)
	if err != nil {
		log.Printf("[WS-Session] Error creating SSH client: %v", err)
		outputCh <- wsMessage{
			Type:    msgTypeError,
			Code:    "CONNECTION_FAILED",
			Message: "Failed to connect to the machine",
		}
		close(doneCh)
		return
	}
	defer sshClient.Close()

	// Create a new client config for the temporary user
	tempUserConfig := &ssh.ClientConfig{
		User:    tempUsername,
		Auth:    []ssh.AuthMethod{},
		Timeout: 10 * time.Second,
	}

	// Get the temporary user's password by executing a command as root
	// This is necessary because we created the user but need a way to authenticate as that user
	randomPasswordBytes := make([]byte, 16)
	rand.Read(randomPasswordBytes)
	randomPassword := hex.EncodeToString(randomPasswordBytes)

	// Set password for temp user
	// After setting the password with chpasswd, check if it worked
	setPasswordCmd := fmt.Sprintf("echo '%s:%s' | sudo chpasswd", tempUsername, randomPassword)
	stdout, stderr, err := h.executeSSHCommandWithOutput(sshClient, setPasswordCmd)
	if err != nil {
		log.Printf("[WS-Session] Error setting password for temp user: %v", err)
		log.Printf("[WS-Session] Command stderr: %s", stderr)
		outputCh <- wsMessage{
			Type:    msgTypeError,
			Code:    "PROVISION_FAILED",
			Message: "Failed to set up user account password",
		}
		close(doneCh)
		return
	}

	// Check if password authentication is allowed in sshd config
	checkSshdCmd := "grep -E '^PasswordAuthentication|^ChallengeResponseAuthentication' /etc/ssh/sshd_config"
	stdout, stderr, err = h.executeSSHCommandWithOutput(sshClient, checkSshdCmd)
	if err == nil {
		log.Printf("[WS-Session] SSH config settings: %s", stdout)
		if strings.Contains(stdout, "PasswordAuthentication no") {
			log.Printf("[WS-Session] Warning: Password authentication is disabled in sshd_config")
			// Continue anyway, try alternative approach
		}
	}

	// Verify that the user was created correctly
	verifyUserCmd := fmt.Sprintf("id %s", tempUsername)
	stdout, stderr, err = h.executeSSHCommandWithOutput(sshClient, verifyUserCmd)
	if err != nil {
		log.Printf("[WS-Session] User verification failed: %v", err)
		log.Printf("[WS-Session] Command stderr: %s", stderr)
		outputCh <- wsMessage{
			Type:    msgTypeError,
			Code:    "PROVISION_FAILED",
			Message: "User account verification failed",
		}
		close(doneCh)
		return
	}
	log.Printf("[WS-Session] User verified: %s", stdout)

	// Alternative approach: Add the temp user to SSH authorized keys temporarily
	pubKey, privKey, err := generateSSHKeyPair()
	if err != nil {
		log.Printf("[WS-Session] Error generating SSH key pair: %v", err)
		// Continue with password auth anyway
	} else {
		authKeysCmd := fmt.Sprintf("sudo mkdir -p /home/%s/.ssh && echo '%s' | sudo tee /home/%s/.ssh/authorized_keys && sudo chmod 600 /home/%s/.ssh/authorized_keys && sudo chown -R %s:%s /home/%s/.ssh",
			tempUsername, pubKey, tempUsername, tempUsername, tempUsername, tempUsername, tempUsername)
		stdout, stderr, err = h.executeSSHCommandWithOutput(sshClient, authKeysCmd)
		if err != nil {
			log.Printf("[WS-Session] Error setting up authorized_keys: %v", err)
			log.Printf("[WS-Session] Command stderr: %s", stderr)
			// Continue with password auth anyway
		} else {
			// Add key authentication as an alternative
			signer, err := ssh.ParsePrivateKey([]byte(privKey))
			if err == nil {
				tempUserConfig.Auth = append(tempUserConfig.Auth, ssh.PublicKeys(signer))
				log.Printf("[WS-Session] Added key authentication as fallback")
			}
		}
	}

	// Try multiple authentication methods
	tempUserConfig.Auth = []ssh.AuthMethod{
		ssh.Password(randomPassword),
		ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i := range questions {
				answers[i] = randomPassword
			}
			return answers, nil
		}),
	}

	// Connect as the temporary user
	ipAddress := machine.IPAddress
	if idx := strings.Index(ipAddress, "/"); idx > 0 {
		ipAddress = ipAddress[:idx]
	}

	// Connect as the temporary user
	log.Printf("[WS-Session] Using su method instead of direct SSH for temporary user %s", tempUsername)

	// Create a session from the existing root connection
	rootSession, err := sshClient.NewSession()
	if err != nil {
		log.Printf("[WS-Session] Error creating root session: %v", err)
		outputCh <- wsMessage{
			Type:    msgTypeError,
			Code:    "SESSION_FAILED",
			Message: "Failed to create session",
		}
		close(doneCh)
		return
	}
	defer rootSession.Close()

	// Set up pipes for I/O
	stdin, err := rootSession.StdinPipe()
	if err != nil {
		log.Printf("[WS-Session] Error getting stdin pipe: %v", err)
		close(doneCh)
		return
	}

	stdoutPipe, err := rootSession.StdoutPipe()
	if err != nil {
		log.Printf("[WS-Session] Error getting stdout pipe: %v", err)
		close(doneCh)
		return
	}

	stderrPipe, err := rootSession.StderrPipe()
	if err != nil {
		log.Printf("[WS-Session] Error getting stderr pipe: %v", err)
		close(doneCh)
		return
	}

	// Set up terminal modes
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	// Default terminal size
	initialCols := 80
	initialRows := 24

	// Request pseudo terminal
	if err := rootSession.RequestPty("xterm", initialRows, initialCols, modes); err != nil {
		log.Printf("[WS-Session] Error requesting PTY: %v", err)
		close(doneCh)
		return
	}

	// Start shell with sudo to switch to temporary user
	suCmd := fmt.Sprintf("sudo -u %s bash -c 'cd ~%s; exec bash'", tempUsername, tempUsername)
	if err := rootSession.Start(suCmd); err != nil {
		log.Printf("[WS-Session] Error starting shell with su: %v", err)
		outputCh <- wsMessage{
			Type:    msgTypeError,
			Code:    "SESSION_FAILED",
			Message: "Failed to start shell as temporary user",
		}
		close(doneCh)
		return
	}

	// After starting the shell, send a welcome command
	time.Sleep(100 * time.Millisecond) // Small delay to let the shell initialize
	welcomeCmd := "echo 'Shell session started. Your temporary account will expire in $(( $(date -d \"$(sudo chage -l " + tempUsername + " | grep 'Account expires' | cut -d: -f2)\" +%s) - $(date +%s) )) seconds.'\n"
	_, err = stdin.Write([]byte(welcomeCmd))
	if err != nil {
		log.Printf("[WS-Session] Error sending welcome command: %v", err)
		// Continue anyway
	}

	// Add a timeout for the SSH session
	sessionTimeout := time.Duration(ttlHours+1) * time.Hour
	sessionTimeoutTimer := time.AfterFunc(sessionTimeout, func() {
		log.Printf("[WS-Session] Session timeout reached after %v", sessionTimeout)
		errorCh <- fmt.Errorf("session timeout reached")
	})
	defer sessionTimeoutTimer.Stop()

	// Update access log to active status
	// Use a new context with timeout instead of the parent context
	updateCtx, updateCancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = h.accessLogService.UpdateAccessLogStatus(updateCtx, machine.ID, tempUsername, model.StatusActive)
	updateCancel() // Always cancel the context to avoid leaks
	if err != nil {
		log.Printf("[WS-Session] Error updating access log status: %v", err)
		// Continue anyway, not critical
	}

	// Send system message
	outputCh <- wsMessage{
		Type: msgTypeSystem,
		Message: fmt.Sprintf("Connected to %s as temporary user %s. Session will expire in %s.",
			machine.Hostname, tempUsername, time.Until(time.Now().Add(time.Hour*time.Duration(ttlHours))).Round(time.Minute)),
	}
	var lastCommand string

	// Handle stdout
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdoutPipe.Read(buf)
			if err != nil {
				if err.Error() != "EOF" {
					log.Printf("[WS-Session] Error reading from stdout: %v", err)
					errorCh <- err
				}
				return
			}
			raw := string(buf[:n])
			// Remove ANSI escape codes
			clean := ansiRegexp.ReplaceAllString(raw, "")
			lines := strings.Split(clean, "\n")
			var filtered []string
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				// Skip empty lines
				if trimmed == "" {
					continue
				}
				// Skip echoed command
				if lastCommand != "" && (trimmed == lastCommand || trimmed == lastCommand+"\r") {
					continue
				}
				// Skip prompt lines (very basic, may need to be smarter)
				if strings.HasPrefix(trimmed, "t_") && strings.Contains(trimmed, "@") && strings.Contains(trimmed, ":") && strings.HasSuffix(trimmed, "$") {
					continue
				}
				filtered = append(filtered, trimmed)
			}
			if len(filtered) > 0 {
				outputCh <- wsMessage{
					Type: msgTypeStdout,
					Data: strings.Join(filtered, "\n"),
				}
			}
		}
	}()

	// Handle stderr
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stderrPipe.Read(buf)
			if err != nil {
				if err.Error() != "EOF" {
					log.Printf("[WS-Session] Error reading from stderr: %v", err)
					errorCh <- err
				}
				return
			}

			outputCh <- wsMessage{
				Type: msgTypeStderr,
				Data: string(buf[:n]),
			}
		}
	}()

	// Handle stdin from WebSocket

	go func() {
		for message := range inputCh {
			switch message.Type {
			case msgTypeStdin:
				data := message.Data
				// Track the last command (strip trailing newline for matching)
				lastCommand = strings.TrimSpace(data)
				if !strings.HasSuffix(data, "\n") {
					data = data + "\n"
				}

				// Write the command to stdin
				_, err := stdin.Write([]byte(data))
				if err != nil {
					log.Printf("[WS-Session] Error writing to stdin: %v", err)
					errorCh <- err
					return
				}

				// Optionally force a flush with a small delay
				time.Sleep(10 * time.Millisecond)

			case msgTypeResize:
				// Resize handling remains the same
				err := rootSession.WindowChange(message.Rows, message.Cols)
				if err != nil {
					log.Printf("[WS-Session] Error resizing window: %v", err)
				}
			}
		}
	}()

	// Wait for session to end
	go func() {
		err := rootSession.Wait()

		// Acquire a semaphore to ensure we don't have race conditions with other termination paths
		select {
		case <-doneCh:
			log.Printf("[WS-Session] Session already terminated, not sending exit message")
			return
		default:
			// Continue with termination handling
		}

		if err != nil {
			log.Printf("[WS-Session] SSH session ended with error: %v", err)
			var exitCode int
			if exitErr, ok := err.(*ssh.ExitError); ok {
				exitCode = exitErr.ExitStatus()
			} else {
				// Handle the "without exit status" case
				if strings.Contains(err.Error(), "without exit status") {
					exitCode = 1
				} else {
					exitCode = 1
				}
				log.Printf("[WS-Session] SSH session terminated abnormally, using exit code %d", exitCode)
			}

			// Use a buffered channel for the done notification to avoid deadlock
			done := make(chan struct{}, 1)
			go func() {
				defer func() { done <- struct{}{} }()
				if conn.UnderlyingConn() != nil {
					conn.SetWriteDeadline(time.Now().Add(300 * time.Millisecond))
					conn.WriteJSON(wsMessage{
						Type: msgTypeExit,
						Code: exitCode,
					})
					log.Printf("[WS-Session] Sent exit message with code %d", exitCode)
				}
			}()

			// Wait for the message to be sent or timeout
			select {
			case <-done:
			case <-time.After(400 * time.Millisecond):
				log.Printf("[WS-Session] Timeout sending exit message")
			}
		} else {
			// Same pattern for normal exit
			done := make(chan struct{}, 1)
			go func() {
				defer func() { done <- struct{}{} }()
				if conn.UnderlyingConn() != nil {
					conn.SetWriteDeadline(time.Now().Add(300 * time.Millisecond))
					conn.WriteJSON(wsMessage{
						Type: msgTypeExit,
						Code: 0,
					})
					log.Printf("[WS-Session] Sent exit message with code 0")
				}
			}()

			select {
			case <-done:
			case <-time.After(400 * time.Millisecond):
				log.Printf("[WS-Session] Timeout sending exit message")
			}
		}

		// Now close the done channel to signal termination
		close(doneCh)
	}()

	// Wait for termination
	select {
	case err := <-errorCh:
		// Attempt to send error message without blocking
		errMsg := wsMessage{
			Type:    msgTypeError,
			Message: fmt.Sprintf("Session error: %v", err),
		}

		// Try to send the error message with a timeout
		writeCtx, writeCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer writeCancel()

		done := make(chan struct{})
		go func() {
			defer close(done)
			if conn.UnderlyingConn() != nil {
				conn.SetWriteDeadline(time.Now().Add(300 * time.Millisecond))
				conn.WriteJSON(errMsg)
			}
		}()

		select {
		case <-done:
		case <-writeCtx.Done():
			log.Printf("[WS-Session] Timeout sending error message")
		}

	case <-doneCh:
		// Normal termination, no action needed
	case <-ctx.Done():
		// Context cancelled
	}

	// Update access log to closed status
	accessLogCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = h.accessLogService.UpdateAccessLogStatus(accessLogCtx, machine.ID, tempUsername, model.StatusClosed)
	cancel()
	if err != nil {
		log.Printf("[WS-Session] Error updating access log status: %v", err)
	}

	// Clean up ephemeral account after session ends
	_, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cleanupCancel()
	if err := h.cleanupEphemeralAccount(machine, tempUsername); err != nil {
		log.Printf("[WS-Session] Error cleaning up ephemeral account %s: %v", tempUsername, err)
	} else {
		log.Printf("[WS-Session] Ephemeral account %s cleaned up successfully", tempUsername)
	}

	// Safely close WebSocket - use a new mutex to prevent multiple close attempts
	if conn.UnderlyingConn() != nil {
		// First try to send a proper close message
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 300*time.Millisecond)

		closeComplete := make(chan struct{})
		go func() {
			defer close(closeComplete)
			err := conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Session ended"),
				time.Now().Add(200*time.Millisecond),
			)
			if err != nil {
				if !strings.Contains(err.Error(), "websocket: close sent") {
					log.Printf("[WS-Session] Error sending close message: %v", err)
				}
			}
		}()

		// Wait for close message to be sent or timeout
		select {
		case <-closeComplete:
			log.Printf("[WS-Session] Close message sent successfully")
		case <-closeCtx.Done():
			log.Printf("[WS-Session] Close message send timed out")
		}
		closeCancel()

		// Force close the connection after attempting clean close
		conn.Close()
	}

	log.Printf("[WS-Session] Session ended for user %s, machine %s, temp user %s (duration: %v)",
		userID, machine.ID, tempUsername, time.Since(sessionStart))
}

// createEphemeralAccount creates a temporary user account on the target machine
func (h *ShellHandler) createEphemeralAccount(ctx context.Context, machine *model.Machine, username, userID string, ttlHours int) (string, error) {
	// Generate a temporary username: t_<slug(user.name)>_<8charRand>
	sanitizedName := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r + 32 // Convert to lowercase
		}
		return '_'
	}, username)

	// Ensure sanitizedName doesn't exceed 20 chars (to leave room for prefix and random suffix)
	if len(sanitizedName) > 20 {
		sanitizedName = sanitizedName[:20]
	}

	// Generate random suffix
	randBytes := make([]byte, 4)
	rand.Read(randBytes)
	randomSuffix := hex.EncodeToString(randBytes)[:8]

	tempUsername := fmt.Sprintf("t_%s_%s", sanitizedName, randomSuffix)

	// Calculate expiry time
	expiryTime := time.Now().Add(time.Hour * time.Duration(ttlHours))

	// Check if this temp user already exists in our logs
	existingLog, err := h.accessLogService.GetActiveAccessLog(ctx, machine.ID, tempUsername)
	if err == nil && existingLog != nil {
		// User already exists, update expiry
		log.Printf("[WS] Temporary user %s already exists, updating expiry", tempUsername)
		if err := h.updateEphemeralAccountExpiry(machine, tempUsername, expiryTime); err != nil {
			log.Printf("[WS] Error updating expiry for existing temp user: %v", err)
			return "", err
		}

		// Update access log
		if err := h.accessLogService.UpdateAccessLogExpiry(ctx, existingLog.ID, expiryTime); err != nil {
			log.Printf("[WS] Error updating access log expiry: %v", err)
			// Continue anyway as the user account was updated
		}

		return tempUsername, nil
	}

	// Create new ephemeral account on the machine
	if err := h.provisionEphemeralAccount(machine, tempUsername, expiryTime); err != nil {
		log.Printf("[WS] Error creating ephemeral account: %v", err)
		return "", err
	}

	// Create access log - use a new background context with timeout to avoid cancellation
	accessLogCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	accessLog := model.AccessLog{
		UserID:        userID,
		MachineID:     machine.ID,
		TempUsername:  tempUsername,
		ExpiresAt:     expiryTime,
		SessionStatus: model.StatusProvisioned,
	}

	if err := h.accessLogService.CreateAccessLog(accessLogCtx, accessLog); err != nil {
		log.Printf("[WS] Error creating access log: %v", err)

		// Try to cleanup the created user with a new context
		_, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()

		cleanupErr := h.cleanupEphemeralAccount(machine, tempUsername)
		if cleanupErr != nil {
			log.Printf("[WS] Failed to cleanup user after access log error: %v", cleanupErr)
			// Continue anyway
		}

		return "", err
	}

	return tempUsername, nil
}

// provisionEphemeralAccount creates a temporary user on the target machine
func (h *ShellHandler) provisionEphemeralAccount(machine *model.Machine, tempUsername string, expiryTime time.Time) error {
	// Create SSH client
	sshClient, err := h.createSSHClient(machine)
	if err != nil {
		return fmt.Errorf("failed to create SSH client: %w", err)
	}
	defer sshClient.Close()

	// First check if useradd exists and get its version/capabilities
	stdout, stderr, err := h.executeSSHCommandWithOutput(sshClient, "command -v useradd")
	if err != nil || stdout == "" {
		log.Printf("[SSH] useradd command not found: %v, stderr: %s", err, stderr)
		return fmt.Errorf("useradd command not available on target system")
	}
	log.Printf("[SSH] Found useradd at: %s", strings.TrimSpace(stdout))

	// Create user with home dir and bash shell - use basic options for maximum compatibility
	createUserCmd := fmt.Sprintf("sudo useradd -m -s /bin/bash %s", tempUsername)
	log.Printf("[SSH] Creating user with command: %s", createUserCmd)

	stdout, stderr, err = h.executeSSHCommandWithOutput(sshClient, createUserCmd)
	if err != nil {
		log.Printf("[SSH] Failed to create user: %v", err)
		log.Printf("[SSH] Command stdout: %s", stdout)
		log.Printf("[SSH] Command stderr: %s", stderr)
		return fmt.Errorf("failed to create user: %s", stderr)
	}

	// Set expiry date
	expiryDateStr := expiryTime.Format("2006-01-02")
	expiryCmd := fmt.Sprintf("sudo chage -E %s %s", expiryDateStr, tempUsername)
	stdout, stderr, err = h.executeSSHCommandWithOutput(sshClient, expiryCmd)
	if err != nil {
		log.Printf("[SSH] Failed to set expiry: %v", err)
		log.Printf("[SSH] Command stderr: %s", stderr)
		return fmt.Errorf("failed to set expiry: %s", stderr)
	}

	// Ensure .ssh directory exists with proper permissions
	sshDirCmd := fmt.Sprintf("sudo mkdir -p /home/%s/.ssh && sudo chmod 700 /home/%s/.ssh && sudo chown %s:%s /home/%s/.ssh",
		tempUsername, tempUsername, tempUsername, tempUsername, tempUsername)
	stdout, stderr, err = h.executeSSHCommandWithOutput(sshClient, sshDirCmd)
	if err != nil {
		log.Printf("[SSH] Failed to create .ssh directory: %v", err)
		log.Printf("[SSH] Command stderr: %s", stderr)
		return fmt.Errorf("failed to create .ssh directory: %s", stderr)
	}

	log.Printf("[SSH] Successfully provisioned user %s with expiry %s", tempUsername, expiryDateStr)
	return nil
}

// updateEphemeralAccountExpiry updates the expiry time for an existing ephemeral account
func (h *ShellHandler) updateEphemeralAccountExpiry(machine *model.Machine, tempUsername string, expiryTime time.Time) error {
	// Create SSH client
	sshClient, err := h.createSSHClient(machine)
	if err != nil {
		return fmt.Errorf("failed to create SSH client: %w", err)
	}
	defer sshClient.Close()

	// Set new expiry date
	expiryDateStr := expiryTime.Format("2006-01-02")
	expiryCmd := fmt.Sprintf("sudo chage -E %s %s", expiryDateStr, tempUsername)
	_, stderr, err := h.executeSSHCommandWithOutput(sshClient, expiryCmd)
	if err != nil {
		log.Printf("[SSH] Failed to update expiry: %v", err)
		log.Printf("[SSH] Command stderr: %s", stderr)
		return fmt.Errorf("failed to update expiry: %s", stderr)
	}

	return nil
}

// cleanupEphemeralAccount removes a temporary user from the target machine
func (h *ShellHandler) cleanupEphemeralAccount(machine *model.Machine, tempUsername string) error {
	// Create SSH client
	sshClient, err := h.createSSHClient(machine)
	if err != nil {
		return fmt.Errorf("failed to create SSH client: %w", err)
	}
	defer sshClient.Close()

	// Remove user and home directory
	removeUserCmd := fmt.Sprintf("sudo userdel -r %s", tempUsername)
	_, stderr, err := h.executeSSHCommandWithOutput(sshClient, removeUserCmd)
	if err != nil {
		log.Printf("[SSH] Failed to remove user: %v", err)
		log.Printf("[SSH] Command stderr: %s", stderr)
		return fmt.Errorf("failed to remove user: %s", stderr)
	}

	return nil
}

// createSSHClient creates an SSH client for the specified machine
func (h *ShellHandler) createSSHClient(machine *model.Machine) (*ssh.Client, error) {
	log.Printf("[SSH] Creating SSH client for machine %s (%s) with auth method: %s",
		machine.ID, machine.IPAddress, machine.AuthMethod)

	// Create SSH config based on machine auth method
	config := &ssh.ClientConfig{
		User:            "ubuntu", // Use service account configured for machine management
		Auth:            []ssh.AuthMethod{},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // In production, use proper host key verification
		Timeout:         10 * time.Second,
	}

	// Add authentication method based on machine config
	switch machine.AuthMethod {
	case string(model.AuthPassword):
		if machine.Password == "" {
			return nil, fmt.Errorf("machine configured for password authentication but no password provided")
		}
		config.Auth = append(config.Auth, ssh.Password(machine.Password))

	case string(model.AuthKey):
		if machine.Key == "" {
			return nil, fmt.Errorf("machine configured for key authentication but no key provided")
		}
		// Ensure the key is properly formatted (might need to add headers/footers)
		keyData := ensureProperKeyFormat(machine.Key)
		signer, err := ssh.ParsePrivateKey([]byte(keyData))
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		config.Auth = append(config.Auth, ssh.PublicKeys(signer))

	case string(model.AuthBoth):
		// Add both authentication methods
		if machine.Password != "" {
			config.Auth = append(config.Auth, ssh.Password(machine.Password))
		}

		if machine.Key != "" {
			keyData := ensureProperKeyFormat(machine.Key)
			signer, err := ssh.ParsePrivateKey([]byte(keyData))
			if err != nil {
				log.Printf("[SSH] Warning: Failed to parse private key: %v, falling back to password auth", err)
				// Continue with password auth if available
				if machine.Password == "" {
					return nil, fmt.Errorf("both key and password auth failed")
				}
			} else {
				config.Auth = append(config.Auth, ssh.PublicKeys(signer))
			}
		} else if machine.Password == "" {
			return nil, fmt.Errorf("no authentication method available")
		}

	default:
		return nil, fmt.Errorf("unsupported authentication method: %s", machine.AuthMethod)
	}

	// Strip CIDR notation from IP address if present
	ipAddress := machine.IPAddress
	if idx := strings.Index(ipAddress, "/"); idx > 0 {
		ipAddress = ipAddress[:idx]
	}

	// Connect to the SSH server
	addr := fmt.Sprintf("%s:22", ipAddress)
	log.Printf("[SSH] Connecting to %s as %s", addr, config.User)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial SSH server: %w", err)
	}

	return client, nil
}

// Helper function to ensure the key is properly formatted
func ensureProperKeyFormat(key string) string {
	// Check if the key already has the proper format
	if strings.HasPrefix(key, "-----BEGIN") {
		return key
	}

	// Otherwise, add the necessary headers and footers for an RSA key
	// This is a simplification - in reality, you would need to determine the key type
	return "-----BEGIN RSA PRIVATE KEY-----\n" + key + "\n-----END RSA PRIVATE KEY-----"
}

// executeSSHCommand executes a command on the SSH server
func (h *ShellHandler) executeSSHCommand(client *ssh.Client, command string) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	return session.Run(command)
}

// executeSSHCommandWithOutput executes a command and returns stdout/stderr
func (h *ShellHandler) executeSSHCommandWithOutput(client *ssh.Client, command string) (string, string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	var stdoutBuf, stderrBuf strings.Builder
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	err = session.Run(command)
	if err != nil {
		return stdoutBuf.String(), stderrBuf.String(), err
	}

	return stdoutBuf.String(), stderrBuf.String(), nil
}

// generateSSHKeyPair creates a new SSH key pair for temporary use
func generateSSHKeyPair() (string, string, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	// Generate private key in PEM format
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	privateKeyBytes := pem.EncodeToMemory(privateKeyPEM)

	// Generate public key
	pub, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", err
	}
	publicKeyBytes := ssh.MarshalAuthorizedKey(pub)

	return string(publicKeyBytes), string(privateKeyBytes), nil
}

var ansiRegexp = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\a]*\a`)
