// Package handlers provides HTTP handlers for the backend application.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/codaxa/tunnelR.git/internal/api/app/service"
	"github.com/codaxa/tunnelR.git/internal/api/core/model"
	"github.com/codaxa/tunnelR.git/internal/api/presentation/utils"
	"github.com/go-chi/chi"
)

// MachineHandler handles HTTP requests related to machine operations
type MachineHandler struct {
	machineService *service.MachineService
}

// NewMachineHandler creates and returns a new MachineHandler instance
func NewMachineHandler(machineService *service.MachineService) *MachineHandler {
	return &MachineHandler{
		machineService: machineService,
	}
}

// CreateMachine handles the creation of a new machine
func (h *MachineHandler) CreateMachine(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Hostname   string `json:"hostname"`
		IPAddress  string `json:"ip_address"`
		AuthMethod string `json:"auth_method"`
		Password   string `json:"password"`
		Key        string `json:"key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	if req.Hostname == "" {
		http.Error(w, "Machine name is required", http.StatusBadRequest)
		return
	}

	if req.IPAddress == "" {
		http.Error(w, "Machine IP address is required", http.StatusBadRequest)
		return
	}

	if req.AuthMethod == "" {
		http.Error(w, "Machine authentication method is required", http.StatusBadRequest)
		return
	}

	machineID, err := h.machineService.CreateMachine(r.Context(), req.Hostname, req.IPAddress, req.AuthMethod, req.Password, req.Key)
	if err != nil {
		if errors.Is(err, service.ErrIPAddressExists) {
			http.Error(w, "Machine with this IP address already exists", http.StatusConflict)
			return
		}

		log.Printf("Error creating machine: %v", err)
		http.Error(w, "Failed to create machine", http.StatusInternalServerError)
		return
	}

	resp := struct {
		MachineID string `json:"machine_id"`
		Hostname  string `json:"hostname"`
		IPAddress string `json:"ip_address"`
	}{
		MachineID: machineID,
		Hostname:  req.Hostname,
		IPAddress: req.IPAddress,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// UpdateMachine handles the update of an existing machine
func (h *MachineHandler) UpdateMachine(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID         string `json:"id"`
		Hostname   string `json:"hostname"`
		IPAddress  string `json:"ip_address"`
		AuthMethod string `json:"auth_method"`
		Password   string `json:"password"`
		Key        string `json:"key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	err := h.machineService.UpdateMachine(r.Context(), req.ID, req.Hostname, req.IPAddress, req.AuthMethod, req.Password, req.Key)
	if err != nil {
		if errors.Is(err, service.ErrMachineNotFound) {
			http.Error(w, "Machine is not found", http.StatusNotFound)
			return
		}

		log.Printf("Error updating machine: %v", err)
		http.Error(w, "Failed to update machine", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleMachineRequest is a helper function to reduce code duplication
func (h *MachineHandler) handleMachineRequest(w http.ResponseWriter, r *http.Request,
	getMachine func(context.Context, string, string) (*model.Machine, error),
	param string, paramName string) {

	paramValue := chi.URLParam(r, param)
	if paramValue == "" {
		http.Error(w, paramName+" is required", http.StatusBadRequest)
		return
	}

	userID, err := authutils.ExtractUserID(r)
	if err != nil {
		log.Printf("Error extracting user ID: %v", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	machine, err := getMachine(r.Context(), paramValue, userID)
	if err != nil {
		if errors.Is(err, service.ErrMachineNotFound) {
			http.Error(w, "Machine is not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, service.ErrUserCanNotUseMachine) {
			http.Error(w, "Unauthorized", http.StatusForbidden)
			return
		}
		log.Printf("Error getting machine: %v", err)
		http.Error(w, "Failed to retrieve machine", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(machine); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// GetMachineByID handles the retrieval of a machine by its ID
func (h *MachineHandler) GetMachineByID(w http.ResponseWriter, r *http.Request) {
	h.handleMachineRequest(w, r, h.machineService.GetMachineByID, "id", "Machine ID")
}

// GetMachineByIPAddress handles the retrieval of a machine by its IP address
func (h *MachineHandler) GetMachineByIPAddress(w http.ResponseWriter, r *http.Request) {
	h.handleMachineRequest(w, r, h.machineService.GetMachineByIPAddress, "ip", "Machine IP address")
}

// GetTeamsForMachine handles the retrieval of teams that have access to a machine
func (h *MachineHandler) GetTeamsForMachine(w http.ResponseWriter, r *http.Request) {
	machineID := chi.URLParam(r, "id")
	teams, err := h.machineService.GetTeamsByMachineID(r.Context(), machineID)
	if err != nil {
		log.Printf("Error getting teams: %v", err)
		http.Error(w, "Failed to retrieve teams", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(teams); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// handleTeamMachineRequest is a helper function for team-machine operations
func (h *MachineHandler) handleTeamMachineRequest(w http.ResponseWriter, r *http.Request,
	operation func(context.Context, string, string) error,
	successMessage string) {

	var req struct {
		TeamID    string `json:"team_id"`
		MachineID string `json:"machine_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	if req.TeamID == "" {
		http.Error(w, "Team ID is required", http.StatusBadRequest)
		return
	}

	if req.MachineID == "" {
		http.Error(w, "Machine ID is required", http.StatusBadRequest)
		return
	}

	err := operation(r.Context(), req.MachineID, req.TeamID)
	if err != nil {
		if errors.Is(err, service.ErrMachineNotFound) {
			http.Error(w, "Machine is not found", http.StatusNotFound)
			return
		}

		if errors.Is(err, service.ErrTeamNotFound) {
			http.Error(w, "Team is not found", http.StatusNotFound)
			return
		}

		log.Printf("Error %s: %v", successMessage, err)
		http.Error(w, "Failed to "+successMessage, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AddTeamToMachine handles adding a team to a machine
func (h *MachineHandler) AddTeamToMachine(w http.ResponseWriter, r *http.Request) {
	h.handleTeamMachineRequest(w, r, h.machineService.AddTeamToMachine, "adding team to machine")
}

// RemoveTeamFromMachine handles removing a team from a machine
func (h *MachineHandler) RemoveTeamFromMachine(w http.ResponseWriter, r *http.Request) {
	h.handleTeamMachineRequest(w, r, h.machineService.RemoveTeamFromMachine, "removing team from machine")
}

// DeleteMachine handles the deletion of a machine
func (h *MachineHandler) DeleteMachine(w http.ResponseWriter, r *http.Request) {
	machineID := chi.URLParam(r, "id")
	if machineID == "" {
		http.Error(w, "Machine ID is required", http.StatusBadRequest)
		return
	}

	err := h.machineService.DeleteMachine(r.Context(), machineID)
	if err != nil {
		if errors.Is(err, service.ErrMachineNotFound) {
			http.Error(w, "Machine is not found", http.StatusNotFound)
			return
		}
		log.Printf("Error deleting machine: %v", err)
		http.Error(w, "Failed to delete machine", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
