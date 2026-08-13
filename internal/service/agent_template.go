package service

import (
	"context"

	v1 "aranea-agents/api/kratos/agent/v1"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// ListAgentTemplates implements GET /v1/agent-templates.
func (s *AgentService) ListAgentTemplates(ctx context.Context, _ *emptypb.Empty) (*v1.ListAgentTemplatesResponse, error) {
	items, err := s.agentTemplateUC.List(ctx)
	if err != nil {
		return nil, err
	}
	out := &v1.ListAgentTemplatesResponse{Items: make([]*v1.AgentTemplate, 0, len(items))}
	for _, t := range items {
		out.Items = append(out.Items, &v1.AgentTemplate{
			Key:         t.Key,
			Label:       t.Label,
			Icon:        t.Icon,
			Description: t.Description,
			DisplayName: t.DisplayName,
			Provider:    t.Provider,
			Model:       t.Model,
		})
	}
	return out, nil
}

// DuplicateAgent implements POST /v1/agents/{id}/duplicate.
func (s *AgentService) DuplicateAgent(ctx context.Context, req *v1.DuplicateAgentRequest) (*v1.Agent, error) {
	if err := s.assertAgentAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	dup, err := s.uc.Duplicate(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return s.toProtoAgentEnriched(ctx, dup), nil
}
