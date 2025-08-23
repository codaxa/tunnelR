// Package handlers provides HTTP handlers for the backend application.
package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	appContext "github.com/codaxa/tunnelR.git/internal/api/app/context"
	"github.com/codaxa/tunnelR.git/internal/api/app/service"
	"github.com/go-chi/chi"
	"github.com/golang-jwt/jwt"
)

// TeamHandler handles HTTP requests related to team operations
type TeamHandler struct {
	teamService *service.TeamService
}

// NewTeamHandler creates and returns a new TeamHandler instance
func NewTeamHandler(teamService *service.TeamService) *TeamHandler {
	return &TeamHandler{
		teamService: teamService,
	}
}

// extractUserID extracts the user ID from the JWT claims in the request context
func extractUserID(r *http.Request) (string, error) {
	ctxValue := r.Context().Value(appContext.UserClaimsKey)
	if ctxValue == nil {
		return "", errors.New("no user claims in context")
	}

	var normalizedClaims map[string]interface{}
	switch v := ctxValue.(type) {
	case *jwt.MapClaims:
		if v == nil {
			return "", errors.New("nil user claims")
		}
		normalizedClaims = *v
	case jwt.MapClaims:
		normalizedClaims = v
	default:
		return "", fmt.Errorf("unexpected claims type: %T", ctxValue)
	}

	userID, ok := normalizedClaims["user_id"].(string)
	if !ok {
		return "", errors.New("user_id not found in claims or not a string")
	}

	return userID, nil
}

// CreateTeam handles the creation of a new team
func (h *TeamHandler) CreateTeam(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Team name is required", http.StatusBadRequest)
		return
	}

	// Extract user ID from token
	userID, err := extractUserID(r)
	if err != nil {
		log.Printf("Error extracting user ID: %v", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Create team
	teamID, err := h.teamService.CreateTeam(r.Context(), req.Name, userID)
	if err != nil {
		if errors.Is(err, service.ErrTeamNameExists) {
			http.Error(w, "Team with this name already exists", http.StatusConflict)
			return
		}

		log.Printf("Error creating team: %v", err)
		http.Error(w, "Failed to create team", http.StatusInternalServerError)
		return
	}

	// Return success response
	resp := struct {
		TeamID string `json:"team_id"`
		Name   string `json:"name"`
	}{
		TeamID: teamID,
		Name:   req.Name,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// GetTeams handles the retrieval of teams that the user is a member of
func (h *TeamHandler) GetTeams(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from token
	userID, err := extractUserID(r)
	if err != nil {
		log.Printf("Error extracting user ID: %v", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get teams
	teams, err := h.teamService.GetTeamsByUserID(r.Context(), userID)
	if err != nil {
		log.Printf("Error getting teams: %v", err)
		http.Error(w, "Failed to retrieve teams", http.StatusInternalServerError)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(teams); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// GetTeam handles the retrieval of a specific team
func (h *TeamHandler) GetTeam(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	if teamID == "" {
		http.Error(w, "Team ID is required", http.StatusBadRequest)
		return
	}

	// Extract user ID from token
	userID, err := extractUserID(r)
	if err != nil {
		log.Printf("Error extracting user ID: %v", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get team
	team, err := h.teamService.GetTeamByID(r.Context(), teamID, userID)
	if err != nil {
		if errors.Is(err, service.ErrTeamNotFound) {
			http.Error(w, "Team not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, service.ErrUserNotInTeam) {
			http.Error(w, "Unauthorized", http.StatusForbidden)
			return
		}
		log.Printf("Error getting team: %v", err)
		http.Error(w, "Failed to retrieve team", http.StatusInternalServerError)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(team); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// DeleteTeam handles the deletion of a team
func (h *TeamHandler) DeleteTeam(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	if teamID == "" {
		http.Error(w, "Team ID is required", http.StatusBadRequest)
		return
	}

	// Delete the team
	err := h.teamService.DeleteTeam(r.Context(), teamID)
	if err != nil {
		if errors.Is(err, service.ErrTeamNotFound) {
			http.Error(w, "Team not found", http.StatusNotFound)
			return
		}
		log.Printf("Error deleting team: %v", err)
		http.Error(w, "Failed to delete team", http.StatusInternalServerError)
		return
	}

	// Return success response with no content
	w.WriteHeader(http.StatusNoContent)
}

// AddUserToTeam handles adding a user to a team
func (h *TeamHandler) AddUserToTeam(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	if teamID == "" {
		http.Error(w, "Team ID is required", http.StatusBadRequest)
		return
	}

	userID := chi.URLParam(r, "userId")
	if userID == "" {
		http.Error(w, "User ID is required", http.StatusBadRequest)
		return
	}

	// Add user to team
	if err := h.teamService.AddUserToTeam(r.Context(), teamID, userID); err != nil {
		if errors.Is(err, service.ErrTeamNotFound) {
			http.Error(w, "Team not found", http.StatusNotFound)
			return
		}
		log.Printf("Error adding user to team: %v", err)
		http.Error(w, "Failed to add user to team", http.StatusInternalServerError)
		return
	}

	// Return success with no content
	w.WriteHeader(http.StatusNoContent)
}

// RemoveUserFromTeam handles removing a user from a team
func (h *TeamHandler) RemoveUserFromTeam(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	if teamID == "" {
		http.Error(w, "Team ID is required", http.StatusBadRequest)
		return
	}

	userID := chi.URLParam(r, "userId")
	if userID == "" {
		http.Error(w, "User ID is required", http.StatusBadRequest)
		return
	}

	// Remove user from team
	if err := h.teamService.RemoveUserFromTeam(r.Context(), teamID, userID); err != nil {
		if errors.Is(err, service.ErrTeamNotFound) {
			http.Error(w, "Team not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, service.ErrUserNotInTeam) {
			http.Error(w, "User is not a member of this team", http.StatusBadRequest)
			return
		}
		log.Printf("Error removing user from team: %v", err)
		http.Error(w, "Failed to remove user from team", http.StatusInternalServerError)
		return
	}

	// Return success with no content
	w.WriteHeader(http.StatusNoContent)
}
