// Code owned by package service; extracted from plan_executor.go for AS-COG-01 file-size fitness.

package service

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

func (r *dagRun) publishUpward(ctx context.Context, phase, summary string, extra map[string]any) {
	if r == nil || r.pe == nil {
		return
	}
	biz.PublishUpwardProgress(r.pe.bus, ctx, r.board.SessionID, phase, summary, extra)
}

// defaultQuotaRearmDelay 是配额活性兜底单飞重扫的默认周期（发现#7）：
// 本 run 在飞=0 且就绪步骤被全数延滞（仅外部占用配额）时按此周期重扫
// admitReadySteps，直至准入或 ctx 取消。测试可收窄 dagRun.quotaRearmDelay。
const defaultQuotaRearmDelay = 2 * time.Second

// admitReadySteps 是 PlanStep Pending→Running 的唯一调度入口（发现#15，
// 87-orchestration-chain-hardening Wave-2）：旧实现把「判定就绪」
// （dispatchReadyPending/checkDownstream）与「转换 Running」（dispatchStep）
// 拆在两个临界区，并发完成两个最后依赖时两路 handleCompletion 会双双判定
// 同一下游就绪并重复 dispatch，第二次 Transition 失败被 failStep 升级为
// PlanStepFailed 误杀链。本函数把两步收进同一个 r.mu 临界区（锁内预标记）：
// 单次扫描 r.board.Steps（按下标序，保根先于下游的准入确定性），每步在同
// 一个 Hold 内完成 status==Pending 检查 → 依赖就绪判定 → 配额闸门 →
// Transition(Running) → wg.Add(1) → 收集出参；临界区只碰内存，锁外逐个
// r.dispatch(ctx, s)。重复就绪判定（并发完成、终态/臂章重扫）被锁内状态
// 检查自然吞掉，任一 dagRun 实例内一个 step 的 Pending→Running 至多一次；
// dispatchStep 是断言方不是转移方。预标记纯内存——崩溃后 DB 仍为
// Pending/Failed，resume/recover 走原有重排队，恢复语义不变。
//
// 配额闸门（发现#7，per-spirit-session）：上限与 plan_and_execute 入口门
// （spirit_tools.go quota gate）同源——PlanExecutor.quota 的
// GetMaxParallelTeams/ListActiveTeams。max<=0 或 port==nil → 不限
// （fail-open，对齐入口门语义）；ListActiveTeams 出错 warn + 按 0 外部
// 占用处理（fail-open）。占用口径与入口门同为 IsTeamSlotOccupied
// （pending||running；interrupted 不占座，D2-P2 同口径）。ownTeams=本 run
// 内 Running 且已回填 MappedTeamStageID 的步数，external=max(0,
// occupied-ownTeams)（Orchestrate 建团到回填的微窗口会短暂双计，偏保守
// 永不欠计，下一重扫自愈）。available=max-external-runningCount；
// runningCount 由锁内 O(n) 计数派生（不写计数器字段，根除 retry/泄漏
// 记账面）。同一临界区按下标序准入：available>0 才允许预标记，每准入一步
// available--；无槽的就绪步骤保持 Pending（延后准入，不产生任何事件/
// 持久化写）。confirm_before 步骤预标记后即持槽（等待人工确认期间占额度，
// 保守语义防批量批准超发；拒绝即释放并经终态重扫准入后续步骤）。
//
// 活性兜底（发现#7）：每次步骤终态转移（Completed/Failed-terminal/
// Skipped-denied）后调用方必有一次 admitReadySteps 全局重扫；当「本次
// 准入 0 且仍存在就绪延滞步骤且 runningCount==0 且配额受限」时，经
// maybeRearmQuotaSweep 置单飞臂章并重扫——外部团队的完成无人能唤醒本
// run，否则 board 会带着 pending 步骤误判 Completed。
func (r *dagRun) admitReadySteps(ctx context.Context) {
	// 配额外部占用在锁外计算：port 查询是 DB 读，不进临界区；只有 max>0
	// 时才需要占用数据。
	limited := false
	occupied := 0
	maxParallel := 0
	if r.pe != nil && r.pe.quota != nil {
		maxParallel = r.pe.quota.GetMaxParallelTeams(ctx, r.board.SessionID)
		if maxParallel > 0 {
			limited = true
			active, err := r.pe.quota.ListActiveTeams(ctx, r.board.SessionID)
			if err != nil {
				r.pe.lg.Warn("admitReadySteps: 查询活跃团队列表失败，按 0 外部占用处理（fail-open）",
					loggateway.Str("plan_board_id", r.board.ID),
					loggateway.Err(err))
			} else {
				for _, t := range active {
					if biz.IsTeamSlotOccupied(t.Status) {
						occupied++
					}
				}
			}
		}
	}
	var admitted []*biz.PlanStep
	deferred := false
	runningCount := 0
	rearm := false
	r.mu.Lock()
	available := 0
	if limited {
		ownTeams := 0
		for i := range r.board.Steps {
			s := &r.board.Steps[i]
			if s.Status == biz.PlanStepStatusRunning {
				runningCount++
				if s.MappedTeamStageID != "" {
					ownTeams++
				}
			}
		}
		external := occupied - ownTeams
		if external < 0 {
			external = 0
		}
		available = maxParallel - external - runningCount
	}
	for i := range r.board.Steps {
		s := &r.board.Steps[i]
		if s.Status != biz.PlanStepStatusPending {
			continue
		}
		if !r.dependenciesCompleted(s) {
			continue
		}
		if limited && available <= 0 {
			deferred = true
			continue
		}
		if err := s.Transition(biz.PlanStepStatusRunning); err != nil {
			// 防御：Pending→Running 是唯一合法出边之一，理论不可达；
			// 打日志跳过即可，绝不升级 failStep（发现#15 误杀链教训）。
			r.pe.lg.Error("admitReadySteps: Pending→Running 预标记失败（跳过该步）",
				loggateway.Str("plan_board_id", r.board.ID),
				loggateway.Str("step_id", s.ID),
				loggateway.Err(err))
			continue
		}
		available--
		r.wg.Add(1)
		admitted = append(admitted, s)
	}
	if limited && len(admitted) == 0 && deferred && runningCount == 0 && !r.quotaRearmActive {
		// 单飞臂章（活性兜底）：臂章期间 wg 持一槽，run() 的 wg 屏障不会
		// 在延滞步骤尚存时提前放开误判终态；rearm goroutine 退出时 Done。
		r.quotaRearmActive = true
		r.wg.Add(1)
		rearm = true
	}
	r.mu.Unlock()
	if rearm {
		r.maybeRearmQuotaSweep(ctx)
	}
	for _, s := range admitted {
		r.dispatch(ctx, s)
	}
}

// maybeRearmQuotaSweep 启动单飞重扫 goroutine（发现#7 活性兜底）：臂章
// （quotaRearmActive）与 wg 槽已由 admitReadySteps 的临界区置位/持有；
// 本函数只负责按 quotaRearmDelay 定时重扫。ctx 取消立即退出（臂章清除、
// wg 释放），run 收尾由 sweepNonTerminalSteps 统一置 Skipped。重扫后仍
// 延滞时 admitReadySteps 会再次置臂章续扫（单飞链式）。
func (r *dagRun) maybeRearmQuotaSweep(ctx context.Context) {
	safego.Go(ctx, "plan_executor.quota_rearm."+r.board.ID, func() {
		defer r.wg.Done()
		delay := r.quotaRearmDelay
		if delay <= 0 {
			delay = defaultQuotaRearmDelay
		}
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			r.mu.Lock()
			r.quotaRearmActive = false
			r.mu.Unlock()
			return
		case <-timer.C:
		}
		r.mu.Lock()
		r.quotaRearmActive = false
		r.mu.Unlock()
		r.admitReadySteps(ctx)
	})
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
			if !ok || depStep == nil || depStep.Status != biz.PlanStepStatusPending {
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
	if step, ok := r.stepsByID[stepID]; ok && step != nil {
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
