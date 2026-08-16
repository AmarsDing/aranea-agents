package service

import (
	"context"

	v1 "aranea-agents/api/kratos/agent/v1"
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// GetAgentEffectiveTools implements GET /v1/agents/{agent_id}/tools/effective.
func (s *AgentService) GetAgentEffectiveTools(ctx context.Context, req *v1.GetAgentEffectiveToolsRequest) (*v1.AgentEffectiveToolsView, error) {
	if err := s.assertAgentAccess(ctx, req.GetAgentId()); err != nil {
		return nil, err
	}
	out, err := s.uc.GetEffectiveTools(ctx, req.GetAgentId())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound(apierror.DomainAgent, "agent not found")
		}
		return nil, err
	}
	return bizEffectiveToolsToProto(out), nil
}

// UpdateAgentToolPolicy implements PUT /v1/agents/{agent_id}/tools/policy.
func (s *AgentService) UpdateAgentToolPolicy(ctx context.Context, req *v1.UpdateAgentToolPolicyRequest) (*v1.AgentEffectiveToolsView, error) {
	if err := s.assertAgentMutateAccess(ctx, req.GetAgentId()); err != nil {
		return nil, err
	}
	in := biz.AgentToolPolicyInput{
		ToolsEnabled: req.GetToolsEnabled(),
		Profile:      req.GetProfile(),
		Allow:        req.GetAllow(),
		Deny:         req.GetDeny(),
	}
	if req.ExecutionTimeoutSec != nil {
		sec := req.GetExecutionTimeoutSec()
		in.ExecutionTimeoutSec = &sec
	}
	out, change, err := s.uc.UpdateAgentToolPolicy(ctx, req.GetAgentId(), in)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound(apierror.DomainAgent, "agent not found")
		}
		return nil, err
	}
	// P1-2：按变更分类决定失效策略——装配字段变化才重建；仅 resolver 化策略
	// 字段（timeout）变化时同步 resolver 即可，缓存 agent 零重建，新调用立即
	// 生效。两者同时变化时先 Set resolver（当前在途 agent 即生效），invalidate
	// 照旧（重建后 resolver 值与指纹语义一致）。
	if change.PolicyChanged {
		chatagent.SetToolExecutionTimeout(req.GetAgentId(), change.NewExecutionTimeoutSec)
	}
	if change.StructureChanged {
		invalidateAgentBuildCache(req.GetAgentId())
	}
	return bizEffectiveToolsToProto(out), nil
}
