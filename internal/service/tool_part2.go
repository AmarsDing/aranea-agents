package service

import (
	v1 "aranea-agents/api/kratos/tool/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"context"
	"strings"
)

func (s *ToolService) GetToolInvocationParams(ctx context.Context, req *v1.GetToolInvocationParamsRequest) (*v1.ToolInvocationParam, error) {
	p, err := s.uc.GetToolInvocationParams(ctx, req.GetInvocationId())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("TOOL", "invocation params not found")
		}
		return nil, err
	}
	return &v1.ToolInvocationParam{
		Id:               p.ID,
		InvocationId:     p.InvocationID,
		ToolKey:          p.ToolKey,
		ParamsJson:       p.ParamsJSON,
		RedactionApplied: p.RedactionApplied,
		CreatedAt:        p.CreatedAt,
	}, nil
}

func (s *ToolService) UpdateToolConfig(ctx context.Context, req *v1.UpdateToolConfigRequest) (*v1.Tool, error) {
	if err := s.assertToolAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	t, err := s.uc.UpdateToolConfig(ctx, req.GetId(), req.GetConfigJson())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("TOOL", "tool not found")
		}
		return nil, err
	}
	invalidateAllAgentBuildCaches()
	return bizToolToProto(t), nil
}

func (s *ToolService) TestTool(ctx context.Context, req *v1.TestToolRequest) (*v1.TestToolResponse, error) {

	if err := s.assertToolAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	res, err := s.uc.TestTool(ctx, req.GetId(), req.GetArgumentsJson(), int(req.GetTimeoutSec()))
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("TOOL", "tool not found")
		}
		return nil, err
	}
	return &v1.TestToolResponse{
		Status:        res.Status,
		ResultPreview: sanitizeUTF8(res.ResultPreview),
		ErrorMessage:  res.ErrorMessage,
		DurationMs:    int32(res.DurationMS),
	}, nil
}

// sanitizeUTF8 replaces invalid UTF-8 sequences with the replacement character.
// Proto3 string fields require valid UTF-8; tool results may contain raw bytes.
func sanitizeUTF8(s string) string {
	return strings.ToValidUTF8(s, "-")
}

func bizToolInvocationAuditToProto(a biz.ToolInvocationAudit) *v1.ToolInvocationAudit {
	return &v1.ToolInvocationAudit{
		Id:            a.ID,
		InvocationId:  a.InvocationID,
		ToolKey:       a.ToolKey,
		AgentId:       a.AgentID,
		UserId:        a.UserID,
		SessionId:     a.SessionID,
		Action:        a.Action,
		ResultSummary: a.ResultSummary,
		Status:        a.Status,
		Source:        a.Source,
		CreatedAt:     a.CreatedAt,
	}
}

func (s *ToolService) ListToolInvocationAudits(ctx context.Context, req *v1.ListToolInvocationAuditsRequest) (*v1.ListToolInvocationAuditsResponse, error) {
	limit, offset, page, pageSize := biz.PageToLimitOffset(req.GetPage(), req.GetPageSize())
	toolKey := req.GetToolKey()
	if toolKey != "" {
		if resolved, err := s.uc.ResolveToolKey(ctx, toolKey); err == nil {
			toolKey = resolved
		}
	}
	q := biz.ToolAuditQuery{
		ToolKey:   toolKey,
		AgentID:   req.GetAgentId(),
		UserID:    req.GetUserId(),
		SessionID: req.GetSessionId(),
		Status:    req.GetStatus(),
		From:      req.GetFrom(),
		To:        req.GetTo(),
		Limit:     limit,
		Offset:    offset,
	}

	if !workspace.IsSystem(ctx) {
		q.WorkspaceID = workspace.IDFromContext(ctx)
	}
	result, err := s.uc.ListInvocationAudits(ctx, q)
	if err != nil {
		return nil, err
	}
	items := make([]*v1.ToolInvocationAudit, 0, len(result.Items))
	for i := range result.Items {
		items = append(items, bizToolInvocationAuditToProto(result.Items[i]))
	}
	return &v1.ListToolInvocationAuditsResponse{
		Items:    items,
		Total:    int32(result.Total),
		Page:     page,
		PageSize: pageSize,
	}, nil
}
