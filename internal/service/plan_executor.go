package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
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
	running sync.Map
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
func taskPlanIDFromBoard(planBoardID string) string {
	return strings.TrimPrefix(planBoardID, "pb_")
}

// propagateTaskPlanExecuting 在 PlanBoard 进入 executing 时把 TaskPlan 推进到
// executing（TS9-BUG-1：此前 plan 永远滞留 confirmed）。幂等：仅
// draft/confirmed 可转换；executing/终态 plan 不动。非阻断：失败仅记日志。
func (e *PlanExecutor) propagateTaskPlanExecuting(ctx context.Context, planBoardID string) {
	if e.taskPlans == nil {
		return
	}
	planID := taskPlanIDFromBoard(planBoardID)
	plan, err := e.taskPlans.GetByID(ctx, planID)
	if err != nil || plan == nil {
		e.lg.Debug("propagateTaskPlanExecuting: TaskPlan 不可达，跳过",
			loggateway.Str("plan_board_id", planBoardID),
			loggateway.Str("plan_id", planID),
			loggateway.Err(err))
		return
	}
	if plan.Status != biz.TaskPlanStatusDraft && plan.Status != biz.TaskPlanStatusConfirmed {
		return
	}
	plan.Status = biz.TaskPlanStatusExecuting
	if _, err := e.taskPlans.Update(ctx, plan); err != nil {
		e.lg.Warn("propagateTaskPlanExecuting: 更新 TaskPlan 失败（不阻断）",
			loggateway.Str("plan_id", planID),
			loggateway.Err(err))
		return
	}
	e.lg.Info("TaskPlan 状态转换: → executing",
		loggateway.Str("plan_id", planID),
		loggateway.Str("plan_board_id", planBoardID))
}

// propagateTaskPlanTerminal 把 PlanBoard 终态映射到 TaskPlan 生命周期
// （TS9-BUG-1）。映射：Completed/PartialFailure → completed（与
// publishOrchestrationTerminal 的 orchestration_completed 语义一致）；
// Failed → failed。已是终态的 plan 不覆盖。非阻断：失败仅记日志。
func (e *PlanExecutor) propagateTaskPlanTerminal(ctx context.Context, planBoardID string, boardStatus biz.PlanStatus) {
	if e.taskPlans == nil {
		return
	}
	var next biz.TaskPlanStatus
	switch boardStatus {
	case biz.PlanStatusCompleted, biz.PlanStatusPartialFailure:
		next = biz.TaskPlanStatusCompleted
	case biz.PlanStatusFailed:
		next = biz.TaskPlanStatusFailed
	default:
		return
	}
	planID := taskPlanIDFromBoard(planBoardID)
	plan, err := e.taskPlans.GetByID(ctx, planID)
	if err != nil || plan == nil {
		e.lg.Debug("propagateTaskPlanTerminal: TaskPlan 不可达，跳过",
			loggateway.Str("plan_board_id", planBoardID),
			loggateway.Str("plan_id", planID),
			loggateway.Err(err))
		return
	}
	if plan.Status == biz.TaskPlanStatusCompleted || plan.Status == biz.TaskPlanStatusFailed {
		return // 已终态，不覆盖
	}
	plan.Status = next
	if _, err := e.taskPlans.Update(ctx, plan); err != nil {
		e.lg.Warn("propagateTaskPlanTerminal: 更新 TaskPlan 失败（不阻断）",
			loggateway.Str("plan_id", planID),
			loggateway.Err(err))
		return
	}
	e.lg.Info("TaskPlan 状态转换: → "+string(next),
		loggateway.Str("plan_id", planID),
		loggateway.Str("plan_board_id", planBoardID))
}

// HasActiveRunForSession reports whether any in-flight dagRun belongs to the
// given spirit session. 2026-07-27 总结重复触发修复：TeamStarter 的门控 —
// lazy 建团下，活跃 dagRun 意味着后续 PlanStep 还会派发新团队，此刻
// 「teams 全终态」只是波次中点，不得触发 synthesis。
func (e *PlanExecutor) HasActiveRunForSession(sessionID string) bool {
	if e == nil || sessionID == "" {
		return false
	}
	active := false
	e.running.Range(func(_, v any) bool {
		if lease, ok := v.(*boardRunLease); ok && lease.sessionID == sessionID {
			active = true
			return false
		}
		return true
	})
	return active
}

// CancelPlanBoard implements tools.PlanBoardOrchFallback (C-18).
// Cancels the in-flight DAG run for the given PlanBoard.ID.
func (e *PlanExecutor) CancelPlanBoard(ctx context.Context, planBoardID string) error {
	planBoardID = strings.TrimSpace(planBoardID)
	if planBoardID == "" {
		return fmt.Errorf("plan board id is required")
	}
	if v, ok := e.running.Load(planBoardID); ok {
		if lease, ok := v.(*boardRunLease); ok && lease.cancel != nil {
			lease.cancel()
			return nil
		}
	}
	board, err := e.repos.GetPlanBoard(ctx, planBoardID)
	if err != nil {
		return err
	}
	if biz.IsPlanBoardTerminal(board.Status) {
		return fmt.Errorf("plan board %s is already terminal (%s)", planBoardID, board.Status)
	}
	// Board exists but no active lease (e.g. not yet subscribed). Best-effort
	// fail-early via state machine so observers see a terminal status.
	newStatus, tErr := e.pbSM.Transition(board.Status, biz.PlanBoardEventFailEarly)
	if tErr != nil {
		newStatus, tErr = e.pbSM.Transition(board.Status, biz.PlanBoardEventFail)
	}
	if tErr != nil {
		return fmt.Errorf("cannot cancel plan board %s from status %s: %w", planBoardID, board.Status, tErr)
	}
	now := time.Now().UTC()
	board.Status = newStatus
	board.CompletedAt = &now
	board.Version++
	if _, err := e.repos.UpsertPlanBoard(ctx, board); err != nil {
		return err
	}
	if e.seq != nil {
		e.seq.Publish(ctx, biz.NewPlanBoardUpdatedEvent(board))
	}
	return nil
}

var _ tools.PlanBoardOrchFallback = (*PlanExecutor)(nil)

// TeamCompletionNotifier is implemented by TeamOrchestrators that track
// pending team_run completions and notify waiting dispatchStep goroutines.
// 2026-07-04 问题 4 修复：让 PlanExecutor 转发 team_run 完成通知给 TeamOrchestrator。
type TeamCompletionNotifier interface {
	NotifyTeamCompletion(teamID, teamRunID string, success bool, errMsg string)
}

// NotifyTeamCompletion forwards a team_run completion event to the
// TeamOrchestrator (if it implements TeamCompletionNotifier). Called by
// TeamStarter.HandleTeamTurnResult when a team_run reaches terminal status.
// 2026-07-04 问题 4 修复：让 PlanExecutor 转发 team_run 完成通知。
func (e *PlanExecutor) NotifyTeamCompletion(teamID, teamRunID string, success bool, errMsg string) {
	if notifier, ok := e.orch.(TeamCompletionNotifier); ok {
		notifier.NotifyTeamCompletion(teamID, teamRunID, success, errMsg)
	}
}

// StartSubscription subscribes to PlanBoard events on the EventBus and
// triggers PlanExecutor.Subscribe in a goroutine for each ready PlanBoard.
// Must be called after SetEventBus. No-op if bus is nil.
// 2026-07-04 问题 4 修复：让 PlanExecutor 自动响应 PlanBoard 创建事件。
//
// 2026-08-06 P0-2 修复（流式空壳契约，20:45 会话计划失败根因）：
// 流式分解路径先由 publishV2BoardShell 发布 Steps=nil 的
// PlanBoardCreatedEvent（供前端先渲染看板），steps 就绪后由 PublishV2Board
// 发布 PlanBoardUpdatedEvent（planning + 完整 steps）。契约：
//   - Created + 空 steps：不启动 DAG、不占 lease、不 fail-closed，
//     仅登记超时兜底看门狗（armShellTimeout），等待 steps 就绪的 Updated；
//   - Created/Updated + planning + steps：启动 DAG（lease 去重）；
//   - Updated + 非 planning：执行器自身发布的 executing/terminal 事件，跳过。
func (e *PlanExecutor) StartSubscription() {
	if e == nil || e.bus == nil {
		return
	}
	if e.subStop != nil {
		return
	}
	ctx, cancel := context.WithCancel(appctx.Ctx())
	ch, unsub := e.bus.Subscribe(biz.EventSubscribeOptions{})
	e.subStop = func() {
		unsub()
		cancel()
	}
	e.lg.Info("PlanExecutor 开始订阅 PlanBoard 事件")
	// V2Bus 永不关闭 subscriber channel（cancel 只从 fan-out 摘除），
	// 必须用 ctx.Done() 退出循环，不能 range ch（红线 #23）。
	safego.Go(ctx, "plan_executor.recover_boards", func() {
		rctx, rcancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer rcancel()
		e.RecoverUnfinishedBoards(rctx)
	})
	safego.Go(ctx, "plan_executor.subscribe", func() {
		defer unsub()
		defer e.lg.Info("PlanExecutor 订阅已退出")
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				switch pbEv := ev.(type) {
				case *biz.PlanBoardCreatedEvent:
					e.handleBoardReady(pbEv.PlanBoard, true)
				case *biz.PlanBoardUpdatedEvent:
					e.handleBoardReady(pbEv.PlanBoard, false)
				}
			}
		}
	})
}

// Stop 停止 PlanBoard 事件订阅并取消在途 DAG lease，与 StartSubscription 对称。
// 接到 ChatService.Close（kratos AfterStop）。幂等、nil-safe。
func (e *PlanExecutor) Stop() {
	if e == nil {
		return
	}
	if e.subStop != nil {
		e.subStop()
		e.subStop = nil
	}
	e.running.Range(func(_, value any) bool {
		if lease, ok := value.(*boardRunLease); ok && lease.cancel != nil {
			lease.cancel()
		}
		return true
	})
}

// handleBoardReady 按流式空壳契约处理 PlanBoard Created/Updated 事件。
// isCreated 区分事件种类：空壳看门狗仅在 Created 时登记；Updated 事件仅在
// planning 状态下允许启动 DAG（排除执行器自身发出的 executing/terminal）。
func (e *PlanExecutor) handleBoardReady(board biz.PlanBoard, isCreated bool) {
	// C-20: skip Subscribe when board is already terminal (replay / stale event).
	if biz.IsPlanBoardTerminal(board.Status) {
		e.lg.Warn("PlanBoard 已是终态，跳过 Subscribe",
			loggateway.Str("plan_board_id", board.ID),
			loggateway.Str("task_id", board.TaskID),
			loggateway.Str("status", string(board.Status)))
		return
	}
	if len(board.Steps) == 0 {
		// 流式空壳：steps 未就绪。登记超时兜底后返回，等待 Updated。
		if isCreated {
			e.lg.Info("PlanBoard 流式空壳（steps 未就绪），等待 Updated",
				loggateway.Str("plan_board_id", board.ID),
				loggateway.Str("task_id", board.TaskID))
			e.armShellTimeout(board.ID)
		}
		return
	}
	// Updated 事件只允许 planning 状态启动（执行器自身发布的 executing
	// Updated 不得重入）。进程重启的 executing 看板走 RecoverUnfinishedBoards。
	if !isCreated && board.Status != biz.PlanStatusPlanning {
		return
	}
	e.startBoardDAG(board)
}

// RecoverUnfinishedBoards re-Subscribes planning/executing boards after process
// restart. StartSubscription only sees new events; an executing board waiting
// on confirm_before would otherwise have no holdPlaybookConfirm waiter.
func (e *PlanExecutor) RecoverUnfinishedBoards(ctx context.Context) {
	if e == nil || e.repos == nil {
		return
	}
	boards, err := e.repos.ListPlanBoardsByStatuses(ctx, []biz.PlanStatus{
		biz.PlanStatusPlanning,
		biz.PlanStatusExecuting,
	})
	if err != nil {
		e.lg.Warn("RecoverUnfinishedBoards: 列出未完成看板失败", loggateway.Err(err))
		return
	}
	for _, board := range boards {
		if biz.IsPlanBoardTerminal(board.Status) {
			continue
		}
		steps, err := e.repos.ListPlanStepsByPlan(ctx, board.ID)
		if err != nil {
			e.lg.Warn("RecoverUnfinishedBoards: 读取步骤失败",
				loggateway.Str("plan_board_id", board.ID),
				loggateway.Err(err))
			continue
		}
		if len(steps) == 0 {
			// 流式空壳仍等 Updated；executing 无步骤也无法派发。
			continue
		}
		board.Steps = steps
		e.lg.Info("RecoverUnfinishedBoards: 恢复未完成看板",
			loggateway.Str("plan_board_id", board.ID),
			loggateway.Str("task_id", board.TaskID),
			loggateway.Str("status", string(board.Status)),
			loggateway.Int("steps", len(steps)))
		e.startBoardDAG(board)
	}
}

// startBoardDAG takes the execution lease and runs Subscribe in a goroutine.
// Idempotent: a second call for the same board ID is ignored.
func (e *PlanExecutor) startBoardDAG(board biz.PlanBoard) {
	if e == nil {
		return
	}
	// C-20 + C-18: execution lease for ready boards.
	runCtx, cancel := context.WithCancel(context.Background())
	lease := &boardRunLease{cancel: cancel, sessionID: board.SessionID}
	if _, loaded := e.running.LoadOrStore(board.ID, lease); loaded {
		cancel() // unused; an existing run owns this board
		e.lg.Warn("PlanBoard 已在执行中，跳过重复启动",
			loggateway.Str("plan_board_id", board.ID),
			loggateway.Str("task_id", board.TaskID))
		return
	}
	e.lg.Info("PlanExecutor 启动 DAG 执行",
		loggateway.Str("plan_board_id", board.ID),
		loggateway.Str("task_id", board.TaskID),
		loggateway.Int("steps", len(board.Steps)))
	// P2-②：注册启动信号通道，供 plan_and_execute 对账首次团队派发结果。
	e.startup.Store(board.ID, make(chan startupResult, 1))
	// Subscribe 是阻塞的，在独立 goroutine 中执行。
	// 2026-07-04 问题 4 修复：从 PlanBoard.TaskID 恢复 RootTaskActivityID
	// 注入 ctx，让下游 buildTeamProjectMeta / publishV2TeamRunAndMemberSessions
	// / publishV2TeamRunCompletion 都能拿到正确的 rootTaskID（之前为空字符串
	// 导致 MemberSession.TaskID 为空，前端 getMemberSessionSteps 返回空数组）。
	b := board
	safego.Go(runCtx, "plan_executor.dag."+b.ID, func() {
		defer e.running.Delete(b.ID) // C-20: release lease on exit
		defer e.startup.Delete(b.ID) // P2-②: 无等待方时兜底释放启动信号登记
		defer cancel()
		ctx := runCtx
		if b.TaskID != "" {
			ctx = agent.ContextWithRootTaskActivityID(
				ctx, agent.RootTaskActivityID(b.TaskID))
		}
		if err := e.Subscribe(ctx, b); err != nil {
			e.lg.Warn("PlanExecutor.Subscribe 失败",
				loggateway.Str("plan_board_id", b.ID),
				loggateway.Err(err))
		}
	})
}

// armShellTimeout 为流式空壳 PlanBoard 登记超时兜底看门狗：shellTimeout 后
// steps 仍未就绪（Plan/Allocate 中途失败，PublishV2Board 永不到达）时强制
// 将看板标记为 Failed，防止前端看板永远停在 planning。
//
// 跳过条件（任一满足即不误伤）：
//   - DAG 已启动（lease 在）；
//   - 看板已离开 planning（steps 就绪后 markPlanBoardExecuting 已推进）；
//   - 看板已有 steps。
func (e *PlanExecutor) armShellTimeout(boardID string) {
	timeout := e.shellTimeout
	if timeout <= 0 {
		timeout = defaultPlanBoardShellTimeout
	}
	time.AfterFunc(timeout, func() {
		// 独立 ctx：此时无请求 ctx 可用，给 DB 读取/失败落库 30s 预算。
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, ok := e.running.Load(boardID); ok {
			return // DAG 已启动，兜底跳过
		}
		board, err := e.repos.GetPlanBoard(ctx, boardID)
		if err != nil {
			e.lg.Warn("shellTimeout: 读取 PlanBoard 失败，跳过兜底",
				loggateway.Str("plan_board_id", boardID),
				loggateway.Err(err))
			return
		}
		if board.Status != biz.PlanStatusPlanning || len(board.Steps) > 0 {
			return
		}
		e.lg.Warn("PlanBoard 流式空壳超时，steps 永未就绪，fail-closed",
			loggateway.Str("plan_board_id", boardID),
			loggateway.Str("task_id", board.TaskID))
		r := newDagRun(e, board)
		r.publishPlanBoardFailed(ctx, "streaming plan board shell timeout: steps never arrived")
	})
}

// Subscribe starts DAG execution for the given board and blocks until all
// steps reach a terminal status or ctx is canceled.
//
// 2026-07-04 补齐：在 DAG 执行前同步创建 GraphStage（与 PlanBoard 一对一关联）
// 和 GraphNode 列表（每个 PlanStep 对应一个 GraphNode），并发布 v2 事件。
func (e *PlanExecutor) Subscribe(ctx context.Context, board biz.PlanBoard) error {
	// C-20: durable-ish guard — refuse to re-enter a board already terminal.
	if biz.IsPlanBoardTerminal(board.Status) {
		return fmt.Errorf("plan board %s is already terminal (%s)", board.ID, board.Status)
	}
	if len(board.Steps) == 0 {
		// Fail-closed: an empty board previously returned nil and could be
		// treated as a successful zero-work completion.
		r := newDagRun(e, board)
		r.publishPlanBoardFailed(ctx, "plan board has no steps")
		return fmt.Errorf("plan board %s has no steps", board.ID)
	}
	// 2026-07-04 问题 D2 修复：DAG 执行开始前，将 PlanBoard 状态从 planning
	// 更新为 executing，让前端能看到计划已进入执行阶段。之前 PlanBoard 创建后
	// Status 始终是 "planning"，DAG 执行完成后直接跳到 "completed"，前端无法
	// 区分"正在编排"和"正在执行"。
	//
	// B-05 fix: markPlanBoardExecuting returns the updated board. Previously
	// it mutated a by-value copy, so newDagRun still held Status=planning and
	// publishPlanBoardTerminal's Executing→Complete transition was skipped,
	// leaving successful DAGs stuck in "executing".
	board = e.markPlanBoardExecuting(ctx, board)
	e.hydrateBoardOrgFields(ctx, &board)
	// 同步创建 GraphStage（与 PlanBoard 一对一）。失败不阻断主流程，
	// 仅记录日志（GraphStage 是可视化层，缺失不影响 DAG 调度正确性）。
	e.initGraphStage(ctx, board)
	r := newDagRun(e, board)
	return r.run(ctx)
}

// Resume mode for ResumePlanBoard.
const (
	// ResumeModeRetry 重跑失败 step：Failed→Pending 重排队，cascade skip 的
	// 下游 Skipped→Pending 一并复活，由 DAG 依赖序重新调度。
	ResumeModeRetry = "retry"
	// ResumeModeSkip 跳过失败 step（降级）：Failed→Skipped，下游复活并按
	// dependencySatisfied 的降级语义放行（skipped 依赖视为已满足）。
	ResumeModeSkip = "skip"
)

// ResumePlanBoard 恢复一个 Failed 的 PlanBoard 继续执行（F2，2026-09-03
// lbg-verify-planner 复盘 问题2）：board Failed 不再是死胡同。mode=retry
// 重跑全部失败 step；mode=skip 跳过全部失败 step（降级放行下游）。两种
// 模式都会把 cascade skip 的下游 step 复活回 Pending。恢复后 board 经
// resume 事件回到 Executing 并重新取得执行 lease 续跑。
//
// 前置约束：board 必须处于 Failed 且当前无执行 lease（正在执行的 board
// 拒绝恢复）。TaskPlan 终态不联动回滚（resume 后 DAG 终态会再次传播覆盖）。
func (e *PlanExecutor) ResumePlanBoard(ctx context.Context, boardID string, mode string) error {
	if e == nil {
		return fmt.Errorf("plan executor is nil")
	}
	boardID = strings.TrimSpace(boardID)
	if boardID == "" {
		return apierror.BadRequest(apierror.DomainOrchestrator, "plan board id is empty")
	}
	if mode != ResumeModeRetry && mode != ResumeModeSkip {
		return apierror.BadRequest(apierror.DomainOrchestrator, "invalid resume mode %q (want %q|%q)", mode, ResumeModeRetry, ResumeModeSkip)
	}
	board, err := e.repos.GetPlanBoard(ctx, boardID)
	if err != nil {
		return fmt.Errorf("load plan board: %w", err)
	}
	if board.Status != biz.PlanStatusFailed {
		return apierror.BadRequest(apierror.DomainOrchestrator, "plan board %s is %s, only failed boards can resume", boardID, board.Status)
	}
	if _, ok := e.running.Load(boardID); ok {
		return apierror.BadRequest(apierror.DomainOrchestrator, "plan board %s is running, refuse to resume", boardID)
	}
	steps, err := e.repos.ListPlanStepsByPlan(ctx, board.ID)
	if err != nil {
		return fmt.Errorf("load plan steps: %w", err)
	}
	board.Steps = steps
	// 前置校验：board Failed 必然至少有一个失败 step（snapshotStepOutcomes），
	// 否则拒绝改造避免半持久化中间态。
	var failedCount int
	for i := range board.Steps {
		if board.Steps[i].Status == biz.PlanStepStatusFailed {
			failedCount++
		}
	}
	if failedCount == 0 {
		return apierror.BadRequest(apierror.DomainOrchestrator, "plan board %s has no failed steps to resume", boardID)
	}
	// 1. 按 mode 改造 step 集合（board 未在运行，无并发写）。
	for i := range board.Steps {
		s := &board.Steps[i]
		switch s.Status {
		case biz.PlanStepStatusFailed:
			if mode == ResumeModeSkip {
				if err := s.Transition(biz.PlanStepStatusSkipped); err != nil {
					return fmt.Errorf("skip step %s: %w", s.ID, err)
				}
			} else {
				if err := s.Transition(biz.PlanStepStatusPending); err != nil {
					return fmt.Errorf("requeue step %s: %w", s.ID, err)
				}
				s.Error = nil // 重跑前清掉上轮错误，避免误导
			}
			s.CompletedAt = nil
			s.Version++
		case biz.PlanStepStatusSkipped:
			// 只复活 cascade skip 的受害者（R1 审查修复）：人工拒绝确认
			//（confirm_denied）与上一轮 resume(mode=skip) 的人工降级跳过
			// 均保持 Skipped——不违背用户已表达的拒绝/跳过意图。
			if s.Error == nil || s.Error.Code != biz.StepErrCodeCascadeSkip {
				continue
			}
			if err := s.Transition(biz.PlanStepStatusPending); err != nil {
				return fmt.Errorf("revive skipped step %s: %w", s.ID, err)
			}
			s.Error = nil // 清掉 cascade 标记，避免误导
			s.CompletedAt = nil
			s.Version++
		default:
			continue
		}
		// 持久化失败必须中止（R1 审查修复）：继续推进会造成 DB 与内存
		// 发散，进程重启后 recover 到旧状态，行为不可预期。已持久化的前
		// 序 step 幂等（再次 resume 时状态已非原值，switch 自然跳过）。
		if _, err := e.repos.UpsertPlanStep(ctx, *s); err != nil {
			return fmt.Errorf("resume: upsert plan_step %s: %w", s.ID, err)
		}
		// GraphNode 与 step 状态对齐（skip→interrupted，requeue→pending）。
		e.syncGraphNodeStatus(ctx, board, *s)
		if mode == ResumeModeSkip && s.Status == biz.PlanStepStatusSkipped {
			e.seq.Publish(ctx, biz.NewPlanStepSkippedEvent(*s, board.SessionID, "resume: skip failed step"))
		}
	}
	// 2. board Failed→Executing（状态机 resume 边）。
	newStatus, err := e.pbSM.Transition(board.Status, biz.PlanBoardEventResume)
	if err != nil {
		return fmt.Errorf("resume transition: %w", err)
	}
	board.Status = newStatus
	board.CompletedAt = nil
	board.Version++
	if _, err := e.repos.UpsertPlanBoard(ctx, board); err != nil {
		return fmt.Errorf("resume: upsert plan_board: %w", err)
	}
	e.seq.Publish(ctx, biz.NewPlanBoardUpdatedEvent(board))
	e.lg.Info("PlanBoard 已恢复续跑",
		loggateway.Str("plan_board_id", board.ID),
		loggateway.Str("mode", mode),
		loggateway.Int("failed_steps", failedCount))
	// 3. 重新启动 DAG（终态时 lease 已释放；Subscribe 的 C-20 守卫要求
	// 非终态，上一步已转 Executing；initGraphStage 幂等跳过已有 GraphStage）。
	e.startBoardDAG(board)
	return nil
}

// syncGraphNodeStatus 按 MapPlanStepToGraphNodeStatus 映射关系同步单个
// GraphNode 状态（resume 用；dagRun 未建，直接走 repo + 事件）。
func (e *PlanExecutor) syncGraphNodeStatus(ctx context.Context, board biz.PlanBoard, step biz.PlanStep) {
	existing, err := e.repos.GetGraphStageByPlanBoard(ctx, board.ID)
	if err != nil || existing.ID == "" {
		return
	}
	stepCopy := step
	r := &dagRun{
		pe:           e,
		graphStageID: existing.ID,
		board:        board,
		stepsByID:    map[string]*biz.PlanStep{step.ID: &stepCopy},
	}
	r.updateGraphNode(ctx, step.ID, biz.MapPlanStepToGraphNodeStatus(step.Status), "")
}

// markPlanBoardExecuting updates the PlanBoard status from "planning" to
// "executing" and publishes a PlanBoardUpdatedEvent so the frontend can
// reflect the transition. Idempotent: if the PlanBoard is already in a
// terminal or executing state, the update is skipped.
//
// Returns the board that should be used for subsequent DAG execution. On
// successful transition the returned board has Status=Executing; otherwise
// the input board is returned unchanged.
//
// 2026-07-04 问题 D2 修复：补齐 planning → executing 状态转换。
// 2026-07-05 P1 #9b（AS-FSM-01）：用 PlanBoardStateMachine 显式校验转换，
// 替代直接 if + 字段赋值。状态机会拒绝任何非法 from 状态（如 terminal）。
// 2026-07-16 B-05：返回更新后的 board，避免 Subscribe 继续使用 planning 副本。
func (e *PlanExecutor) markPlanBoardExecuting(ctx context.Context, board biz.PlanBoard) biz.PlanBoard {
	// 状态机校验：只有 Planning 才能 Transition(Execute) → Executing。
	// 若 board 已是 Executing 或 terminal 状态，Transition 返回错误，跳过。
	newStatus, err := e.pbSM.Transition(board.Status, biz.PlanBoardEventExecute)
	if err != nil {
		e.lg.Debug("markPlanBoardExecuting: skip (invalid transition)",
			loggateway.Str("plan_board_id", board.ID),
			loggateway.Str("from_status", string(board.Status)),
			loggateway.Err(err))
		return board
	}
	board.Status = newStatus
	board.Version++
	if _, err := e.repos.UpsertPlanBoard(ctx, board); err != nil {
		e.lg.Warn("markPlanBoardExecuting: upsert plan_board (executing) failed",
			loggateway.Str("plan_board_id", board.ID),
			loggateway.Err(err))
		// Keep the in-memory Executing status so terminal transition can still
		// succeed; the next UpsertPlanBoard (terminal) will reconcile DB.
		return board
	}
	e.seq.Publish(ctx, biz.NewPlanBoardUpdatedEvent(board))
	e.lg.Info("PlanBoard 状态转换: planning → executing",
		loggateway.Str("plan_board_id", board.ID),
		loggateway.Str("task_id", board.TaskID))
	// TS9-BUG-1: 同步推进 TaskPlan confirmed/draft → executing。
	e.propagateTaskPlanExecuting(ctx, board.ID)
	return board
}

// initGraphStage creates the GraphStage (and its GraphNodes) for the given
// PlanBoard if it doesn't already exist. Idempotent: if a GraphStage is
// already associated with the PlanBoard, it's left as-is.
//
// B.10.5 / P-FIX5：PublishV2Board 已通过 seq.Publish 发送 GraphStageCreatedEvent。
// 此处仅做同步 Upsert 作为 crash-recovery fallback（确保 newDagRun 的
// GetGraphStageByPlanBoard 能查到），**禁止**再 Publish，避免重复 created。
//
// 设计：docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md §3.2.4
// GraphStage ID 由 planBoardID 确定性派生（uuid.NewSHA1(aranea.graph_stage.v2, planBoardID)）。
// GraphNode ID = plan_step.id（直接复用，确定性）。
func (e *PlanExecutor) initGraphStage(ctx context.Context, board biz.PlanBoard) {
	// 检查是否已存在 GraphStage（避免重复创建）。
	if existing, err := e.repos.GetGraphStageByPlanBoard(ctx, board.ID); err == nil && existing.ID != "" {
		// 已存在，跳过创建（可能来自 task_planner 的异步持久化或 crash recovery）。
		e.lg.Info("initGraphStage: GraphStage 已存在，跳过创建",
			loggateway.Str("plan_board_id", board.ID),
			loggateway.Str("graph_stage_id", existing.ID),
		)
		return
	}
	// 派生 GraphStage ID（确定性，确保多次调用产生相同 ID）。
	gsID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.graph_stage.v2:"+board.ID)).String()
	now := time.Now()
	gs := biz.GraphStage{
		ID:          gsID,
		TaskID:      board.TaskID,
		TurnID:      board.TurnID,
		SessionID:   board.SessionID,
		PlanBoardID: board.ID,
		Status:      biz.GraphStageStatusRunning,
		StartedAt:   now,
		Seq:         board.Seq, // 与 PlanBoard 同 Seq
		Version:     1,
	}
	// 构建 GraphNode 列表（每个 PlanStep 对应一个 GraphNode）。
	nodes := make([]biz.GraphNode, 0, len(board.Steps))
	for _, step := range board.Steps {
		gn := biz.GraphNode{
			ID:           step.ID, // GraphNode.ID = PlanStep.ID（确定性派生）
			GraphStageID: gsID,
			Label:        step.Label,
			DagNodeID:    step.ID,
			Status:       biz.MapPlanStepToGraphNodeStatus(step.Status),
			DependsOn:    append([]string(nil), step.DependsOn...),
		}
		nodes = append(nodes, gn)
	}
	gs.Nodes = nodes
	// 持久化 GraphStage（同步，确保 newDagRun 能查到）。
	// VersionLT 守卫使此写入幂等：若 PublishV2Board 的异步持久化已完成，此写入被拒绝。
	if _, err := e.repos.UpsertGraphStage(ctx, gs); err != nil {
		e.lg.Warn("upsert graph_stage failed (non-blocking)",
			loggateway.Str("graph_stage_id", gsID),
			loggateway.Str("plan_board_id", board.ID),
			loggateway.Err(err),
		)
		return
	}
	// 持久化 GraphNodes。
	for _, gn := range nodes {
		if _, err := e.repos.UpsertGraphNode(ctx, gn); err != nil {
			e.lg.Warn("upsert graph_node failed (non-blocking)",
				loggateway.Str("graph_node_id", gn.ID),
				loggateway.Err(err),
			)
		}
	}
	// 不 Publish GraphStageCreatedEvent（B.10.5）：事件由 PublishV2Board 负责。
	e.lg.Info("initGraphStage: 同步 Upsert GraphStage（无 created 事件，避免重复）",
		loggateway.Str("plan_board_id", board.ID),
		loggateway.Str("graph_stage_id", gsID),
		loggateway.Int("node_count", len(nodes)),
	)
}

// dagRun encapsulates the per-Subscribe DAG state. Created fresh for each
// Subscribe call; not safe for concurrent reuse (one Subscribe = one dagRun).
type dagRun struct {
	pe    *PlanExecutor
	board biz.PlanBoard

	// graphStageID 是与 PlanBoard 一对一关联的 GraphStage 的 ID（在 initGraphStage
	// 中创建）。如果创建失败则为空，此时跳过 GraphNode 更新。
	graphStageID string

	// canceled is set when the Subscribe ctx is cancelled. Terminal publish
	// uses a fresh context budget, so this flag (not ctx.Err()) drives Fail/
	// Interrupt classification after the worker barrier.
	canceled bool

	mu         sync.Mutex
	stepsByID  map[string]*biz.PlanStep
	dependents map[string][]string // stepID → stepIDs that depend on it
	wg         sync.WaitGroup

	// attempts 记录各 step 已消耗的自动重试次数（F2，2026-09-03）。memory-only
	// （与 PlanStep.Mode/DepartmentID 同模式）：进程重启后重试预算重置，
	// resume 重建 dagRun 亦获得全新预算。
	attempts map[string]int
}

func newDagRun(pe *PlanExecutor, board biz.PlanBoard) *dagRun {
	stepsByID := make(map[string]*biz.PlanStep, len(board.Steps))
	dependents := make(map[string][]string)
	for i := range board.Steps {
		s := &board.Steps[i]
		stepsByID[s.ID] = s
		for _, dep := range s.DependsOn {
			dependents[dep] = append(dependents[dep], s.ID)
		}
	}
	// 查找与 PlanBoard 关联的 GraphStage ID（在 initGraphStage 中创建）。
	// 如果未找到（创建失败或未调用 initGraphStage），graphStageID 为空，
	// dagRun 会跳过 GraphNode 更新，但 DAG 调度仍正常运行。
	var gsID string
	if existing, err := pe.repos.GetGraphStageByPlanBoard(context.Background(), board.ID); err == nil && existing.ID != "" {
		gsID = existing.ID
	}
	return &dagRun{
		pe:           pe,
		board:        board,
		graphStageID: gsID,
		stepsByID:    stepsByID,
		dependents:   dependents,
		attempts:     make(map[string]int),
	}
}

// run dispatches root steps and blocks until all steps are terminal.
// If no root steps exist (all steps have dependencies — a cycle or empty
// board), the WaitGroup count stays 0 and Wait returns immediately.
//
// 2026-07-04 问题 2 修复（Gap A）：DAG 执行结束后必须发布 GraphStage
// terminal 事件（Completed/Failed/Interrupted），否则 graph_stages_v2 表
// status 永远停留在 "running"，刷新后前端流程图显示状态过期。terminal
// 状态判定：
//   - ctx.Err() != nil  → Interrupted（被取消）
//   - 任一 step Failed/PartialFailure → Failed
//   - 否则 → Completed
//
// 2026-07-15 P0-2 修复（fail-closed DAG validation）：在 dispatch 之前
// 校验 DAG（环检测 + 悬挂依赖），失败时强制标 PlanBoard/GraphStage 为
// Failed 并返回 error。之前 run() 只派发根 step，环图无根 → wg=0 →
// 返回 nil → publishPlanBoardTerminal 标 Completed，导致 cyclic PlanBoard
// 静默成功（审计报告 P0-2）。
func (r *dagRun) run(ctx context.Context) error {
	// P0-2: fail-closed DAG validation. Reject cyclic or malformed DAGs
	// before dispatching any step. Without this guard, a cyclic DAG has
	// no root steps → WaitGroup stays 0 → run() returns nil and the
	// board is silently marked Completed without executing anything.
	if err := r.validateDAG(); err != nil {
		r.pe.lg.Error("DAG 校验失败，拒绝执行（fail-closed）",
			loggateway.Str("plan_board_id", r.board.ID),
			loggateway.Str("task_id", r.board.TaskID),
			loggateway.Err(err))
		r.publishPlanBoardFailed(ctx, err.Error())
		r.publishGraphStageFailed(ctx, err.Error())
		return err
	}
	// P1 形式契约（B.10.15.2）：启动时基于 board.Steps 做 advisory 契约验证。
	// 团队是惰性组建的（dispatch 时才 AssembleTeam），不存在"全部组建完成"
	// 的统一时点；而 PlanStep 契约在 PublishV2Board 时已持久化，此处可全量校验。
	// warnings 不阻断派发：记日志 + 发 SystemNoticeEvent（contract_mismatch，
	// WS-only 不持久化），供前端 PlanBlock 展示黄色提示。
	if warnings := biz.ValidatePlanStepContracts(r.board.Steps); len(warnings) > 0 {
		r.pe.lg.Info("交付物契约 advisory 校验发现不匹配",
			loggateway.StepID("plan_executor.contract_validate.mismatch"),
			loggateway.Str("plan_board_id", r.board.ID),
			loggateway.Int("warning_count", len(warnings)))
		if r.pe.bus != nil {
			r.pe.bus.Publish(ctx, biz.NewSystemNoticeEvent(
				r.board.SessionID,
				"contract_mismatch",
				fmt.Sprintf("交付物契约校验发现 %d 处不匹配", len(warnings)),
				map[string]any{
					"plan_board_id": r.board.ID,
					"warnings":      warnings,
				},
			))
		}
	}
	// Dispatch every pending step whose dependencies are already completed.
	// Fresh boards: only roots (empty DependsOn). Restart recover: completed
	// ancestors leave pending confirm_before / downstream ready to resume.
	r.dispatchReadyPending(ctx)
	// Wait for all goroutines (root + downstream) to finish.
	done := make(chan struct{})
	safego.Go(ctx, "plan_executor.wg_wait."+r.board.ID, func() {
		r.wg.Wait()
		close(done)
	})
	var runErr error
	select {
	case <-done:
		runErr = nil
	case <-ctx.Done():
		runErr = ctx.Err()
		r.canceled = true
		// P1 (audit): do not compute terminal state until in-flight workers
		// exit. Previously <-ctx.Done() raced ahead of wg.Wait(), so
		// publish*Terminal could run while dispatch goroutines still wrote.
		<-done
	}
	// Terminal persist/publish needs a live context: the Subscribe ctx may
	// already be cancelled. Keep a short independent budget after cancel.
	termCtx := ctx
	if r.canceled {
		var cancel context.CancelFunc
		termCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// P1-6: sweep steps that never reached a terminal state. In-flight
		// steps exit dispatch via `<-ctx.Done(): return` without writing a
		// terminal status, and never-dispatched steps stay pending — both
		// would otherwise remain running/pending in the DB forever (audit
		// hole + stale UI after refresh). Runs after the wg barrier, so no
		// writer remains (same invariant as snapshotStepOutcomes, C-17).
		r.sweepNonTerminalSteps(termCtx, "run canceled")
	}
	// 发布 terminal 事件（无论成功/失败/取消），让前端流程图和计划列表
	// 在刷新后能正确显示最终状态。失败仅记录日志，不阻断返回。
	// 2026-07-04 问题 2 修复（Gap A + Gap B）：
	//   - Gap A: GraphStage terminal 事件（Completed/Failed/Interrupted）
	//   - Gap B: PlanBoard terminal 状态更新（Completed/Failed/PartialFailure）
	r.publishPlanBoardTerminal(termCtx)
	r.publishGraphStageTerminal(termCtx)
	// 2026-07-27 总结重复触发修复：board 终态是 synthesis 的唯一触发点。
	// 必须先释放 lease 再通知 —— TeamStarter 门控（HasActiveRunForSession）
	// 会把 lease 存续期间的触发当作「波次中点」拦掉，包括这一次最终触发。
	r.releaseLeaseAndNotifyCompletion()
	return runErr
}

// releaseLeaseAndNotifyCompletion releases the board execution lease, then
// fires the AllTeamsCompletedNotifier so TeamStarter triggers the synthesis
// turn exactly once per orchestration round. 2026-07-27 修复：lazy 建团下
// 波次中点的 team 回调会被门控拦截（HasActiveRunForSession），最终总结只能
// 从这里发出。ctx 用 Background：synthesis turn 长达分钟级，不能继承可能
// 已取消的 run ctx。
func (r *dagRun) releaseLeaseAndNotifyCompletion() {
	r.pe.running.Delete(r.board.ID)
	if r.pe.completionNotifier == nil || r.board.SessionID == "" {
		return
	}
	r.pe.completionNotifier.NotifyAllTeamsCompleted(context.Background(), r.board.SessionID)
}

// snapshotStepOutcomes returns aggregated PlanStep failure flags under r.mu.
// C-17: call only after the worker WaitGroup barrier so no writer remains;
// the lock still documents the shared-state invariant and is race-detector safe.
func (r *dagRun) snapshotStepOutcomes() (hasFailed, hasPartial bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.board.Steps {
		switch r.board.Steps[i].Status {
		case biz.PlanStepStatusFailed:
			hasFailed = true
		case biz.PlanStepStatusPartialFailure:
			hasPartial = true
		}
	}
	return hasFailed, hasPartial
}

// firstStepError 返回首个失败步骤的错误信息（P2-② 启动对账 reason）。
// 无失败步骤时返回兜底文案。与 snapshotStepOutcomes 同锁约定。
func (r *dagRun) firstStepError() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.board.Steps {
		s := &r.board.Steps[i]
		if s.Status == biz.PlanStepStatusFailed {
			if s.Error != nil && strings.TrimSpace(s.Error.Message) != "" {
				return "step " + s.ID + " failed: " + s.Error.Message
			}
			return "step " + s.ID + " failed"
		}
	}
	return "plan board failed"
}

// publishPlanBoardTerminal 根据 DAG 执行结果更新 PlanBoard terminal 状态
// 并发布 PlanBoardUpdatedEvent。让计划列表在刷新后能正确显示最终状态。
//
// 2026-07-04 问题 2 修复（Gap B）：之前 PlanBoard 创建后 Status 始终是
// "executing"，DAG 完成后也不更新，刷新后状态过期。
// 2026-07-05 P1 #9b（AS-FSM-01）：用 PlanBoardStateMachine 显式校验 terminal
// 转换。事件映射：
//   - ctx.Err() != nil 或 hasFailed → PlanBoardEventFail (Executing → Failed)
//   - hasPartial → PlanBoardEventPartial (Executing → PartialFailure)
//   - default → PlanBoardEventComplete (Executing → Completed)
//
// 若 from 状态不是 Executing（如 markPlanBoardExecuting 失败导致仍是 Planning），
// 状态机拒绝转换并记 warn 日志，跳过发布——避免非法状态跳转。
func (r *dagRun) publishPlanBoardTerminal(ctx context.Context) {
	// C-17: scan under mu even after wg.Wait — defensive happens-before with
	// any late unlock ordering, and documents the lock invariant for readers.
	hasFailed, hasPartial := r.snapshotStepOutcomes()
	var event biz.PlanBoardEvent
	switch {
	case r.canceled:
		event = biz.PlanBoardEventFail
	case hasFailed:
		event = biz.PlanBoardEventFail
	case hasPartial:
		event = biz.PlanBoardEventPartial
	default:
		event = biz.PlanBoardEventComplete
	}
	// P2-② 假启动拦截：board 终态 Failed 且尚无成功信号（signalStartup 首发
	// 有效，已创建 team 时此信号被丢弃）→ 通知 WaitBoardStartup 对账失败。
	// 覆盖步骤级失败路径（orchestrate 校验/agent key 无效 → failStep → 零
	// team_runs），此前仅 publishPlanBoardFailed（校验 fail-closed）发信号，
	// S07 类事故会静默漏过对账。
	if event == biz.PlanBoardEventFail {
		reason := "plan board run canceled"
		if !r.canceled {
			reason = r.firstStepError()
		}
		r.pe.signalStartup(r.board.ID, false, reason)
	}
	// 状态机校验：from=Executing → terminal 状态。
	newStatus, err := r.pe.pbSM.Transition(r.board.Status, event)
	if err != nil {
		r.pe.lg.Warn("publishPlanBoardTerminal: invalid transition (skip)",
			loggateway.Str("plan_board_id", r.board.ID),
			loggateway.Str("from_status", string(r.board.Status)),
			loggateway.Str("event", string(event)),
			loggateway.Err(err))
		return
	}
	now := time.Now().UTC()
	r.board.Status = newStatus
	r.board.CompletedAt = &now
	r.board.Version++
	// 持久化（不阻断主流程；失败仅记录日志）。
	if _, err := r.pe.repos.UpsertPlanBoard(ctx, r.board); err != nil {
		r.pe.lg.Warn("upsert plan_board (terminal) failed",
			loggateway.Str("plan_board_id", r.board.ID),
			loggateway.Err(err))
	}
	// 发布事件让前端更新。
	r.pe.seq.Publish(ctx, biz.NewPlanBoardUpdatedEvent(r.board))
	r.pe.lg.Info("PlanBoard terminal 状态已发布",
		loggateway.Str("plan_board_id", r.board.ID),
		loggateway.Str("status", string(newStatus)))

	// B-04 fix: publish orchestration terminal event NOW (not prematurely in
	// spirit_tools.go). Previously plan_and_execute emitted orchestration_completed
	// right after PublishV2Board, before the DAG had executed — a false success.
	// Now the event fires only when the DAG reaches a terminal state.
	r.publishOrchestrationTerminal(ctx, newStatus)
	// TS9-BUG-1: 同步推进 TaskPlan executing → completed/failed。
	r.pe.propagateTaskPlanTerminal(ctx, r.board.ID, newStatus)
}

// publishOrchestrationTerminal emits orchestration_completed or
// orchestration_failed based on the PlanBoard terminal status. B-04 fix:
// this replaces the premature publishOrchestrationCompleted calls that were
// in spirit_tools.go (which fired before DAG execution).
func (r *dagRun) publishOrchestrationTerminal(ctx context.Context, status biz.PlanStatus) {
	if r.pe.bus == nil || r.board.SessionID == "" {
		return
	}
	var noticeType string
	switch status {
	case biz.PlanStatusCompleted, biz.PlanStatusPartialFailure:
		noticeType = "orchestration_completed"
	case biz.PlanStatusFailed:
		noticeType = "orchestration_failed"
	default:
		return // non-terminal (shouldn't reach here)
	}
	meta := map[string]any{
		"orchestration_id": r.board.ID,
		"strategy":         string(r.board.Strategy),
		"subtask_count":    len(r.board.Steps),
		"agent_key":        "plan_executor",
	}
	r.pe.bus.Publish(ctx, biz.NewSystemNoticeEvent(r.board.SessionID, noticeType, "", meta))
	r.pe.lg.Info("orchestration terminal 事件已发布",
		loggateway.Str("plan_board_id", r.board.ID),
		loggateway.Str("session_id", r.board.SessionID),
		loggateway.Str("notice_type", noticeType))
}

// publishGraphStageTerminal 根据 DAG 执行结果发布 GraphStage terminal 事件。
// 仅在 graphStageID 非空（initGraphStage 成功创建了 GraphStage）时发布。
// 失败仅记录日志，不影响主流程返回值。
//
// 2026-07-04 问题 2 修复（Gap A）：补齐 terminal 事件发布，避免 graph_stages_v2
// 表 status 永远为 "running"。
// 2026-07-05 P1 #9c（AS-FSM-01）：用 GraphStageStateMachine 显式校验 terminal
// 转换，并修复 Version=3 硬编码 Bug（改为 current.Version+1）。
//
// 事件映射：
//   - ctx.Err() != nil → GraphStageEventInterrupt (Running → Interrupted)
//   - hasFailed → GraphStageEventFail (Running → Failed)
//   - default → GraphStageEventComplete (Running → Completed)
//
// Version 修复说明：之前硬编码 Version=3，假设 initGraphStage 创建时 Version=1、
// 中间更新 Version=2。但如果 GraphStage 被其他路径多次更新（如 event_router
// 处理多个 GraphStage 事件），Version 可能已 > 3，导致 VersionLT 守卫失败、
// terminal 状态无法写入。修复：先读取当前 GraphStage，新 Version = current.Version+1。
func (r *dagRun) publishGraphStageTerminal(ctx context.Context) {
	if r.graphStageID == "" {
		return
	}
	// 读取当前 GraphStage，获取准确的 Version 和 Status（避免硬编码 Version）。
	current, err := r.pe.repos.GetGraphStageByPlanBoard(ctx, r.board.ID)
	if err != nil || current.ID == "" {
		r.pe.lg.Warn("publishGraphStageTerminal: failed to load current GraphStage (skip)",
			loggateway.Str("plan_board_id", r.board.ID),
			loggateway.Str("graph_stage_id", r.graphStageID),
			loggateway.Err(err))
		return
	}
	// C-17: locked snapshot after worker barrier.
	hasFailed, hasPartial := r.snapshotStepOutcomes()
	var event biz.GraphStageEvent
	switch {
	case r.canceled:
		event = biz.GraphStageEventInterrupt
	case hasFailed || hasPartial:
		event = biz.GraphStageEventFail
	default:
		event = biz.GraphStageEventComplete
	}
	// 状态机校验：from=Running → terminal 状态。
	// 若 current.Status 已是 terminal（其他路径已更新），Transition 返回错误，跳过。
	newStatus, err := r.pe.gsSM.Transition(current.Status, event)
	if err != nil {
		r.pe.lg.Warn("publishGraphStageTerminal: invalid transition (skip)",
			loggateway.Str("graph_stage_id", r.graphStageID),
			loggateway.Str("from_status", string(current.Status)),
			loggateway.Str("event", string(event)),
			loggateway.Err(err))
		return
	}
	now := time.Now().UTC()
	gs := biz.GraphStage{
		ID:          r.graphStageID,
		TaskID:      r.board.TaskID,
		TurnID:      r.board.TurnID,
		SessionID:   r.board.SessionID,
		PlanBoardID: r.board.ID,
		Status:      newStatus,
		StartedAt:   current.StartedAt, // 保留原 StartedAt
		CompletedAt: &now,
		Seq:         current.Seq,         // 保留原 Seq
		Version:     current.Version + 1, // 递增 Version（替代硬编码 Version=3）
	}
	var publishEvent biz.Event
	switch newStatus {
	case biz.GraphStageStatusCompleted:
		publishEvent = biz.NewGraphStageCompletedEvent(gs)
	case biz.GraphStageStatusFailed:
		publishEvent = biz.NewGraphStageFailedEvent(gs)
	case biz.GraphStageStatusInterrupted:
		publishEvent = biz.NewGraphStageInterruptedEvent(gs)
	default:
		return
	}
	// 2026-07-05 修复：与 publishPlanBoardTerminal 对齐，先持久化再发布事件。
	// Version=current.Version+1 通过 VersionLT 守卫；若并发冲突则 idempotent
	// 返回 existing（状态可能未更新，但说明已有其他路径更新——可接受）。
	if _, err := r.pe.repos.UpsertGraphStage(ctx, gs); err != nil {
		r.pe.lg.Warn("upsert graph_stage (terminal) failed",
			loggateway.Str("graph_stage_id", r.graphStageID),
			loggateway.Str("plan_board_id", r.board.ID),
			loggateway.Err(err))
	}
	r.pe.seq.Publish(ctx, publishEvent)
	r.pe.lg.Info("GraphStage terminal 状态已发布",
		loggateway.Str("graph_stage_id", r.graphStageID),
		loggateway.Str("plan_board_id", r.board.ID),
		loggateway.Str("status", string(newStatus)))
}

// validateDAG 校验 PlanBoard 的 DAG 是否合法：
//  1. 无悬挂依赖：每个 step 的 DependsOn 引用的 stepID 必须存在于 board.Steps
//  2. 无环：Kahn 拓扑排序后所有节点都被访问（visited == len(steps)）
//
// 2026-07-15 P0-2 修复（审计报告 P0-2）：之前 run() 只派发根 step，环图无根
// → wg=0 → 返回 nil → publishPlanBoardTerminal 标 Completed，导致 cyclic
// PlanBoard 静默成功。此函数在 dispatch 前强制校验，fail-closed。
func (r *dagRun) validateDAG() error {
	if len(r.board.Steps) == 0 {
		return fmt.Errorf("plan board has no steps")
	}
	// 1. 悬挂依赖检测：每个 DependsOn 必须指向已存在的 step。
	for i := range r.board.Steps {
		s := &r.board.Steps[i]
		for _, dep := range s.DependsOn {
			if _, ok := r.stepsByID[dep]; !ok {
				return fmt.Errorf("step %s depends on non-existent step %q (dangling dependency)", s.ID, dep)
			}
		}
	}
	// 2. 环检测（Kahn 拓扑排序）。
	// 入度 = step.DependsOn 的长度（每条入边代表一个前置依赖）。
	inDegree := make(map[string]int, len(r.board.Steps))
	for i := range r.board.Steps {
		s := &r.board.Steps[i]
		inDegree[s.ID] = len(s.DependsOn)
	}
	// 队列初始化：入度为 0 的节点（根 step）。
	queue := make([]string, 0, len(r.board.Steps))
	for i := range r.board.Steps {
		s := &r.board.Steps[i]
		if inDegree[s.ID] == 0 {
			queue = append(queue, s.ID)
		}
	}
	// BFS：每次出队一个节点，将其所有 dependents 的入度 -1；入度归 0 入队。
	visited := 0
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		visited++
		for _, depID := range r.dependents[cur] {
			inDegree[depID]--
			if inDegree[depID] == 0 {
				queue = append(queue, depID)
			}
		}
	}
	if visited != len(r.board.Steps) {
		// 入度仍 > 0 的节点构成环（或环的下游）。
		var cyclicNodes []string
		for i := range r.board.Steps {
			s := &r.board.Steps[i]
			if inDegree[s.ID] > 0 {
				cyclicNodes = append(cyclicNodes, s.ID)
			}
		}
		return fmt.Errorf("cyclic dependency detected: steps %v form a cycle (visited %d of %d steps)", cyclicNodes, visited, len(r.board.Steps))
	}
	return nil
}

// publishPlanBoardFailed 强制将 PlanBoard 标记为 Failed 并发布事件。
// 用于 DAG 校验失败时的 fail-closed 路径。
//
// 2026-07-15 P0-2 修复：publishPlanBoardTerminal 基于 step 状态扫描判定
// terminal 事件，环图所有 step 都是 Pending 会走 default 分支标 Completed。
// 此函数绕过 step 状态扫描，直接用状态机强制 Fail：
//   - board.Status == Planning → PlanBoardEventFailEarly（Planning → Failed）
//   - board.Status == Executing → PlanBoardEventFail（Executing → Failed）
//   - 其他（已 terminal 等）→ 跳过，不覆盖已有 terminal 状态
func (r *dagRun) publishPlanBoardFailed(ctx context.Context, reason string) {
	// P2-② 假启动拦截：board 失败 → 通知 WaitBoardStartup 对账失败，
	// plan_and_execute 将真实原因回传 Spirit（首发有效；已有成功信号时丢弃）。
	r.pe.signalStartup(r.board.ID, false, reason)
	var event biz.PlanBoardEvent
	switch r.board.Status {
	case biz.PlanStatusPlanning:
		event = biz.PlanBoardEventFailEarly
	case biz.PlanStatusExecuting:
		event = biz.PlanBoardEventFail
	default:
		// 已是 terminal 或其他状态，不强制覆盖。
		r.pe.lg.Warn("publishPlanBoardFailed: skip (board already terminal or unknown state)",
			loggateway.Str("plan_board_id", r.board.ID),
			loggateway.Str("status", string(r.board.Status)))
		return
	}
	newStatus, err := r.pe.pbSM.Transition(r.board.Status, event)
	if err != nil {
		r.pe.lg.Warn("publishPlanBoardFailed: invalid transition (skip)",
			loggateway.Str("plan_board_id", r.board.ID),
			loggateway.Str("from_status", string(r.board.Status)),
			loggateway.Str("event", string(event)),
			loggateway.Err(err))
		return
	}
	now := time.Now().UTC()
	r.board.Status = newStatus
	r.board.CompletedAt = &now
	r.board.Version++
	if _, err := r.pe.repos.UpsertPlanBoard(ctx, r.board); err != nil {
		r.pe.lg.Warn("publishPlanBoardFailed: upsert plan_board failed",
			loggateway.Str("plan_board_id", r.board.ID),
			loggateway.Err(err))
	}
	r.pe.seq.Publish(ctx, biz.NewPlanBoardUpdatedEvent(r.board))
	r.pe.lg.Info("PlanBoard 强制 Failed（DAG 校验失败）",
		loggateway.Str("plan_board_id", r.board.ID),
		loggateway.Str("status", string(newStatus)),
		loggateway.Str("reason", reason))
	// TS9-BUG-1: 同步推进 TaskPlan → failed。
	r.pe.propagateTaskPlanTerminal(ctx, r.board.ID, newStatus)
}

// publishGraphStageFailed 强制将 GraphStage 标记为 Failed 并发布事件。
// 用于 DAG 校验失败时的 fail-closed 路径。
//
// 2026-07-15 P0-2 修复：与 publishPlanBoardFailed 同理，绕过 step 状态扫描。
// 仅在 graphStageID 非空（initGraphStage 成功创建了 GraphStage）时发布。
func (r *dagRun) publishGraphStageFailed(ctx context.Context, reason string) {
	if r.graphStageID == "" {
		return
	}
	current, err := r.pe.repos.GetGraphStageByPlanBoard(ctx, r.board.ID)
	if err != nil || current.ID == "" {
		r.pe.lg.Warn("publishGraphStageFailed: failed to load current GraphStage (skip)",
			loggateway.Str("plan_board_id", r.board.ID),
			loggateway.Str("graph_stage_id", r.graphStageID),
			loggateway.Err(err))
		return
	}
	// 状态机校验：from=Running → Failed。若 current.Status 已是 terminal，
	// Transition 返回错误，跳过（不覆盖已有 terminal 状态）。
	newStatus, err := r.pe.gsSM.Transition(current.Status, biz.GraphStageEventFail)
	if err != nil {
		r.pe.lg.Warn("publishGraphStageFailed: invalid transition (skip)",
			loggateway.Str("graph_stage_id", r.graphStageID),
			loggateway.Str("from_status", string(current.Status)),
			loggateway.Err(err))
		return
	}
	now := time.Now().UTC()
	gs := biz.GraphStage{
		ID:          r.graphStageID,
		TaskID:      r.board.TaskID,
		TurnID:      r.board.TurnID,
		SessionID:   r.board.SessionID,
		PlanBoardID: r.board.ID,
		Status:      newStatus,
		StartedAt:   current.StartedAt,
		CompletedAt: &now,
		Seq:         current.Seq,
		Version:     current.Version + 1,
	}
	if _, err := r.pe.repos.UpsertGraphStage(ctx, gs); err != nil {
		r.pe.lg.Warn("publishGraphStageFailed: upsert graph_stage failed",
			loggateway.Str("graph_stage_id", r.graphStageID),
			loggateway.Str("plan_board_id", r.board.ID),
			loggateway.Err(err))
	}
	r.pe.seq.Publish(ctx, biz.NewGraphStageFailedEvent(gs))
	r.pe.lg.Info("GraphStage 强制 Failed（DAG 校验失败）",
		loggateway.Str("graph_stage_id", r.graphStageID),
		loggateway.Str("plan_board_id", r.board.ID),
		loggateway.Str("reason", reason))
}

// dispatch sends a single step to the TeamOrchestrator and listens for its
// completion. Runs in its own goroutine (via safego.Go).
func (r *dagRun) dispatch(ctx context.Context, step *biz.PlanStep) {
	safego.Go(ctx, "plan_executor.dispatch."+step.ID, func() {
		defer r.wg.Done()
		r.dispatchStep(ctx, step)
	})
}

// dispatchStep performs the full dispatch lifecycle for one step:
// transition step to Running → persist → publish → call orchestrator
// (creates TeamStage with derived ID) → update TeamStage with TaskID/DagNodeID
// → persist → publish → await completion.
//
// 2026-07-04 问题 4 修复：原先 dispatchStep 用 uuid.NewString() 创建 TeamStage，
// 而 publishSpiritTeamAssembled 内部用 agent.NewTeamStageActivityID(team.ID)
// 创建另一个 TeamStage，导致同一 team 有两条不同 ID 的记录，且 TeamRun/
// MemberSession 的 TeamStageID 关联到 publishSpiritTeamAssembled 的记录，
// dispatchStep 创建的记录在前端成为孤儿。
//
// 修复：dispatchStep 不再创建 TeamStage，而是让 Orchestrate 内部的
// publishSpiritTeamAssembled 创建（带 Members + 派生 ID），dispatchStep 在
// Orchestrate 返回后用 result.TeamStageID 更新同一记录（补充 TaskID/DagNodeID
// /Status=Running/Stage=Executing）。
func (r *dagRun) dispatchStep(ctx context.Context, step *biz.PlanStep) {
	if approved, held := r.holdPlaybookConfirm(ctx, step); held {
		if ctx.Err() != nil {
			r.abortPlaybookConfirm(ctx, step)
			return
		}
		if !approved {
			r.skipPlaybookConfirmDenied(ctx, step)
			return
		}
	}
	now := time.Now()
	// 1. Transition step to Running.
	r.mu.Lock()
	if err := step.Transition(biz.PlanStepStatusRunning); err != nil {
		r.mu.Unlock()
		r.pe.lg.Error("transition to running failed",
			loggateway.Str("step_id", step.ID), loggateway.Err(err))
		r.failStep(ctx, step, "transition: "+err.Error())
		return
	}
	step.StartedAt = now
	step.Version++
	runningStep := *step
	if strings.TrimSpace(runningStep.Mode) == "" {
		runningStep.Mode = biz.SpiritTeamModeForStep(string(r.board.Strategy), len(runningStep.AgentKeys))
	}
	r.mu.Unlock()
	// 2. Persist + publish PlanStepStarted.
	if _, err := r.pe.repos.UpsertPlanStep(ctx, runningStep); err != nil {
		r.pe.lg.Error("upsert plan_step (running) failed",
			loggateway.Str("step_id", step.ID), loggateway.Err(err))
	}
	r.pe.seq.Publish(ctx, biz.NewPlanStepStartedEvent(runningStep, r.board.SessionID))
	// 3. Call orchestrator (creates team + TeamStage via publishSpiritTeamAssembled).
	// 传入带 SessionID 的 TeamStage（Orchestrate 从 ts.SessionID 获取 spiritSessionID；
	// 不传入完整 TeamStage 是因为 TeamStage 的 ID/members 由 Orchestrate 内部派生）。
	result, err := r.pe.orch.Orchestrate(ctx, runningStep, biz.TeamStage{
		SessionID: r.board.SessionID,
		TaskID:    r.board.TaskID,
		TurnID:    r.board.TurnID,
		DagNodeID: step.ID,
	})
	if err != nil {
		r.pe.lg.Error("orchestrate failed",
			loggateway.Str("step_id", step.ID), loggateway.Err(err))
		r.failStep(ctx, step, "orchestrate: "+err.Error())
		return
	}
	if result == nil || result.Team.ID == "" || result.TeamStageID == "" {
		r.failStep(ctx, step, "orchestrate returned empty team or team_stage_id")
		return
	}
	// P2-② 假启动拦截：首个 team 真实创建成功 → 通知 WaitBoardStartup
	// 对账通过（plan_and_execute 声明的编排确有 team_run 落地）。
	r.pe.signalStartup(r.board.ID, true, "")
	// 2026-07-04 问题 P5/D1 修复：标记此 task 已派发 team。
	// OnTurnEnd 检查此标记，若为 true 则跳过 task.completed，
	// 等 synthesis turn 完成后再发 task.completed。
	if r.pe.marker != nil && r.board.TaskID != "" {
		r.pe.marker.MarkTeamDispatched(r.board.TaskID)
		r.pe.lg.Info("dispatchStep: 标记 task 已派发 team，延迟 task.completed",
			loggateway.Str("task_id", r.board.TaskID),
			loggateway.Str("step_id", step.ID),
			loggateway.Str("team_id", result.Team.ID),
		)
	}
	// 4. Update TeamStage (created inside Orchestrate) with TaskID/DagNodeID/
	//    Status=Running/Stage=Executing. Uses the same derived ID so the
	//    TeamRun/MemberSession records (already published with the same ID)
	//    stay associated.
	// Members is intentionally left nil: publishSpiritTeamAssembled already
	// set Members (with displayName/avatarUrl from agent config) on the
	// Version=1 record. Setting Members here would overwrite with degraded
	// data (AgentName=AgentKey, missing displayName/avatarUrl). The repo's
	// UpsertTeamStage skips SetMembers when nil, preserving the existing
	// value. Frontend also preserves existing Members when incoming is empty.
	//
	// 2026-07-05 P1 #9d（AS-FSM-01）：用 TeamStageStateMachine 校验 Pending → Running
	// 转换，并修复 Version=2 硬编码 Bug（改为 current.Version+1）。读取失败或状态机
	// 校验失败时降级为原行为（Version=2, Status=Running），保证主流程不中断。
	currentTS, getErr := r.pe.repos.GetTeamStage(ctx, result.TeamStageID)
	newStatus := biz.TeamStageStatusRunning
	newVersion := int64(2) // 降级默认值（与原硬编码一致）
	if getErr != nil {
		r.pe.lg.Warn("dispatchStep: failed to load current TeamStage, fallback to Version=2",
			loggateway.Str("team_stage_id", result.TeamStageID),
			loggateway.Err(getErr))
	} else {
		newVersion = currentTS.Version + 1
		if ns, smErr := r.pe.tsSM.Transition(currentTS.Status, biz.TeamStageEventStart); smErr == nil {
			newStatus = ns
		} else {
			r.pe.lg.Warn("dispatchStep: invalid TeamStage transition, fallback to Running",
				loggateway.Str("team_stage_id", result.TeamStageID),
				loggateway.Str("from_status", string(currentTS.Status)),
				loggateway.Err(smErr))
		}
	}
	ts := biz.TeamStage{
		ID:        result.TeamStageID,
		TaskID:    r.board.TaskID,
		TurnID:    r.board.TurnID,
		SessionID: r.board.SessionID,
		TeamID:    result.Team.ID,
		DagNodeID: step.ID,
		Status:    newStatus,
		Stage:     biz.TeamStageStageExecuting,
		DependsOn: result.Team.DependsOn,
		StartedAt: now,
		Version:   newVersion,
	}
	if _, err := r.pe.repos.UpsertTeamStage(ctx, ts); err != nil {
		r.pe.lg.Error("upsert team_stage (running) failed",
			loggateway.Str("team_stage_id", ts.ID), loggateway.Err(err))
	}
	r.pe.seq.Publish(ctx, biz.NewTeamStageUpdatedEvent(ts))
	r.publishUpward(ctx, biz.PipeUpwardHeartbeat, "阶段已开工："+runningStep.Label, map[string]any{
		"step_id": step.ID,
		"team_id": result.Team.ID,
	})
	// 5. Update GraphNode status + TeamStageID.
	r.updateGraphNode(ctx, step.ID, biz.GraphNodeStatusRunning, result.TeamStageID)
	// 6. Set MappedTeamStageID on the step.
	r.mu.Lock()
	step.MappedTeamStageID = result.TeamStageID
	r.mu.Unlock()
	// 7. Await completion (single event then channel closes).
	ch := result.CompletionChan
	select {
	case ev, ok := <-ch:
		if !ok {
			r.failStep(ctx, step, "orchestrator channel closed without event")
			return
		}
		r.handleCompletion(ctx, step, ev)
	case <-ctx.Done():
		return
	}
}

// handleCompletion processes a TeamCompleteEvent: marks the step completed or
// failed, then checks downstream.
func (r *dagRun) handleCompletion(ctx context.Context, step *biz.PlanStep, ev biz.TeamCompleteEvent) {
	now := time.Now()
	r.mu.Lock()
	if ev.Success {
		_ = step.Transition(biz.PlanStepStatusCompleted)
		step.CompletedAt = &now
		step.Version++
		current := *step
		r.mu.Unlock()
		// Publish terminal event + direct persist.
		if _, err := r.pe.repos.UpsertPlanStep(ctx, current); err != nil {
			r.pe.lg.Error("upsert plan_step (terminal) failed",
				loggateway.Str("step_id", step.ID), loggateway.Err(err))
		}
		r.pe.seq.Publish(ctx, biz.NewPlanStepCompletedEvent(current, r.board.SessionID))
		r.publishUpward(ctx, biz.PipeUpwardHeartbeat, "阶段完成："+current.Label, map[string]any{
			"step_id": step.ID,
		})
		// 2026-07-04 补齐：GraphNode → Completed
		r.updateGraphNode(ctx, step.ID, biz.GraphNodeStatusCompleted, "")
		r.checkDownstream(ctx, step.ID)
		return
	}
	r.mu.Unlock()
	// team_failed：成员执行期失败（LLM 首字节超时等瞬时故障），可自动重试。
	r.failStepWith(ctx, step, "team_failed", ev.ErrorMsg, true)
}

// failStep marks a step as failed without orchestrator completion (used for
// internal errors like persist failures or orchestrator invocation errors).
// 不消耗自动重试预算（F2 2026-09-03 全面检查修正）：启动前失败多为永久性
// 配置/校验错误（S07 类 "agent keys not found"），重试只会推迟
// WaitBoardStartup 对账信号（生产窗口 10s，5s 退避+二次失败会逼近上限）。
func (r *dagRun) failStep(ctx context.Context, step *biz.PlanStep, msg string) {
	r.failStepWith(ctx, step, "internal", msg, false)
}

// failStepWith 是 step 失败的统一出口（F2，2026-09-03 lbg-verify-planner
// 复盘 问题2）：标记 Failed → 持久化 → 发布事件（保持失败可见性）；
// retryable=true（team 执行失败）时先经 L4 失败分类器（P2-3）判定——能力
// 缺失/语义错误重试不会改变结果，跳过自动重试直接 cascade；瞬时故障（LLM
// 首字节超时等）才走 maybeRetryStep，重试耗尽才 cascadeSkip 下游，board 由
// publishPlanBoardTerminal 收敛终态；retryable=false（启动前内部错误）
// 立即 cascade，保持 P2-② 假启动对账的快速失败语义。之前失败即 cascade +
// board 终态，瞬时故障直接杀死整个任务计划。
func (r *dagRun) failStepWith(ctx context.Context, step *biz.PlanStep, code, msg string, retryable bool) {
	now := time.Now()
	r.mu.Lock()
	_ = step.Transition(biz.PlanStepStatusFailed)
	step.CompletedAt = &now
	step.Error = &biz.StepError{Code: code, Message: msg}
	step.Version++
	current := *step
	r.mu.Unlock()
	if _, err := r.pe.repos.UpsertPlanStep(ctx, current); err != nil {
		r.pe.lg.Error("upsert plan_step (failed) failed",
			loggateway.Str("step_id", step.ID), loggateway.Err(err))
	}
	r.pe.seq.Publish(ctx, biz.NewPlanStepFailedEvent(current, r.board.SessionID))
	r.publishUpward(ctx, biz.PipeUpwardException, "阶段例外："+current.Label+" "+msg, map[string]any{
		"step_id": step.ID,
	})
	// 2026-07-04 补齐：GraphNode → Failed
	r.updateGraphNode(ctx, step.ID, biz.GraphNodeStatusFailed, "")
	// P2-3（2026-09-03 L4 失败分类器规则版）：retryable 的 team 失败先分类，
	// 非瞬时故障（能力缺失/语义错误）跳过重试直接 cascade——重试不会改变
	// 结果，只会推迟对账信号并浪费一轮退避+整体团队重跑。
	if retryable {
		if class := biz.ClassifyStepFailure(msg); class != biz.StepFailureTransient {
			r.pe.lg.Info("step 失败分类为非瞬时故障，跳过自动重试",
				loggateway.Str("plan_board_id", r.board.ID),
				loggateway.Str("step_id", step.ID),
				loggateway.Str("failure_class", string(class)),
				loggateway.Str("error", msg))
		} else if r.maybeRetryStep(ctx, step) {
			return
		}
	}
	r.cascadeSkip(ctx, step.ID)
}

// planStepMaxRetries 是 step 级自动重试次数上限（F2）：首次失败后重试 1 次
// （共 2 次尝试）。叠于 F4 图级成员重试（retry_then_block×2）之上——图级
// 重试耗尽仍失败的 team 才走到这里，故 step 级再给 1 次整体重跑即可，
// 指数退避交给上层 idle 超时与 maxLifetime 兜底。
const planStepMaxRetries = 1

// planStepRetryBackoff 是自动重试前的退避等待（瞬时故障恢复窗口），
// 作为 NewPlanExecutor 中 stepRetryBackoff 字段的默认值（测试覆盖字段为 0）。
const planStepRetryBackoff = 5 * time.Second

// maybeRetryStep 在重试预算内自动重新 dispatch 失败 step（Failed→Running，
// 状态机允许）。返回 true 表示已重排队——此时不 cascadeSkip，board 保持
// executing，下游等待重试结果；返回 false 表示预算耗尽或已取消。
func (r *dagRun) maybeRetryStep(ctx context.Context, step *biz.PlanStep) bool {
	r.mu.Lock()
	if r.canceled {
		r.mu.Unlock()
		return false
	}
	r.attempts[step.ID]++
	attempt := r.attempts[step.ID]
	if attempt > planStepMaxRetries {
		r.mu.Unlock()
		return false
	}
	// wg.Add 必须在旧 dispatch goroutine 的 Done 之前（本函数运行在旧
	// goroutine 内，Done 是 defer 尚未触发），计数不会瞬时归零。
	r.wg.Add(1)
	r.mu.Unlock()
	r.pe.lg.Info("plan step 失败，自动重试",
		loggateway.Str("plan_board_id", r.board.ID),
		loggateway.Str("step_id", step.ID),
		loggateway.Int("attempt", attempt),
		loggateway.Int("max_retries", planStepMaxRetries))
	safego.Go(ctx, "plan_executor.retry."+step.ID, func() {
		defer r.wg.Done()
		if backoff := r.pe.stepRetryBackoff; backoff > 0 {
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		r.dispatchStep(ctx, step)
	})
	return true
}

func (e *PlanExecutor) hydrateBoardOrgFields(ctx context.Context, board *biz.PlanBoard) {
	if e == nil || e.taskPlans == nil || board == nil || len(board.Steps) == 0 {
		return
	}
	planID := taskPlanIDFromBoard(board.ID)
	if planID == "" && len(board.Steps) > 0 {
		planID = board.Steps[0].PlanID
	}
	if planID == "" {
		return
	}
	plan, err := e.taskPlans.GetByID(ctx, planID)
	if err != nil || plan == nil {
		return
	}
	biz.HydratePlanStepsFromSubTasks(plan.ID, board.Steps, plan.SubTasks)
}

func playbookConfirmKey(sessionID, stepID string) string {
	return strings.TrimSpace(sessionID) + "\x00" + strings.TrimSpace(stepID)
}

func playbookConfirmLookupIDs(sessionID, id string) []string {
	id = strings.TrimSpace(id)
	sessionID = strings.TrimSpace(sessionID)
	ids := []string{id}
	if stepID, ok := biz.ParsePlaybookConfirmActivityID(sessionID, id); ok {
		ids = append(ids, stepID)
	} else if id != "" {
		ids = append(ids, biz.PlaybookConfirmActivityID(sessionID, id))
	}
	return ids
}

// HasPlaybookStageConfirm reports whether a playbook stage is waiting for R18 confirm.
// id may be the plan-step id or PlaybookConfirmActivityID (ConfirmBlock card).
func (e *PlanExecutor) HasPlaybookStageConfirm(sessionID, stepID string) bool {
	if e == nil {
		return false
	}
	for _, id := range playbookConfirmLookupIDs(sessionID, stepID) {
		if _, ok := e.playbookConfirm.Load(playbookConfirmKey(sessionID, id)); ok {
			return true
		}
	}
	return false
}

// ResolvePlaybookStageConfirm unblocks a held playbook stage. Returns false when
// no waiter is registered (already resolved or not a confirm_before step).
func (e *PlanExecutor) ResolvePlaybookStageConfirm(sessionID, stepID string, approved bool) bool {
	if e == nil {
		return false
	}
	w := e.loadPlaybookConfirmWait(sessionID, stepID)
	if w == nil || w.ch == nil {
		return false
	}
	select {
	case w.ch <- approved:
	default:
	}
	for _, id := range playbookConfirmLookupIDs(sessionID, stepID) {
		e.playbookConfirm.Delete(playbookConfirmKey(sessionID, id))
	}
	e.finalizePlaybookConfirmCard(context.Background(), w.step, approved)
	return true
}

func (e *PlanExecutor) loadPlaybookConfirmWait(sessionID, stepID string) *playbookConfirmWait {
	for _, id := range playbookConfirmLookupIDs(sessionID, stepID) {
		v, ok := e.playbookConfirm.Load(playbookConfirmKey(sessionID, id))
		if !ok {
			continue
		}
		if w, ok := v.(*playbookConfirmWait); ok && w != nil {
			return w
		}
	}
	return nil
}

func (e *PlanExecutor) registerPlaybookConfirm(sessionID, stepID string) *playbookConfirmWait {
	w := &playbookConfirmWait{ch: make(chan bool, 1)}
	primary := playbookConfirmKey(sessionID, stepID)
	if actual, loaded := e.playbookConfirm.LoadOrStore(primary, w); loaded {
		if existing, ok := actual.(*playbookConfirmWait); ok && existing != nil {
			w = existing
		}
	}
	e.playbookConfirm.Store(playbookConfirmKey(sessionID, biz.PlaybookConfirmActivityID(sessionID, stepID)), w)
	return w
}

func (e *PlanExecutor) finalizePlaybookConfirmCard(ctx context.Context, step biz.Step, approved bool) {
	if e == nil || e.seq == nil || strings.TrimSpace(step.ID) == "" {
		return
	}
	now := time.Now()
	step.CompletedAt = &now
	step.Version++
	step.Status = biz.StepStatusCompleted
	if !approved {
		step.Status = biz.StepStatusCancelled
	}
	e.seq.Publish(ctx, biz.NewStepUpdatedEvent(step))
}

func (e *PlanExecutor) NotePlaybookConfirmDecision(sessionID, id string, approved bool) {
	if e == nil {
		return
	}
	for _, keyID := range playbookConfirmLookupIDs(sessionID, id) {
		e.playbookConfirmDecided.Store(playbookConfirmKey(sessionID, keyID), approved)
	}
}

func (e *PlanExecutor) lookupPlaybookConfirmDecision(sessionID, stepID string) (approved bool, ok bool) {
	if e == nil {
		return false, false
	}
	for _, id := range playbookConfirmLookupIDs(sessionID, stepID) {
		if v, loaded := e.playbookConfirmDecided.Load(playbookConfirmKey(sessionID, id)); loaded {
			approved, ok = v.(bool)
			return approved, ok
		}
	}
	if e.confirmSteps == nil {
		return false, false
	}
	actID := biz.PlaybookConfirmActivityID(sessionID, stepID)
	st, err := e.confirmSteps.GetStep(context.Background(), actID)
	if err != nil {
		return false, false
	}
	switch st.Status {
	case biz.StepStatusCompleted:
		return true, true
	case biz.StepStatusCancelled:
		return false, true
	default:
		return false, false
	}
}

func (e *PlanExecutor) abortPlaybookConfirmWaiters(ctx context.Context, sessionID, stepID string) {
	if e == nil {
		return
	}
	w := e.loadPlaybookConfirmWait(sessionID, stepID)
	for _, id := range playbookConfirmLookupIDs(sessionID, stepID) {
		e.playbookConfirm.Delete(playbookConfirmKey(sessionID, id))
	}
	if w != nil {
		e.finalizePlaybookConfirmCard(ctx, w.step, false)
	} else {
		e.finalizePlaybookConfirmCard(ctx, biz.Step{
			ID:              biz.PlaybookConfirmActivityID(sessionID, stepID),
			SessionID:       sessionID,
			SpiritSessionID: sessionID,
			Kind:            biz.StepKindConfirm,
			ToolName:        biz.ToolPlaybookConfirmBefore,
			Status:          biz.StepStatusToolBlocked,
			Version:         1,
		}, false)
	}
}

func (r *dagRun) abortPlaybookConfirm(ctx context.Context, step *biz.PlanStep) {
	if r == nil || r.pe == nil || step == nil {
		return
	}
	r.pe.abortPlaybookConfirmWaiters(ctx, r.board.SessionID, step.ID)
}

func (r *dagRun) holdPlaybookConfirm(ctx context.Context, step *biz.PlanStep) (approved bool, held bool) {
	if step == nil || !step.ConfirmBefore {
		return true, false
	}
	if biz.NeedsUserConfirm(biz.ConfirmInput{PlaybookConfirmBefore: true}) != biz.ConfirmPlaybookStage {
		return true, false
	}
	if r == nil || r.pe == nil {
		return true, false
	}
	if decided, ok := r.pe.lookupPlaybookConfirmDecision(r.board.SessionID, step.ID); ok {
		return decided, true
	}
	wait := r.pe.registerPlaybookConfirm(r.board.SessionID, step.ID)
	r.publishConfirmRequired(ctx, step, wait)
	select {
	case approved = <-wait.ch:
		return approved, true
	case <-ctx.Done():
		return false, true
	}
}

func (r *dagRun) publishConfirmRequired(ctx context.Context, step *biz.PlanStep, wait *playbookConfirmWait) {
	if r == nil || r.pe == nil || step == nil {
		return
	}
	summary := "请确认后继续阶段：" + step.Label
	biz.PublishUpwardProgress(r.pe.bus, ctx, r.board.SessionID, biz.PipeConfirmRequired, summary, map[string]any{
		"step_id":      step.ID,
		"confirm_kind": string(biz.ConfirmPlaybookStage),
	})
	if r.pe.seq == nil {
		return
	}
	now := time.Now()
	activityID := biz.PlaybookConfirmActivityID(r.board.SessionID, step.ID)
	confirm := biz.Step{
		ID:              activityID,
		TurnID:          r.board.TurnID,
		TaskID:          r.board.TaskID,
		SessionID:       r.board.SessionID,
		SpiritSessionID: r.board.SessionID,
		Kind:            biz.StepKindConfirm,
		Version:         1,
		Content:         summary,
		ToolName:        biz.ToolPlaybookConfirmBefore,
		Status:          biz.StepStatusToolBlocked,
		StartedAt:       now,
	}
	if wait != nil {
		wait.step = confirm
	}
	r.pe.seq.Publish(ctx, biz.NewStepCreatedEvent(confirm))
}

func (r *dagRun) skipPlaybookConfirmDenied(ctx context.Context, step *biz.PlanStep) {
	now := time.Now()
	r.mu.Lock()
	_ = step.Transition(biz.PlanStepStatusSkipped)
	// 标记 skip 来源（R1 审查修复）：人工拒绝确认的 step 不被 resume 复活。
	step.Error = &biz.StepError{Code: biz.StepErrCodeConfirmDenied, Message: "playbook stage confirmation denied"}
	step.CompletedAt = &now
	step.Version++
	skipped := *step
	r.mu.Unlock()
	if _, err := r.pe.repos.UpsertPlanStep(ctx, skipped); err != nil {
		r.pe.lg.Error("upsert plan_step (confirm denied) failed",
			loggateway.Str("step_id", step.ID), loggateway.Err(err))
	}
	r.pe.seq.Publish(ctx, biz.NewPlanStepSkippedEvent(skipped, r.board.SessionID, "playbook stage confirmation denied"))
	r.updateGraphNode(ctx, step.ID, biz.GraphNodeStatusInterrupted, "")
	r.cascadeSkip(ctx, step.ID)
}

func (r *dagRun) publishUpward(ctx context.Context, phase, summary string, extra map[string]any) {
	if r == nil || r.pe == nil {
		return
	}
	biz.PublishUpwardProgress(r.pe.bus, ctx, r.board.SessionID, phase, summary, extra)
}

// dispatchReadyPending starts every pending step whose dependencies are already
// completed. Called once at run() start (fresh roots + recover-ready steps).
func (r *dagRun) dispatchReadyPending(ctx context.Context) {
	for i := range r.board.Steps {
		s := &r.board.Steps[i]
		if s.Status != biz.PlanStepStatusPending {
			continue
		}
		if !r.dependenciesCompleted(s) {
			continue
		}
		r.wg.Add(1)
		r.dispatch(ctx, s)
	}
}

// dependencySatisfied 判定依赖 step 是否「已满足」（F2）：Completed 正常满足；
// Skipped 视为降级满足——resume(skip) 人工跳过失败 step 后其下游照常被
// dispatch（输入缺失由下游 team 自行应对）。正常执行流中该分支不可达：
// cascadeSkip 会把 skipped step 的全部 pending 下游同步置 skipped，不存在
// 「依赖 skipped 而自身 pending」的稳态。
func dependencySatisfied(st biz.PlanStepStatus) bool {
	return st == biz.PlanStepStatusCompleted || st == biz.PlanStepStatusSkipped
}

func (r *dagRun) dependenciesCompleted(s *biz.PlanStep) bool {
	if s == nil {
		return false
	}
	for _, d := range s.DependsOn {
		dep := r.stepsByID[d]
		if dep == nil || !dependencySatisfied(dep.Status) {
			return false
		}
	}
	return true
}

// checkDownstream dispatches pending steps whose dependencies are now all
// completed. Called after a step completes successfully.
func (r *dagRun) checkDownstream(ctx context.Context, completedID string) {
	deps := r.dependents[completedID]
	println("DEBUG checkDownstream", completedID, "deps", len(deps))
	for _, depID := range deps {
		r.mu.Lock()
		depStep, ok := r.stepsByID[depID]
		println("DEBUG dep", depID, "status", string(depStep.Status))
		if !ok || depStep.Status != biz.PlanStepStatusPending {
			r.mu.Unlock()
			continue
		}
		allCompleted := true
		for _, d := range depStep.DependsOn {
			s := r.stepsByID[d]
			if s == nil || !dependencySatisfied(s.Status) {
				allCompleted = false
				break
			}
		}
		if allCompleted {
			r.wg.Add(1)
			r.mu.Unlock()
			r.dispatch(ctx, depStep)
		} else {
			r.mu.Unlock()
		}
	}
}

// sweepNonTerminalSteps marks every step that never reached a terminal state
// as Skipped and publishes a PlanStepSkippedEvent (P1-6, cancel-path audit).
// Caller must hold the worker WaitGroup barrier (no in-flight writers).
func (r *dagRun) sweepNonTerminalSteps(ctx context.Context, reason string) {
	for i := range r.board.Steps {
		r.mu.Lock()
		step := &r.board.Steps[i]
		if step.Status != biz.PlanStepStatusPending && step.Status != biz.PlanStepStatusRunning {
			r.mu.Unlock()
			continue
		}
		if err := step.Transition(biz.PlanStepStatusSkipped); err != nil {
			r.mu.Unlock()
			r.pe.lg.Warn("cancel sweep: transition rejected",
				loggateway.Str("step_id", step.ID), loggateway.Err(err))
			continue
		}
		now := time.Now()
		step.CompletedAt = &now
		step.Version++
		skipped := *step
		r.mu.Unlock()
		if _, err := r.pe.repos.UpsertPlanStep(ctx, skipped); err != nil {
			r.pe.lg.Error("upsert plan_step (cancel-swept) failed",
				loggateway.Str("step_id", step.ID), loggateway.Err(err))
		}
		r.pe.seq.Publish(ctx, biz.NewPlanStepSkippedEvent(skipped, r.board.SessionID, reason))
		if skipped.ConfirmBefore {
			r.abortPlaybookConfirm(ctx, &skipped)
		}
		// GraphNode 映射与 cascadeSkip 一致：skipped → interrupted。
		r.updateGraphNode(ctx, step.ID, biz.GraphNodeStatusInterrupted, "")
	}
}

// cascadeSkip marks all transitive downstream dependents of a failed step as
// skipped (BFS). Each skipped step publishes a PlanStepSkippedEvent.
func (r *dagRun) cascadeSkip(ctx context.Context, failedID string) {
	visited := make(map[string]bool)
	queue := []string{failedID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current] {
			continue
		}
		visited[current] = true
		for _, depID := range r.dependents[current] {
			r.mu.Lock()
			depStep, ok := r.stepsByID[depID]
			if !ok || depStep.Status != biz.PlanStepStatusPending {
				r.mu.Unlock()
				continue
			}
			reason := fmt.Sprintf("dependency %s failed", failedID)
			_ = depStep.Transition(biz.PlanStepStatusSkipped)
			// 标记 skip 来源（R1 审查修复）：resume 只复活 cascade_skip
			// 受害者，人工拒绝/人工降级跳过的 step 不复活。
			depStep.Error = &biz.StepError{Code: biz.StepErrCodeCascadeSkip, Message: reason}
			depStep.Version++
			skipped := *depStep
			r.mu.Unlock()
			if _, err := r.pe.repos.UpsertPlanStep(ctx, skipped); err != nil {
				r.pe.lg.Error("upsert plan_step (skipped) failed",
					loggateway.Str("step_id", depID), loggateway.Err(err))
			}
			r.pe.seq.Publish(ctx, biz.NewPlanStepSkippedEvent(skipped, r.board.SessionID, reason))
			// 2026-07-04 补齐：GraphNode → Interrupted（skipped 映射为 interrupted）
			r.updateGraphNode(ctx, depID, biz.GraphNodeStatusInterrupted, "")
			queue = append(queue, depID)
		}
	}
}

// updateGraphNode updates the GraphNode status (and optionally TeamStageID)
// and publishes a GraphNodeUpdatedEvent. No-op if graphStageID is empty
// (GraphStage creation failed or was skipped).
//
// 设计：docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md §3.7.5
// GraphNode 状态由 PlanStep.Status 通过 MapPlanStepToGraphNodeStatus 映射得到。
// TeamStageID 在 dispatchStep 时回填，便于前端高亮显示对应节点。
//
// TeamStageID 保留规则：显式传入非空值优先；否则回退到 step.MappedTeamStageID。
// 终态更新（completed/failed）传入空字符串时不得清空已回填的 TeamStageID。
func (r *dagRun) updateGraphNode(ctx context.Context, stepID string, status biz.GraphNodeStatus, teamStageID string) {
	if r.graphStageID == "" {
		return // GraphStage 未创建，跳过
	}
	gn := biz.GraphNode{
		ID:           stepID, // GraphNode.ID = PlanStep.ID
		GraphStageID: r.graphStageID,
		Status:       status,
	}
	// 从 stepsByID 读取 Label/DagNodeID/DependsOn/MappedTeamStageID，避免
	// Upsert 用空字段覆盖 initGraphStage 与 dispatch 已写入的正确值。
	r.mu.Lock()
	if step, ok := r.stepsByID[stepID]; ok {
		gn.Label = step.Label
		gn.DagNodeID = step.ID
		gn.DependsOn = append([]string(nil), step.DependsOn...)
		if teamStageID == "" {
			teamStageID = step.MappedTeamStageID
		}
	}
	r.mu.Unlock()
	if teamStageID != "" {
		gn.TeamStageID = teamStageID
	}
	if _, err := r.pe.repos.UpsertGraphNode(ctx, gn); err != nil {
		r.pe.lg.Warn("upsert graph_node (status update) failed (non-blocking)",
			loggateway.Str("graph_node_id", stepID),
			loggateway.Str("status", string(status)),
			loggateway.Err(err),
		)
		return
	}
	// 发布 GraphNodeUpdatedEvent。taskID 和 spiritSessionID 从 board 派生。
	r.pe.seq.Publish(ctx, biz.NewGraphNodeUpdatedEvent(gn, r.board.TaskID, r.board.SessionID))
}
