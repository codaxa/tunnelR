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

// MachineRepository is a repository that provides machine storage operations using PostgreSQL database
type MachineRepository struct {
	db *pgxpool.Pool
}

// NewMachineRepository creates and returns a new MachineRepository instance with the provided database connection
func NewMachineRepository(db *pgxpool.Pool) *MachineRepository {
	return &MachineRepository{
		db: db,
	}
}

// CreateMachine inserts a new machine into the database
func (r *MachineRepository) CreateMachine(ctx context.Context, machine model.Machine) (string, error) {
	var machineID string
	query := `INSERT INTO machines (hostname, ip_address, auth_method, password, key) VALUES ($1, $2, $3, $4, $5) RETURNING id`
	err := r.db.QueryRow(ctx, query, machine.Hostname, machine.IPAddress, machine.AuthMethod, machine.Password, machine.Key).Scan(&machineID)
	if err != nil {
		return "", fmt.Errorf("failed to create machine: %w", err)
	}

	return machineID, nil
}

// UpdateMachine updates an existing machine in the database
func (r *MachineRepository) UpdateMachine(ctx context.Context, machine model.Machine) error {
	query := `UPDATE machines SET hostname = $1, ip_address = $2, auth_method = $3, password = $4, key = $5 WHERE id = $6`
	cmdTag, err := r.db.Exec(ctx, query, machine.Hostname, machine.IPAddress, machine.AuthMethod, machine.Password, machine.Key, machine.ID)
	if err != nil {
		return fmt.Errorf("failed to update machine: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("no machine found with id: %s", machine.ID)
	}

	return nil
}

// GetMachineByID retrieves a machine from the database by its ID
func (r *MachineRepository) GetMachineByID(ctx context.Context, id string) (*model.Machine, error) {
	query := `
        SELECT id, hostname, ip_address::text, auth_method, password, key, created_at, updated_at 
        FROM machines 
        WHERE id = $1
    `

	var machine model.Machine
	err := r.db.QueryRow(ctx, query, id).Scan(
		&machine.ID,
		&machine.Hostname,
		&machine.IPAddress,
		&machine.AuthMethod,
		&machine.Password,
		&machine.Key,
		&machine.CreatedAt,
		&machine.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("error fetching machine: %w", err)
	}

	return &machine, nil
}

// GetMachineByIPAddress retrieves a machine from the database by its IP address
func (r *MachineRepository) GetMachineByIPAddress(ctx context.Context, ipAddress string) (*model.Machine, error) {
	query := `SELECT id, hostname, ip_address::text, auth_method, created_at, updated_at FROM machines WHERE ip_address = $1`
	row := r.db.QueryRow(ctx, query, ipAddress)

	var machine model.Machine
	if err := row.Scan(&machine.ID, &machine.Hostname, &machine.IPAddress, &machine.AuthMethod, &machine.CreatedAt, &machine.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &machine, nil
}

// GetTeamsByMachineID retrieves all teams that have access to a machine
func (r *MachineRepository) GetTeamsByMachineID(ctx context.Context, machineID string) ([]*model.Team, error) {
	query := `
        SELECT t.id, t.name, t.created_at, t.updated_at 
        FROM teams t 
        JOIN machine_teams mt ON t.id = mt.team_id 
        WHERE mt.machine_id = $1
    `

	rows, err := r.db.Query(ctx, query, machineID)
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

// AddTeamToMachine adds a team to a machine, granting access
func (r *MachineRepository) AddTeamToMachine(ctx context.Context, machineID, teamID string) error {
	query := `INSERT INTO machine_teams (machine_id, team_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	_, err := r.db.Exec(ctx, query, machineID, teamID)
	return err
}

// RemoveTeamFromMachine removes a team from a machine, revoking access
func (r *MachineRepository) RemoveTeamFromMachine(ctx context.Context, machineID string, teamID string) error {
	query := `DELETE FROM machine_teams WHERE machine_id = $1 AND team_id = $2`
	cmdTag, err := r.db.Exec(ctx, query, machineID, teamID)
	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("no relation found with machine with id: %s and team with id: %s", machineID, teamID)
	}
	return err
}

// CanUserUseMachine checks if a user has access to a machine through team membership
func (r *MachineRepository) CanUserUseMachine(ctx context.Context, machineID string, userID string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM machine_teams WHERE machine_id = $1 AND team_id in (SELECT team_id FROM user_teams WHERE user_id = $2))`

	var isEligible bool
	if err := r.db.QueryRow(ctx, query, machineID, userID).Scan(&isEligible); err != nil {
		return false, err
	}

	return isEligible, nil
}

// CanTeamUseMachine checks if a team has access to a machine
func (r *MachineRepository) CanTeamUseMachine(ctx context.Context, machineID string, teamID string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM machine_teams WHERE machine_id = $1 AND team_id = $2)`

	var eligible bool
	if err := r.db.QueryRow(ctx, query, machineID, teamID).Scan(&eligible); err != nil {
		return false, err
	}

	return eligible, nil
}

// DeleteMachine deletes a machine and all associated team relationships
func (r *MachineRepository) DeleteMachine(ctx context.Context, machineID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				fmt.Printf("Error rolling back transaction: %v\n", rbErr)
			}
		}
	}()

	if _, err = tx.Exec(ctx, `DELETE FROM machine_teams WHERE machine_id = $1`, machineID); err != nil {
		return fmt.Errorf("failed to delete team assignment to machine: %w", err)
	}

	cmdTag, err := tx.Exec(ctx, `DELETE FROM machines WHERE id = $1`, machineID)
	if err != nil {
		return fmt.Errorf("failed to delete machine: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("no machine found with id: %s", machineID)
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
