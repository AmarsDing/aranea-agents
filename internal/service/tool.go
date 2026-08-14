package service

import (
	"context"
	"fmt"
	"strings"

	v1 "aranea-agents/api/kratos/tool/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// ToolService implements kratos tool.v1.
type ToolService struct {
	v1.UnimplementedToolServiceServer

	uc     *biz.ToolUsecase
	agents *biz.AgentUsecase
	mon    *biz.MonitorUsecase
}

func NewToolService(uc *biz.ToolUsecase, agents *biz.AgentUsecase, mon *biz.MonitorUsecase) *ToolService {
	return &ToolService{uc: uc, agents: agents, mon: mon}
}

// assertToolAccess 校验 caller 是否可访问指定 tool（P2-B IDOR 防护）。
// 跨租户访问返回 NotFound（避免泄露 tool 存在性）。
// 系统 caller（cron/admin）绕过校验；空 workspace_id 的 tool 视为全局共享。
func (s *ToolService) assertToolAccess(ctx context.Context, toolID string) error {
	if toolID == "" {
		return nil
	}
	t, err := s.uc.GetTool(ctx, toolID)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return apierror.NotFound("TOOL", "tool not found")
		}
		return err
	}
	if err := workspace.AssertWorkspaceOrShared(workspace.IDFromContext(ctx), t.WorkspaceID); err != nil {
		return apierror.NotFound("TOOL", "tool not found")
	}
	return nil
}

// assertAgentAccess 校验 caller 是否可读取指定 agent 作用域下的工具数据
// （覆盖项/授权列表）。共享 agent（workspace_id=""）对所有租户可读。
func (s *ToolService) assertAgentAccess(ctx context.Context, agentID string) error {
	return s.checkAgentAccess(ctx, agentID, false)
}

// assertAgentMutateAccess 校验 caller 是否可变更指定 agent 作用域下的工具数据。
// 共享 agent 对租户只读（fail-closed）。
func (s *ToolService) assertAgentMutateAccess(ctx context.Context, agentID string) error {
	return s.checkAgentAccess(ctx, agentID, true)
}

func (s *ToolService) checkAgentAccess(ctx context.Context, agentID string, mutate bool) error {
	if agentID == "" {
		return nil
	}
	a, err := s.agents.Get(ctx, agentID)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return apierror.NotFound(apierror.DomainAgent, "agent not found")
		}
		return err
	}
	callerWS := workspace.IDFromContext(ctx)
	if mutate {
		err = workspace.AssertWorkspaceMutate(callerWS, a.WorkspaceID)
	} else {
		err = workspace.AssertWorkspaceOrShared(callerWS, a.WorkspaceID)
	}
	if err != nil {
		return apierror.NotFound(apierror.DomainAgent, "agent not found")
	}
	return nil
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
		// A4: never leak secrets — sensitive config values leave the process
		// only in masked form; write paths treat the mask as "keep stored".
		ConfigJson:           biz.RedactToolConfigJSON(t.ConfigJSON, t.ConfigSchemaJSON),
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
		RepairedCount:        int32(t.RepairedCount),
		InvalidCount:         int32(t.InvalidCount),
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
		Streaming:        x.Streaming,
		ChunkCount:       int32(x.ChunkCount),
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
		Abnormal:  req.GetAbnormal(),
	}
	// P2-B: workspace visibility filter.
	// System caller (cron/admin) sees all; tenant caller sees shared + own.
	if !workspace.IsSystem(ctx) {
		q.WorkspaceID = workspace.IDFromContext(ctx)
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
	if err := s.assertToolAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	t, err := s.uc.GetTool(ctx, req.GetId())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("TOOL", "tool not found")
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
	// P2-B: assign tool to caller workspace (system caller → shared).
	if !workspace.IsSystem(ctx) {
		in.WorkspaceID = workspace.IDFromContext(ctx)
	}
	t, err := s.uc.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	invalidateAllAgentBuildCaches()
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:     biz.AuditAction(biz.AuditVerbCreate, "tool"),
		Resource:   "tool",
		ResourceID: t.ID,
		Summary:    fmt.Sprintf("key=%s", t.Key),
	})
	return bizToolToProto(t), nil
}

func (s *ToolService) UpdateTool(ctx context.Context, req *v1.UpdateToolRequest) (*v1.Tool, error) {
	if err := s.assertToolAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
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
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("TOOL", "tool not found")
		}
		return nil, err
	}
	invalidateAllAgentBuildCaches()
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:     biz.AuditAction(biz.AuditVerbUpdate, "tool"),
		Resource:   "tool",
		ResourceID: t.ID,
		Summary:    fmt.Sprintf("key=%s", t.Key),
	})
	return bizToolToProto(t), nil
}

func (s *ToolService) DeleteTool(ctx context.Context, req *v1.DeleteToolRequest) (*emptypb.Empty, error) {
	if err := s.assertToolAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	// 先取 key 供审计 detail 使用（best-effort，取不到不阻断删除）。
	summary := ""
	if t, err := s.uc.GetTool(ctx, req.GetId()); err == nil {
		summary = fmt.Sprintf("key=%s", t.Key)
	}
	err := s.uc.Delete(ctx, req.GetId())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("TOOL", "tool not found")
		}
		return nil, err
	}
	invalidateAllAgentBuildCaches()
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:     biz.AuditAction(biz.AuditVerbDelete, "tool"),
		Resource:   "tool",
		ResourceID: req.GetId(),
		Summary:    summary,
	})
	return &emptypb.Empty{}, nil
}

func (s *ToolService) ToggleToolEnabled(ctx context.Context, req *v1.ToggleToolEnabledRequest) (*v1.Tool, error) {
	if err := s.assertToolAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	t, err := s.uc.ToggleEnabled(ctx, req.GetId(), req.GetEnabled(), req.GetConfirmIntent())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound("TOOL", "tool not found")
		}
		return nil, err
	}
	invalidateAllAgentBuildCaches()
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
	// A3: tenant callers only see runs owned by their workspace.
	if !workspace.IsSystem(ctx) {
		q.WorkspaceID = workspace.IDFromContext(ctx)
	}
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
		AgentID:   req.GetAgentId(),
		SessionID: req.GetSessionId(),
		Status:    req.GetStatus(),
		From:      req.GetFrom(),
		To:        req.GetTo(),
		Limit:     limit,
		Offset:    offset,
	}
	result, err := s.uc.ListRunsForTool(ctx, req.GetToolId(), q)
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

// GetToolAgentBindings implements GET /v1/tools/{tool_id}/agent-bindings.
// Bulk-computes the tool's effective state across all visible agents,
// replacing the frontend N+1 scan of GetAgentEffectiveTools.
func (s *ToolService) GetToolAgentBindings(ctx context.Context, req *v1.GetToolAgentBindingsRequest) (*v1.ToolAgentBindingsView, error) {
	if err := s.assertToolAccess(ctx, req.GetToolId()); err != nil {
		return nil, err
	}
	callerWS := workspace.IDFromContext(ctx)
	if workspace.IsSystem(ctx) {
		callerWS = ""
	}
	bindings, err := s.agents.GetToolAgentBindings(ctx, req.GetToolId(), callerWS)
	if err != nil {
		return nil, err
	}
	out := &v1.ToolAgentBindingsView{Items: make([]*v1.ToolAgentBinding, 0, len(bindings))}
	for _, b := range bindings {
		out.Items = append(out.Items, &v1.ToolAgentBinding{
			AgentId:        b.AgentID,
			AgentKey:       b.AgentKey,
			AgentName:      b.AgentName,
			AgentStatus:    b.AgentStatus,
			ToolsEnabled:   b.ToolsEnabled,
			Profile:        b.Profile,
			EffectiveState: b.State,
			Reason:         b.Reason,
			OverrideMode:   b.OverrideMode,
		})
	}
	return out, nil
}

func (s *ToolService) ListToolAgentOverridesByAgent(ctx context.Context, req *v1.ListToolAgentOverridesByAgentRequest) (*v1.ListToolAgentOverridesByAgentResponse, error) {
	if err := s.assertAgentAccess(ctx, req.GetAgentId()); err != nil {
		return nil, err
	}
	overrides, err := s.uc.ListToolAgentOverridesByAgent(ctx, req.GetAgentId())
	if err != nil {
		return nil, err
	}
	items := make([]*v1.ToolAgentOverride, 0, len(overrides))
	for i := range overrides {
		items = append(items, bizOverrideToProto(overrides[i]))
	}
	return &v1.ListToolAgentOverridesByAgentResponse{Items: items}, nil
}

func (s *ToolService) UpsertToolAgentOverride(ctx context.Context, req *v1.UpsertToolAgentOverrideRequest) (*v1.ToolAgentOverride, error) {
	if err := s.assertAgentMutateAccess(ctx, req.GetAgentId()); err != nil {
		return nil, err
	}
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
	invalidateAgentBuildCache(req.GetAgentId())
	return bizOverrideToProto(o), nil
}

func (s *ToolService) DeleteToolAgentOverride(ctx context.Context, req *v1.DeleteToolAgentOverrideRequest) (*emptypb.Empty, error) {
	if err := s.assertAgentMutateAccess(ctx, req.GetAgentId()); err != nil {
		return nil, err
	}
	err := s.uc.DeleteToolAgentOverride(ctx, req.GetToolId(), req.GetAgentId())
	if err != nil {
		return nil, err
	}
	invalidateAgentBuildCache(req.GetAgentId())
	return &emptypb.Empty{}, nil
}

// ListToolGrants lists persisted "always allow" grants for an agent.
func (s *ToolService) ListToolGrants(ctx context.Context, req *v1.ListToolGrantsRequest) (*v1.ListToolGrantsResponse, error) {
	if err := s.assertAgentAccess(ctx, req.GetAgentId()); err != nil {
		return nil, err
	}
	grants, err := s.uc.ListToolGrants(ctx, req.GetAgentId())
	if err != nil {
		return nil, err
	}
	items := make([]*v1.ToolGrant, 0, len(grants))
	for i := range grants {
		items = append(items, &v1.ToolGrant{
			Id:        grants[i].ID,
			AgentId:   grants[i].AgentID,
			ToolKey:   grants[i].ToolKey,
			GrantedBy: grants[i].GrantedBy,
			CreatedAt: grants[i].CreatedAt,
		})
	}
	return &v1.ListToolGrantsResponse{Items: items}, nil
}

// DeleteToolGrant revokes a persisted grant. Idempotent. The confirmation
// decision chain queries grants per decision, so revocation takes effect on
// the next tool invocation without any build-cache invalidation.
func (s *ToolService) DeleteToolGrant(ctx context.Context, req *v1.DeleteToolGrantRequest) (*emptypb.Empty, error) {
	if err := s.assertAgentMutateAccess(ctx, req.GetAgentId()); err != nil {
		return nil, err
	}
	if err := s.uc.RevokeToolGrant(ctx, req.GetAgentId(), req.GetToolKey()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

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
	// A1: TestTool executes the tool with its stored config — gate on access.
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
	// A3: tenant callers only see audit rows owned by their workspace.
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
