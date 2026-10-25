// Code owned by package service; extracted from plan_executor.go for AS-COG-01 file-size fitness.

package service

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

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

// claimBoardRun 认领 board 执行租约：创建 runCtx/cancel/本地 lease 行，
// 端口接线时先认领跨副本租约。ok=false 仅当端口接线且端口认领被拒（含
// 错误 fail-closed）；nil 端口恒 ok（字节级旧行为，端口零调用）。
// 拆出独立阶段（对抗评审 #12）：resume 须在任何共享态变更前完成认领，
// 否则本副本改完状态后认领被拒 → board 停 Executing 却无人跑（跨副本楔死）。
func (e *PlanExecutor) claimBoardRun(board biz.PlanBoard) (context.Context, context.CancelFunc, *boardRunLease, bool) {
	// C-20 + C-18: execution lease for ready boards.
	runCtx, cancel := context.WithCancel(context.Background())
	lease := &boardRunLease{cancel: cancel, sessionID: board.SessionID}
	// P0-6 阶段 1a（90-multi-replica-board-lease）：端口接线时先认领跨副本
	// 租约——claimed=false 等价本地 LoadOrStore loaded（先到者赢，含跨进程）；
	// 任何错误 fail-closed 不派发。nil 端口零调用，字节级旧行为。
	if e.leasePort != nil && !e.leasePort.TryClaim(runCtx, biz.BoardRunLease{BoardID: board.ID, SessionID: board.SessionID, Cancel: cancel}) {
		cancel()
		e.lg.Warn("PlanBoard 执行租约已被持有（含跨副本认领），跳过重复启动",
			loggateway.Str("plan_board_id", board.ID),
			loggateway.Str("task_id", board.TaskID))
		return nil, nil, nil, false
	}
	return runCtx, cancel, lease, true
}

// startBoardDAG takes the execution lease and runs Subscribe in a goroutine.
// Idempotent: a second call for the same board ID is ignored.
func (e *PlanExecutor) startBoardDAG(board biz.PlanBoard) {
	if e == nil {
		return
	}
	runCtx, cancel, lease, ok := e.claimBoardRun(board)
	if !ok {
		return
	}
	e.startClaimedBoardDAG(board, runCtx, cancel, lease)
}

// startClaimedBoardDAG 在调用方已完成端口认领后本地落册并起 goroutine
// （对抗评审 #12 拆出的后半段）。本地 LoadOrStore 撞车时回滚端口认领，
// 与 claimBoardRun 的认领分支成对。
func (e *PlanExecutor) startClaimedBoardDAG(board biz.PlanBoard, runCtx context.Context, cancel context.CancelFunc, lease *boardRunLease) {
	if _, loaded := e.running.LoadOrStore(board.ID, lease); loaded {
		if e.leasePort != nil {
			e.leasePort.Release(board.ID) // 端口已认领但本地落册失败——回滚认领
		}
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
		defer func() {
			e.running.Delete(b.ID) // C-20: release lease on exit
			if e.leasePort != nil {
				e.leasePort.Release(b.ID) // 阶段 1a：跨副本行租约随本地释放
			}
		}()
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
