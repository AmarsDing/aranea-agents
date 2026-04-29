package service

import (
	"arenea/backend/internal/capability/application"
)

// ToolService 在 Capability 应用层实现；本包仅保留类型别名以兼容 transport / server 的导入路径。
type ToolService = application.ToolService

// EvolutionToolPolicySource 与 application 包定义一致。
type EvolutionToolPolicySource = application.EvolutionToolPolicySource

// NewToolService 委托 capability/application.
func NewToolService(store application.ToolDataStore) *ToolService {
	return application.NewToolService(store)
}
