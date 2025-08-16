// Package service provides business logic implementations for the application.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/codaxa/tunnelR.git/internal/api/core/model"
	"github.com/codaxa/tunnelR.git/internal/api/core/repository"
)

var (
	// ErrTeamNotFound indicates a team was not found
	ErrTeamNotFound = errors.New("team not found")
	// ErrUserNotInTeam indicates a user is not a member of a team
	ErrUserNotInTeam = errors.New("user is not a member of this team")
	// ErrUnauthorizedRole indicates a user doesn't have the required role for an operation
	ErrUnauthorizedRole = errors.New("user does not have the required role for this operation")
)

// TeamService handles team-related operations
type TeamService struct {
	teamRepository repository.TeamRepository
	userRepository repository.UserRepository
}

// NewTeamService creates and returns a new TeamService instance
func NewTeamService(teamRepo repository.TeamRepository, userRepo repository.UserRepository) *TeamService {
	return &TeamService{
		teamRepository: teamRepo,
		userRepository: userRepo,
	}
}

// CreateTeam creates a new team and adds the creator as a member
// Only users with admin role can create teams
func (s *TeamService) CreateTeam(ctx context.Context, name, userID string) (string, error) {
	// Check if user has admin role
	user, err := s.userRepository.GetUserByID(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	if user == nil {
		return "", fmt.Errorf("user not found")
	}

	// Check if user is an admin
	if !user.IsAdmin() {
		return "", ErrUnauthorizedRole
	}

	// Initialize team with current time for timestamps
	now := time.Now()
	team := model.Team{
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Validate team
	if err := team.Validate(); err != nil {
		return "", fmt.Errorf("invalid team data: %w", err)
	}

	// Create team and add user in a single transaction
	teamID, err := s.teamRepository.CreateTeam(ctx, team, userID)
	if err != nil {
		return "", fmt.Errorf("failed to create team: %w", err)
	}

	return teamID, nil
}

// GetTeamsByUserID retrieves all teams that a user is a member of
func (s *TeamService) GetTeamsByUserID(ctx context.Context, userID string) ([]*model.Team, error) {
	teams, err := s.teamRepository.GetTeamsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get teams: %w", err)
	}

	return teams, nil
}

// GetTeamByID retrieves a team by its ID, checking if the user has access
func (s *TeamService) GetTeamByID(ctx context.Context, teamID, userID string) (*model.Team, error) {
	// Check if user is in team
	inTeam, err := s.teamRepository.IsUserInTeam(ctx, userID, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to check team membership: %w", err)
	}

	if !inTeam {
		return nil, ErrUserNotInTeam
	}

	// Get team
	team, err := s.teamRepository.GetTeamByID(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to get team: %w", err)
	}

	if team == nil {
		return nil, ErrTeamNotFound
	}

	return team, nil
}
