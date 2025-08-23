// Package repository contains actual implementation for repo interfaces
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/codaxa/tunnelR.git/internal/api/core/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TeamRepository is a repository that provides team storage operations using PostgreSQL database
type TeamRepository struct {
	db *pgxpool.Pool
}

// NewTeamRepository creates and returns a new TeamRepository instance with the provided database connection
func NewTeamRepository(db *pgxpool.Pool) *TeamRepository {
	return &TeamRepository{
		db: db,
	}
}

// CreateTeam creates a new team and adds the creator as a member in a single transaction
func (r *TeamRepository) CreateTeam(ctx context.Context, team model.Team, userID string) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to start transaction: %w", err)
	}

	// Ensure transaction is rolled back on error
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				// Log rollback error, but return the original error
				fmt.Printf("Error rolling back transaction: %v\n", rbErr)
			}
		}
	}()

	// Insert team
	var teamID string
	query := `INSERT INTO teams (name) VALUES ($1) RETURNING id`
	if err = tx.QueryRow(ctx, query, team.Name).Scan(&teamID); err != nil {
		return "", fmt.Errorf("failed to create team: %w", err)
	}

	// Add user to team
	query = `INSERT INTO user_teams (user_id, team_id) VALUES ($1, $2)`
	if _, err = tx.Exec(ctx, query, userID, teamID); err != nil {
		return "", fmt.Errorf("failed to add user to team: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	return teamID, nil
}

// GetTeamByID retrieves a team from the database by its ID
func (r *TeamRepository) GetTeamByID(ctx context.Context, teamID string) (*model.Team, error) {
	query := `SELECT id, name, created_at, updated_at FROM teams WHERE id = $1`
	row := r.db.QueryRow(ctx, query, teamID)

	var team model.Team
	if err := row.Scan(&team.ID, &team.Name, &team.CreatedAt, &team.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &team, nil
}

// GetTeamsByUserID retrieves all teams that a user is a member of
func (r *TeamRepository) GetTeamsByUserID(ctx context.Context, userID string) ([]*model.Team, error) {
	query := `
        SELECT t.id, t.name, t.created_at, t.updated_at 
        FROM teams t 
        JOIN user_teams ut ON t.id = ut.team_id 
        WHERE ut.user_id = $1
    `

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []*model.Team
	for rows.Next() {
		var team model.Team
		if err := rows.Scan(&team.ID, &team.Name, &team.CreatedAt, &team.UpdatedAt); err != nil {
			return nil, err
		}
		teams = append(teams, &team)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return teams, nil
}

// AddUserToTeam adds a user to a team
func (r *TeamRepository) AddUserToTeam(ctx context.Context, userID, teamID string) error {
	query := `INSERT INTO user_teams (user_id, team_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	_, err := r.db.Exec(ctx, query, userID, teamID)
	return err
}

// RemoveUserFromTeam removes a user from a team
func (r *TeamRepository) RemoveUserFromTeam(ctx context.Context, userID, teamID string) error {
	query := `DELETE FROM user_teams WHERE user_id = $1 AND team_id = $2`
	_, err := r.db.Exec(ctx, query, userID, teamID)
	return err
}

// IsUserInTeam checks if a user is a member of a team
func (r *TeamRepository) IsUserInTeam(ctx context.Context, userID, teamID string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM user_teams WHERE user_id = $1 AND team_id = $2)`

	var exists bool
	if err := r.db.QueryRow(ctx, query, userID, teamID).Scan(&exists); err != nil {
		return false, err
	}

	return exists, nil
}

// GetTeamUsers gets team users
func (r *TeamRepository) GetTeamUsers(ctx context.Context, teamID string) ([]model.User, error) {
	query := `SELECT id, username, role, created_at, updated_at FROM users WHERE ID IN(SELECT user_id FROM user_teams WHERE team_id = $1)`

	rows, err := r.db.Query(ctx, query, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var user model.User
		if err := rows.Scan(&user.ID, &user.Username, &user.Role, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

// GetMachinesByTeamID retrieves all machines that belong to a team
func (r *TeamRepository) GetMachinesByTeamID(ctx context.Context, teamID string) ([]*model.Machine, error) {
	query := `SELECT id, hostname, ip_address::text, created_at, updated_at FROM machines WHERE ID IN (SELECT machine_id FROM machine_teams WHERE team_id = $1)`

	rows, err := r.db.Query(ctx, query, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var machines []*model.Machine
	for rows.Next() {
		var machine model.Machine
		if err := rows.Scan(&machine.ID, &machine.Hostname, &machine.IPAddress, &machine.CreatedAt, &machine.UpdatedAt); err != nil {
			return nil, err
		}
		machines = append(machines, &machine)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return machines, nil
}

// DeleteTeam deletes a team and all associated user_team relationships
func (r *TeamRepository) DeleteTeam(ctx context.Context, teamID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	// Ensure transaction is rolled back on error
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				fmt.Printf("Error rolling back transaction: %v\n", rbErr)
			}
		}
	}()

	// Delete from user_teams first (respecting foreign key constraints)
	if _, err = tx.Exec(ctx, `DELETE FROM user_teams WHERE team_id = $1`, teamID); err != nil {
		return fmt.Errorf("failed to delete team memberships: %w", err)
	}

	// Now delete the team
	if _, err = tx.Exec(ctx, `DELETE FROM teams WHERE id = $1`, teamID); err != nil {
		return fmt.Errorf("failed to delete team: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetTeamByName retrieves a team by its name
func (r *TeamRepository) GetTeamByName(ctx context.Context, name string) (*model.Team, error) {
	query := `SELECT id, name, created_at, updated_at FROM teams WHERE name = $1`
	row := r.db.QueryRow(ctx, query, name)

	var team model.Team
	err := row.Scan(&team.ID, &team.Name, &team.CreatedAt, &team.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query team by name: %w", err)
	}

	return &team, nil
}
