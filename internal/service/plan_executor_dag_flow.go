// Code owned by package service; extracted from plan_executor.go for AS-COG-01 file-size fitness.

package service

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

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
	for _, depID := range deps {
		r.mu.Lock()
		depStep, ok := r.stepsByID[depID]
		if !ok || depStep == nil || depStep.Status != biz.PlanStepStatusPending {
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
