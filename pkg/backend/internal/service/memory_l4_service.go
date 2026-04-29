package service

import (
	"arenea/backend/internal/memory/application"
	"arenea/backend/internal/repository"
)

type (
	MemoryL4Service       = application.MemoryL4Service
	EntityUpsertInput     = application.EntityUpsertInput
	RelationUpsertInput   = application.RelationUpsertInput
	EntityListResult      = application.EntityListResult
	ExtractionReport      = application.ExtractionReport
)

// NewMemoryL4Service 委托 internal/memory/application。
func NewMemoryL4Service(repo repository.Store) *MemoryL4Service {
	return application.NewMemoryL4Service(repo)
}
