// Code owned by package service; extracted from plan_executor.go for AS-COG-01 file-size fitness.

package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

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
	// P0-6 阶段 1a（对抗评审 #12）：端口接线时**先认领跨副本租约再做任何
	// 共享态变更**——否则本副本改完 step/board 后 startBoardDAG 认领被拒
	//（另一副本持有）→ board 停 Executing 却无人跑（跨副本楔死）。
	// nil 端口恒 ok，字节级旧行为。认领失败映射为 BadRequest（与上方
	// running 拦截同一语义域：「当前不可 resume，稍后重试」语义）。
	runCtx, cancel, lease, ok := e.claimBoardRun(board)
	if !ok {
		return apierror.BadRequest(apierror.DomainOrchestrator, "plan board %s 租约已被其他副本持有（或 DB 暂不可用），拒绝 resume", boardID)
	}
	// resume 中途失败必须把已认领的租约放回（否则行租约心跳续着、
	// HasActive 恒 true，后续 resume/恢复扫描被永久拦截直到进程退出）。
	// startClaimedBoardDAG 接管所有权（goroutine 退出口径释放）。
	claimActive := true
	defer func() {
		if claimActive {
			if e.leasePort != nil {
				e.leasePort.Release(board.ID)
			}
			cancel()
		}
	}()
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
	// 租约已在前置认领阶段取得（对抗评审 #12），此处移交所有权。
	claimActive = false
	e.startClaimedBoardDAG(board, runCtx, cancel, lease)
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
