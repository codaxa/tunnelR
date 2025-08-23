// Package model contains the domain models for the backend application.
package model

import (
	"errors"
	"time"
)

// AuthMethod represents the authentication method for machine access
type AuthMethod string

// AuthPassword is the const password auth
const (
	AuthPassword AuthMethod = "password"
	AuthKey      AuthMethod = "key"
	AuthBoth     AuthMethod = "both"
)

// Machine represents a machine in the backend application.
type Machine struct {
	ID         string    `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Hostname   string    `json:"hostname" gorm:"type:varchar(50);not null" validate:"required,min=3,max=50,alphanum"`
	IPAddress  string    `json:"ip_address" gorm:"type:inet;uniqueIndex;not null"`
	AuthMethod string    `json:"auth_method" gorm:"type:varchar(20);default:readonly;check:auth_method IN ('password','key','both')" validate:"required,oneof=password key both"`
	Password   string    `gorm:"type:varchar(100)"`
	Key        string    `gorm:"type:varchar(100)"`
	CreatedAt  time.Time `json:"created_at" gorm:"default:now();autoCreateTime"`
	UpdatedAt  time.Time `json:"updated_at" gorm:"default:now();autoUpdateTime"`
}

// TableName returns the table name for the Machine model
func (Machine) TableName() string {
	return "machines"
}

// Validate checks the Machine fields for validity.
func (m *Machine) Validate() error {
	if err := validate.Struct(m); err != nil {
		return err
	}

	switch AuthMethod(m.AuthMethod) {
	case AuthPassword:
		if m.Password == "" {
			return errors.New("password is required when auth_method is 'password'")
		}
	case AuthKey:
		if m.Key == "" {
			return errors.New("key is required when auth_method is 'key'")
		}
	case AuthBoth:
		if m.Password == "" {
			return errors.New("password is required when auth_method is 'both'")
		}
		if m.Key == "" {
			return errors.New("key is required when auth_method is 'both'")
		}
	}

	return nil
}

// MachineTeam represents the many-to-many relationship between machines and teams
type MachineTeam struct {
	ID        string    `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	MachineID string    `json:"machine_id" gorm:"type:uuid;primaryKey;not null"`
	TeamID    string    `json:"team_id" gorm:"type:uuid;primaryKey;not null"`
	CreatedAt time.Time `json:"created_at" gorm:"default:now();autoCreateTime"`
	Machine   Machine   `gorm:"foreignKey:MachineID;references:ID"`
	Team      Team      `gorm:"foreignKey:TeamID;references:ID"`
}

// TableName returns the table name for the MachineTeam model
func (MachineTeam) TableName() string {
	return "machine_teams"
}
