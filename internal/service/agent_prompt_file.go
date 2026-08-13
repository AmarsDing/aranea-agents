package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/agent/v1"
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// GetAgentPromptPreview implements GET /v1/agents/{id}/system-prompt/preview.
func (s *AgentService) GetAgentPromptPreview(ctx context.Context, req *v1.GetAgentPromptPreviewRequest) (*v1.GetAgentPromptPreviewResponse, error) {
	if err := s.assertAgentAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	a, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound(apierror.DomainAgent, "agent not found")
		}
		return nil, err
	}
	mode := strings.TrimSpace(req.GetMode())
	report := chatagent.BuildPreviewReport(ctx, a, mode, chatagent.Deps{AgentUC: s.uc, LG: s.lg})
	sections := make([]*v1.PromptSectionEstimate, 0, len(report.Sections))
	for _, sec := range report.Sections {
		sections = append(sections, &v1.PromptSectionEstimate{
			Key:       sec.Key,
			Label:     sec.Label,
			EstTokens: int32(sec.EstTokens),
			Source:    sec.Source,
		})
	}
	return &v1.GetAgentPromptPreviewResponse{
		Preview:                 report.Summary,
		Instruction:             report.Instruction,
		Sections:                sections,
		StaticTotalTokens:       int32(report.StaticTotalTokens),
		RuntimeOverlayEstTokens: int32(report.RuntimeOverlayEst),
		RuntimeNote:             report.RuntimeNote,
	}, nil
}

// CreateAgentPromptFile implements POST /v1/agents/{agent_id}/files.
func (s *AgentService) CreateAgentPromptFile(ctx context.Context, req *v1.CreateAgentPromptFileRequest) (*v1.AgentPromptFile, error) {
	if err := s.assertAgentMutateAccess(ctx, req.GetAgentId()); err != nil {
		return nil, err
	}
	f := biz.AgentPromptFile{
		AgentID:   req.GetAgentId(),
		Name:      req.GetName(),
		Body:      req.GetBody(),
		SortOrder: int(req.GetSortOrder()),
	}
	created, err := s.uc.CreatePromptFile(ctx, f)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound(apierror.DomainAgent, "agent not found")
		}
		return nil, err
	}
	invalidateAgentBuildCache(req.GetAgentId())
	return toProtoFile(created), nil
}

// UpdateAgentPromptFile implements PATCH /v1/agents/{agent_id}/files/{id}.
func (s *AgentService) UpdateAgentPromptFile(ctx context.Context, req *v1.UpdateAgentPromptFileRequest) (*v1.AgentPromptFile, error) {
	if err := s.assertAgentMutateAccess(ctx, req.GetAgentId()); err != nil {
		return nil, err
	}
	f := biz.AgentPromptFile{
		ID:        req.GetId(),
		AgentID:   req.GetAgentId(),
		Name:      req.GetName(),
		Body:      req.GetBody(),
		SortOrder: int(req.GetSortOrder()),
	}
	updated, err := s.uc.UpdatePromptFile(ctx, f)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound(apierror.DomainAgentFile, "prompt file not found")
		}
		return nil, err
	}
	invalidateAgentBuildCache(req.GetAgentId())
	return toProtoFile(updated), nil
}

// DeleteAgentPromptFile implements DELETE /v1/agents/{agent_id}/files/{id}.
func (s *AgentService) DeleteAgentPromptFile(ctx context.Context, req *v1.DeleteAgentPromptFileRequest) (*emptypb.Empty, error) {
	if err := s.assertAgentMutateAccess(ctx, req.GetAgentId()); err != nil {
		return nil, err
	}
	if err := s.uc.DeletePromptFile(ctx, req.GetAgentId(), req.GetId()); err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound(apierror.DomainAgentFile, "prompt file not found")
		}
		return nil, err
	}
	invalidateAgentBuildCache(req.GetAgentId())
	return &emptypb.Empty{}, nil
}

// EstimateTokens implements POST /v1/agents/{agent_id}/files/estimate-tokens.
func (s *AgentService) EstimateTokens(ctx context.Context, req *v1.EstimateTokensRequest) (*v1.EstimateTokensResponse, error) {
	if err := s.assertAgentAccess(ctx, req.GetAgentId()); err != nil {
		return nil, err
	}
	estimates, err := s.uc.EstimateTokens(ctx, req.GetAgentId())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound(apierror.DomainAgent, "agent not found")
		}
		return nil, err
	}
	resp := &v1.EstimateTokensResponse{
		TotalTokens: int32(estimates.TotalTokens),
	}
	for _, fe := range estimates.FileEstimates {
		resp.FileEstimates = append(resp.FileEstimates, &v1.FileTokenEstimate{
			FileId:          fe.FileID,
			FileName:        fe.FileName,
			EstimatedTokens: int32(fe.EstimatedTokens),
		})
	}
	return resp, nil
}

// EditPromptFileByAI implements POST /v1/agents/{agent_id}/files/{file_id}/ai-edit.
func (s *AgentService) EditPromptFileByAI(ctx context.Context, req *v1.EditPromptFileByAIRequest) (*v1.EditPromptFileByAIResponse, error) {
	if s.promptAI == nil {
		return nil, apierror.Internal(apierror.DomainAgentFile, "prompt file AI editor not configured")
	}
	agentID := strings.TrimSpace(req.GetAgentId())
	fileID := strings.TrimSpace(req.GetFileId())
	instruction := strings.TrimSpace(req.GetInstruction())
	if agentID == "" || fileID == "" || instruction == "" {
		return nil, apierror.BadRequest(apierror.DomainAgentFile, "agent_id, file_id and instruction are required")
	}
	if err := s.assertAgentMutateAccess(ctx, agentID); err != nil {
		return nil, err
	}
	a, err := s.uc.Get(ctx, agentID)
	if err != nil {
		return nil, err
	}
	var target *biz.AgentPromptFile
	for i := range a.Files {
		if a.Files[i].ID == fileID {
			target = &a.Files[i]
			break
		}
	}
	if target == nil {
		return nil, apierror.NotFound(apierror.DomainAgentFile, "prompt file not found")
	}
	revised, err := s.promptAI.Revise(ctx, a.Provider, a.Model, target.Name, target.Body, instruction)
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainAgent)
	}
	target.Body = revised
	updated, err := s.uc.UpdatePromptFile(ctx, *target)
	if err != nil {
		return nil, err
	}
	invalidateAgentBuildCache(agentID)
	s.lg.Info("AI 修订提示文件完成", loggateway.StepID("agent.prompt.ai_edit"), loggateway.Str("flow_status", "done"), loggateway.Str("agent_id", agentID), loggateway.Str("file_id", fileID))
	return &v1.EditPromptFileByAIResponse{File: toProtoFile(updated)}, nil
}
