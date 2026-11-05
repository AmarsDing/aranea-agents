package service

import (
	"context"
	"encoding/json"
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
		// 03 P0：extras 透传（省略=保留旧值；capabilities 空数组=清空）
		IconURL:         strings.TrimSpace(req.IconUrl),
		Capabilities:    req.Capabilities,
		DefaultAutonomy: strings.TrimSpace(req.DefaultAutonomy),
	}
	// M4 P3-6：liveness_override JSON 文本透传（省略=保留旧值；"{}"=显式清空回全局默认）。
	if s := strings.TrimSpace(req.LivenessOverrideJson); s != "" {
		var ov map[string]any
		if err := json.Unmarshal([]byte(s), &ov); err != nil {
			return nil, apierror.BadRequest(apierror.DomainAgentBridge, "liveness_override_json is not valid JSON")
		}
		agent.LivenessOverride = ov
	}
	// E9/M4 §B: known keys（codebuddy/workbuddy 归一 / claude_code / codex / deepseek / grok）
	// fill argv when omitted.
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
	// 03 P0：session_id 空=跨会话看板视图（按创建时间倒序，可按 agent_key 过滤）。
	if strings.TrimSpace(req.SessionId) == "" {
		tasks, err := a.svc.ListRecentTasks(ctx, int(req.Limit), strings.TrimSpace(req.AgentKey))
		if err != nil {
			return nil, err
		}
		out := &v1.ListTasksResponse{Items: make([]*v1.CodingTask, 0, len(tasks))}
		for _, t := range tasks {
			out.Items = append(out.Items, a.taskToProto(t))
		}
		return out, nil
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

// DispatchTask 03 P0：工作台表单直发（不经 LLM）。agent_key 空=自动路由；
// 项目名歧义时不落任务、返回 candidates 由前端消歧重发。
func (a *AgentBridgeAPI) DispatchTask(ctx context.Context, req *v1.DispatchTaskRequest) (*v1.DispatchTaskResponse, error) {
	if strings.TrimSpace(req.ProjectQuery) == "" {
		return nil, apierror.BadRequest(apierror.DomainAgentBridge, "project_query is required")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, apierror.BadRequest(apierror.DomainAgentBridge, "prompt is required")
	}
	origin := strings.TrimSpace(req.Origin)
	if origin == "" {
		origin = "workbench"
	}
	agentKey := strings.TrimSpace(req.AgentKey)
	if agentKey == "" {
		var err error
		agentKey, err = a.svc.AutoRouteAgentKey(ctx, agentBridgeWorkspace)
		if err != nil {
			return nil, err
		}
	}
	res, err := a.svc.DispatchTaskWithOrigin(ctx, req.SessionId, agentKey, req.ProjectQuery, req.Prompt, origin)
	if err != nil {
		return nil, err
	}
	out := &v1.DispatchTaskResponse{}
	if len(res.Candidates) > 0 {
		out.Candidates = make([]*v1.ProjectCandidate, 0, len(res.Candidates))
		for _, c := range res.Candidates {
			out.Candidates = append(out.Candidates, &v1.ProjectCandidate{
				Id: c.ID, Name: c.Name, Path: c.Path, Description: c.Description,
			})
		}
		return out, nil
	}
	out.Task = a.taskToProto(res.Task)
	return out, nil
}

// ListTaskEvents 03 P0 ④：任务步骤事件增量拉取（after_seq 回放游标）。
func (a *AgentBridgeAPI) ListTaskEvents(ctx context.Context, req *v1.ListTaskEventsRequest) (*v1.ListTaskEventsResponse, error) {
	if strings.TrimSpace(req.TaskId) == "" {
		return nil, apierror.BadRequest(apierror.DomainAgentBridge, "task_id is required")
	}
	events, err := a.svc.ListTaskEvents(ctx, req.TaskId, req.AfterSeq, int(req.Limit))
	if err != nil {
		return nil, err
	}
	out := &v1.ListTaskEventsResponse{Items: make([]*v1.CodingTaskEvent, 0, len(events))}
	for _, ev := range events {
		out.Items = append(out.Items, &v1.CodingTaskEvent{
			Id:        ev.ID,
			TaskId:    ev.TaskID,
			Seq:       ev.Seq,
			Kind:      ev.Kind,
			Title:     ev.Title,
			Text:      ev.Text,
			CreatedAt: ev.CreatedAt,
		})
	}
	return out, nil
}

// ReviewTask 03 P0：验收三动作（approve/reject/archive）。
func (a *AgentBridgeAPI) ReviewTask(ctx context.Context, req *v1.ReviewTaskRequest) (*v1.CodingTask, error) {
	if strings.TrimSpace(req.Id) == "" {
		return nil, apierror.BadRequest(apierror.DomainAgentBridge, "id is required")
	}
	if strings.TrimSpace(req.Action) == "" {
		return nil, apierror.BadRequest(apierror.DomainAgentBridge, "action is required")
	}
	t, err := a.svc.ReviewTask(ctx, req.Id, req.Action, req.Comment)
	if err != nil {
		return nil, err
	}
	return a.taskToProto(t), nil
}

// --- converters ---

// sensitiveEnvMark S1（M4 P0-1，AC-11）：API 响应中密钥形态 env 值一律掩码，
// 落库值不受影响。非敏感键（PATH 等）保留原值便于管理页展示。
func maskAgentEnv(env map[string]string) map[string]string {
	if len(env) == 0 {
		return env
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		if isSensitiveEnvKey(k) {
			out[k] = "***"
		} else {
			out[k] = v
		}
	}
	return out
}

// isSensitiveEnvKey 判定密钥形态键：KEY/TOKEN/SECRET/PASSWORD/CREDENTIAL（大小写不敏感）。
func isSensitiveEnvKey(key string) bool {
	up := strings.ToUpper(key)
	for _, tok := range []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "CREDENTIAL"} {
		if strings.Contains(up, tok) {
			return true
		}
	}
	return false
}

func agentToProto(agent *agentbridge.CodingAgent) *v1.CodingAgent {
	return &v1.CodingAgent{
		Id:             agent.ID,
		AgentKey:       agent.AgentKey,
		DisplayName:    agent.DisplayName,
		Command:        agent.Command,
		Args:           agent.Args,
		Env:            maskAgentEnv(agent.Env),
		Enabled:        agent.Enabled,
		LastProbeOk:    agent.LastProbeOK,
		LastProbeError: agent.LastProbeError,
		CreatedAt:      agent.CreatedAt,
		UpdatedAt:      agent.UpdatedAt,
		// M4 P3-6：liveness_override 透传（空=未配置，全局默认生效）
		LivenessOverrideJson: marshalLivenessOverride(agent.LivenessOverride),
		// 03 P0：注册表驱动前端渲染
		IconUrl:         agent.IconURL,
		Capabilities:    agent.Capabilities,
		DefaultAutonomy: agent.DefaultAutonomy,
	}
}

// marshalLivenessOverride 序列化阈值为 JSON 文本；未配置/空表回落空串。
func marshalLivenessOverride(ov map[string]any) string {
	if len(ov) == 0 {
		return ""
	}
	data, err := json.Marshal(ov)
	if err != nil {
		return ""
	}
	return string(data)
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
		// M4：活跃性徽标数据源（设计 §B/§F）
		ChildPid:      int32(t.ChildPID),
		LastActiveAt:  t.LastActiveAt,
		LastExcerpt:   t.LastExcerpt,
		HeartbeatMiss: int32(t.HeartbeatMiss),
		LivenessState: t.LivenessState,
		// 03 P0：发起来源与验收
		Origin:        t.Origin,
		ReviewStatus:  t.ReviewStatus,
		ReviewComment: t.ReviewComment,
		ReviewedAt:    t.ReviewedAt,
	}
}

// --- interface guards ---

var _ v1.AgentBridgeServiceServer = (*AgentBridgeAPI)(nil)
