package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/codaxa/tunnelR.git/internal/api/core/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AccessLogRepository is a repository that provides access log storage operations
type AccessLogRepository struct {
	db *pgxpool.Pool
}

// NewAccessLogRepository creates a new AccessLogRepository with the provided database connection
func NewAccessLogRepository(db *pgxpool.Pool) *AccessLogRepository {
	return &AccessLogRepository{
		db: db,
	}
}

// CreateAccessLog inserts a new access log into the database
func (r *AccessLogRepository) CreateAccessLog(ctx context.Context, accessLog model.AccessLog) error {
	query := `INSERT INTO access_logs (user_id, machine_id, temp_username, expires_at, session_status) 
              VALUES ($1, $2, $3, $4, $5)`
	_, err := r.db.Exec(ctx, query, accessLog.UserID, accessLog.MachineID,
		accessLog.TempUsername, accessLog.ExpiresAt, accessLog.SessionStatus)
	if err != nil {
		return fmt.Errorf("failed to create access log: %w", err)
	}
	return nil
}

// GetActiveAccessLog retrieves an active access log for a machine and temp username
func (r *AccessLogRepository) GetActiveAccessLog(ctx context.Context, machineID, tempUsername string) (*model.AccessLog, error) {
	query := `SELECT id, user_id, machine_id, temp_username, expires_at, created_at, updated_at, session_status 
              FROM access_logs 
              WHERE machine_id = $1 AND temp_username = $2 AND expires_at > NOW()
              AND session_status IN ('provisioned', 'active')
              ORDER BY created_at DESC LIMIT 1`

	row := r.db.QueryRow(ctx, query, machineID, tempUsername)

	var accessLog model.AccessLog
	err := row.Scan(&accessLog.ID, &accessLog.UserID, &accessLog.MachineID,
		&accessLog.TempUsername, &accessLog.ExpiresAt, &accessLog.CreatedAt,
		&accessLog.UpdatedAt, &accessLog.SessionStatus)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &accessLog, nil
}

// UpdateAccessLogExpiry updates the expiry time of an access log
func (r *AccessLogRepository) UpdateAccessLogExpiry(ctx context.Context, accessLogID string, expiresAt time.Time) error {
	query := `UPDATE access_logs SET expires_at = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(ctx, query, expiresAt, accessLogID)
	if err != nil {
		return fmt.Errorf("failed to update access log expiry: %w", err)
	}
	return nil
}

// UpdateAccessLogStatus updates the status of an access log
func (r *AccessLogRepository) UpdateAccessLogStatus(ctx context.Context, machineID, tempUsername string, status model.SessionStatus) error {
	query := `UPDATE access_logs 
              SET session_status = $1, updated_at = NOW() 
              WHERE machine_id = $2 AND temp_username = $3 AND expires_at > NOW()
              ORDER BY created_at DESC LIMIT 1`

	_, err := r.db.Exec(ctx, query, status, machineID, tempUsername)
	if err != nil {
		return fmt.Errorf("failed to update access log status: %w", err)
	}
	return nil
}
