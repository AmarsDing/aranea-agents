package service

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/agentbridge"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// bridgeProgressWindow 进度事件限流窗口（每任务 leading-edge，设计 §11：
// session/update 每任务 5s 窗口聚合 1 条，禁止每事件一条）。
const bridgeProgressWindow = 5 * time.Second

// 前端 WS 通知类型（设计 §9：coding_task_progress / coding_task_completed 等）。
const (
	noticeCodingTaskProgress  = "coding_task_progress"
	noticeCodingTaskCompleted = "coding_task_completed"
	noticeCodingTaskFailed    = "coding_task_failed"
	noticeCodingTaskCancelled = "coding_task_cancelled"
)

// AgentBridgeServiceDeps 是 AgentBridgeService 的构造依赖。
// 用例经 BindUsecase 二次绑定（构造环：usecase 的 TaskListener 是 service 自身）。
type AgentBridgeServiceDeps struct {
	Agents   agentbridge.AgentRepo
	Projects agentbridge.ProjectRepo
	Bus      biz.EventBus
	Infra    *event.Infra      // 可选：nil 时流程日志仅落进程日志
	Clock    func() time.Time  // 可选：默认 time.Now（测试注入假时钟）
	Logger   loggateway.Logger // 可选：默认 Noop
}

// AgentBridgeService 对 tools/proto 暴露编程桥接用例，并负责：
//   - 进度事件限流聚合（每任务 5s 窗口 leading-edge）
//   - 终态事件发射（实现 agentbridge.TaskListener）
//   - 流程日志（agentbridge.task.* / agentbridge.probe.degraded）
type AgentBridgeService struct {
	agents   agentbridge.AgentRepo
	projects agentbridge.ProjectRepo
	bus      biz.EventBus
	infra    *event.Infra
	clock    func() time.Time
	lg       loggateway.Logger

	mu sync.RWMutex
	uc *agentbridge.AgentBridgeUsecase
}

// NewAgentBridgeService 构造服务。
func NewAgentBridgeService(deps AgentBridgeServiceDeps) *AgentBridgeService {
	clock := deps.Clock
	if clock == nil {
		clock = time.Now
	}
	lg := deps.Logger
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &AgentBridgeService{
		agents:   deps.Agents,
		projects: deps.Projects,
		bus:      deps.Bus,
		infra:    deps.Infra,
		clock:    clock,
		lg:       lg.With(loggateway.Domain("agentbridge")),
	}
}

// ProvideAgentBridgeService 是 Wire provider：装配 service + usecase 构造环
// （usecase 的 TaskListener 是 service 自身，须 NewAgentBridgeService →
// NewAgentBridgeUsecase(Listener) → BindUsecase 二次绑定，wire 无法表达）。
func ProvideAgentBridgeService(agents agentbridge.AgentRepo, projects agentbridge.ProjectRepo, tasks agentbridge.TaskRepo, factory agentbridge.SessionFactory, bus biz.EventBus, infra *event.Infra, lg loggateway.Logger) *AgentBridgeService {
	svc := NewAgentBridgeService(AgentBridgeServiceDeps{
		Agents:   agents,
		Projects: projects,
		Bus:      bus,
		Infra:    infra,
		Logger:   lg,
	})
	uc := agentbridge.NewAgentBridgeUsecase(agentbridge.UsecaseDeps{
		Agents:   agents,
		Projects: projects,
		Tasks:    tasks,
		Sessions: factory,
		Listener: svc,
		Logger:   lg,
	})
	svc.BindUsecase(uc)
	return svc
}

// BindUsecase 绑定用例（Wire/测试装配顺序：NewAgentBridgeService →
// NewAgentBridgeUsecase(Listener: svc) → BindUsecase）。
func (s *AgentBridgeService) BindUsecase(uc *agentbridge.AgentBridgeUsecase) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uc = uc
}

func (s *AgentBridgeService) usecase() (*agentbridge.AgentBridgeUsecase, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.uc == nil {
		return nil, apierror.Internal(apierror.DomainAgentBridge, "agentbridge usecase not bound")
	}
	return s.uc, nil
}

// DispatchTask 派发编程任务：解析项目（歧义返回候选，不发事件）→ 落任务 →
// spawn ACP 会话异步执行。进度经限流 handler 转发，终态经 OnTaskTerminal 发射。
func (s *AgentBridgeService) DispatchTask(ctx context.Context, sessionID, agentKey, projectQuery, prompt string) (*agentbridge.DispatchResult, error) {
	uc, err := s.usecase()
	if err != nil {
		return nil, err
	}
	h := newBridgeProgressHandler(s, sessionID)
	res, err := uc.Dispatch(ctx, agentbridge.DispatchInput{
		SessionID:    sessionID,
		AgentKey:     agentKey,
		ProjectQuery: projectQuery,
		Prompt:       prompt,
		Handler:      h,
	})
	if res == nil || res.Task == nil {
		// 歧义/错误路径：无任务实体，释放 handler 等待者且不发任何事件。
		h.bind("", "", "")
		return res, err
	}
	agentKeyResolved, projectName := s.resolveNames(res.Task.Workspace, res.Task.AgentID, res.Task.ProjectID)
	h.bind(res.Task.ID, agentKeyResolved, projectName)
	s.emitter(ctx, sessionID).LogDone("agentbridge.task.dispatch", "编程任务已派发",
		event.P("task_id", res.Task.ID),
		event.P("agent_key", agentKeyResolved),
		event.P("project_name", projectName))
	// K7：进程生命周期流程日志。uc.Dispatch 返回成功即 ACP 进程已 spawn。
	s.emitter(ctx, sessionID).LogDone("agentbridge.process.spawn", "编程 Agent 进程已启动",
		event.P("task_id", res.Task.ID),
		event.P("agent_key", agentKeyResolved),
		event.P("project_name", projectName))
	return res, nil
}

// GetTask 查询单个任务。
func (s *AgentBridgeService) GetTask(ctx context.Context, id string) (*agentbridge.CodingTask, error) {
	uc, err := s.usecase()
	if err != nil {
		return nil, err
	}
	return uc.GetTask(ctx, id)
}

// ListSessionTasks 列出会话任务（按创建时间倒序）。
func (s *AgentBridgeService) ListSessionTasks(ctx context.Context, sessionID string, limit int) ([]*agentbridge.CodingTask, error) {
	uc, err := s.usecase()
	if err != nil {
		return nil, err
	}
	return uc.ListSessionTasks(ctx, sessionID, limit)
}

// CancelTask 取消任务（running/awaiting_approval → cancelling → cancelled）。
func (s *AgentBridgeService) CancelTask(ctx context.Context, id string) error {
	uc, err := s.usecase()
	if err != nil {
		return err
	}
	return uc.Cancel(ctx, id)
}

// OnTaskTerminal 实现 agentbridge.TaskListener：终态事件 + 流程日志。
// 在状态机推进路径上同步调用，必须快速返回（仅内存解析 + 异步总线发布）。
func (s *AgentBridgeService) OnTaskTerminal(t *agentbridge.CodingTask) {
	if t == nil {
		return
	}
	agentKey, projectName := s.resolveNames(t.Workspace, t.AgentID, t.ProjectID)
	meta := map[string]any{
		"task_id":      t.ID,
		"agent_key":    agentKey,
		"project_name": projectName,
		"session_id":   t.SessionID,
		"status":       string(t.Status),
	}
	ctx := context.Background()
	switch t.Status {
	case agentbridge.StatusDone:
		meta["summary"] = t.Summary
		s.publish(biz.NewSystemNoticeEvent(t.SessionID, noticeCodingTaskCompleted, t.Summary, meta))
		s.emitter(ctx, t.SessionID).LogDone("agentbridge.task.done", "编程任务完成",
			event.P("task_id", t.ID), event.P("agent_key", agentKey))
		s.emitter(ctx, t.SessionID).LogDone("agentbridge.process.exit", "编程 Agent 进程已退出",
			event.P("task_id", t.ID), event.P("agent_key", agentKey))
	case agentbridge.StatusFailed:
		meta["error"] = t.Error
		s.publish(biz.NewSystemNoticeEvent(t.SessionID, noticeCodingTaskFailed, t.Error, meta))
		s.emitter(ctx, t.SessionID).LogError("agentbridge.task.failed", "编程任务失败",
			event.P("task_id", t.ID), event.P("agent_key", agentKey), event.P("error", t.Error))
		// ACPSessionID 为空 = spawn 失败，进程从未启动，不发 process.exit。
		if t.ACPSessionID != "" {
			s.emitter(ctx, t.SessionID).LogError("agentbridge.process.exit", "编程 Agent 进程异常退出",
				event.P("task_id", t.ID), event.P("agent_key", agentKey), event.P("error", t.Error))
		}
	case agentbridge.StatusCancelled:
		s.publish(biz.NewSystemNoticeEvent(t.SessionID, noticeCodingTaskCancelled, "编程任务已取消", meta))
		s.emitter(ctx, t.SessionID).LogDone("agentbridge.task.cancelled", "编程任务取消",
			event.P("task_id", t.ID), event.P("agent_key", agentKey))
		s.emitter(ctx, t.SessionID).LogDone("agentbridge.process.exit", "编程 Agent 进程已退出",
			event.P("task_id", t.ID), event.P("agent_key", agentKey))
	}
}

// ProbeAgent 探测工具命令可用性（exec.LookPath），结果落库；
// 探测失败是业务结果而非系统错误，返回 nil（设计 §10）。
func (s *AgentBridgeService) ProbeAgent(ctx context.Context, workspace, agentKey string) error {
	agent, err := s.agents.GetByKey(ctx, workspace, agentKey)
	if err != nil {
		return err
	}
	if _, lookErr := exec.LookPath(agent.Command); lookErr != nil {
		msg := fmt.Sprintf("command %q not found in PATH", agent.Command)
		if uerr := s.agents.UpdateProbe(ctx, agent.ID, false, msg); uerr != nil {
			return uerr
		}
		s.lg.Warn("agentbridge probe degraded",
			loggateway.Str("agent_key", agentKey), loggateway.Str("command", agent.Command))
		s.emitter(ctx, "").LogWarn("agentbridge.probe.degraded", "", msg,
			event.P("agent_key", agentKey), event.P("command", agent.Command))
		return nil
	}
	return s.agents.UpdateProbe(ctx, agent.ID, true, "")
}

// RecoverActiveTasks 服务重启恢复：活跃任务标记 failed（启动钩子调用）。
func (s *AgentBridgeService) RecoverActiveTasks(ctx context.Context) error {
	uc, err := s.usecase()
	if err != nil {
		return err
	}
	return uc.RecoverActiveTasks(ctx)
}

// --- 管理 API 支撑（agentbridge_api.go 的 proto 处理器调用） ---

// UpsertAgent 注册/更新编程工具（删除经 enabled=false，M1 不提供物理删除）。
func (s *AgentBridgeService) UpsertAgent(ctx context.Context, agent *agentbridge.CodingAgent) error {
	return s.agents.Upsert(ctx, agent)
}

// ListAgents 列出工作区编程工具。
func (s *AgentBridgeService) ListAgents(ctx context.Context, workspace string) ([]*agentbridge.CodingAgent, error) {
	return s.agents.List(ctx, workspace)
}

// UpsertProject 注册/更新项目目录（cwd 白名单）。
func (s *AgentBridgeService) UpsertProject(ctx context.Context, p *agentbridge.CodingProject) error {
	return s.projects.Upsert(ctx, p)
}

// ListProjects 列出工作区项目目录。
func (s *AgentBridgeService) ListProjects(ctx context.Context, workspace string) ([]*agentbridge.CodingProject, error) {
	return s.projects.List(ctx, workspace)
}

// DeleteProject 删除项目目录。
func (s *AgentBridgeService) DeleteProject(ctx context.Context, id string) error {
	return s.projects.Delete(ctx, id)
}

// resolveNames 由任务外键解析展示用 agent_key / project_name（best-effort，
// 解析失败留空串，不阻断事件发射）。
func (s *AgentBridgeService) resolveNames(workspace, agentID, projectID string) (agentKey, projectName string) {
	if agents, err := s.agents.List(context.Background(), workspace); err == nil {
		for _, a := range agents {
			if a.ID == agentID {
				agentKey = a.AgentKey
				break
			}
		}
	}
	if projects, err := s.projects.List(context.Background(), workspace); err == nil {
		for _, p := range projects {
			if p.ID == projectID {
				projectName = p.Name
				break
			}
		}
	}
	return agentKey, projectName
}

func (s *AgentBridgeService) publish(e biz.Event) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(context.Background(), e)
}

func (s *AgentBridgeService) emitter(ctx context.Context, sessionID string) *event.TraceEmitter {
	return event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:       ctx,
		SessionID: sessionID,
		Domain:    event.TraceDomainAgentBridge,
		LG:        s.lg,
		Infra:     s.infra,
	})
}

// bridgeProgressHandler 是单任务的进度回调：OnUpdate 限流（5s 窗口
// leading-edge）后经 EventBus 发射 coding_task_progress。
// taskID 等元数据在 Dispatch 返回后由 bind 填充（任务 ID 由 data 层分配），
// OnUpdate 在 bind 前阻塞等待，保证事件必带 task_id。
type bridgeProgressHandler struct {
	svc       *AgentBridgeService
	sessionID string

	ready    chan struct{}
	bindOnce sync.Once
	taskID   string
	agentKey string
	project  string
	mu       sync.Mutex
	lastEmit time.Time
}

func newBridgeProgressHandler(svc *AgentBridgeService, sessionID string) *bridgeProgressHandler {
	return &bridgeProgressHandler{svc: svc, sessionID: sessionID, ready: make(chan struct{})}
}

func (h *bridgeProgressHandler) bind(taskID, agentKey, projectName string) {
	h.bindOnce.Do(func() {
		h.taskID = taskID
		h.agentKey = agentKey
		h.project = projectName
		close(h.ready)
	})
}

// OnUpdate 实现 agentbridge.EventHandler（在 run goroutine 上调用）。
func (h *bridgeProgressHandler) OnUpdate(kind, text string) {
	<-h.ready
	if h.taskID == "" {
		return
	}
	h.mu.Lock()
	now := h.svc.clock()
	if now.Sub(h.lastEmit) < bridgeProgressWindow {
		h.mu.Unlock()
		return
	}
	h.lastEmit = now
	h.mu.Unlock()
	h.svc.publish(biz.NewSystemNoticeEvent(h.sessionID, noticeCodingTaskProgress, text, map[string]any{
		"task_id":      h.taskID,
		"agent_key":    h.agentKey,
		"project_name": h.project,
		"session_id":   h.sessionID,
		"update_kind":  kind,
	}))
}

// OnPermission 实现 agentbridge.EventHandler。M1 审批中继未启用（设计 §8：
// M2 起中继），取首选项放行，与 biz discardEvents 默认行为一致。
func (h *bridgeProgressHandler) OnPermission(_ context.Context, _ string, opts []agentbridge.PermissionOption) (string, error) {
	if len(opts) == 0 {
		return "", errors.New("agentbridge: permission request without options")
	}
	return opts[0].OptionID, nil
}

// --- interface guards ---

var _ agentbridge.TaskListener = (*AgentBridgeService)(nil)
