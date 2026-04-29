package service

import (
	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

type AuditService struct {
	repo repository.Store
}

func NewAuditService(repo repository.Store) *AuditService {
	return &AuditService{repo: repo}
}

func (s *AuditService) Log(action string, resource string, resourceID string, requestID string, detail string) error {
	return s.repo.AddAuditLog(domain.AuditLog{
		ID:         newID(),
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		RequestID:  requestID,
		Detail:     detail,
	})
}

func (s *AuditService) List(limit int) ([]domain.AuditLog, error) {
	return s.repo.ListAuditLogs(limit)
}
