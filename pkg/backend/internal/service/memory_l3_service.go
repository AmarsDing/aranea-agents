package service

import (
	"arenea/backend/internal/memory/application"
	"arenea/backend/internal/repository"
)

type (
	MemoryL3Service          = application.MemoryL3Service
	EmbeddingSource            = application.EmbeddingSource
	L4FactExtractionSource     = application.L4FactExtractionSource
	FactListResult             = application.FactListResult
	FactPatch                  = application.FactPatch
	BulkUpsertReport           = application.BulkUpsertReport
	DecayReport                = application.DecayReport
	L3StatsReport              = application.L3StatsReport
)

// NewMemoryL3Service 委托 internal/memory/application。
func NewMemoryL3Service(repo repository.Store) *MemoryL3Service {
	return application.NewMemoryL3Service(repo)
}
