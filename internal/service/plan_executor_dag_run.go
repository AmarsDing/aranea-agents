// Code owned by package service; extracted from plan_executor.go for AS-COG-01 file-size fitness.

package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

type dagRun struct {
	pe    *PlanExecutor
	board biz.PlanBoard

	// graphStageID 是与 PlanBoard 一对一关联的 GraphStage 的 ID（在 initGraphStage
	// 中创建）。如果创建失败则为空，此时跳过 GraphNode 更新。
	graphStageID string

	// canceled is set when the Subscribe ctx is cancelled. Terminal publish
	// uses a fresh context budget, so this flag (not ctx.Err()) drives Fail/
	// Interrupt classification after the worker barrier.
	//
	// 锁契约（发现#17，87-orchestration-chain-hardening Wave-2）：全部写
	// 必须持 r.mu（run() 的 ctx.Done 分支，且在 wg 屏障 <-done 之前）；
	// 全部读要么持锁（maybeRetryStep），要么在屏障后的 run goroutine 程序
	// 序内（publishPlanBoardTerminal/publishGraphStageTerminal）。新增读点
	// 必须遵守同一契约，否则重新引入与持锁读之间的 data race。
	canceled bool

	mu         sync.Mutex
	stepsByID  map[string]*biz.PlanStep
	dependents map[string][]string // stepID → stepIDs that depend on it
	wg         sync.WaitGroup

	// quotaRearmActive 是发现#7 配额活性兜底的单飞臂章（r.mu 保护）：
	// 本 run 在飞=0 且就绪步骤被配额全数延滞时置位，重扫 goroutine 退出
	// 时清除并重调 admitReadySteps；臂章期间 wg 持一槽防 run() 误判终态。
	quotaRearmActive bool
	// quotaRearmDelay 是单飞重扫周期（默认 defaultQuotaRearmDelay；
	// 测试可收窄）。
	quotaRearmDelay time.Duration

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
		// 发现#7：配额活性兜底重扫周期默认值（测试可收窄字段）。
		quotaRearmDelay: defaultQuotaRearmDelay,
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
	// 发现#15：唯一调度入口改为 admitReadySteps（锁内预标记+配额闸门）。
	r.admitReadySteps(ctx)
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
		// 发现#17：canceled 的全部写必须持 r.mu——与 maybeRetryStep 的持锁
		// 读构成 happens-before；屏障后的 publish 读点在 run goroutine 程序
		// 序内，天然有序（见 dagRun.canceled 字段注释的锁契约）。
		r.mu.Lock()
		r.canceled = true
		r.mu.Unlock()
		// P1 (audit): do not compute terminal state until in-flight workers
		// exit. Previously <-ctx.Done() raced ahead of wg.Wait(), so
		// publish*Terminal could run while dispatch goroutines still wrote.
		<-done
		// 取消拦截假启动：首发有效（成功信号已投递则本信号被丢弃），
		// 尚未派发首步即被取消的板让 WaitBoardStartup 拿到失败原因而非超时。
		r.pe.signalStartup(r.board.ID, false, "plan board run canceled")
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
