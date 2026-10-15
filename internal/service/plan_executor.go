package service

import (
	"context"
	"strings"
	"sync"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// executorRepos is the narrow repo interface required by PlanExecutor.
// Satisfied by composing PlanStepV2Repo + TeamStageV2Repo + PlanBoardV2Writer
// + GraphStageV2Repo + GraphNodeV2Repo.
//
// 2026-07-04 补齐：新增 UpsertGraphStage / UpsertGraphNode / GetGraphStageByPlanBoard
// 用于同步创建 GraphStage 和更新 GraphNode 状态（与 PlanBoard 一对一关联）。
// 2026-07-05 P1 #9d 补齐：新增 GetTeamStage 用于读取当前 Version 和 Status，
// 修复 dispatchStep 中 Version=2 硬编码 Bug（改为 current.Version+1）。
//
// TECH-DEBT(COG): 接口方法数>5（CS-B4），但本接口是组合接口（compose 5 个 v2 repo），
// 组合接口的方法数放宽限制是合理的。拆分会引入 5 个独立 adapter，增加复杂度无收益。
type executorRepos interface {
	UpsertPlanStep(ctx context.Context, ps biz.PlanStep) (biz.PlanStep, error)
	UpsertTeamStage(ctx context.Context, ts biz.TeamStage) (biz.TeamStage, error)
	UpsertPlanBoard(ctx context.Context, pb biz.PlanBoard) (biz.PlanBoard, error)
	GetPlanBoard(ctx context.Context, id string) (biz.PlanBoard, error)
	GetPlanStep(ctx context.Context, id string) (biz.PlanStep, error)
	ListPlanStepsByPlan(ctx context.Context, planID string) ([]biz.PlanStep, error)
	ListPlanBoardsByStatuses(ctx context.Context, statuses []biz.PlanStatus) ([]biz.PlanBoard, error)
	UpsertGraphStage(ctx context.Context, gs biz.GraphStage) (biz.GraphStage, error)
	UpsertGraphNode(ctx context.Context, gn biz.GraphNode) (biz.GraphNode, error)
	GetGraphStageByPlanBoard(ctx context.Context, planBoardID string) (biz.GraphStage, error)
	GetTeamStage(ctx context.Context, id string) (biz.TeamStage, error)
}

// sequencerPublisher mirrors v2.SequencerPublisher to avoid importing
// internal/agent/v2 (service → agent would be a reversed dependency).
// The *v2.Sequencer satisfies this interface.
type sequencerPublisher interface {
	Publish(ctx context.Context, e biz.Event)
}

// boardRunRegistry tracks in-flight PlanBoard DAG leases (AS-COG-01 named
// sync.Map wrapper). Key: board.ID, Value: *boardRunLease.
type boardRunRegistry struct {
	m sync.Map
}

func (r *boardRunRegistry) Store(key, value any) { r.m.Store(key, value) }
func (r *boardRunRegistry) Load(key any) (any, bool) { return r.m.Load(key) }
func (r *boardRunRegistry) LoadOrStore(key, value any) (any, bool) {
	return r.m.LoadOrStore(key, value)
}
func (r *boardRunRegistry) Delete(key any) { r.m.Delete(key) }
func (r *boardRunRegistry) Range(f func(key, value any) bool) { r.m.Range(f) }
// PlanExecutor is the forward DAG scheduler that replaces spirit_team's
// reverse-sync updatePlanStepForTeam. It dispatches PlanSteps to a
// TeamOrchestrator, correlates team completions, and cascades downstream
// steps (or skips them on failure).
//
// Lifecycle:
//   - Subscribe(ctx, board) blocks until all steps reach a terminal status
//     (completed / failed / skipped) or ctx is canceled.
//   - Each dispatched step emits: TeamStageCreated + PlanStepStarted events.
//   - On completion: PlanStepCompleted event.
//   - On failure: PlanStepFailed event + cascade-skip all transitive downstream.
type PlanExecutor struct {
	repos executorRepos
	orch  TeamOrchestrator
	seq   sequencerPublisher
	bus   biz.EventBus // 2026-07-04 问题 4 修复：订阅 PlanBoardCreatedEvent 触发 Subscribe
	lg    loggateway.Logger
	// 2026-07-04 问题 P5/D1 修复：team 派发标记器，让 OnTurnEnd 知道此 task
	// 有 team 在异步执行，不应立即发 task.completed。
	marker TeamDispatchMarker
	// 2026-07-05 P1 #9b 修复（AS-FSM-01）：PlanBoard 状态机驱动状态转换，
	// 替代直接字段赋值。状态机无状态，构造时一次性创建复用。
	pbSM *biz.PlanBoardStateMachine
	// 2026-07-05 P1 #9c 修复（AS-FSM-01）：GraphStage 状态机驱动状态转换。
	gsSM *biz.GraphStageStateMachine
	// 2026-07-05 P1 #9d 修复（AS-FSM-01）：TeamStage 状态机驱动状态转换。
	tsSM *biz.TeamStageStateMachine
	// C-20 fix: in-process execution lease. Prevents duplicate Team creation
	// when the same PlanBoardCreatedEvent is delivered multiple times (replay,
	// multi-instance, or event bus redelivery). Key: board.ID, Value: *boardRunLease.
	running boardRunRegistry
	// 2026-08-06 P0-2 修复（流式空壳契约）：publishV2BoardShell 先发 Steps=nil
	// 的 PlanBoardCreatedEvent（供前端先渲染看板），steps 就绪后由 PublishV2Board
	// 发 PlanBoardUpdatedEvent。shellTimeout 是空壳等待 steps 的兜底超时，
	// 超时仍未就绪则 fail-closed（防止 Plan/Allocate 中途失败时看板永停 planning）。
	// 测试可覆盖此字段缩短超时。
	shellTimeout time.Duration
	// F2（2026-09-03）：step 自动重试前的退避等待。默认 defaultStepRetryBackoff，
	// 测试置 0 关闭退避（字段随实例走，避免并行测试改包级变量的数据竞争）。
	stepRetryBackoff time.Duration
	// 2026-07-27 总结重复触发修复：board 终态后的完成通知器（TeamStarter）。
	// lazy 建团下 checkAllTeamsCompleted 在波次中点会误判全完成（后续 step
	// 尚无团队记录），改为 dagRun 活跃期间门控拦截 + board 终态唯一触发。
	// 后注入打破 Wire 循环（PlanExecutor → TeamStarter → PlanExecutor）。
	completionNotifier biz.AllTeamsCompletedNotifier
	// taskPlans 把 PlanBoard 生命周期转换传播到 TaskPlan（task_plans 表），
	// 修复 TS9-BUG-1：用户可见的 plan 状态此前永远滞留 confirmed。
	// 可选依赖：nil 时跳过传播（测试 / v1-only 部署）。
	taskPlans taskPlanStatusUpdater
	// subStop 取消 StartSubscription 的事件循环并立刻从 EventBus 摘除订阅
	// （V2Bus 的 cancel 只摘 fan-out，channel 永不关闭）。Start 写入、Stop 调用；
	// 生产路径为 ProvideChatService → ChatService.Close 串行。
	subStop func()
	// playbookConfirm holds R18 playbook_confirm_before waiters.
	// key: sessionID + NUL + (plan-step id or card id) → *playbookConfirmWait.
	playbookConfirm sync.Map
	// playbookConfirmDecided records approve/deny before or without a waiter
	// so holdPlaybookConfirm can resume after refresh (same process).
	playbookConfirmDecided sync.Map
	// confirmSteps reads persisted ConfirmBlock steps after restart.
	confirmSteps biz.StepV2Reader
	// startup 跟踪每个 board 的「首次团队派发」结果信号（P2-② 假启动拦截，
	// session-eval-20260829-r2 R4-Q1）。key: board.ID → chan startupResult
	// （buffered 1，首发有效、后续丢弃）。plan_and_execute 通过
	// WaitBoardStartup 在返回前对账：声明的编排是否真实产出 team（首个
	// team_run 创建成功），失败则把真实原因回传 Spirit，杜绝「声称已组建、
	// 实际零团队运行」。条目随进程生命周期保留（同 playbookConfirmDecided
	// 先例），单条仅一个 buffered chan，量级可忽略。
	startup sync.Map
	// teamAgentLoads maps teamID -> agent keys acquired on the process-local
	// AgentLoadTracker (DEV-03 soft capacity). Released in NotifyTeamCompletion.
	teamAgentLoads sync.Map
}

type playbookConfirmWait struct {
	ch   chan bool
	step biz.Step
}

// boardRunLease holds the cancel func for an in-flight PlanBoard DAG run (C-18).
// sessionID 供 HasActiveRunForSession 门控按 spirit session 匹配活跃 dagRun。
type boardRunLease struct {
	cancel    context.CancelFunc
	sessionID string
}

// startupResult 是 board 首次团队派发的结果信号（P2-②）。
type startupResult struct {
	ok     bool
	reason string
}

// signalStartup 非阻塞发送 board 启动信号；仅首个信号有效（chan buffered 1）。
func (e *PlanExecutor) signalStartup(boardID string, ok bool, reason string) {
	if e == nil || boardID == "" {
		return
	}
	v, loaded := e.startup.Load(boardID)
	if !loaded {
		return
	}
	ch, _ := v.(chan startupResult)
	select {
	case ch <- startupResult{ok: ok, reason: reason}:
	default:
	}
}

// WaitBoardStartup 等待 board 的首次团队派发结果（P2-② 假启动拦截）：
//   - 首个 team 创建成功 → (true, "")；
//   - board 启动失败（assembly/agent key 校验等）→ (false, reason)；
//   - 超时 / ctx 取消 / 信号缺失（HITL 挂起、慢创建）→ (true, "") 不阻断，
//     保持既有「running」语义（宁可放行、不可误报失败）。
//
// 通道注册（startBoardDAG）与等待方（plan_and_execute）存在竞态：事件总线
// 异步分发时等待方可能先到。故先轮询等待通道出现，叠加 board 终态 DB 兜底
// （通道信号因进程重启丢失时仍能识别已 failed 的看板）。
func (e *PlanExecutor) WaitBoardStartup(ctx context.Context, boardID string, timeout time.Duration) (bool, string) {
	if e == nil || boardID == "" || timeout <= 0 {
		return true, ""
	}
	deadline := time.Now().Add(timeout)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if v, ok := e.startup.Load(boardID); ok {
			if ch, _ := v.(chan startupResult); ch != nil {
				remain := time.Until(deadline)
				if remain <= 0 {
					return true, ""
				}
				t := time.NewTimer(remain)
				select {
				case r := <-ch:
					t.Stop()
					// 信号已消费，释放登记（runBoard 退出时也会兜底删除）。
					e.startup.Delete(boardID)
					return r.ok, r.reason
				case <-t.C:
					return true, ""
				case <-ctx.Done():
					t.Stop()
					return true, ""
				}
			}
		}
		// 通道未注册（事件尚未分发或信号已消费）：board 已到终态则直接对账。
		// Completed/PartialFailure 都代表确有 team 落地（partial = 至少一个
		// step 成功完成），声明的「running」属实，不误报。
		if board, err := e.repos.GetPlanBoard(ctx, boardID); err == nil && biz.IsPlanBoardTerminal(board.Status) {
			if board.Status == biz.PlanStatusCompleted || board.Status == biz.PlanStatusPartialFailure {
				return true, ""
			}
			return false, "plan board " + string(board.Status)
		}
		if time.Now().After(deadline) {
			return true, ""
		}
		select {
		case <-tick.C:
		case <-ctx.Done():
			return true, ""
		}
	}
}

// TeamDispatchMarker 标记一个 task 已派发 team。
// 由 ProjectorFactory 实现（internal/agent/v2）。
// 2026-07-04 问题 P5/D1 修复。
type TeamDispatchMarker interface {
	MarkTeamDispatched(taskID string)
}

// defaultPlanBoardShellTimeout 是流式空壳 PlanBoard 等待 steps 就绪的兜底
// 超时（2026-08-06 P0-2）。正常 LLM 分解在工具预算内完成；超过即视为分解
// 失败，看板 fail-closed 标记 Failed。
// 2026-09-04（项 2b）：2min → 5min。实证：首次分解 LLM 流式耗时 74s，
// 校验门有界重分解（R1/R3 违例时）再叠加一次完整分解（~1min），分解+重试
// 最坏 ~2.5-3min——2min 看门狗会在重试进行中误杀看板（看板 Failed 后，
// 重试成功的 PublishV2Board 批量 steps 被终态拒收，前端永远显示失败）。
// 5min 覆盖分解+重试全链路且对进程崩溃仍 fail-closed 有界。
const defaultPlanBoardShellTimeout = 5 * time.Minute

// NewPlanExecutor constructs a PlanExecutor. All dependencies are required.
func NewPlanExecutor(repos executorRepos, orch TeamOrchestrator, seq sequencerPublisher, lg loggateway.Logger) *PlanExecutor {
	return &PlanExecutor{
		repos: repos,
		orch:  orch,
		seq:   seq,
		lg:    lg.With(loggateway.Domain("plan_executor")),
		// 2026-07-05 P1 #9b：PlanBoard 状态机一次性创建复用（状态机本身无状态）。
		pbSM: biz.NewPlanBoardStateMachine(),
		// 2026-07-05 P1 #9c：GraphStage 状态机一次性创建复用。
		gsSM: biz.NewGraphStageStateMachine(),
		// 2026-07-05 P1 #9d：TeamStage 状态机一次性创建复用。
		tsSM: biz.NewTeamStageStateMachine(),
		// 2026-08-06 P0-2：流式空壳兜底超时默认值（测试可覆盖）。
		shellTimeout: defaultPlanBoardShellTimeout,
		// F2：step 自动重试退避默认值（测试可置 0 关闭）。
		stepRetryBackoff: planStepRetryBackoff,
	}
}

// SetEventBus injects the v2 EventBus after construction to break the Wire
// cycle: Sequencer → EventBus → PlanExecutor → Sequencer. May be nil
// (subscription disabled; PlanExecutor.Subscribe must be called manually).
// 2026-07-04 问题 4 修复：通过订阅 PlanBoardCreatedEvent 自动触发 Subscribe。
func (e *PlanExecutor) SetEventBus(bus biz.EventBus) {
	e.bus = bus
}

// SetTeamDispatchMarker injects the team dispatch marker (ProjectorFactory).
// 2026-07-04 问题 P5/D1 修复：让 dispatchStep 在 Orchestrate 成功后标记 task，
// OnTurnEnd 据此延迟 task.completed 直到 synthesis turn 完成。
func (e *PlanExecutor) SetTeamDispatchMarker(m TeamDispatchMarker) {
	e.marker = m
}

// SetCompletionNotifier injects the AllTeamsCompletedNotifier (TeamStarter)
// after construction. 2026-07-27 总结重复触发修复：synthesis 的唯一触发点
// 移到 dagRun 终态（releaseLeaseAndNotifyCompletion）。
func (e *PlanExecutor) SetCompletionNotifier(n biz.AllTeamsCompletedNotifier) {
	e.completionNotifier = n
}

// taskPlanStatusUpdater 是 TaskPlan 状态传播所需的窄持久化端口
// （TS9-BUG-1）。由 biz.TaskPlanRepository 满足。
type taskPlanStatusUpdater interface {
	GetByID(ctx context.Context, id string) (*biz.TaskPlan, error)
	Update(ctx context.Context, plan *biz.TaskPlan) (*biz.TaskPlan, error)
}

// SetTaskPlanUpdater 注入 TaskPlan 状态传播端口（TS9-BUG-1）。
// 后注入风格与 SetCompletionNotifier 一致。
func (e *PlanExecutor) SetTaskPlanUpdater(u taskPlanStatusUpdater) {
	e.taskPlans = u
}

// SetConfirmStepReader lets holdPlaybookConfirm see a persisted ConfirmBlock
// after process restart (card completed/cancelled = already decided).
func (e *PlanExecutor) SetConfirmStepReader(r biz.StepV2Reader) {
	if e == nil {
		return
	}
	e.confirmSteps = r
}

// taskPlanIDFromBoard 从 PlanBoard ID 还原 TaskPlan ID。PublishV2Board 以
// "pb_"+plan.ID 派生 board ID；以 rootTaskID/uuid 为 seed 的 board 还原出的
// ID 不是 plan 主键，GetByID 会 NotFound，传播安全跳过。

// trackTeamAgentLoads acquires process-local capacity slots for agents in a
// dispatched team (DEV-03 soft gate). Idempotent per teamID.
func (e *PlanExecutor) trackTeamAgentLoads(teamID string, agentKeys []string) {
	if e == nil || teamID == "" || len(agentKeys) == 0 {
		return
	}
	keys := make([]string, 0, len(agentKeys))
	seen := map[string]struct{}{}
	for _, k := range agentKeys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return
	}
	if _, loaded := e.teamAgentLoads.LoadOrStore(teamID, keys); loaded {
		return
	}
	chatagent.GlobalAgentLoadTracker.Acquire(keys...)
}

// releaseTeamAgentLoads releases capacity slots acquired for teamID.
func (e *PlanExecutor) releaseTeamAgentLoads(teamID string) {
	if e == nil || teamID == "" {
		return
	}
	v, ok := e.teamAgentLoads.LoadAndDelete(teamID)
	if !ok {
		return
	}
	keys, _ := v.([]string)
	chatagent.GlobalAgentLoadTracker.Release(keys...)
}
