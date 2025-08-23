// Package service provides business logic implementations for the application.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/codaxa/tunnelR.git/internal/api/core/model"
	"github.com/codaxa/tunnelR.git/internal/api/core/repository"
)

// ErrMachineNotFound indicates a machine was not found
var (
	ErrMachineNotFound       = errors.New("machine not found")
	ErrTeamCanNotUseMachine  = errors.New("team is not eligible to use this machine")
	ErrUserCanNotUseMachine  = errors.New("user is not eligible to use this machine")
	ErrIPAddressExists       = errors.New("a machine with this IP address already exists")
	ErrInvalidAuthMethod     = errors.New("invalid auth_method. Must be one of these: password, key or both")
	ErrMissingPassword       = errors.New("missing password value, please make sure to set it")
	ErrMissingKey            = errors.New("missing key value, please make sure to set it")
	ErrMissingPasswordAndKey = errors.New("missing password and key values, please make sure to set them")
)

// MachineService handles machine-related operations
type MachineService struct {
	teamRepository    repository.TeamRepository
	machineRepository repository.MachineRepository
}

// NewMachineService creates and returns a new MachineService instance
func NewMachineService(teamRepo repository.TeamRepository, machineRepo repository.MachineRepository) *MachineService {
	return &MachineService{
		teamRepository:    teamRepo,
		machineRepository: machineRepo,
	}
}

// CreateMachine creates a new machine with the provided details
func (s *MachineService) CreateMachine(ctx context.Context, hostname, ipAddress, authMethod, password, key string) (string, error) {
	switch authMethod {
	case string(model.AuthPassword):
		if password == "" {
			return "", ErrMissingPassword
		}
	case string(model.AuthKey):
		if key == "" {
			return "", ErrMissingKey
		}
	case string(model.AuthBoth):
		if password == "" || key == "" {
			return "", ErrMissingPasswordAndKey
		}
	default:
		return "", ErrInvalidAuthMethod
	}

	machine := model.Machine{
		Hostname:   hostname,
		IPAddress:  ipAddress,
		AuthMethod: authMethod,
		Password:   password,
		Key:        key,
	}

	if err := machine.Validate(); err != nil {
		return "", fmt.Errorf("invalid machine data: %w", err)
	}

	machineID, err := s.machineRepository.CreateMachine(ctx, machine)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") ||
			strings.Contains(err.Error(), "Duplicate entry") {
			return "", ErrIPAddressExists
		}
		return "", fmt.Errorf("failed to create machine: %w", err)
	}

	return machineID, nil
}

// UpdateMachine updates an existing machine with the provided details
func (s *MachineService) UpdateMachine(ctx context.Context, machineID, hostname, ipAddress, authMethod, password, key string) error {
	if authMethod != "" {
		switch authMethod {
		case string(model.AuthPassword):
			if password == "" {
				return ErrMissingPassword
			}
		case string(model.AuthKey):
			if key == "" {
				return ErrMissingKey
			}
		case string(model.AuthBoth):
			if password == "" || key == "" {
				return ErrMissingPasswordAndKey
			}
		default:
			return ErrInvalidAuthMethod
		}
	}

	machine, err := s.machineRepository.GetMachineByID(ctx, machineID)
	if err != nil {
		return ErrMachineNotFound
	}

	if hostname != "" {
		machine.Hostname = hostname
	}

	if ipAddress != "" {
		machine.IPAddress = ipAddress
	}

	switch authMethod {
	case string(model.AuthPassword):
		machine.AuthMethod = authMethod
		machine.Password = password
	case string(model.AuthKey):
		machine.AuthMethod = authMethod
		machine.Key = key
	case string(model.AuthBoth):
		machine.AuthMethod = authMethod
		machine.Password = password
		machine.Key = key
	}

	if err := machine.Validate(); err != nil {
		return fmt.Errorf("invalid machine data: %w", err)
	}

	err = s.machineRepository.UpdateMachine(ctx, *machine)
	if err != nil {
		return fmt.Errorf("failed to update machine: %w", err)
	}

	return nil
}

// GetMachineByID retrieves a machine by its ID, checking if the user has access
func (s *MachineService) GetMachineByID(ctx context.Context, machineID string, userID string) (*model.Machine, error) {
	machine, err := s.machineRepository.GetMachineByID(ctx, machineID)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine: %w", err)
	}

	if machine == nil {
		return nil, ErrMachineNotFound
	}

	canUserUseMachine, err := s.machineRepository.CanUserUseMachine(ctx, machineID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to check user authorization: %w", err)
	}

	if !canUserUseMachine {
		return nil, ErrUserCanNotUseMachine
	}

	return machine, nil
}

// GetMachineByIPAddress retrieves a machine by its IP address, checking if the user has access
func (s *MachineService) GetMachineByIPAddress(ctx context.Context, ipAddress string, userID string) (*model.Machine, error) {
	machine, err := s.machineRepository.GetMachineByIPAddress(ctx, ipAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine: %w", err)
	}

	if machine == nil {
		return nil, ErrMachineNotFound
	}

	canUserUseMachine, err := s.machineRepository.CanUserUseMachine(ctx, machine.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to check user authorization: %w", err)
	}

	if !canUserUseMachine {
		return nil, ErrUserCanNotUseMachine
	}

	return machine, nil
}

// GetTeamsByMachineID retrieves all teams that have access to a machine
func (s *MachineService) GetTeamsByMachineID(ctx context.Context, machineID string) ([]*model.Team, error) {
	teams, err := s.machineRepository.GetTeamsByMachineID(ctx, machineID)
	if err != nil {
		return nil, fmt.Errorf("failed to get teams: %w", err)
	}

	return teams, nil
}

// AddTeamToMachine adds a team to a machine, granting access
func (s *MachineService) AddTeamToMachine(ctx context.Context, machineID string, teamID string) error {
	machine, err := s.machineRepository.GetMachineByID(ctx, machineID)
	if err != nil {
		return fmt.Errorf("failed to get machine: %w", err)
	}

	if machine == nil {
		return ErrMachineNotFound
	}

	team, err := s.teamRepository.GetTeamByID(ctx, teamID)
	if err != nil {
		return fmt.Errorf("failed to get team: %w", err)
	}

	if team == nil {
		return ErrTeamNotFound
	}

	err = s.machineRepository.AddTeamToMachine(ctx, machineID, teamID)
	if err != nil {
		return fmt.Errorf("failed to add team to machine: %w", err)
	}

	return nil
}

// RemoveTeamFromMachine removes a team from a machine, revoking access
func (s *MachineService) RemoveTeamFromMachine(ctx context.Context, machineID string, teamID string) error {
	machine, err := s.machineRepository.GetMachineByID(ctx, machineID)
	if err != nil {
		return fmt.Errorf("failed to get machine: %w", err)
	}

	if machine == nil {
		return ErrMachineNotFound
	}

	team, err := s.teamRepository.GetTeamByID(ctx, teamID)
	if err != nil {
		return fmt.Errorf("failed to get team: %w", err)
	}

	if team == nil {
		return ErrTeamNotFound
	}

	err = s.machineRepository.RemoveTeamFromMachine(ctx, machineID, teamID)
	if err != nil {
		return fmt.Errorf("failed to remove team from machine: %w", err)
	}

	return nil
}

// DeleteMachine deletes a machine by its ID
func (s *MachineService) DeleteMachine(ctx context.Context, machineID string) error {
	machine, err := s.machineRepository.GetMachineByID(ctx, machineID)
	if err != nil {
		return fmt.Errorf("failed to get machine: %w", err)
	}

	if machine == nil {
		return ErrMachineNotFound
	}

	if err := s.machineRepository.DeleteMachine(ctx, machineID); err != nil {
		return fmt.Errorf("failed to delete machine: %w", err)
	}

	return nil
}
