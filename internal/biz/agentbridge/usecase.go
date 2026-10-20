package agentbridge

import (
	"context"
	"fmt"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// MaxConcurrentPerAgent 同 agent_key 活跃任务上限（设计 §10：M1 超限直接拒绝）。
const MaxConcurrentPerAgent = 2

// SessionFactory 按 agent 注册信息 spawn ACP 会话（service 层实现注入）。
// Stability:internal
type SessionFactory interface {
	Spawn(ctx context.Context, agent *CodingAgent) (ACPSession, error)
}

// UsecaseDeps 是 AgentBridgeUsecase 的构造依赖。
type UsecaseDeps struct {
	Agents   AgentRepo
	Projects ProjectRepo
	Tasks    TaskRepo
	Sessions SessionFactory
	Listener TaskListener      // 可选：终态通知（事件发射/播报）
	Logger   loggateway.Logger // 可选，nil 时 Noop
	// Reaper 可选（M4 P0-4）：按 child_pid（进程组根）收割孤儿子进程树，
	// 服务重启恢复用；nil 时退化为仅标记 failed（旧行为）。
	// 生产装配 = acp.ReapOrphanPID（Windows taskkill /T /F）。
	Reaper func(pid int) error
	// Liveness M4 P3（活跃性监管，设计 §E）：全局阈值默认；零字段回落 §E 默认。
	// 按 key 覆盖经 coding_agents.liveness_override 在派发时解析（ResolveLivenessConfig）。
	Liveness LivenessConfig
	// Decider 可选（M4 P3）：G3 失联确认卡（service 实现，复用审批卡通道）；
	// nil 时失联停留 G3 观望（活动信号可救回，不自动杀）。
	Decider LivenessDecider
	// Hints 可选（M4 P3）：阶梯迁移/用户提示出口（service 流程日志 + 进度面板）。
	Hints LivenessListener
}

// AgentBridgeUsecase 编排外部编程任务的生命周期：派发/取消/消歧/恢复。
type AgentBridgeUsecase struct {
	agents      AgentRepo
	projects    ProjectRepo
	tasks       TaskRepo
	sessions    SessionFactory
	listener    TaskListener
	reaper      func(pid int) error
	livenessCfg LivenessConfig
	decider     LivenessDecider
	hints       LivenessListener
	lg          loggateway.Logger

	mu      sync.Mutex
	running map[string]ACPSession          // taskID → 活跃会话（取消用）
	sups    map[string]*livenessSupervisor // taskID → 活跃性监管器（M4）
}

// NewAgentBridgeUsecase 构造用例。
func NewAgentBridgeUsecase(deps UsecaseDeps) *AgentBridgeUsecase {
	lg := deps.Logger
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &AgentBridgeUsecase{
		agents:      deps.Agents,
		projects:    deps.Projects,
		tasks:       deps.Tasks,
		sessions:    deps.Sessions,
		listener:    deps.Listener,
		reaper:      deps.Reaper,
		livenessCfg: deps.Liveness.withDefaults(),
		decider:     deps.Decider,
		hints:       deps.Hints,
		lg:          lg.With(loggateway.Domain("agentbridge")),
		running:     map[string]ACPSession{},
		sups:        map[string]*livenessSupervisor{},
	}
}

// DispatchInput 是派发入参。
type DispatchInput struct {
	SessionID    string // 精灵会话 ID（审批卡片路由目标）
	Workspace    string // 缺省 default
	AgentKey     string
	ProjectQuery string // 项目名（语音解析键）
	Prompt       string
	Handler      EventHandler // 进度/审批回调（service 注入）；nil 时丢弃
}

// DispatchResult 是派发出参。Candidates 非空表示项目名歧义（未落任务）。
type DispatchResult struct {
	Task       *CodingTask
	Candidates []ProjectCandidate
}

// Dispatch 解析项目 → 校验并发 → 落任务 → spawn ACP 会话 → 异步执行 Prompt。
// 返回时任务已处于 running（spawn 失败则落 failed 并返回错误）。
func (u *AgentBridgeUsecase) Dispatch(ctx context.Context, in DispatchInput) (*DispatchResult, error) {
	if in.Workspace == "" {
		in.Workspace = "default"
	}
	if in.Handler == nil {
		in.Handler = discardEvents{}
	}
	agent, project, candidates, err := u.resolveTarget(ctx, in)
	if err != nil || candidates != nil {
		return &DispatchResult{Candidates: candidates}, err
	}
	if err := u.checkConcurrency(ctx, agent.ID); err != nil {
		return nil, err
	}

	task := &CodingTask{
		Workspace: in.Workspace,
		SessionID: in.SessionID,
		AgentID:   agent.ID,
		ProjectID: project.ID,
		Prompt:    in.Prompt,
		Status:    StatusDispatched,
	}
	if err := u.tasks.Create(ctx, task); err != nil {
		return nil, err
	}

	sess, err := u.sessions.Spawn(ctx, agent)
	if err != nil {
		u.failTaskFrom(StatusDispatched, task.ID, fmt.Sprintf("spawn failed: %v", err))
		u.notifyTerminal(task.ID)
		return nil, apierror.BadRequest(apierror.DomainAgentBridge,
			"start agent %s failed: %v", agent.AgentKey, err)
	}
	acpSessID := task.ID // M1：ACP session id 由会话实现持有；此处先用任务 ID 占位关联
	// M4 P0-2b（S4）：任务行持久化子进程 PID（Windows 为进程组根），重启收割用
	pid := sess.ProcessPID()
	if err := u.transition(task.ID, StatusDispatched, StatusRunning, TaskPatch{ACPSessionID: &acpSessID, ChildPID: &pid}); err != nil {
		_ = sess.Close()
		return nil, err
	}
	task.Status = StatusRunning
	task.ChildPID = pid

	u.track(task.ID, sess)
	u.lg.Info("agentbridge process spawned",
		loggateway.Str("task_id", task.ID),
		loggateway.Str("agent_key", agent.AgentKey),
		loggateway.Int("child_pid", pid))
	// M4 P3（活跃性监管）：派发即开监管器（阈值按 agent.liveness_override 解析）。
	sup := u.trackSuper(newLivenessSupervisor(u, task, sess,
		ResolveLivenessConfig(u.livenessCfg, agent.LivenessOverride)))
	safego.GoBackground("agentbridge.liveness.watch", sup.run)
	safego.GoBackground("agentbridge.task.run", func() { u.run(task, sess, sup, project.Path, in.Handler) })
	return &DispatchResult{Task: task}, nil
}

// resolveTarget 解析 agent + 项目（消歧三分支）。
func (u *AgentBridgeUsecase) resolveTarget(ctx context.Context, in DispatchInput) (*CodingAgent, *CodingProject, []ProjectCandidate, error) {
	agent, err := u.agents.GetByKey(ctx, in.Workspace, in.AgentKey)
	if err != nil {
		return nil, nil, nil, err
	}
	if !agent.Enabled {
		return nil, nil, nil, apierror.BadRequest(apierror.DomainAgentBridge,
			"agent %s is disabled", in.AgentKey)
	}

	project, err := u.projects.GetByName(ctx, in.Workspace, in.ProjectQuery)
	if err == nil {
		return agent, project, nil, nil
	}
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		return nil, nil, nil, err
	}
	matches, err := u.projects.Match(ctx, in.Workspace, in.ProjectQuery)
	if err != nil {
		return nil, nil, nil, err
	}
	switch len(matches) {
	case 0:
		return nil, nil, nil, apierror.NotFound(apierror.DomainAgentBridge,
			"no registered project matches %q", in.ProjectQuery)
	case 1:
		return agent, matches[0], nil, nil
	}
	candidates := make([]ProjectCandidate, 0, len(matches))
	for _, m := range matches {
		candidates = append(candidates, ProjectCandidate{
			ID: m.ID, Name: m.Name, Path: m.Path, Description: m.Description,
		})
	}
	return nil, nil, candidates, nil
}

// checkConcurrency 拒绝同 agent 活跃任务超上限的派发（M1：直接报错，不入队）。
func (u *AgentBridgeUsecase) checkConcurrency(ctx context.Context, agentID string) error {
	active, err := u.tasks.ListActive(ctx)
	if err != nil {
		return err
	}
	n := 0
	for _, t := range active {
		if t.AgentID == agentID {
			n++
		}
	}
	if n >= MaxConcurrentPerAgent {
		return apierror.Conflict(apierror.DomainAgentBridge,
			"agent already has %d active tasks (limit %d), retry later", n, MaxConcurrentPerAgent)
	}
	return nil
}

// run 在独立 ctx 中执行 Prompt 至结束，按结果推进终态。
// 调用方 ctx 随工具调用返回即取消，任务必须脱离之运行。M4 P3-6（ADR-88）：
// 单任务硬超时已废除——长任务合法性由活跃性阶梯保障（liveness.go，
// 静默→探测→失联处置），ctx 仅随 cancel/kill 经 ACP 会话作用于 Prompt。
func (u *AgentBridgeUsecase) run(task *CodingTask, sess ACPSession, sup *livenessSupervisor, cwd string, h EventHandler) {
	// defer 执行逆序：recover（最先）→ notifyTerminal → Close → untrack → exit 日志。
	// recover 必须先于 notifyTerminal：panic 路径在此推进 failed，listener 才能收到终态。
	defer func() {
		u.lg.Info("agentbridge process exited", loggateway.Str("task_id", task.ID))
	}()
	defer u.untrackSuper(task.ID)
	defer sup.stop()
	defer u.untrack(task.ID)
	defer sess.Close()
	defer u.notifyTerminal(task.ID)
	defer func() {
		if r := recover(); r != nil {
			u.lg.Error("agentbridge run goroutine panic recovered",
				loggateway.Str("task_id", task.ID),
				loggateway.Err(fmt.Errorf("%v", r)))
			cur, getErr := u.tasks.Get(context.Background(), task.ID)
			if getErr == nil && !cur.Status.IsTerminal() {
				u.failTaskFrom(cur.Status, task.ID, fmt.Sprintf("internal panic: %v", r))
			}
		}
	}()

	ctx := context.Background()

	counter := &countingHandler{inner: h, onActivity: sup.noteActivity}
	summary, err := sess.Prompt(ctx, cwd, task.Prompt, counter)
	now := nowRFC3339()
	progress := counter.count

	current, getErr := u.tasks.Get(context.Background(), task.ID)
	if getErr != nil {
		u.lg.Error("agentbridge task lost during run", loggateway.Str("task_id", task.ID), loggateway.Err(getErr))
		return
	}
	// 取消优先：Cancel() 已推进到 cancelling，这里收尾到 cancelled。
	if current.Status == StatusCancelling {
		if uerr := u.transition(task.ID, StatusCancelling, StatusCancelled, TaskPatch{
			CompletedAt: &now, ProgressCount: &progress,
		}); uerr != nil {
			u.lg.Warn("agentbridge cancel finalize failed", loggateway.Str("task_id", task.ID), loggateway.Err(uerr))
		}
		return
	}
	if err != nil {
		u.failTaskFrom(current.Status, task.ID, err.Error())
		return
	}
	if uerr := u.transition(task.ID, current.Status, StatusDone, TaskPatch{
		Summary: &summary, CompletedAt: &now, ProgressCount: &progress,
	}); uerr != nil {
		u.lg.Warn("agentbridge done transition failed", loggateway.Str("task_id", task.ID), loggateway.Err(uerr))
	}
}

// countingHandler 统计进度事件数（终态落库 ProgressCount）。
// M4 §F：`_aranea/heartbeat` 心跳只算活跃性证据，不计入 progress_count。
type countingHandler struct {
	inner EventHandler
	count int
	// onActivity M4：活跃性信号上报（含心跳，由监管器消费）。
	onActivity func(kind, text string)
}

func (c *countingHandler) OnUpdate(kind, text string) {
	if kind != heartbeatKind {
		c.count++
	}
	if c.onActivity != nil {
		c.onActivity(kind, text)
	}
	c.inner.OnUpdate(kind, text)
}

func (c *countingHandler) OnPermission(ctx context.Context, title string, opts []PermissionOption) (string, error) {
	return c.inner.OnPermission(ctx, title, opts)
}

// notifyTerminal 任务到终态后通知 listener（best-effort，空 listener 跳过）。
func (u *AgentBridgeUsecase) notifyTerminal(taskID string) {
	if u.listener == nil {
		return
	}
	task, err := u.tasks.Get(context.Background(), taskID)
	if err != nil || !task.Status.IsTerminal() {
		return
	}
	u.listener.OnTaskTerminal(task)
}

// MarkAwaitingApproval 将 running 任务推进 awaiting_approval（审批中继开始）。
func (u *AgentBridgeUsecase) MarkAwaitingApproval(taskID string) error {
	if u == nil {
		return apierror.Internal(apierror.DomainAgentBridge, "usecase not bound")
	}
	task, err := u.tasks.Get(context.Background(), taskID)
	if err != nil {
		return err
	}
	if task.Status == StatusAwaitingApproval {
		return nil
	}
	return u.transition(taskID, StatusRunning, StatusAwaitingApproval, TaskPatch{})
}

// ResumeFromApproval 将 awaiting_approval 任务推回 running（用户已决议）。
func (u *AgentBridgeUsecase) ResumeFromApproval(taskID string) error {
	if u == nil {
		return apierror.Internal(apierror.DomainAgentBridge, "usecase not bound")
	}
	return u.transition(taskID, StatusAwaitingApproval, StatusRunning, TaskPatch{})
}

// CancelFromApprovalTimeout 审批超时：awaiting_approval → cancelled。
func (u *AgentBridgeUsecase) CancelFromApprovalTimeout(taskID string) error {
	if u == nil {
		return apierror.Internal(apierror.DomainAgentBridge, "usecase not bound")
	}
	now := nowRFC3339()
	reason := "approval_timeout"
	if err := u.transition(taskID, StatusAwaitingApproval, StatusCancelled, TaskPatch{
		Error: &reason, CompletedAt: &now,
	}); err != nil {
		return err
	}
	if sess := u.sessionOf(taskID); sess != nil {
		_ = sess.Cancel(context.Background())
	}
	u.notifyTerminal(taskID)
	return nil
}

// Cancel 推进 running/awaiting_approval → cancelling 并向 ACP 会话发取消。
// M4 §D（统一升级链）：session/cancel 发出后进入宽限升级——宽限期内会话未
// 退出则强制 Kill 进程组（宽限取该任务监管器的 cancel_grace 配置）。
func (u *AgentBridgeUsecase) Cancel(ctx context.Context, taskID string) error {
	task, err := u.tasks.Get(ctx, taskID)
	if err != nil {
		return err
	}
	if err := Transition(task.Status, StatusCancelling); err != nil {
		return apierror.Conflict(apierror.DomainAgentBridge,
			"task %s in status %s cannot be cancelled", taskID, task.Status)
	}
	if err := u.transition(taskID, task.Status, StatusCancelling, TaskPatch{}); err != nil {
		return err
	}
	if sess := u.sessionOf(taskID); sess != nil {
		if err := sess.Cancel(ctx); err != nil {
			u.lg.Warn("agentbridge acp cancel failed", loggateway.Str("task_id", taskID), loggateway.Err(err))
		}
		u.escalateCancel(taskID, sess, u.cancelGraceOf(taskID))
		return nil // run goroutine 或升级链收尾到 cancelled
	}
	// 无活跃会话（如 awaiting_approval 但会话已丢）：直接收尾。
	now := nowRFC3339()
	err = u.transition(taskID, StatusCancelling, StatusCancelled, TaskPatch{CompletedAt: &now})
	u.notifyTerminal(taskID)
	return err
}

// GetTask 查询单个任务。
func (u *AgentBridgeUsecase) GetTask(ctx context.Context, id string) (*CodingTask, error) {
	return u.tasks.Get(ctx, id)
}

// ListSessionTasks 列出会话任务（播报/查询用）。
func (u *AgentBridgeUsecase) ListSessionTasks(ctx context.Context, sessionID string, limit int) ([]*CodingTask, error) {
	return u.tasks.ListBySession(ctx, sessionID, limit)
}

// RecoverActiveTasks 服务重启恢复：所有非终态任务标记 failed（设计 §10）。
// M4 P0-4（S4）：标记失败前先按 child_pid（进程组根）收割孤儿子进程树——
// 宿主重启不会带走已 spawn 的 agent 进程，不收割则钥匙持有方继续跑（AC 语义
// 之外的地带）。收割失败容错（记日志，不阻断恢复）。
func (u *AgentBridgeUsecase) RecoverActiveTasks(ctx context.Context) error {
	active, err := u.tasks.ListActive(ctx)
	if err != nil {
		return err
	}
	now := nowRFC3339()
	for _, t := range active {
		if t.ChildPID > 0 && u.reaper != nil {
			if rerr := u.reaper(t.ChildPID); rerr != nil {
				u.lg.Warn("agentbridge orphan reap failed",
					loggateway.Str("task_id", t.ID),
					loggateway.Int("child_pid", t.ChildPID),
					loggateway.Err(rerr))
			}
		}
		reason := "service_restart: process lost while task active"
		if err := u.tasks.UpdateStatus(ctx, t.ID, t.Status, StatusFailed, TaskPatch{
			Error: &reason, CompletedAt: &now,
		}); err != nil {
			u.lg.Warn("agentbridge recover task failed", loggateway.Str("task_id", t.ID), loggateway.Err(err))
		}
	}
	if len(active) > 0 {
		u.lg.Info("agentbridge recovered active tasks as failed", loggateway.Int("count", len(active)))
	}
	return nil
}

// transition 先校验状态机再 CAS 落库。
func (u *AgentBridgeUsecase) transition(id string, from, to TaskStatus, patch TaskPatch) error {
	if err := Transition(from, to); err != nil {
		return apierror.Conflict(apierror.DomainAgentBridge, "%v", err)
	}
	return u.tasks.UpdateStatus(context.Background(), id, from, to, patch)
}

// failTaskFrom 将任务从 from 推进 failed（best-effort，不覆盖错误给调用方）。
func (u *AgentBridgeUsecase) failTaskFrom(from TaskStatus, id, reason string) {
	now := nowRFC3339()
	if err := u.transition(id, from, StatusFailed, TaskPatch{Error: &reason, CompletedAt: &now}); err != nil {
		u.lg.Warn("agentbridge fail transition failed", loggateway.Str("task_id", id), loggateway.Err(err))
	}
}

func (u *AgentBridgeUsecase) track(taskID string, sess ACPSession) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.running[taskID] = sess
}

func (u *AgentBridgeUsecase) untrack(taskID string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.running, taskID)
}

func (u *AgentBridgeUsecase) sessionOf(taskID string) ACPSession {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.running[taskID]
}

// trackSuper 登记任务监管器（M4；Dispatch 在 spawn 成功后调用）。
func (u *AgentBridgeUsecase) trackSuper(s *livenessSupervisor) *livenessSupervisor {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.sups[s.taskID] = s
	return s
}

// untrackSuper 摘除监管器（run() 退出路径）。
func (u *AgentBridgeUsecase) untrackSuper(taskID string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.sups, taskID)
}

// cancelGraceOf 取该任务监管器配置的 cancel_grace（无监管器时用全局默认）。
func (u *AgentBridgeUsecase) cancelGraceOf(taskID string) time.Duration {
	u.mu.Lock()
	s := u.sups[taskID]
	u.mu.Unlock()
	if s == nil {
		return u.livenessCfg.CancelGrace
	}
	return s.cfg.CancelGrace
}

// discardEvents 是 Handler 缺省值（丢弃进度，审批取首选项）。
type discardEvents struct{}

func (discardEvents) OnUpdate(_, _ string) {}
func (discardEvents) OnPermission(_ context.Context, _ string, opts []PermissionOption) (string, error) {
	if len(opts) == 0 {
		return "", apierror.Internal(apierror.DomainAgentBridge, "no permission options")
	}
	return opts[0].OptionID, nil
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }
