package service

import (
	"context"
	"fmt"
	"strings"

	v1 "aranea-agents/api/kratos/agent/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// AgentService implements kratos agent.v1.
type AgentService struct {
	v1.UnimplementedAgentServiceServer

	uc              *biz.AgentUsecase
	evoUC           *biz.EvolutionUsecase
	mon             *biz.MonitorUsecase
	a2aUC           *biz.A2AUsecase
	promptAI        *PromptFileAIEditor
	agentTemplateUC *biz.AgentTemplateUsecase
	lg              loggateway.Logger
	monitorBus      contract.MonitorBus
}

func NewAgentService(uc *biz.AgentUsecase, evoUC *biz.EvolutionUsecase, mon *biz.MonitorUsecase, a2aUC *biz.A2AUsecase, promptAI *PromptFileAIEditor, agentTemplateUC *biz.AgentTemplateUsecase, lg loggateway.Logger, monitorBus contract.MonitorBus) *AgentService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &AgentService{uc: uc, evoUC: evoUC, mon: mon, a2aUC: a2aUC, promptAI: promptAI, agentTemplateUC: agentTemplateUC, lg: lg, monitorBus: monitorBus}
}

// logAgentFlow emits a user-visible flow log (流程日志) for agent CRUD steps.
// err != nil → error phase; otherwise done phase. Nil-safe: skipped when the
// monitor bus is not wired (tests).
func (s *AgentService) logAgentFlow(ctx context.Context, step, message string, err error, pairs ...event.Pair) {
	if s == nil || s.monitorBus == nil {
		return
	}
	flow := event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:    ctx,
		Domain: event.TraceDomainSystem,
		LG:     s.lg,
		Infra:  event.NewInfraFromBus(s.monitorBus),
	})
	if err != nil {
		flow.LogError(step, message, append(pairs, event.P("error", err.Error()))...)
		return
	}
	flow.LogDone(step, message, pairs...)
}

// assertAgentAccess 验证 caller 是否可读取目标 agent（P2-B IDOR 防护）。
// 共享 agent（workspace_id=""）对所有租户可读；变更须用 assertAgentMutateAccess。
func (s *AgentService) assertAgentAccess(ctx context.Context, agentID string) error {
	return s.checkAgentAccess(ctx, agentID, false)
}

// assertAgentMutateAccess 验证 caller 是否可变更目标 agent。
// 共享 agent（workspace_id=""）对租户只读（fail-closed）。
func (s *AgentService) assertAgentMutateAccess(ctx context.Context, agentID string) error {
	return s.checkAgentAccess(ctx, agentID, true)
}

func (s *AgentService) checkAgentAccess(ctx context.Context, agentID string, mutate bool) error {
	if agentID == "" {
		return nil
	}
	a, err := s.uc.Get(ctx, agentID)
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
		s.lg.Warn("agent access denied: workspace mismatch",
			loggateway.StepID("agent.idor"),
			loggateway.Str("agent_id", agentID),
			loggateway.Str("caller_ws", callerWS))
		return apierror.NotFound(apierror.DomainAgent, "agent not found")
	}
	return nil
}

func (s *AgentService) enrichEndpointFlags(ctx context.Context, agents []biz.Agent) {
	if s == nil || s.a2aUC == nil || len(agents) == 0 {
		return
	}
	ids := make([]string, 0, len(agents))
	for i := range agents {
		if id := strings.TrimSpace(agents[i].ID); id != "" {
			ids = append(ids, id)
		}
	}
	enabled, err := s.a2aUC.MapEndpointEnabled(ctx, ids)
	if err != nil {
		s.lg.Warn("agent list: a2a endpoint lookup failed",
			loggateway.StepID("agent.enrich_endpoints"), loggateway.Err(err))
		return
	}
	for i := range agents {
		agents[i].A2AEndpointEnabled = enabled[agents[i].ID]
	}
}

func (s *AgentService) enrichAgentEndpoint(ctx context.Context, a *biz.Agent) {
	if s == nil || s.a2aUC == nil || a == nil || strings.TrimSpace(a.ID) == "" {
		return
	}
	enabled, err := s.a2aUC.MapEndpointEnabled(ctx, []string{a.ID})
	if err != nil {
		s.lg.Warn("agent get: a2a endpoint lookup failed",
			loggateway.StepID("agent.enrich_endpoint"), loggateway.Str("agent_id", a.ID), loggateway.Err(err))
		return
	}
	a.A2AEndpointEnabled = enabled[a.ID]
}

func (s *AgentService) toProtoAgentEnriched(ctx context.Context, a biz.Agent) *v1.Agent {
	s.enrichAgentEndpoint(ctx, &a)
	return toProtoAgent(a)
}

// ListAgents implements GET /v1/agents.
// CheckAgentKey GET /v1/agent-keys/check?agent_key=
func (s *AgentService) CheckAgentKey(ctx context.Context, req *v1.CheckAgentKeyRequest) (*v1.CheckAgentKeyResponse, error) {
	available, msg, err := s.uc.CheckAgentKeyAvailability(ctx, req.GetAgentKey())
	if err != nil {
		return nil, err
	}
	return &v1.CheckAgentKeyResponse{Available: available, Message: msg}, nil
}

func (s *AgentService) ListAgents(ctx context.Context, req *v1.ListAgentsRequest) (*v1.ListAgentsResponse, error) {
	// P2-B: 系统 caller（WorkspaceID="")看全部；租户 caller 看 shared + own。
	callerWS := workspace.IDFromContext(ctx)
	if workspace.IsSystem(ctx) {
		callerWS = ""
	}
	page, err := s.uc.List(ctx, biz.AgentListQuery{
		Keyword:     req.GetKeyword(),
		Status:      req.GetStatus(),
		Provider:    req.GetProvider(),
		OrgNodeID:   req.GetOrgNodeId(),
		CreatedBy:   biz.ResolveListCreatedByFilter(ctx, req.GetCreatedBy()),
		WorkspaceID: callerWS,
		Limit:       int(req.GetLimit()),
		Offset:      int(req.GetOffset()),
	})
	if err != nil {
		return nil, err
	}
	s.enrichEndpointFlags(ctx, page.Items)
	out := &v1.ListAgentsResponse{
		Total:  int32(page.Total),
		Limit:  int32(page.Limit),
		Offset: int32(page.Offset),
	}
	for i := range page.Items {
		out.Items = append(out.Items, toProtoAgent(page.Items[i]))
	}
	return out, nil
}

// CreateAgent implements POST /v1/agents.
func (s *AgentService) CreateAgent(ctx context.Context, req *v1.CreateAgentRequest) (*v1.Agent, error) {
	a := fromProtoCreate(req)
	// P2-B: 租户 caller 创建的 agent 绑定到 caller workspace；系统 caller 创建共享 agent（workspace_id=""）。
	if !workspace.IsSystem(ctx) {
		a.WorkspaceID = workspace.IDFromContext(ctx)
	}
	s.lg.Info("创建 Agent", loggateway.StepID("agent.crud.create"), loggateway.Str("agent_key", a.AgentKey))
	created, err := s.uc.Create(ctx, a)
	if err != nil {
		s.lg.Error("创建 Agent 失败", loggateway.StepID("agent.crud.create"), loggateway.Str("agent_key", a.AgentKey), loggateway.Err(err))
		s.logAgentFlow(ctx, "agent.crud.create", "Agent 创建失败", err, event.P("agent_key", a.AgentKey))
		return nil, err
	}
	s.logAgentFlow(ctx, "agent.crud.create", "Agent 已创建", nil,
		event.P("agent_id", created.ID), event.P("agent_key", created.AgentKey))
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:     biz.AuditAction(biz.AuditVerbCreate, "agent"),
		Resource:   "agent",
		ResourceID: created.ID,
		Summary:    fmt.Sprintf("key=%s", created.AgentKey),
	})
	return s.toProtoAgentEnriched(ctx, created), nil
}

// GetAgent implements GET /v1/agents/{id}.
func (s *AgentService) GetAgent(ctx context.Context, req *v1.GetAgentRequest) (*v1.Agent, error) {
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
	return s.toProtoAgentEnriched(ctx, a), nil
}

// UpdateAgent implements PATCH /v1/agents/{id}.
func (s *AgentService) UpdateAgent(ctx context.Context, req *v1.UpdateAgentRequest) (*v1.Agent, error) {
	if req.GetAgent() == nil {
		return nil, apierror.BadRequest(apierror.DomainAgent, "agent body is required")
	}
	if err := s.assertAgentMutateAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	patch := fromProtoAgent(req.GetAgent())
	patch.WorkspaceID = "" // P2-B: workspace_id immutable on update
	s.lg.Info("更新 Agent", loggateway.StepID("agent.crud.update"), loggateway.Str("agent_id", req.GetId()), loggateway.Str("agent_key", patch.AgentKey))
	a, err := s.uc.Update(ctx, req.GetId(), patch)
	if err != nil {
		s.lg.Error("更新 Agent 失败", loggateway.StepID("agent.crud.update"), loggateway.Str("agent_id", req.GetId()), loggateway.Str("agent_key", patch.AgentKey), loggateway.Err(err))
		s.logAgentFlow(ctx, "agent.crud.update", "Agent 更新失败", err,
			event.P("agent_id", req.GetId()), event.P("agent_key", patch.AgentKey))
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound(apierror.DomainAgent, "agent not found")
		}
		return nil, err
	}
	s.logAgentFlow(ctx, "agent.crud.update", "Agent 已更新", nil,
		event.P("agent_id", a.ID), event.P("agent_key", a.AgentKey))
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:     biz.AuditAction(biz.AuditVerbUpdate, "agent"),
		Resource:   "agent",
		ResourceID: a.ID,
		Summary:    fmt.Sprintf("key=%s", a.AgentKey),
	})
	invalidateAgentBuildCache(a.ID)
	return s.toProtoAgentEnriched(ctx, a), nil
}

// DeleteAgent implements DELETE /v1/agents/{id}.
func (s *AgentService) DeleteAgent(ctx context.Context, req *v1.DeleteAgentRequest) (*emptypb.Empty, error) {
	if err := s.assertAgentMutateAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	// 先取 key 供审计 detail 使用（best-effort，取不到不阻断删除）。
	agentKey := ""
	if a, err := s.uc.Get(ctx, req.GetId()); err == nil {
		agentKey = a.AgentKey
	}
	s.lg.Info("删除 Agent", loggateway.StepID("agent.crud.delete"), loggateway.Str("agent_id", req.GetId()), loggateway.Str("agent_key", agentKey))
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		s.lg.Error("删除 Agent 失败", loggateway.StepID("agent.crud.delete"), loggateway.Str("agent_id", req.GetId()), loggateway.Str("agent_key", agentKey), loggateway.Err(err))
		s.logAgentFlow(ctx, "agent.crud.delete", "Agent 删除失败", err,
			event.P("agent_id", req.GetId()), event.P("agent_key", agentKey))
		return nil, err
	}
	s.logAgentFlow(ctx, "agent.crud.delete", "Agent 已删除", nil,
		event.P("agent_id", req.GetId()), event.P("agent_key", agentKey))
	invalidateAgentBuildCache(req.GetId())
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:     biz.AuditAction(biz.AuditVerbDelete, "agent"),
		Resource:   "agent",
		ResourceID: req.GetId(),
		Summary:    fmt.Sprintf("key=%s", agentKey),
	})
	return &emptypb.Empty{}, nil
}

// ToggleFavorite implements PATCH /v1/agents/{id}/favorite.
func (s *AgentService) ToggleFavorite(ctx context.Context, req *v1.ToggleFavoriteRequest) (*v1.Agent, error) {
	if err := s.assertAgentMutateAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	a, err := s.uc.ToggleFavorite(ctx, req.GetId())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, apierror.NotFound(apierror.DomainAgent, "agent not found")
		}
		return nil, err
	}
	return s.toProtoAgentEnriched(ctx, a), nil
}

// ListAgentCreators implements GET /v1/agents/creators.
func (s *AgentService) ListAgentCreators(ctx context.Context, _ *emptypb.Empty) (*v1.ListAgentCreatorsResponse, error) {
	items, err := s.uc.ListAgentCreators(ctx)
	if err != nil {
		return nil, err
	}
	out := &v1.ListAgentCreatorsResponse{Items: make([]*v1.AgentCreator, 0, len(items))}
	for _, c := range items {
		out.Items = append(out.Items, &v1.AgentCreator{UserId: c.UserID, Label: c.Label})
	}
	return out, nil
}
