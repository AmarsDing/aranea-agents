package service

import (
	"arenea/backend/internal/memory/application"
	"arenea/backend/internal/repository"
)

// MemoryL0Service 在 Memory 应用层实现；本包保留类型别名以兼容 chat / server / transport 导入路径。
type MemoryL0Service = application.MemoryL0Service

// L0 所需的窄依赖接口，与 application 同形。
type (
	L1PromptSource         = application.L1PromptSource
	L2RecallSource         = application.L2RecallSource
	L3RecallSource         = application.L3RecallSource
	L4RecallSource         = application.L4RecallSource
	EvolutionPromptSource  = application.EvolutionPromptSource
)

// NewMemoryL0Service 委托 internal/memory/application。
func NewMemoryL0Service(repo repository.Store) *MemoryL0Service {
	return application.NewMemoryL0Service(repo)
}
