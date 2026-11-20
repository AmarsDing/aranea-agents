package service

import (
	"context"

	mcpv1 "aranea-agents/api/kratos/mcp_server/v1"
	skillv1 "aranea-agents/api/kratos/skill/v1"
	toolv1 "aranea-agents/api/kratos/tool/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"

	"google.golang.org/protobuf/proto"
)

// 能力目录页的「多少 Agent 能用」：列表接口在返回前批量补上。尽力而为——算不出来就不填，
// 前端显示「—」，不拖垮列表本身。

func capabilityCallerWS(ctx context.Context) string {
	if workspace.IsSystem(ctx) {
		return ""
	}
	return workspace.IDFromContext(ctx)
}

func attachToolAgentCounts(ctx context.Context, agents *biz.AgentUsecase, items []*toolv1.Tool) {
	if agents == nil || len(items) == 0 {
		return
	}
	keys := make([]string, 0, len(items))
	for _, it := range items {
		keys = append(keys, it.GetKey())
	}
	counts, err := agents.CountAgentsAllowedByTool(ctx, keys, capabilityCallerWS(ctx))
	if err != nil {
		return
	}
	for _, it := range items {
		it.AllowedAgentCount = proto.Int32(int32(counts[it.GetKey()]))
	}
}

func withMCPAgentCounts(ctx context.Context, agents *biz.AgentUsecase, resp *mcpv1.ListMCPServersResponse) *mcpv1.ListMCPServersResponse {
	attachMCPAgentCounts(ctx, agents, resp.GetItems())
	return resp
}

func withSkillAgentCounts(ctx context.Context, agents *biz.AgentUsecase, resp *skillv1.ListSkillsResponse) *skillv1.ListSkillsResponse {
	attachSkillAgentCounts(ctx, agents, resp.GetItems())
	return resp
}

func attachMCPAgentCounts(ctx context.Context, agents *biz.AgentUsecase, items []*mcpv1.MCPServer) {
	if agents == nil || len(items) == 0 {
		return
	}
	keys := make([]string, 0, len(items))
	for _, it := range items {
		keys = append(keys, it.GetKey())
	}
	counts, err := agents.CountAgentsByMCPServer(ctx, keys, capabilityCallerWS(ctx))
	if err != nil {
		return
	}
	for _, it := range items {
		it.AgentCount = proto.Int32(int32(counts[it.GetKey()]))
	}
}

func attachSkillAgentCounts(ctx context.Context, agents *biz.AgentUsecase, items []*skillv1.Skill) {
	if agents == nil || len(items) == 0 {
		return
	}
	slugs := make([]string, 0, len(items))
	for _, it := range items {
		slugs = append(slugs, it.GetSlug())
	}
	counts, err := agents.CountAgentsSeeingSkill(ctx, slugs, capabilityCallerWS(ctx))
	if err != nil {
		return
	}
	for _, it := range items {
		it.VisibleAgentCount = proto.Int32(int32(counts[it.GetSlug()]))
	}
}
