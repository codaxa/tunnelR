package repository

import (
	"context"
	"time"

	"github.com/codaxa/tunnelR.git/internal/api/core/model"
)

// AccessLogRepository defines operations for managing access logs in the data store
type AccessLogRepository interface {
	CreateAccessLog(ctx context.Context, accessLog model.AccessLog) error
	GetActiveAccessLog(ctx context.Context, machineID, tempUsername string) (*model.AccessLog, error)
	UpdateAccessLogExpiry(ctx context.Context, accessLogID string, expiresAt time.Time) error
	UpdateAccessLogStatus(ctx context.Context, machineID, tempUsername string, status model.SessionStatus) error
}
