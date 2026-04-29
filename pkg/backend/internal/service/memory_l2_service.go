package service

import (
	"arenea/backend/internal/memory/application"
	"arenea/backend/internal/repository"
)

type (
	MemoryL2Service              = application.MemoryL2Service
	L1SnapshotSource             = application.L1SnapshotSource
	L4EpisodeExtractionSource    = application.L4EpisodeExtractionSource
	CreateEpisodeInput           = application.CreateEpisodeInput
	EpisodeListResult            = application.EpisodeListResult
	EpisodeDetail                = application.EpisodeDetail
	EventListResult              = application.EventListResult
	MarkInput                    = application.MarkInput
	RetentionReport              = application.RetentionReport
)

// NewMemoryL2Service 委托 internal/memory/application。
func NewMemoryL2Service(repo repository.Store) *MemoryL2Service {
	return application.NewMemoryL2Service(repo)
}
