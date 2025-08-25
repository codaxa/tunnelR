package model

import (
	"time"
)

// SessionStatus represents the status of a shell session
type SessionStatus string

// StatusProvisioned represents the session status when it has been successfully provisioned but is not yet active.
const (
	StatusProvisioned SessionStatus = "provisioned"
	StatusActive      SessionStatus = "active"
	StatusClosed      SessionStatus = "closed"
	StatusFailed      SessionStatus = "failed"
)

// AccessLog represents a record of SSH access to a machine
type AccessLog struct {
	ID            string        `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID        string        `json:"user_id" gorm:"type:uuid;not null"`
	MachineID     string        `json:"machine_id" gorm:"type:uuid;not null"`
	TempUsername  string        `json:"temp_username" gorm:"type:varchar(50);not null"`
	ExpiresAt     time.Time     `json:"expires_at" gorm:"not null"`
	CreatedAt     time.Time     `json:"created_at" gorm:"default:now();autoCreateTime"`
	UpdatedAt     time.Time     `json:"updated_at" gorm:"default:now();autoUpdateTime"`
	SessionStatus SessionStatus `json:"session_status" gorm:"type:varchar(20);default:'provisioned'"`
}

// TableName returns the table name for the AccessLog model
func (AccessLog) TableName() string {
	return "access_logs"
}
