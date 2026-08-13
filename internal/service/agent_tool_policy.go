package service

import (
	"context"

	v1 "aranea-agents/api/kratos/agent/v1"
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
	out, err := s.uc.UpdateAgentToolPolicy(ctx, req.GetAgentId(), in)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound(apierror.DomainAgent, "agent not found")
		}
		return nil, err
	}
	invalidateAgentBuildCache(req.GetAgentId())
	return bizEffectiveToolsToProto(out), nil
}
