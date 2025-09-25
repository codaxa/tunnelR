// Package repository defines interfaces for data access operations
package repository

import (
	"context"

	"github.com/codaxa/tunnelR.git/internal/api/core/model"
)

// TeamRepository defines operations for managing teams in the data store
type TeamRepository interface {
	// CreateTeam creates a new team and adds the creator as a member
	CreateTeam(ctx context.Context, team model.Team, userID string) (string, error)

	// GetTeamByID retrieves a team by its ID
	GetTeamByID(ctx context.Context, teamID string) (*model.Team, error)

	// GetTeamsByUserID retrieves all teams that a user is a member of
	GetTeamsByUserID(ctx context.Context, userID string) ([]*model.Team, error)

	// AddUserToTeam adds a user to a team
	AddUserToTeam(ctx context.Context, userID, teamID string) error

	// RemoveUserFromTeam removes a user from a team
	RemoveUserFromTeam(ctx context.Context, userID, teamID string) error

	// IsUserInTeam checks if a user is a member of a team
	IsUserInTeam(ctx context.Context, userID, teamID string) (bool, error)

	// DeleteTeam deletes a team by its ID
	DeleteTeam(ctx context.Context, teamID string) error

	// GetTeamByName retrieves a team by its name
	GetTeamByName(ctx context.Context, name string) (*model.Team, error)

	GetTeamUsers(ctx context.Context, teamID string) ([]model.User, error)

	GetMachinesByTeamID(ctx context.Context, teamID string) ([]model.Machine, error)
}
