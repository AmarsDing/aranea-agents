package service

import (
	"context"

	v1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/pkg/loggateway"
)

// withMemoryFootprint 在不带 Agent、不限作用域的（管理员）查询上补平台视角：哪些 Agent 有记忆、无主事实按作用域。
// 尽力而为，失败只记日志。
func (s *MemoryService) withMemoryFootprint(ctx context.Context, platformView bool, out *v1.ListMemoryFactsResponse) *v1.ListMemoryFactsResponse {
	if !platformView {
		return out
	}
	agents, scopes, err := s.admin.MemoryFootprint(ctx, 20)
	if err != nil {
		s.lg.Warn("ListMemoryFacts: footprint failed", loggateway.StepID("memory.footprint"), loggateway.Err(err))
		return out
	}
	for _, a := range agents {
		out.AgentFootprints = append(out.AgentFootprints, &v1.MemoryAgentFootprint{AgentId: a.AgentID, Facts: a.Facts, Episodes: a.Episodes})
	}
	for _, sc := range scopes {
		out.UnownedScopes = append(out.UnownedScopes, &v1.MemoryScopeFactCount{ScopeType: sc.ScopeType, Facts: sc.Facts})
	}
	return out
}
