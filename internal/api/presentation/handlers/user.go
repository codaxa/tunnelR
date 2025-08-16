package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/codaxa/tunnelR.git/internal/api/app/service"
	"github.com/codaxa/tunnelR.git/internal/api/core/model"
)

// AuthServicer defines the authentication service interface
type AuthServicer interface {
	Register(ctx context.Context, username, password, role string) error
	Login(ctx context.Context, username, password string) (string, error)
}

// UserHandler handles HTTP requests related to user operations.
type UserHandler struct {
	authService AuthServicer
}

// NewUserHandler creates and returns a new UserHandler instance.
func NewUserHandler(authService AuthServicer) *UserHandler {
	return &UserHandler{authService: authService}
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
