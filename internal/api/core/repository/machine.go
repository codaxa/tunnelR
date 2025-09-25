// Package repository defines interfaces for data access operations
package repository

import (
	"context"

	"github.com/codaxa/tunnelR.git/internal/api/core/model"
)

// MachineRepository defines operations for managing machines in the data store
type MachineRepository interface {
	CreateMachine(ctx context.Context, machine model.Machine) (string, error)

	UpdateMachine(ctx context.Context, machine model.Machine) error

	GetMachineByID(ctx context.Context, machineID string) (*model.Machine, error)
	GetMachineByIPAddress(ctx context.Context, ipAddress string) (*model.Machine, error)
	GetTeamsByMachineID(ctx context.Context, machineID string) ([]*model.Team, error)

	AddTeamToMachine(ctx context.Context, machineID string, teamID string) error
	RemoveTeamFromMachine(ctx context.Context, machineID string, teamID string) error

	CanUserUseMachine(ctx context.Context, machineID string, userID string) (bool, error)
	CanTeamUseMachine(ctx context.Context, machineID string, teamID string) (bool, error)

	DeleteMachine(ctx context.Context, machineID string) error
}
