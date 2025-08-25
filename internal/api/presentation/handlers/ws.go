package handlers

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

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
		// Create temporary username
		tempUsername, err := h.createEphemeralAccount(r.Context(), machine, username, userID, ttlHours)
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
		h.handleWebSocketSession(r.Context(), conn, machine, userID, tempUsername, ttlHours)
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
		User:            tempUsername,
		Auth:            []ssh.AuthMethod{},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	// Get the temporary user's password by executing a command as root
	// This is necessary because we created the user but need a way to authenticate as that user
	randomPasswordBytes := make([]byte, 16)
	rand.Read(randomPasswordBytes)
	randomPassword := hex.EncodeToString(randomPasswordBytes)

	// Set password for temp user
	setPasswordCmd := fmt.Sprintf("echo '%s:%s' | sudo chpasswd", tempUsername, randomPassword)
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

	err = rootSession.Run(setPasswordCmd)
	rootSession.Close()
	if err != nil {
		log.Printf("[WS-Session] Error setting password for temp user: %v", err)
		outputCh <- wsMessage{
			Type:    msgTypeError,
			Code:    "PROVISION_FAILED",
			Message: "Failed to set up user account",
		}
		close(doneCh)
		return
	}

	// Add password authentication for the temp user
	tempUserConfig.Auth = append(tempUserConfig.Auth, ssh.Password(randomPassword))

	// Connect as the temporary user
	tempUserClient, err := ssh.Dial("tcp", machine.IPAddress+":22", tempUserConfig)
	if err != nil {
		log.Printf("[WS-Session] Error connecting as temp user: %v", err)
		outputCh <- wsMessage{
			Type:    msgTypeError,
			Code:    "LOGIN_FAILED",
			Message: "Failed to login as temporary user",
		}
		close(doneCh)
		return
	}
	defer tempUserClient.Close()

	// Create session as the temporary user
	session, err := tempUserClient.NewSession()
	if err != nil {
		log.Printf("[WS-Session] Error creating SSH session: %v", err)
		outputCh <- wsMessage{
			Type:    msgTypeError,
			Code:    "SESSION_FAILED",
			Message: "Failed to create SSH session",
		}
		close(doneCh)
		return
	}
	defer session.Close()

	// Update access log to active status
	err = h.accessLogService.UpdateAccessLogStatus(ctx, machine.ID, tempUsername, model.StatusActive)
	if err != nil {
		log.Printf("[WS-Session] Error updating access log status: %v", err)
		// Continue anyway, not critical
	}

	// Set up pipes for I/O
	stdin, err := session.StdinPipe()
	if err != nil {
		log.Printf("[WS-Session] Error getting stdin pipe: %v", err)
		close(doneCh)
		return
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		log.Printf("[WS-Session] Error getting stdout pipe: %v", err)
		close(doneCh)
		return
	}

	stderr, err := session.StderrPipe()
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
	if err := session.RequestPty("xterm", initialCols, initialRows, modes); err != nil {
		log.Printf("[WS-Session] Error requesting PTY: %v", err)
		close(doneCh)
		return
	}

	// Start shell with restricted options
	if err := session.Start("/bin/bash --noprofile --norc"); err != nil {
		log.Printf("[WS-Session] Error starting shell: %v", err)
		close(doneCh)
		return
	}

	// Send system message
	outputCh <- wsMessage{
		Type: msgTypeSystem,
		Message: fmt.Sprintf("Connected to %s as temporary user %s. Session will expire in %s.",
			machine.Hostname, tempUsername, time.Until(time.Now().Add(time.Hour*time.Duration(ttlHours))).Round(time.Minute)),
	}

	// Handle stdout
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stdout.Read(buf)
			if err != nil {
				if err.Error() != "EOF" {
					log.Printf("[WS-Session] Error reading from stdout: %v", err)
					errorCh <- err
				}
				return
			}

			outputCh <- wsMessage{
				Type: msgTypeStdout,
				Data: string(buf[:n]),
			}
		}
	}()

	// Handle stderr
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stderr.Read(buf)
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
				_, err := stdin.Write([]byte(message.Data))
				if err != nil {
					log.Printf("[WS-Session] Error writing to stdin: %v", err)
					errorCh <- err
					return
				}
			case msgTypeResize:
				// Note: Window change parameters are (height, width) for SSH but (cols, rows) for the client
				// So we need to swap them to match the expected order
				err := session.WindowChange(message.Rows, message.Cols)
				if err != nil {
					log.Printf("[WS-Session] Error resizing window: %v", err)
					// Non-fatal error, continue
				}
			}
		}
	}()

	// Wait for session to end
	go func() {
		err := session.Wait()
		if err != nil {
			log.Printf("[WS-Session] SSH session ended with error: %v", err)
			var exitCode int
			if exitErr, ok := err.(*ssh.ExitError); ok {
				exitCode = exitErr.ExitStatus()
			} else {
				exitCode = 1
			}

			outputCh <- wsMessage{
				Type: msgTypeExit,
				Code: exitCode,
			}
		} else {
			outputCh <- wsMessage{
				Type: msgTypeExit,
				Code: 0,
			}
		}
		close(doneCh)
	}()

	// Wait for termination
	select {
	case err := <-errorCh:
		// Send error message
		errMsg := wsMessage{
			Type:    msgTypeError,
			Message: fmt.Sprintf("Session error: %v", err),
		}
		conn.WriteJSON(errMsg)
	case <-doneCh:
		// Normal termination
	case <-ctx.Done():
		// Context cancelled
	}

	// Update access log to closed status
	err = h.accessLogService.UpdateAccessLogStatus(ctx, machine.ID, tempUsername, model.StatusClosed)
	if err != nil {
		log.Printf("[WS-Session] Error updating access log status: %v", err)
		// Continue anyway, not critical
	}

	// Close WebSocket with normal closure
	if err := conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Session ended"),
		time.Now().Add(time.Second),
	); err != nil {
		log.Printf("[WS-Session] Error closing WebSocket: %v", err)
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

	// Create access log
	accessLog := model.AccessLog{
		UserID:        userID,
		MachineID:     machine.ID,
		TempUsername:  tempUsername,
		ExpiresAt:     expiryTime,
		SessionStatus: model.StatusProvisioned,
	}

	if err := h.accessLogService.CreateAccessLog(ctx, accessLog); err != nil {
		log.Printf("[WS] Error creating access log: %v", err)
		// Try to cleanup the created user
		h.cleanupEphemeralAccount(machine, tempUsername)
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

	// Create user with home dir and bash shell
	createUserCmd := fmt.Sprintf("sudo useradd -m -s /bin/bash -U -k /etc/skel %s", tempUsername)
	if err := h.executeSSHCommand(sshClient, createUserCmd); err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	// Set expiry date
	expiryDateStr := expiryTime.Format("2006-01-02")
	expiryCmd := fmt.Sprintf("sudo chage -E %s %s", expiryDateStr, tempUsername)
	if err := h.executeSSHCommand(sshClient, expiryCmd); err != nil {
		return fmt.Errorf("failed to set expiry: %w", err)
	}

	// Ensure .ssh directory exists
	sshDirCmd := fmt.Sprintf("sudo mkdir -p /home/%s/.ssh && sudo chmod 700 /home/%s/.ssh && sudo chown %s:%s /home/%s/.ssh",
		tempUsername, tempUsername, tempUsername, tempUsername, tempUsername)
	if err := h.executeSSHCommand(sshClient, sshDirCmd); err != nil {
		return fmt.Errorf("failed to create .ssh directory: %w", err)
	}

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
	if err := h.executeSSHCommand(sshClient, expiryCmd); err != nil {
		return fmt.Errorf("failed to update expiry: %w", err)
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
	if err := h.executeSSHCommand(sshClient, removeUserCmd); err != nil {
		return fmt.Errorf("failed to remove user: %w", err)
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
