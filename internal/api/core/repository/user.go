// Package repository defines interfaces for data access operations
package repository

import (
	"context"

	"github.com/codaxa/tunnelR.git/internal/api/core/model"
)

// UserRepository defines operations for managing users in the data store
type UserRepository interface {
	GetUserByUsername(ctx context.Context, username string) (*model.User, error)
	CreateUser(ctx context.Context, u model.User) error
}
