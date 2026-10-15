// Code owned by package service; extracted from plan_executor.go for AS-COG-01 file-size fitness.

package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

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
