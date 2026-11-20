package biz

import "context"

// MemoryAgentFootprint 一个 Agent 名下的记忆量（有效事实、未删情景）。
type MemoryAgentFootprint struct {
	AgentID  string
	Facts    int32
	Episodes int32
}

// MemoryScopeCount 不属于任何 Agent 的有效事实，按作用域计（canary / user / team …）。
type MemoryScopeCount struct {
	ScopeType string
	Facts     int32
}

// memoryFootprintReader 由数据层的记忆适配器实现；做成可选接口，测试替身不必跟着实现。
type memoryFootprintReader interface {
	MemoryFootprint(ctx context.Context, limit int) ([]MemoryAgentFootprint, []MemoryScopeCount, error)
}

// MemoryFootprint 平台视角的记忆分布；存储不支持时返回空，不报错。
func (uc *MemoryAdminUsecase) MemoryFootprint(ctx context.Context, limit int) ([]MemoryAgentFootprint, []MemoryScopeCount, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, nil, err
	}
	r, ok := uc.admin.(memoryFootprintReader)
	if !ok {
		return nil, nil, nil
	}
	return r.MemoryFootprint(ctx, limit)
}
