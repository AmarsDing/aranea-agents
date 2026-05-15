package service

import (
	"context"
	"database/sql"
	"errors"

	v1 "aranea-agents/api/kratos/tool/v1"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// ToolService implements kratos tool.v1.
type ToolService struct {
	v1.UnimplementedToolServiceServer

	uc *biz.ToolUsecase
}

func NewToolService(uc *biz.ToolUsecase) *ToolService {
	return &ToolService{uc: uc}
}

func bizToolToProto(t biz.Tool) *v1.Tool {
	out := &v1.Tool{
		Id:                   t.ID,
		Key:                  t.Key,
		DisplayName:          t.DisplayName,
		Description:          t.Description,
		Category:             t.Category,
		Source:               t.Source,
		RiskLevel:            t.RiskLevel,
		Enabled:              t.Enabled,
		Readonly:             t.Readonly,
		RequiresConfirmation: t.RequiresConfirmation,
		SupportsStreaming:    t.SupportsStreaming,
		SupportsConcurrency:  t.SupportsConcurrency,
		ParametersSchemaJson: t.ParametersSchemaJSON,
		ResultSchemaJson:     t.ResultSchemaJSON,
		ConfigSchemaJson:     t.ConfigSchemaJSON,
		ConfigJson:           t.ConfigJSON,
		DefaultConfigJson:    t.DefaultConfigJSON,
		MetadataJson:         t.MetadataJSON,
		RuntimeStatus:        t.RuntimeStatus,
		RuntimeKind:          t.RuntimeKind,
		InvokeCount:          int32(t.InvokeCount),
		InvokeCount_24H:      int32(t.InvokeCount24h),
		SuccessCount:         int32(t.SuccessCount),
		FailureCount:         int32(t.FailureCount),
		BlockedCount:         int32(t.BlockedCount),
		AgentOverrideCount:   int32(t.AgentOverrideCount),
		LastInvokedAt:        t.LastInvokedAt,
		LastStatus:           t.LastStatus,
		CreatedAt:            t.CreatedAt,
		UpdatedAt:            t.UpdatedAt,
		Permissions:          &v1.ToolPermissions{CanManage: t.Permissions.CanManage},
	}
	if t.AvgDurationMS != nil {
		v := *t.AvgDurationMS
		out.AvgDurationMs = &v
	}
	out.P95DurationMs = t.P95DurationMS
	return out
}

func bizSummaryToProto(s biz.ToolSummary) *v1.ToolSummary {
	return &v1.ToolSummary{
		TotalTools:      int32(s.TotalTools),
		EnabledTools:    int32(s.EnabledTools),
		HighRiskEnabled: int32(s.HighRiskEnabled),
		Calls_24H:       int32(s.Calls24h),
		FailureRate_24H: s.FailureRate24h,
	}
}

func bizInvocationToProto(x biz.ToolInvocation) *v1.ToolInvocation {
	return &v1.ToolInvocation{
		Id:               x.ID,
		RequestId:        x.RequestID,
		InvocationId:     x.InvocationID,
		ToolId:           x.ToolID,
		ToolKey:          x.ToolKey,
		ToolDisplayName:  x.ToolDisplayName,
		AgentId:          x.AgentID,
		AgentKey:         x.AgentKey,
		AgentDisplayName: x.AgentDisplayName,
		SessionId:        x.SessionID,
		MessageId:        x.MessageID,
		UserId:           x.UserID,
		Source:           x.Source,
		Status:           x.Status,
		StartedAt:        x.StartedAt,
		EndedAt:          x.EndedAt,
		DurationMs:       int32(x.DurationMS),
		InputPreview:     x.InputPreview,
		InputHash:        x.InputHash,
		OutputPreview:    x.OutputPreview,
		OutputHash:       x.OutputHash,
		ErrorCode:        x.ErrorCode,
		ErrorMessage:     x.ErrorMessage,
		RedactionApplied: x.RedactionApplied,
		MetadataJson:     x.MetadataJSON,
		CreatedAt:        x.CreatedAt,
	}
}

func (s *ToolService) ListTools(ctx context.Context, req *v1.ListToolsRequest) (*v1.ListToolsResponse, error) {
	limit, offset, page, pageSize := biz.PageToLimitOffset(req.GetPage(), req.GetPageSize())
	enabled := ""
	switch {
	case req.GetEnabled() == "true", req.GetEnabled() == "false":
		enabled = req.GetEnabled()
	}
	q := biz.ToolListQuery{
		Search:    req.GetSearch(),
		Category:  req.GetCategory(),
		Source:    req.GetSource(),
		RiskLevel: req.GetRiskLevel(),
		Enabled:   enabled,
		Sort:      req.GetSort(),
		Limit:     limit,
		Offset:    offset,
	}
	result, err := s.uc.ListTools(ctx, q)
	if err != nil {
		return nil, err
	}
	items := make([]*v1.Tool, 0, len(result.Items))
	for i := range result.Items {
		items = append(items, bizToolToProto(result.Items[i]))
	}
	return &v1.ListToolsResponse{
		Items:    items,
		Total:    int32(result.Total),
		Page:     page,
		PageSize: pageSize,
		Summary:  bizSummaryToProto(result.Summary),
	}, nil
}

func (s *ToolService) GetTool(ctx context.Context, req *v1.GetToolRequest) (*v1.Tool, error) {
	t, err := s.uc.GetTool(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("TOOL", "tool not found")
		}
		return nil, err
	}
	return bizToolToProto(t), nil
}

func (s *ToolService) CreateTool(ctx context.Context, req *v1.CreateToolRequest) (*v1.Tool, error) {
	in := biz.ToolUpsertInput{
		Key:                  req.GetKey(),
		DisplayName:          req.GetDisplayName(),
		Description:          req.GetDescription(),
		Category:             req.GetCategory(),
		Source:               req.GetSource(),
		RiskLevel:            req.GetRiskLevel(),
		Enabled:              req.GetEnabled(),
		Readonly:             req.GetReadonly(),
		RequiresConfirmation: req.GetRequiresConfirmation(),
		SupportsStreaming:    req.GetSupportsStreaming(),
		SupportsConcurrency:  req.GetSupportsConcurrency(),
		ParametersSchemaJSON: req.GetParametersSchemaJson(),
		ResultSchemaJSON:     req.GetResultSchemaJson(),
		ConfigSchemaJSON:     req.GetConfigSchemaJson(),
		ConfigJSON:           req.GetConfigJson(),
		DefaultConfigJSON:    req.GetDefaultConfigJson(),
		MetadataJSON:         req.GetMetadataJson(),
	}
	t, err := s.uc.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	return bizToolToProto(t), nil
}

func (s *ToolService) UpdateTool(ctx context.Context, req *v1.UpdateToolRequest) (*v1.Tool, error) {
	in := biz.ToolUpsertInput{
		Key:                  req.GetKey(),
		DisplayName:          req.GetDisplayName(),
		Description:          req.GetDescription(),
		Category:             req.GetCategory(),
		Source:               req.GetSource(),
		RiskLevel:            req.GetRiskLevel(),
		Enabled:              req.GetEnabled(),
		Readonly:             req.GetReadonly(),
		RequiresConfirmation: req.GetRequiresConfirmation(),
		SupportsStreaming:    req.GetSupportsStreaming(),
		SupportsConcurrency:  req.GetSupportsConcurrency(),
		ParametersSchemaJSON: req.GetParametersSchemaJson(),
		ResultSchemaJSON:     req.GetResultSchemaJson(),
		ConfigSchemaJSON:     req.GetConfigSchemaJson(),
		ConfigJSON:           req.GetConfigJson(),
		DefaultConfigJSON:    req.GetDefaultConfigJson(),
		MetadataJSON:         req.GetMetadataJson(),
	}
	t, err := s.uc.Update(ctx, req.GetId(), in)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("TOOL", "tool not found")
		}
		return nil, err
	}
	return bizToolToProto(t), nil
}

func (s *ToolService) DeleteTool(ctx context.Context, req *v1.DeleteToolRequest) (*emptypb.Empty, error) {
	err := s.uc.Delete(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("TOOL", "tool not found")
		}
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *ToolService) ToggleToolEnabled(ctx context.Context, req *v1.ToggleToolEnabledRequest) (*v1.Tool, error) {
	t, err := s.uc.ToggleEnabled(ctx, req.GetId(), req.GetEnabled(), req.GetConfirmKey())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("TOOL", "tool not found")
		}
		return nil, err
	}
	return bizToolToProto(t), nil
}

func runsQuery(req *v1.ListToolRunsRequest) biz.ToolRunQuery {
	limit, offset, _, _ := biz.PageToLimitOffset(req.GetPage(), req.GetPageSize())
	var hasError *bool
	if req.HasError != nil {
		hasError = req.HasError
	}
	return biz.ToolRunQuery{
		ToolKey:   req.GetToolKey(),
		AgentID:   req.GetAgentId(),
		SessionID: req.GetSessionId(),
		Status:    req.GetStatus(),
		From:      req.GetFrom(),
		To:        req.GetTo(),
		HasError:  hasError,
		Limit:     limit,
		Offset:    offset,
	}
}

func (s *ToolService) ListToolRuns(ctx context.Context, req *v1.ListToolRunsRequest) (*v1.ListToolRunsResponse, error) {
	q := runsQuery(req)
	result, err := s.uc.ListRuns(ctx, q)
	if err != nil {
		return nil, err
	}
	_, _, page, pageSize := biz.PageToLimitOffset(req.GetPage(), req.GetPageSize())
	items := make([]*v1.ToolInvocation, 0, len(result.Items))
	for i := range result.Items {
		items = append(items, bizInvocationToProto(result.Items[i]))
	}
	return &v1.ListToolRunsResponse{
		Items:    items,
		Total:    int32(result.Total),
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *ToolService) ListToolRunsForTool(ctx context.Context, req *v1.ListToolRunsForToolRequest) (*v1.ListToolRunsResponse, error) {
	limit, offset, page, pageSize := biz.PageToLimitOffset(req.GetPage(), req.GetPageSize())
	q := biz.ToolRunQuery{
		ToolKey:   req.GetToolId(),
		AgentID:   req.GetAgentId(),
		SessionID: req.GetSessionId(),
		Status:    req.GetStatus(),
		From:      req.GetFrom(),
		To:        req.GetTo(),
		Limit:     limit,
		Offset:    offset,
	}
	result, err := s.uc.ListRuns(ctx, q)
	if err != nil {
		return nil, err
	}
	items := make([]*v1.ToolInvocation, 0, len(result.Items))
	for i := range result.Items {
		items = append(items, bizInvocationToProto(result.Items[i]))
	}
	return &v1.ListToolRunsResponse{
		Items:    items,
		Total:    int32(result.Total),
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func bizOverrideToProto(o biz.ToolAgentOverride) *v1.ToolAgentOverride {
	return &v1.ToolAgentOverride{
		Id:                   o.ID,
		ToolId:               o.ToolID,
		ToolKey:              o.ToolKey,
		AgentId:              o.AgentID,
		Enabled:              o.Enabled,
		Mode:                 o.Mode,
		ConfigOverrideJson:   o.ConfigOverrideJSON,
		RequiresConfirmation: o.RequiresConfirmation,
		CreatedAt:            o.CreatedAt,
		UpdatedAt:            o.UpdatedAt,
	}
}

func (s *ToolService) ListToolAgentOverrides(ctx context.Context, req *v1.ListToolAgentOverridesRequest) (*v1.ListToolAgentOverridesResponse, error) {
	toolKey := req.GetToolId()
	overrides, err := s.uc.ListToolAgentOverrides(ctx, toolKey)
	if err != nil {
		return nil, err
	}
	items := make([]*v1.ToolAgentOverride, 0, len(overrides))
	for i := range overrides {
		items = append(items, bizOverrideToProto(overrides[i]))
	}
	return &v1.ListToolAgentOverridesResponse{Items: items}, nil
}

func (s *ToolService) UpsertToolAgentOverride(ctx context.Context, req *v1.UpsertToolAgentOverrideRequest) (*v1.ToolAgentOverride, error) {
	in := biz.ToolAgentOverrideInput{
		ToolKey:              req.GetToolId(),
		AgentID:              req.GetAgentId(),
		Enabled:              req.GetEnabled(),
		Mode:                 req.GetMode(),
		ConfigOverrideJSON:   req.GetConfigOverrideJson(),
		RequiresConfirmation: req.GetRequiresConfirmation(),
	}
	o, err := s.uc.UpsertToolAgentOverride(ctx, in)
	if err != nil {
		return nil, err
	}
	return bizOverrideToProto(o), nil
}

func (s *ToolService) DeleteToolAgentOverride(ctx context.Context, req *v1.DeleteToolAgentOverrideRequest) (*emptypb.Empty, error) {
	err := s.uc.DeleteToolAgentOverride(ctx, req.GetToolId(), req.GetAgentId())
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *ToolService) GetToolInvocationParams(ctx context.Context, req *v1.GetToolInvocationParamsRequest) (*v1.ToolInvocationParam, error) {
	p, err := s.uc.GetToolInvocationParams(ctx, req.GetInvocationId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("TOOL", "invocation params not found")
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
	t, err := s.uc.UpdateToolConfig(ctx, req.GetId(), req.GetConfigJson())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("TOOL", "tool not found")
		}
		return nil, err
	}
	return bizToolToProto(t), nil
}
