package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/agentbridge/v1"
	"aranea-agents/internal/biz/agentbridge"
	"aranea-agents/pkg/apierror"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// agentBridgeWorkspace M1 固定单工作区（管理 API 不带 workspace 参数）。
const agentBridgeWorkspace = "default"

// AgentBridgeAPI 是 kratos agentbridge.v1 的 gRPC/HTTP 处理器，
// 包装 AgentBridgeService 做 proto ↔ biz 转换（域名方法与 RPC 签名
// 存在冲突——GetTask/CancelTask/ProbeAgent——故不直接内嵌实现）。
type AgentBridgeAPI struct {
	v1.UnimplementedAgentBridgeServiceServer

	svc *AgentBridgeService
}

// NewAgentBridgeAPI 构造 proto 处理器。
func NewAgentBridgeAPI(svc *AgentBridgeService) *AgentBridgeAPI {
	return &AgentBridgeAPI{svc: svc}
}

// --- agents ---

func (a *AgentBridgeAPI) UpsertAgent(ctx context.Context, req *v1.UpsertAgentRequest) (*v1.CodingAgent, error) {
	if strings.TrimSpace(req.AgentKey) == "" {
		return nil, apierror.BadRequest(apierror.DomainAgentBridge, "agent_key is required")
	}
	agent := &agentbridge.CodingAgent{
		Workspace:   agentBridgeWorkspace,
		AgentKey:    strings.TrimSpace(req.AgentKey),
		DisplayName: req.DisplayName,
		Command:     strings.TrimSpace(req.Command),
		Args:        req.Args,
		Env:         req.Env,
		Enabled:     req.Enabled,
	}
	// E9: known keys (codebuddy / claude_code / codex) fill argv when omitted.
	agentbridge.ApplyDefaultLaunch(agent)
	if strings.TrimSpace(agent.Command) == "" {
		return nil, apierror.BadRequest(apierror.DomainAgentBridge, "command is required")
	}
	if agent.DisplayName == "" {
		agent.DisplayName = agent.AgentKey
	}
	if err := a.svc.UpsertAgent(ctx, agent); err != nil {
		return nil, err
	}
	stored, err := a.svc.agents.GetByKey(ctx, agentBridgeWorkspace, agent.AgentKey)
	if err != nil {
		return nil, err
	}
	return agentToProto(stored), nil
}

func (a *AgentBridgeAPI) ListAgents(ctx context.Context, _ *emptypb.Empty) (*v1.ListAgentsResponse, error) {
	agents, err := a.svc.ListAgents(ctx, agentBridgeWorkspace)
	if err != nil {
		return nil, err
	}
	out := &v1.ListAgentsResponse{Items: make([]*v1.CodingAgent, 0, len(agents))}
	for _, agent := range agents {
		out.Items = append(out.Items, agentToProto(agent))
	}
	return out, nil
}

func (a *AgentBridgeAPI) ProbeAgent(ctx context.Context, req *v1.ProbeAgentRequest) (*v1.ProbeAgentResponse, error) {
	if strings.TrimSpace(req.AgentKey) == "" {
		return nil, apierror.BadRequest(apierror.DomainAgentBridge, "agent_key is required")
	}
	if err := a.svc.ProbeAgent(ctx, agentBridgeWorkspace, req.AgentKey); err != nil {
		return nil, err
	}
	stored, err := a.svc.agents.GetByKey(ctx, agentBridgeWorkspace, req.AgentKey)
	if err != nil {
		return nil, err
	}
	return &v1.ProbeAgentResponse{Ok: stored.LastProbeOK, Error: stored.LastProbeError}, nil
}

// --- projects ---

func (a *AgentBridgeAPI) UpsertProject(ctx context.Context, req *v1.UpsertProjectRequest) (*v1.CodingProject, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, apierror.BadRequest(apierror.DomainAgentBridge, "name is required")
	}
	if strings.TrimSpace(req.Path) == "" {
		return nil, apierror.BadRequest(apierror.DomainAgentBridge, "path is required")
	}
	p := &agentbridge.CodingProject{
		Workspace:   agentBridgeWorkspace,
		Name:        strings.TrimSpace(req.Name),
		Path:        strings.TrimSpace(req.Path),
		Description: req.Description,
	}
	if err := a.svc.UpsertProject(ctx, p); err != nil {
		return nil, err
	}
	stored, err := a.svc.projects.GetByName(ctx, agentBridgeWorkspace, p.Name)
	if err != nil {
		return nil, err
	}
	return projectToProto(stored), nil
}

func (a *AgentBridgeAPI) ListProjects(ctx context.Context, _ *emptypb.Empty) (*v1.ListProjectsResponse, error) {
	projects, err := a.svc.ListProjects(ctx, agentBridgeWorkspace)
	if err != nil {
		return nil, err
	}
	out := &v1.ListProjectsResponse{Items: make([]*v1.CodingProject, 0, len(projects))}
	for _, p := range projects {
		out.Items = append(out.Items, projectToProto(p))
	}
	return out, nil
}

func (a *AgentBridgeAPI) DeleteProject(ctx context.Context, req *v1.DeleteProjectRequest) (*emptypb.Empty, error) {
	if strings.TrimSpace(req.Id) == "" {
		return nil, apierror.BadRequest(apierror.DomainAgentBridge, "id is required")
	}
	if err := a.svc.DeleteProject(ctx, req.Id); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// --- tasks ---

func (a *AgentBridgeAPI) ListTasks(ctx context.Context, req *v1.ListTasksRequest) (*v1.ListTasksResponse, error) {
	if strings.TrimSpace(req.SessionId) == "" {
		return nil, apierror.BadRequest(apierror.DomainAgentBridge, "session_id is required")
	}
	tasks, err := a.svc.ListSessionTasks(ctx, req.SessionId, int(req.Limit))
	if err != nil {
		return nil, err
	}
	out := &v1.ListTasksResponse{Items: make([]*v1.CodingTask, 0, len(tasks))}
	for _, t := range tasks {
		out.Items = append(out.Items, a.taskToProto(t))
	}
	return out, nil
}

func (a *AgentBridgeAPI) GetTask(ctx context.Context, req *v1.GetTaskRequest) (*v1.CodingTask, error) {
	if strings.TrimSpace(req.Id) == "" {
		return nil, apierror.BadRequest(apierror.DomainAgentBridge, "id is required")
	}
	t, err := a.svc.GetTask(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return a.taskToProto(t), nil
}

func (a *AgentBridgeAPI) CancelTask(ctx context.Context, req *v1.CancelTaskRequest) (*v1.CodingTask, error) {
	if strings.TrimSpace(req.Id) == "" {
		return nil, apierror.BadRequest(apierror.DomainAgentBridge, "id is required")
	}
	if err := a.svc.CancelTask(ctx, req.Id); err != nil {
		return nil, err
	}
	t, err := a.svc.GetTask(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return a.taskToProto(t), nil
}

// --- converters ---

func agentToProto(agent *agentbridge.CodingAgent) *v1.CodingAgent {
	return &v1.CodingAgent{
		Id:             agent.ID,
		AgentKey:       agent.AgentKey,
		DisplayName:    agent.DisplayName,
		Command:        agent.Command,
		Args:           agent.Args,
		Env:            agent.Env,
		Enabled:        agent.Enabled,
		LastProbeOk:    agent.LastProbeOK,
		LastProbeError: agent.LastProbeError,
		CreatedAt:      agent.CreatedAt,
		UpdatedAt:      agent.UpdatedAt,
	}
}

func projectToProto(p *agentbridge.CodingProject) *v1.CodingProject {
	return &v1.CodingProject{
		Id:          p.ID,
		Name:        p.Name,
		Path:        p.Path,
		Description: p.Description,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

func (a *AgentBridgeAPI) taskToProto(t *agentbridge.CodingTask) *v1.CodingTask {
	agentKey, projectName := a.svc.resolveNames(t.Workspace, t.AgentID, t.ProjectID)
	return &v1.CodingTask{
		Id:            t.ID,
		SessionId:     t.SessionID,
		AgentKey:      agentKey,
		ProjectName:   projectName,
		Prompt:        t.Prompt,
		Status:        string(t.Status),
		Summary:       t.Summary,
		Error:         t.Error,
		ProgressCount: int32(t.ProgressCount),
		CreatedAt:     t.CreatedAt,
		CompletedAt:   t.CompletedAt,
	}
}

// --- interface guards ---

var _ v1.AgentBridgeServiceServer = (*AgentBridgeAPI)(nil)
