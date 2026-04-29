package service

import (
	"arenea/backend/internal/memory/application"
	"arenea/backend/internal/repository"
)

type (
	MemoryL1Service   = application.MemoryL1Service
	StartL1TaskInput  = application.StartL1TaskInput
	L1TaskView        = application.L1TaskView
)

// NewMemoryL1Service 委托 internal/memory/application。
func NewMemoryL1Service(repo repository.Store) *MemoryL1Service {
	return application.NewMemoryL1Service(repo)
}
