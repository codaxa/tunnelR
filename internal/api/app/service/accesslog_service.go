package service

import (
	"context"
	"time"

	"github.com/codaxa/tunnelR.git/internal/api/core/model"
	"github.com/codaxa/tunnelR.git/internal/api/core/repository"
)

// AccessLogService handles access log-related operations
type AccessLogService struct {
	accessLogRepository repository.AccessLogRepository
}

// NewAccessLogService creates a new AccessLogService
func NewAccessLogService(accessLogRepo repository.AccessLogRepository) *AccessLogService {
	return &AccessLogService{
		accessLogRepository: accessLogRepo,
	}
}

// CreateAccessLog creates a new access log entry
func (s *AccessLogService) CreateAccessLog(ctx context.Context, accessLog model.AccessLog) error {
	return s.accessLogRepository.CreateAccessLog(ctx, accessLog)
}

// GetActiveAccessLog retrieves an active access log for a machine and temporary username
func (s *AccessLogService) GetActiveAccessLog(ctx context.Context, machineID, tempUsername string) (*model.AccessLog, error) {
	return s.accessLogRepository.GetActiveAccessLog(ctx, machineID, tempUsername)
}

// UpdateAccessLogExpiry updates the expiry time of an access log
func (s *AccessLogService) UpdateAccessLogExpiry(ctx context.Context, accessLogID string, expiresAt time.Time) error {
	return s.accessLogRepository.UpdateAccessLogExpiry(ctx, accessLogID, expiresAt)
}

// UpdateAccessLogStatus updates the status of an access log
func (s *AccessLogService) UpdateAccessLogStatus(ctx context.Context, machineID, tempUsername string, status model.SessionStatus) error {
	return s.accessLogRepository.UpdateAccessLogStatus(ctx, machineID, tempUsername, status)
}
