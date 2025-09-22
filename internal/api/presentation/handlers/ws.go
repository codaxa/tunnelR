package handlers

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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

var ansiRegexp = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\a]*\a`)

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
			if err := conn.WriteJSON(errMsg); err != nil {
				log.Printf("[WS] Error writing JSON: %v", err)
			}
			if err := conn.Close(); err != nil {
				log.Printf("[WS] Error closing connection: %v", err)
			}
			return
		}

		// Start WebSocket session with the ephemeral account
		h.handleWebSocketSession(newCtx, conn, machine, userID, tempUsername, ttlHours)
	}()
}

// handleWebSocketSession manages the WebSocket connection for shell access
func (h *ShellHandler) handleWebSocketSession(ctx context.Context, conn *websocket.Conn, machine *model.Machine, userID, tempUsername string, ttlHours int) {
	sessionStart := time.Now()
	log.Printf("[WS-Session] Starting WebSocket session for user %s, machine %s, temp user %s", userID, machine.ID, tempUsername)

	// Setup WebSocket
	inputCh, outputCh, errorCh, doneCh, readDeadline, writeDeadline := h.setupWebSocketHandlers(ctx, conn)

	// Start heartbeat, reader, writer
	h.startWebSocketHeartbeat(ctx, conn, outputCh, errorCh, doneCh, readDeadline, writeDeadline)
	h.startWebSocketReader(ctx, conn, inputCh, outputCh, errorCh, doneCh, readDeadline)
	h.startWebSocketWriter(ctx, conn, outputCh, errorCh, doneCh, writeDeadline)

	// Setup ephemeral user and SSH session
	sshClient, _, _, err := h.setupEphemeralUserSession(machine, tempUsername, outputCh, doneCh)
	if err != nil {
		return
	}
	defer func() {
		if err := sshClient.Close(); err != nil {
			log.Printf("Error closing SSH client: %v", err)
		}
	}()

	rootSession, stdin, stdoutPipe, stderrPipe, err := h.startSSHSessionAsTempUser(sshClient, tempUsername, outputCh, doneCh, ttlHours)
	if err != nil {
		return
	}
	defer func() {
		if err := rootSession.Close(); err != nil {
			log.Printf("[WS-Session] Error closing root session: %v", err)
		}
	}()

	// Session lifecycle
	h.handleSessionLifecycle(ctx, conn, machine, tempUsername, ttlHours, sessionStart, inputCh, outputCh, errorCh, doneCh, rootSession, stdin, stdoutPipe, stderrPipe, userID)
}

// --- Helper Functions ---

func (h *ShellHandler) setupWebSocketHandlers(_ context.Context, conn *websocket.Conn) (
	inputCh chan wsMessage, outputCh chan wsMessage, errorCh chan error, doneCh chan struct{},
	readDeadline, writeDeadline time.Duration,
) {
	readDeadline = 11 * time.Minute
	writeDeadline = 10 * time.Second
	inputCh = make(chan wsMessage, 100)
	outputCh = make(chan wsMessage, 100)
	errorCh = make(chan error, 10)
	doneCh = make(chan struct{})

	_ = conn.SetReadDeadline(time.Now().Add(readDeadline))
	conn.SetPingHandler(func(data string) error {
		return conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(writeDeadline))
	})
	conn.SetPongHandler(func(_ string) error {
		return conn.SetReadDeadline(time.Now().Add(readDeadline))
	})
	return
}

func (h *ShellHandler) startWebSocketHeartbeat(
	ctx context.Context, conn *websocket.Conn, outputCh chan wsMessage, _ chan error, doneCh chan struct{},
	_, writeDeadline time.Duration,
) {
	pingInterval := 5 * time.Minute
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(writeDeadline))
				outputCh <- wsMessage{Type: msgTypePing, TS: time.Now().Unix()}
			case <-doneCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (h *ShellHandler) startWebSocketReader(
	_ context.Context, conn *websocket.Conn, inputCh chan wsMessage, outputCh chan wsMessage, errorCh chan error, doneCh chan struct{},
	readDeadline time.Duration,
) {
	go func() {
		defer close(inputCh)
		for {
			_, messageBytes, err := conn.ReadMessage()
			if err != nil {
				errorCh <- err
				return
			}
			_ = conn.SetReadDeadline(time.Now().Add(readDeadline))
			var message wsMessage
			if err := json.Unmarshal(messageBytes, &message); err != nil {
				continue
			}
			switch message.Type {
			case msgTypePing:
				outputCh <- wsMessage{Type: msgTypePong, TS: message.TS}
			case msgTypeClose:
				close(doneCh)
				return
			default:
				inputCh <- message
			}
		}
	}()
}

func (h *ShellHandler) startWebSocketWriter(
	_ context.Context, conn *websocket.Conn, outputCh chan wsMessage, errorCh chan error, doneCh chan struct{},
	writeDeadline time.Duration,
) {
	go func() {
		for {
			select {
			case message, ok := <-outputCh:
				if !ok {
					return
				}
				_ = conn.SetWriteDeadline(time.Now().Add(writeDeadline))
				if err := conn.WriteJSON(message); err != nil {
					errorCh <- err
					return
				}
			case <-doneCh:
				return
			}
		}
	}()
}

func (h *ShellHandler) setupEphemeralUserSession(machine *model.Machine, tempUsername string, outputCh chan wsMessage, doneCh chan struct{}) (*ssh.Client, *ssh.ClientConfig, string, error) {
	// Create SSH client as root (for system access)
	sshClient, randomPassword, err := h.setupEphemeralSSH(machine, tempUsername)
	if err != nil {
		log.Printf("[WS-Session] %v", err)
		outputCh <- wsMessage{
			Type:    msgTypeError,
			Code:    "PROVISION_FAILED",
			Message: "Failed to set up user account",
		}
		close(doneCh)
		return nil, nil, "", err
	}

	// Create a new client config for the temporary user
	tempUserConfig := &ssh.ClientConfig{
		User:    tempUsername,
		Auth:    []ssh.AuthMethod{},
		Timeout: 10 * time.Second,
	}

	// Get the temporary user's password by executing a command as root
	// This is necessary because we created the user but need a way to authenticate as that user
	randomPasswordBytes := make([]byte, 16)
	if _, err := rand.Read(randomPasswordBytes); err != nil {
		log.Printf("[WS-Session] Error generating random password: %v", err)
		return nil, nil, "", err
	}
	randomPassword = hex.EncodeToString(randomPasswordBytes)

	// Set password for temp user
	// After setting the password with chpasswd, check if it worked
	setPasswordCmd := fmt.Sprintf("echo '%s:%s' | sudo chpasswd", tempUsername, randomPassword)
	_, stderr, err := h.executeSSHCommandWithOutput(sshClient, setPasswordCmd)
	if err != nil {
		log.Printf("[WS-Session] Error setting password for temp user: %v", err)
		log.Printf("[WS-Session] Command stderr: %s", stderr)
		outputCh <- wsMessage{
			Type:    msgTypeError,
			Code:    "PROVISION_FAILED",
			Message: "Failed to set up user account password",
		}
		close(doneCh)
		return nil, nil, "", err
	}

	// Check if password authentication is allowed in sshd config
	checkSshdCmd := "grep -E '^PasswordAuthentication|^ChallengeResponseAuthentication' /etc/ssh/sshd_config"
	stdout, _, err := h.executeSSHCommandWithOutput(sshClient, checkSshdCmd)
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
		return nil, nil, "", err
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
		_, stderr, err = h.executeSSHCommandWithOutput(sshClient, authKeysCmd)
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
		ssh.KeyboardInteractive(func(_ string, _ string, questions []string, _ []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i := range questions {
				answers[i] = randomPassword
			}
			return answers, nil
		}),
	}

	return sshClient, tempUserConfig, randomPassword, nil
}

func (h *ShellHandler) startSSHSessionAsTempUser(sshClient *ssh.Client, tempUsername string, outputCh chan wsMessage, doneCh chan struct{}, ttlHours int) (*ssh.Session, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
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
		return nil, nil, nil, nil, err
	}

	// Set up pipes for I/O
	stdin, err := rootSession.StdinPipe()
	if err != nil {
		log.Printf("[WS-Session] Error getting stdin pipe: %v", err)
		close(doneCh)
		return nil, nil, nil, nil, err
	}

	stdoutPipe, err := rootSession.StdoutPipe()
	if err != nil {
		log.Printf("[WS-Session] Error getting stdout pipe: %v", err)
		close(doneCh)
		return nil, nil, nil, nil, err
	}

	stderrPipe, err := rootSession.StderrPipe()
	if err != nil {
		log.Printf("[WS-Session] Error getting stderr pipe: %v", err)
		close(doneCh)
		return nil, nil, nil, nil, err
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
		return nil, nil, nil, nil, err
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
		return nil, nil, nil, nil, err
	}

	// After starting the shell, send a welcome command
	time.Sleep(100 * time.Millisecond) // Small delay to let the shell initialize
	expiryTime := time.Now().Add(time.Hour * time.Duration(ttlHours))
	expiry := time.Until(expiryTime).Round(time.Minute)
	welcomeMsg := fmt.Sprintf("Shell session started. Your temporary account will expire in %s.", expiry)
	outputCh <- wsMessage{
		Type: msgTypeStdout,
		Data: welcomeMsg,
	}

	return rootSession, stdin, io.NopCloser(stdoutPipe), io.NopCloser(stderrPipe), nil
}

func (h *ShellHandler) handleSessionLifecycle(
	ctx context.Context,
	conn *websocket.Conn,
	machine *model.Machine,
	tempUsername string,
	ttlHours int,
	sessionStart time.Time,
	inputCh chan wsMessage,
	outputCh chan wsMessage,
	errorCh chan error,
	doneCh chan struct{},
	rootSession *ssh.Session,
	stdin io.WriteCloser,
	stdoutPipe io.ReadCloser,
	stderrPipe io.ReadCloser,
	userID string,
) {
	sessionTimeout := time.Duration(ttlHours+1) * time.Hour
	sessionTimeoutTimer := time.AfterFunc(sessionTimeout, func() {
		log.Printf("[WS-Session] Session timeout reached after %v", sessionTimeout)
		errorCh <- fmt.Errorf("session timeout reached")
	})
	defer sessionTimeoutTimer.Stop()

	h.updateAccessLogStatusActive(machine, tempUsername)
	h.sendSystemMessage(outputCh, machine, tempUsername, ttlHours)

	var lastCommand string

	go h.handleStdin(inputCh, stdin, errorCh, rootSession, &lastCommand)
	go h.handleStdout(stdoutPipe, outputCh, errorCh, &lastCommand)
	go h.handleStderr(stderrPipe, outputCh, errorCh)

	go h.waitForSessionEnd(conn, rootSession, doneCh, errorCh)

	h.waitForTermination(ctx, conn, machine, tempUsername, sessionStart, outputCh, errorCh, doneCh, userID)
}

func (h *ShellHandler) updateAccessLogStatusActive(machine *model.Machine, tempUsername string) {
	updateCtx, updateCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer updateCancel()
	err := h.accessLogService.UpdateAccessLogStatus(updateCtx, machine.ID, tempUsername, model.StatusActive)
	if err != nil {
		log.Printf("[WS-Session] Error updating access log status: %v", err)
	}
}

func (h *ShellHandler) sendSystemMessage(outputCh chan wsMessage, machine *model.Machine, tempUsername string, ttlHours int) {
	outputCh <- wsMessage{
		Type: msgTypeSystem,
		Message: fmt.Sprintf("Connected to %s as temporary user %s. Session will expire in %s.",
			machine.Hostname, tempUsername, time.Until(time.Now().Add(time.Hour*time.Duration(ttlHours))).Round(time.Minute)),
	}
}

func (h *ShellHandler) handleStdout(stdoutPipe io.ReadCloser, outputCh chan wsMessage, errorCh chan error, lastCommand *string) {
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
		if raw != "" {
			lines := strings.Split(raw, "\n")
			var filtered []string

			for _, line := range lines {
				cleanLine := ansiRegexp.ReplaceAllString(line, "")
				trimmed := strings.TrimSpace(cleanLine)

				if strings.HasPrefix(trimmed, "t_") && strings.HasSuffix(trimmed, "$") && strings.Contains(trimmed, "@") {
					parts := strings.Split(trimmed, "@")
					username := strings.Split(parts[0], "_")[1]
					cleanLine = strings.Join([]string{username, parts[1]}, "@")
					cleanLine += " "
				}
				if trimmed == "" {
					continue
				}

				if *lastCommand != "" && trimmed == *lastCommand {
					continue
				}

				filtered = append(filtered, cleanLine)
			}

			if len(filtered) > 0 {
				filteredOutput := strings.Join(filtered, "\n")
				outputCh <- wsMessage{
					Type: msgTypeStdout,
					Data: filteredOutput,
				}
			}
		}
	}
}

func (h *ShellHandler) handleStderr(stderrPipe io.ReadCloser, outputCh chan wsMessage, errorCh chan error) {
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
}

func (h *ShellHandler) handleStdin(inputCh chan wsMessage, stdin io.WriteCloser, errorCh chan error, rootSession *ssh.Session, lastCommand *string) {
	for message := range inputCh {
		switch message.Type {
		case msgTypeStdin:
			data := message.Data
			if strings.HasSuffix(data, "\n") {
				*lastCommand = strings.TrimSpace(data)
			}
			_, err := stdin.Write([]byte(data))
			if err != nil {
				log.Printf("[WS-Session] Error writing to stdin: %v", err)
				errorCh <- err
				return
			}
		case msgTypeResize:
			err := rootSession.WindowChange(message.Rows, message.Cols)
			if err != nil {
				log.Printf("[WS-Session] Error resizing window: %v", err)
			}
		}
	}
}

func (h *ShellHandler) waitForSessionEnd(conn *websocket.Conn, rootSession *ssh.Session, doneCh chan struct{}, _ chan error) {
	err := rootSession.Wait()
	select {
	case <-doneCh:
		log.Printf("[WS-Session] Session already terminated, not sending exit message")
		return
	default:
	}
	h.sendExitMessage(conn, err)
	close(doneCh)
}

func (h *ShellHandler) sendExitMessage(conn *websocket.Conn, err error) {
	var exitCode int
	if err != nil {
		log.Printf("[WS-Session] SSH session ended with error: %v", err)
		if exitErr, ok := err.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			exitCode = 1
			log.Printf("[WS-Session] SSH session terminated abnormally, using exit code %d", exitCode)
		}
	} else {
		exitCode = 0
	}
	done := make(chan struct{}, 1)
	go func() {
		defer func() { done <- struct{}{} }()
		if conn.UnderlyingConn() != nil {
			if err := conn.SetWriteDeadline(time.Now().Add(300 * time.Millisecond)); err != nil {
				log.Printf("[WS-Session] Error setting write deadline: %v", err)
			}
			if err := conn.WriteJSON(wsMessage{
				Type: msgTypeExit,
				Code: exitCode,
			}); err != nil {
				log.Printf("[WS-Session] Error writing JSON message: %v", err)
			}
			log.Printf("[WS-Session] Sent exit message with code %d", exitCode)
		}
	}()
	select {
	case <-done:
	case <-time.After(400 * time.Millisecond):
		log.Printf("[WS-Session] Timeout sending exit message")
	}
}

func (h *ShellHandler) waitForTermination(
	ctx context.Context,
	conn *websocket.Conn,
	machine *model.Machine,
	tempUsername string,
	sessionStart time.Time,
	_ chan wsMessage,
	errorCh chan error,
	doneCh chan struct{},
	userID string,
) {
	select {
	case err := <-errorCh:
		h.sendSessionError(conn, err)
	case <-doneCh:
	case <-ctx.Done():
	}
	h.updateAccessLogStatusClosed(machine, tempUsername)
	h.cleanupAndClose(conn, machine, tempUsername, sessionStart, userID)
}

func (h *ShellHandler) sendSessionError(conn *websocket.Conn, err error) {
	errMsg := wsMessage{
		Type:    msgTypeError,
		Message: fmt.Sprintf("Session error: %v", err),
	}
	writeCtx, writeCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer writeCancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		if conn.UnderlyingConn() != nil {
			if err := conn.SetWriteDeadline(time.Now().Add(300 * time.Millisecond)); err != nil {
				log.Printf("[WS-Session] Error setting write deadline: %v", err)
			}
			if err := conn.WriteJSON(errMsg); err != nil {
				log.Printf("[WS-Session] Error writing JSON message: %v", err)
			}
		}
	}()
	select {
	case <-done:
	case <-writeCtx.Done():
		log.Printf("[WS-Session] Timeout sending error message")
	}
}

func (h *ShellHandler) updateAccessLogStatusClosed(machine *model.Machine, tempUsername string) {
	accessLogCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := h.accessLogService.UpdateAccessLogStatus(accessLogCtx, machine.ID, tempUsername, model.StatusClosed)
	if err != nil {
		log.Printf("[WS-Session] Error updating access log status: %v", err)
	}
}

func (h *ShellHandler) cleanupAndClose(conn *websocket.Conn, machine *model.Machine, tempUsername string, sessionStart time.Time, userID string) {
	_, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cleanupCancel()
	if err := h.cleanupEphemeralAccount(machine, tempUsername); err != nil {
		log.Printf("[WS-Session] Error cleaning up ephemeral account %s: %v", tempUsername, err)
	} else {
		log.Printf("[WS-Session] Ephemeral account %s cleaned up successfully", tempUsername)
	}
	if conn.UnderlyingConn() != nil {
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
		select {
		case <-closeComplete:
			log.Printf("[WS-Session] Close message sent successfully")
		case <-closeCtx.Done():
			log.Printf("[WS-Session] Close message send timed out")
		}
		closeCancel()
		if err := conn.Close(); err != nil {
			log.Printf("[WS-Session] Error closing WebSocket connection: %v", err)
		}
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
	if _, err := rand.Read(randBytes); err != nil {
		log.Printf("[WS-Session] Error generating random bytes: %v", err)
		return "", err
	}
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
	defer func() {
		if err := sshClient.Close(); err != nil {
			log.Printf("[SSH] Error closing SSH client: %v", err)
		}
	}()

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
	_, stderr, err = h.executeSSHCommandWithOutput(sshClient, expiryCmd)
	if err != nil {
		log.Printf("[SSH] Failed to set expiry: %v", err)
		log.Printf("[SSH] Command stderr: %s", stderr)
		return fmt.Errorf("failed to set expiry: %s", stderr)
	}

	// Ensure .ssh directory exists with proper permissions
	sshDirCmd := fmt.Sprintf("sudo mkdir -p /home/%s/.ssh && sudo chmod 700 /home/%s/.ssh && sudo chown %s:%s /home/%s/.ssh",
		tempUsername, tempUsername, tempUsername, tempUsername, tempUsername)
	_, stderr, err = h.executeSSHCommandWithOutput(sshClient, sshDirCmd)
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
	defer func() {
		if err := sshClient.Close(); err != nil {
			log.Printf("[SSH] Error closing SSH client: %v", err)
		}
	}()

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
	defer func() {
		if err := sshClient.Close(); err != nil {
			log.Printf("[SSH] Error closing SSH client: %v", err)
		}
	}()

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

// executeSSHCommandWithOutput executes a command and returns stdout/stderr
func (h *ShellHandler) executeSSHCommandWithOutput(client *ssh.Client, command string) (string, string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", "", fmt.Errorf("failed to create session: %w", err)
	}
	defer func() {
		_ = session.Close()
	}()

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

// setupEphemeralSSH prepares SSH session, sets password, authorized_keys, and verifies user.
func (h *ShellHandler) setupEphemeralSSH(machine *model.Machine, tempUsername string) (*ssh.Client, string, error) {
	sshClient, err := h.createSSHClient(machine)
	if err != nil {
		return nil, "", fmt.Errorf("error creating SSH client: %w", err)
	}

	randomPasswordBytes := make([]byte, 16)
	if _, err := rand.Read(randomPasswordBytes); err != nil {
		if err := sshClient.Close(); err != nil {
			log.Printf("Error closing SSH client: %v", err)
		}
		return nil, "", fmt.Errorf("error generating random password: %w", err)
	}
	randomPassword := hex.EncodeToString(randomPasswordBytes)

	setPasswordCmd := fmt.Sprintf("echo '%s:%s' | sudo chpasswd", tempUsername, randomPassword)
	_, stderr, err := h.executeSSHCommandWithOutput(sshClient, setPasswordCmd)
	if err != nil {
		if err := sshClient.Close(); err != nil {
			log.Printf("Error closing SSH client: %v", err)
		}
		return nil, "", fmt.Errorf("error setting password: %s", stderr)
	}

	verifyUserCmd := fmt.Sprintf("id %s", tempUsername)
	_, stderr, err = h.executeSSHCommandWithOutput(sshClient, verifyUserCmd)
	if err != nil {
		if err := sshClient.Close(); err != nil {
			log.Printf("Error closing SSH client: %v", err)
		}
		return nil, "", fmt.Errorf("user verification failed: %s", stderr)
	}

	return sshClient, randomPassword, nil
}
