// Code owned by package service; extracted from plan_executor.go for AS-COG-01 file-size fitness.

package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/loggateway"
)

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
	// P0-6 阶段 1a：端口接线时放行端口决议（跨副本行租约，含 DB 错误
	// fail-closed=true 保守拦截）；nil 端口纯进程内 Range 字节级旧行为。
	if e.leasePort != nil {
		return e.leasePort.HasActiveForSession(sessionID)
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
	e.releaseTeamAgentLoads(teamID)
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
