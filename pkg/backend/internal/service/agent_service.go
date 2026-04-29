package service

import (
	"arenea/backend/internal/catalog/application"
	"arenea/backend/internal/repository"
)

// AgentService 在 Catalog 应用层实现；本包保留类型别名以兼容 transport / server 导入路径。
type AgentService = application.AgentService

// NewAgentService 委托 internal/catalog/application。
func NewAgentService(repo repository.Store) *AgentService {
	return application.NewAgentService(repo)
}
