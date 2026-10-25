// Code owned by package service; extracted from plan_executor.go for AS-COG-01 file-size fitness.

package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func (r *dagRun) releaseLeaseAndNotifyCompletion() {
	r.pe.running.Delete(r.board.ID)
	// 先释放端口租约再通知终态（对齐 dag_run.go『先释放再通知』不变量）：
	// 防止 final synthesis 触发方在租约仍活时被 HasActive 判定撞车（对抗评审 #2）。
	// lease-aware run 的 leasePort 与本地 running 行成对存在；idempotent。
	if r.pe.leasePort != nil {
		r.pe.leasePort.Release(r.board.ID)
	}
	if r.pe.completionNotifier == nil || r.board.SessionID == "" {
		return
	}
	// 2026-07-04 问题 5 ctx fallback 死路径修复（D3-FIX-2，87-orchestration-
	// chain-hardening）：释放 lease 的通知路径此前裸用 context.Background()，
	// resolveLatestUserTaskID 的「ctx 权威」分支恒走不到（RootTaskActivityID
	// 空），挂靠只能依赖 DB 的『最新 user task』推断——重启/重排场景会错挂
	// 到别的轮回。此处注入 board.TaskID 作为 ctx 权威根。
	notifyCtx := context.Background()
	if tid := strings.TrimSpace(r.board.TaskID); tid != "" {
		notifyCtx = chatagent.ContextWithRootTaskActivityID(notifyCtx, chatagent.RootTaskActivityID(tid))
	}
	r.pe.completionNotifier.NotifyAllTeamsCompleted(notifyCtx, r.board.SessionID)
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
