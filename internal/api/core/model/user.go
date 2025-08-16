// Package model contains the domain models for the backend application.
package model

import "time"

// RoleAdmin is the const admin role
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleReadonly = "readonly"
)

// User represents a user in the backend application.
type User struct {
	ID        string    `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Username  string    `json:"username" gorm:"type:varchar(50);uniqueIndex;not null" validate:"required,min=3,max=50,alphanum"`
	Password  string    `json:"password" gorm:"type:varchar(64);not null" validate:"required,sha256"`
	Role      string    `json:"role" gorm:"type:varchar(20);default:readonly;check:role IN ('admin','operator','readonly')" validate:"required,oneof=admin operator user readonly"`
	CreatedAt time.Time `json:"created_at" gorm:"default:now();autoCreateTime" validate:"required"`
	UpdatedAt time.Time `json:"updated_at" gorm:"default:now();autoUpdateTime" validate:"required"`
}

// TableName returns the table name for the User model
func (User) TableName() string {
	return "users"
}

// Validate checks the User fields for validity.
func (u *User) Validate() error {
	return validate.Struct(u)
}

// IsAdmin checks if the user has admin role
func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

// IsOperator checks if the user has operator role
func (u *User) IsOperator() bool {
	return u.Role == RoleOperator
}

// CanManageUsers checks if user can manage other users
func (u *User) CanManageUsers() bool {
	return u.Role == RoleAdmin
}

// HasSSHAccess checks if user can request SSH access
func (u *User) HasSSHAccess() bool {
	return u.Role != RoleReadonly
}
