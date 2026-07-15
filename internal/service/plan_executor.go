package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
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
// TECH-DEBT(COG): 接口方法数=8 > 5（CS-B4），但本接口是组合接口（compose 5 个 v2 repo），
// 组合接口的方法数放宽限制是合理的。拆分会引入 5 个独立 adapter，增加复杂度无收益。
type executorRepos interface {
	UpsertPlanStep(ctx context.Context, ps biz.PlanStep) (biz.PlanStep, error)
	UpsertTeamStage(ctx context.Context, ts biz.TeamStage) (biz.TeamStage, error)
	UpsertPlanBoard(ctx context.Context, pb biz.PlanBoard) (biz.PlanBoard, error)
	GetPlanStep(ctx context.Context, id string) (biz.PlanStep, error)
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
	// multi-instance, or event bus redelivery). Key: board.ID, Value: struct{}.
	running sync.Map
}

// TeamDispatchMarker 标记一个 task 已派发 team。
// 由 ProjectorFactory 实现（internal/agent/v2）。
// 2026-07-04 问题 P5/D1 修复。
type TeamDispatchMarker interface {
	MarkTeamDispatched(taskID string)
}

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

// TeamCompletionNotifier is implemented by TeamOrchestrators that track
// pending team_run completions and notify waiting dispatchStep goroutines.
// 2026-07-04 问题 4 修复：让 PlanExecutor 转发 team_run 完成通知给 TeamOrchestrator。
type TeamCompletionNotifier interface {
	NotifyTeamCompletion(teamID string, success bool, errMsg string)
}

// NotifyTeamCompletion forwards a team_run completion event to the
// TeamOrchestrator (if it implements TeamCompletionNotifier). Called by
// TeamStarter.HandleTeamTurnResult when a team_run reaches terminal status.
// 2026-07-04 问题 4 修复：让 PlanExecutor 转发 team_run 完成通知。
func (e *PlanExecutor) NotifyTeamCompletion(teamID string, success bool, errMsg string) {
	if notifier, ok := e.orch.(TeamCompletionNotifier); ok {
		notifier.NotifyTeamCompletion(teamID, success, errMsg)
	}
}

// StartSubscription subscribes to PlanBoardCreatedEvent on the EventBus and
// triggers PlanExecutor.Subscribe in a goroutine for each new PlanBoard.
// Must be called after SetEventBus. No-op if bus is nil.
// 2026-07-04 问题 4 修复：让 PlanExecutor 自动响应 PlanBoard 创建事件。
func (e *PlanExecutor) StartSubscription() {
	if e.bus == nil {
		return
	}
	ch, cancel := e.bus.Subscribe(biz.EventSubscribeOptions{})
	e.lg.Info("PlanExecutor 开始订阅 PlanBoardCreatedEvent")
	go func() {
		defer cancel()
		for ev := range ch {
			pbEv, ok := ev.(*biz.PlanBoardCreatedEvent)
			if !ok {
				continue
			}
			board := pbEv.PlanBoard
			if len(board.Steps) == 0 {
				continue
			}
			// C-20 fix: execution lease — skip duplicate events for the same
			// board. LoadOrStore is atomic: if board.ID already exists, the
			// event is a replay/duplicate → skip. Otherwise mark as running.
			if _, loaded := e.running.LoadOrStore(board.ID, struct{}{}); loaded {
				e.lg.Warn("PlanBoard 已在执行中，跳过重复事件",
					loggateway.Str("plan_board_id", board.ID),
					loggateway.Str("task_id", board.TaskID))
				continue
			}
			e.lg.Info("PlanExecutor 收到 PlanBoardCreatedEvent，启动 DAG 执行",
				loggateway.Str("plan_board_id", board.ID),
				loggateway.Str("task_id", board.TaskID),
				loggateway.Int("steps", len(board.Steps)))
			// Subscribe 是阻塞的，在独立 goroutine 中执行。
			// 使用 context.Background() 因为 DAG 执行可能比原始 ctx 生命周期长。
			// 2026-07-04 问题 4 修复：从 PlanBoard.TaskID 恢复 RootTaskActivityID
			// 注入 ctx，让下游 buildTeamProjectMeta / publishV2TeamRunAndMemberSessions
			// / publishV2TeamRunCompletion 都能拿到正确的 rootTaskID（之前为空字符串
			// 导致 MemberSession.TaskID 为空，前端 getMemberSessionSteps 返回空数组）。
			go func(b biz.PlanBoard) {
				defer e.running.Delete(b.ID) // C-20: release lease on exit
				runCtx := context.Background()
				if b.TaskID != "" {
					runCtx = agent.ContextWithRootTaskActivityID(
						runCtx, agent.RootTaskActivityID(b.TaskID))
				}
				if err := e.Subscribe(runCtx, b); err != nil {
					e.lg.Warn("PlanExecutor.Subscribe 失败",
						loggateway.Str("plan_board_id", b.ID),
						loggateway.Err(err))
				}
			}(board)
		}
	}()
}

// Subscribe starts DAG execution for the given board and blocks until all
// steps reach a terminal status or ctx is canceled.
//
// 2026-07-04 补齐：在 DAG 执行前同步创建 GraphStage（与 PlanBoard 一对一关联）
// 和 GraphNode 列表（每个 PlanStep 对应一个 GraphNode），并发布 v2 事件。
func (e *PlanExecutor) Subscribe(ctx context.Context, board biz.PlanBoard) error {
	if len(board.Steps) == 0 {
		return nil
	}
	// 2026-07-04 问题 D2 修复：DAG 执行开始前，将 PlanBoard 状态从 planning
	// 更新为 executing，让前端能看到计划已进入执行阶段。之前 PlanBoard 创建后
	// Status 始终是 "planning"，DAG 执行完成后直接跳到 "completed"，前端无法
	// 区分"正在编排"和"正在执行"。
	e.markPlanBoardExecuting(ctx, board)
	// 同步创建 GraphStage（与 PlanBoard 一对一）。失败不阻断主流程，
	// 仅记录日志（GraphStage 是可视化层，缺失不影响 DAG 调度正确性）。
	e.initGraphStage(ctx, board)
	r := newDagRun(e, board)
	return r.run(ctx)
}

// markPlanBoardExecuting updates the PlanBoard status from "planning" to
// "executing" and publishes a PlanBoardUpdatedEvent so the frontend can
// reflect the transition. Idempotent: if the PlanBoard is already in a
// terminal or executing state, the update is skipped.
//
// 2026-07-04 问题 D2 修复：补齐 planning → executing 状态转换。
// 2026-07-05 P1 #9b（AS-FSM-01）：用 PlanBoardStateMachine 显式校验转换，
// 替代直接 if + 字段赋值。状态机会拒绝任何非法 from 状态（如 terminal）。
func (e *PlanExecutor) markPlanBoardExecuting(ctx context.Context, board biz.PlanBoard) {
	// 状态机校验：只有 Planning 才能 Transition(Execute) → Executing。
	// 若 board 已是 Executing 或 terminal 状态，Transition 返回错误，跳过。
	newStatus, err := e.pbSM.Transition(board.Status, biz.PlanBoardEventExecute)
	if err != nil {
		e.lg.Debug("markPlanBoardExecuting: skip (invalid transition)",
			loggateway.Str("plan_board_id", board.ID),
			loggateway.Str("from_status", string(board.Status)),
			loggateway.Err(err))
		return
	}
	board.Status = newStatus
	board.Version++
	if _, err := e.repos.UpsertPlanBoard(ctx, board); err != nil {
		e.lg.Warn("markPlanBoardExecuting: upsert plan_board (executing) failed",
			loggateway.Str("plan_board_id", board.ID),
			loggateway.Err(err))
		return
	}
	e.seq.Publish(ctx, biz.NewPlanBoardUpdatedEvent(board))
	e.lg.Info("PlanBoard 状态转换: planning → executing",
		loggateway.Str("plan_board_id", board.ID),
		loggateway.Str("task_id", board.TaskID))
}

// initGraphStage creates the GraphStage (and its GraphNodes) for the given
// PlanBoard if it doesn't already exist. Idempotent: if a GraphStage is
// already associated with the PlanBoard, it's left as-is.
//
// 2026-07-04 问题 5 修复：task_planner_impl.go:publishV2PlanBoard 已通过
// seq.Publish 异步创建 GraphStage + GraphNodes + 发送事件。此处保留同步
// UpsertGraphStage 作为 crash recovery fallback（确保 newDagRun 的
// GetGraphStageByPlanBoard 能查到），但移除 seq.Publish 避免重复发送
// GraphStageCreatedEvent（task_planner 已发）。
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
	// VersionLT 守卫使此写入幂等：若 task_planner 的异步持久化已完成，此写入被拒绝。
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
	e.seq.Publish(ctx, biz.NewGraphStageCreatedEvent(gs))
	e.lg.Info("initGraphStage: 同步创建 GraphStage 并发布 created 事件",
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

	mu         sync.Mutex
	stepsByID  map[string]*biz.PlanStep
	dependents map[string][]string // stepID → stepIDs that depend on it
	wg         sync.WaitGroup
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
	// Dispatch root steps (empty DependsOn). Add to WaitGroup before
	// starting the goroutine to guarantee Wait sees the correct count.
	for i := range r.board.Steps {
		s := &r.board.Steps[i]
		if len(s.DependsOn) == 0 && s.Status == biz.PlanStepStatusPending {
			r.wg.Add(1)
			r.dispatch(ctx, s)
		}
	}
	// Wait for all goroutines (root + downstream) to finish.
	done := make(chan struct{})
	go func() { r.wg.Wait(); close(done) }()
	var runErr error
	select {
	case <-done:
		runErr = nil
	case <-ctx.Done():
		runErr = ctx.Err()
	}
	// 发布 terminal 事件（无论成功/失败/取消），让前端流程图和计划列表
	// 在刷新后能正确显示最终状态。失败仅记录日志，不阻断返回。
	// 2026-07-04 问题 2 修复（Gap A + Gap B）：
	//   - Gap A: GraphStage terminal 事件（Completed/Failed/Interrupted）
	//   - Gap B: PlanBoard terminal 状态更新（Completed/Failed/PartialFailure）
	r.publishPlanBoardTerminal(ctx)
	r.publishGraphStageTerminal(ctx)
	return runErr
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
	hasFailed := false
	hasPartial := false
	for i := range r.board.Steps {
		s := &r.board.Steps[i]
		switch s.Status {
		case biz.PlanStepStatusFailed:
			hasFailed = true
		case biz.PlanStepStatusPartialFailure:
			hasPartial = true
		}
	}
	var event biz.PlanBoardEvent
	switch {
	case ctx.Err() != nil:
		event = biz.PlanBoardEventFail
	case hasFailed:
		event = biz.PlanBoardEventFail
	case hasPartial:
		event = biz.PlanBoardEventPartial
	default:
		event = biz.PlanBoardEventComplete
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
	// 扫描所有 step 状态，判定 terminal 事件。
	hasFailed := false
	hasRunning := false
	for i := range r.board.Steps {
		s := &r.board.Steps[i]
		switch s.Status {
		case biz.PlanStepStatusFailed, biz.PlanStepStatusPartialFailure:
			hasFailed = true
		case biz.PlanStepStatusRunning, biz.PlanStepStatusPending:
			hasRunning = true
		}
	}
	var event biz.GraphStageEvent
	switch {
	case ctx.Err() != nil:
		event = biz.GraphStageEventInterrupt
	case hasFailed:
		event = biz.GraphStageEventFail
	default:
		// 没有 failed step；若仍有 running/pending（理论上不该出现，因为
		// Wait 已返回），保守视为 Completed——状态详情由 GraphNode 体现。
		_ = hasRunning
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
		StartedAt:   current.StartedAt,    // 保留原 StartedAt
		CompletedAt: &now,
		Seq:         current.Seq,          // 保留原 Seq
		Version:     current.Version + 1,  // 递增 Version（替代硬编码 Version=3）
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
	} else {
		_ = step.Transition(biz.PlanStepStatusFailed)
		step.CompletedAt = &now
		step.Error = &biz.StepError{Code: "team_failed", Message: ev.ErrorMsg}
	}
	step.Version++
	current := *step
	r.mu.Unlock()
	// Publish terminal event + direct persist.
	if _, err := r.pe.repos.UpsertPlanStep(ctx, current); err != nil {
		r.pe.lg.Error("upsert plan_step (terminal) failed",
			loggateway.Str("step_id", step.ID), loggateway.Err(err))
	}
	if ev.Success {
		r.pe.seq.Publish(ctx, biz.NewPlanStepCompletedEvent(current, r.board.SessionID))
		// 2026-07-04 补齐：GraphNode → Completed
		r.updateGraphNode(ctx, step.ID, biz.GraphNodeStatusCompleted, "")
		r.checkDownstream(ctx, step.ID)
	} else {
		r.pe.seq.Publish(ctx, biz.NewPlanStepFailedEvent(current, r.board.SessionID))
		// 2026-07-04 补齐：GraphNode → Failed
		r.updateGraphNode(ctx, step.ID, biz.GraphNodeStatusFailed, "")
		r.cascadeSkip(ctx, step.ID)
	}
}

// failStep marks a step as failed without orchestrator completion (used for
// internal errors like persist failures or orchestrator invocation errors).
func (r *dagRun) failStep(ctx context.Context, step *biz.PlanStep, msg string) {
	now := time.Now()
	r.mu.Lock()
	_ = step.Transition(biz.PlanStepStatusFailed)
	step.CompletedAt = &now
	step.Error = &biz.StepError{Code: "internal", Message: msg}
	step.Version++
	current := *step
	r.mu.Unlock()
	if _, err := r.pe.repos.UpsertPlanStep(ctx, current); err != nil {
		r.pe.lg.Error("upsert plan_step (failed) failed",
			loggateway.Str("step_id", step.ID), loggateway.Err(err))
	}
	r.pe.seq.Publish(ctx, biz.NewPlanStepFailedEvent(current, r.board.SessionID))
	// 2026-07-04 补齐：GraphNode → Failed
	r.updateGraphNode(ctx, step.ID, biz.GraphNodeStatusFailed, "")
	r.cascadeSkip(ctx, step.ID)
}

// checkDownstream dispatches pending steps whose dependencies are now all
// completed. Called after a step completes successfully.
func (r *dagRun) checkDownstream(ctx context.Context, completedID string) {
	deps := r.dependents[completedID]
	for _, depID := range deps {
		r.mu.Lock()
		depStep, ok := r.stepsByID[depID]
		if !ok || depStep.Status != biz.PlanStepStatusPending {
			r.mu.Unlock()
			continue
		}
		allCompleted := true
		for _, d := range depStep.DependsOn {
			s := r.stepsByID[d]
			if s == nil || s.Status != biz.PlanStepStatusCompleted {
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

// cascadeSkip marks all transitive downstream dependents of a failed step as
// skipped (BFS). Each skipped step publishes a PlanStepSkippedEvent.
func (r *dagRun) cascadeSkip(ctx context.Context, failedID string) {
	queue := []string{failedID}
	visited := make(map[string]bool)
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
func (r *dagRun) updateGraphNode(ctx context.Context, stepID string, status biz.GraphNodeStatus, teamStageID string) {
	if r.graphStageID == "" {
		return // GraphStage 未创建，跳过
	}
	gn := biz.GraphNode{
		ID:           stepID, // GraphNode.ID = PlanStep.ID
		GraphStageID: r.graphStageID,
		Status:       status,
	}
	// 2026-07-05 修复：从 stepsByID 读取 step.Label 和 step.ID 填充 gn.Label
	// 和 gn.DagNodeID，避免 UpsertGraphNode 的 Update 分支用空字符串覆盖
	// initGraphStage 之前写入的正确值。
	r.mu.Lock()
	if step, ok := r.stepsByID[stepID]; ok {
		gn.Label = step.Label
		gn.DagNodeID = step.ID
		gn.DependsOn = append([]string(nil), step.DependsOn...)
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
