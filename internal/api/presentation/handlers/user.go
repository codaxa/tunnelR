package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	appctx "github.com/codaxa/tunnelR.git/internal/api/app/context"
	"github.com/codaxa/tunnelR.git/internal/api/app/service"
	"github.com/codaxa/tunnelR.git/internal/api/core/model"
	"github.com/codaxa/tunnelR.git/internal/api/presentation/utils"
	"github.com/golang-jwt/jwt"
)

// UserHandler handles HTTP requests related to user operations.
type UserHandler struct {
	authService *service.AuthService
}

// NewUserHandler creates and returns a new UserHandler instance.
func NewUserHandler(authService *service.AuthService) *UserHandler {
	return &UserHandler{
		authService: authService,
	}
}

type tokenResponse struct {
	Token string `json:"token"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Register handles HTTP requests for user registration.
func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("ERROR parsing request body: %v", err)
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)
	req.Role = strings.TrimSpace(req.Role)

	if req.Username == "" || req.Password == "" {
		http.Error(w, "Username, and password are required", http.StatusBadRequest)
		return
	}

	if req.Role == "" {
		req.Role = model.RoleReadonly
	}

	if req.Role != model.RoleAdmin && req.Role != model.RoleOperator && req.Role != model.RoleReadonly {
		http.Error(w, "Invalid role. Role should be one of: admin, operator or readonly.", http.StatusBadRequest)
		return
	}

	if len(req.Username) < 3 || len(req.Username) > 50 {
		http.Error(w, "Username must be between 3 and 50 characters", http.StatusBadRequest)
		return
	}

	if len(req.Password) < 8 {
		http.Error(w, "Password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	log.Printf("Registration request received for username: %s", req.Username)
	err := h.authService.Register(r.Context(), req.Username, req.Password, req.Role)
	if err != nil {
		log.Printf("ERROR in registration: %v", err)
		http.Error(w, "Failed to register user", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// Login handles user authentication requests.
func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	token, err := h.authService.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}
		log.Printf("ERROR during login: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	resp := tokenResponse{Token: token}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("ERROR encoding response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// GetUserMachines handles the retrieval of machines that a user has access to
func (h *UserHandler) GetUserMachines(w http.ResponseWriter, r *http.Request) {

	userID, err := authutils.ExtractUserID(r)
	if err != nil {
		log.Printf("Error extracting user ID: %v", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	machines, err := h.authService.GetMachinesByUserID(r.Context(), userID)
	if err != nil {
		log.Printf("Error getting machines: %v", err)
		http.Error(w, "Failed to retrieve machines", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(machines); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// GetUserInfo retrieves the current user's information from the JWT token
func (h *UserHandler) GetUserInfo(w http.ResponseWriter, r *http.Request) {
	// Get claims from context and handle both pointer and value types
	var normalizedClaims map[string]interface{}
	ctxValue := r.Context().Value(appctx.UserClaimsKey)

	switch v := ctxValue.(type) {
	case *jwt.MapClaims:
		if v == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		normalizedClaims = *v
	case jwt.MapClaims:
		normalizedClaims = v
	default:
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract username
	username, ok := normalizedClaims["username"].(string)
	if !ok {
		http.Error(w, "Invalid token claims", http.StatusUnauthorized)
		return
	}

	// Extract role
	role, ok := normalizedClaims["role"].(string)
	if !ok {
		http.Error(w, "Invalid token claims", http.StatusUnauthorized)
		return
	}

	response := struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}{
		Username: username,
		Role:     role,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("ERROR encoding response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}
