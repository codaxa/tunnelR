// Package model contains the domain models for the backend application.
package model

import "time"

// Team represents a team in the system
type Team struct {
	ID        string    `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name      string    `json:"name" gorm:"type:varchar(100);not null" validate:"required,min=3,max=100"`
	CreatedAt time.Time `json:"created_at" gorm:"default:now();autoCreateTime" validate:"required"`
	UpdatedAt time.Time `json:"updated_at" gorm:"default:now();autoUpdateTime" validate:"required"`
}

// TableName returns the table name for the Team model
func (Team) TableName() string {
	return "teams"
}

// Validate checks the Team fields for validity
func (t *Team) Validate() error {
	return validate.Struct(t)
}

// UserTeam represents the many-to-many relationship between users and teams
type UserTeam struct {
	UserID    string    `json:"user_id" gorm:"type:uuid;primaryKey"`
	TeamID    string    `json:"team_id" gorm:"type:uuid;primaryKey"`
	CreatedAt time.Time `json:"created_at" gorm:"default:now();autoCreateTime" validate:"required"`
}

// TableName returns the table name for the UserTeam model
func (UserTeam) TableName() string {
	return "user_teams"
}
