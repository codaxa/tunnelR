// Package repository conatins actual implementation for repo interface
package repository

import (
	"context"
	"errors"

	"github.com/codaxa/tunnelR.git/internal/api/core/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserRepository is a repository that provides user storage operations using PostgreSQL database
type UserRepository struct {
	db *pgxpool.Pool
}

// NewUserRepository creates and returns a new UserRepository instance with the provided database connection
func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{
		db: db,
	}
}

// GetUserByUsername retrieves a user from the database by their username
func (r *UserRepository) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	query := `SELECT id, username, password, role FROM users WHERE username = $1`
	row := r.db.QueryRow(ctx, query, username)
	var user model.User
	if err := row.Scan(&user.ID, &user.Username, &user.Password, &user.Role); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

// CreateUser inserts a new user into the database
func (r *UserRepository) CreateUser(ctx context.Context, u model.User) error {
	query := `INSERT INTO users (username, password, role) VALUES ($1, $2, $3)`
	_, err := r.db.Exec(ctx, query, u.Username, u.Password, u.Role)
	if err != nil {
		return err
	}
	return nil
}
