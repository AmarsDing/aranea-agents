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
			// D6（97-multi-replica-executor-waiters）验收堵口：ctx 死亡瞬间
			// 先终查决议（同进程 noted 图 / 决议表 / 持久确认卡，off 档同样
			// 生效的存量读径）——已有决议按决议定案并清 waiter，绝不落
			// abort 取消路径（跨副本批准被误杀为 cancelled 的链路在此截断；
			// 顺带修复 chan 已送达但 select 抢中 ctx.Done 的同进程误杀窗）。
			if decided, ok := r.pe.lookupPlaybookConfirmDecision(r.board.SessionID, step.ID); ok {
				r.pe.settlePlaybookConfirmWaiters(ctx, r.board.SessionID, step.ID, decided)
				approved = decided
			} else {
				r.abortPlaybookConfirm(ctx, step)
				return
			}
		}
		if !approved {
			r.skipPlaybookConfirmDenied(ctx, step)
			return
		}
	}
	now := time.Now()
	// 1. 预标记断言（发现#15，87-orchestration-chain-hardening Wave-2）：
	// Pending→Running 的唯一转移点是 admitReadySteps 的 r.mu 临界区，
	// Failed→Running 的唯一转移点是 maybeRetryStep 的 r.mu 临界区；本函数
	// 只消费预标记（断言方，不是转移方）。凡不是被调度临界区预标记为
	// Running 的条目一律打 Error 日志后 return——严禁调 failStep：旧实现
	// 在此 Transition，并发完成两个最后依赖时第二次 dispatch 的
	// Transition 失败会被升级成 PlanStepFailed 误杀链。
	r.mu.Lock()
	if step.Status != biz.PlanStepStatusRunning {
		status := step.Status
		r.mu.Unlock()
		r.pe.lg.Error("dispatchStep 拒绝未预标记的步骤（非 Running，跳过本次 dispatch）",
			loggateway.Str("plan_board_id", r.board.ID),
			loggateway.Str("step_id", step.ID),
			loggateway.Str("status", string(status)))
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
	r.pe.trackTeamAgentLoads(result.Team.ID, runningStep.AgentKeys)
	// D6：team 槽位快照落决议表（waiterDecisions 在籍才写；跨副本孤儿收养
	// 对账的消费面随收养链批次定稿，本批仅保证快照不丢）。D6-f：刻意用
	// context.Background()（同 signalStartup）——Orchestrate 已成功后 dag
	// ctx 死亡（本批验收堵口放行的可达窗口）不得带走快照：孤儿 team 恰好
	// 是该快照的标称消费场景。
	if r.pe.waiterDecisions != nil {
		if _, err := r.pe.waiterDecisions.TryRecordDecision(context.Background(), biz.ExecutorWaiterDecision{
			ExecutorWaiterDecisionKey: biz.ExecutorWaiterDecisionKey{
				Kind:      biz.WaiterDecisionKindTeamAgentLoads,
				SessionID: r.board.SessionID,
				ScopeID:   result.Team.ID,
			},
			Approved:  true,
			Reason:    strings.Join(runningStep.AgentKeys, ","),
			Actor:     "plan_executor.dispatch",
			DecidedAt: time.Now().UTC(),
		}); err != nil {
			r.pe.lg.Warn("record team agent loads snapshot failed",
				loggateway.Str("team_id", result.Team.ID), loggateway.Err(err))
		}
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
	// D6（97-multi-replica-executor-waiters）：teamRuns 在籍时叠加
	// team_runs_v2 终态轮询（与 playbook confirm 共用
	// executorWaiterPollInterval 拍）——完成通知落在别副本/进程内 chan
	// 丢失时按 DB 终态行推进。chan 与轮询同 select 同拍唤醒单推进
	//（handleCompletion 唯一调用点），并发重复推进由 UpsertPlanStep
	// VersionLT 乐观守卫兜底；off 档 teamRuns=nil → tick 为 nil chan 永不
	// 触发，单 chan select 零 DB 读，字节级旧行为。
	ch := result.CompletionChan
	var tick <-chan time.Time
	var ticker *time.Ticker
	if r.pe.teamRuns != nil {
		ticker = time.NewTicker(executorWaiterPollInterval)
		tick = ticker.C
		defer ticker.Stop()
	}
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				r.failStep(ctx, step, "orchestrator channel closed without event")
				return
			}
			r.handleCompletion(ctx, step, ev)
			return
		case <-tick:
			if ev, done := r.pollTeamRunTerminal(ctx, result.TeamStageID, step.ID); done {
				r.handleCompletion(ctx, step, ev)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// pollTeamRunTerminal 是 D6 跨副本 team 完成轮询的单拍实现：取
// teamStageID 下 Seq 最大的 run（重试/假启动对账会产生同 stage 多 run，
// 仅最新一条权威），终态才合成 TeamCompleteEvent——Completed/
// PartialFailure 等同成功（镜像 spirit_team_handle_result.go 的
// NotifyTeamCompletion 映射：partial_failure 交付物门已过，对下游等同
// 成功）；Cancelled 错误为空时补 "team_cancelled" 让 L4 分类器命中
// cancel 类（不消耗自动重试预算）。读失败/无行/非终态返回 ok=false
// 等下一拍。
func (r *dagRun) pollTeamRunTerminal(ctx context.Context, teamStageID, stepID string) (biz.TeamCompleteEvent, bool) {
	if r == nil || r.pe == nil || r.pe.teamRuns == nil || teamStageID == "" {
		return biz.TeamCompleteEvent{}, false
	}
	runs, err := r.pe.teamRuns.ListTeamRunsByStage(ctx, teamStageID)
	if err != nil {
		r.pe.lg.Warn("team completion poll failed (下一拍重试)",
			loggateway.Str("team_stage_id", teamStageID),
			loggateway.Str("step_id", stepID), loggateway.Err(err))
		return biz.TeamCompleteEvent{}, false
	}
	if len(runs) == 0 {
		return biz.TeamCompleteEvent{}, false
	}
	latest := runs[0]
	for _, run := range runs[1:] {
		if run.Seq > latest.Seq {
			latest = run
		}
	}
	if !biz.IsTeamRunV2Terminal(latest.Status) {
		return biz.TeamCompleteEvent{}, false
	}
	ev := biz.TeamCompleteEvent{StepID: stepID, TeamRunID: latest.ID}
	switch latest.Status {
	case biz.TeamRunV2StatusCompleted, biz.TeamRunV2StatusPartialFailure:
		ev.Success = true
		ev.ErrorMsg = latest.Error
	case biz.TeamRunV2StatusCancelled:
		ev.ErrorMsg = strings.TrimSpace(latest.Error)
		if ev.ErrorMsg == "" {
			ev.ErrorMsg = "team_cancelled"
		}
	default: // failed
		ev.ErrorMsg = strings.TrimSpace(latest.Error)
		if ev.ErrorMsg == "" {
			ev.ErrorMsg = "team run failed"
		}
	}
	r.pe.lg.Info("team completion 经跨副本轮询推进（chan 未先触发）",
		loggateway.Str("team_stage_id", teamStageID),
		loggateway.Str("team_run_id", latest.ID),
		loggateway.Str("step_id", stepID),
		loggateway.Str("status", string(latest.Status)))
	return ev, true
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
		// 发现#15/#7：终态转移后全局重扫准入（替代 checkDownstream 局部
		// 扫描）——锁内预标记吞掉并发完成的重复就绪判定，配额槽位释放后
		// 延滞步骤在此被准入。
		r.admitReadySteps(ctx)
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
	// D1-F3（87-orchestration-chain-hardening）：cancel 类单列——用户取消的
	// 团队自动重跑违背用户意图，不消耗 attempts 预算、不触发退避重跑，
	// 直接走末尾 cascadeSkip（下游标 skipped 可被 ResumePlanBoard 复活）。
	if retryable {
		switch class := biz.ClassifyStepFailure(msg); {
		case class == biz.StepFailureCancel:
			r.pe.lg.Info("step 失败分类为用户取消，不重试不重跑",
				loggateway.Str("plan_board_id", r.board.ID),
				loggateway.Str("step_id", step.ID),
				loggateway.Str("failure_class", string(class)),
				loggateway.Str("error", msg))
		case class != biz.StepFailureTransient:
			r.pe.lg.Info("step 失败分类为非瞬时故障，跳过自动重试",
				loggateway.Str("plan_board_id", r.board.ID),
				loggateway.Str("step_id", step.ID),
				loggateway.Str("failure_class", string(class)),
				loggateway.Str("error", msg))
		case r.maybeRetryStep(ctx, step):
			return
		}
	}
	r.cascadeSkip(ctx, step.ID)
	// 发现#7：失败终态释放的配额槽位可准入其他就绪 pending；cascade 子树
	// 内部已被全量 skipped，不会产生子树内准入。
	r.admitReadySteps(ctx)
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
//
// 发现#15（87-orchestration-chain-hardening Wave-2）：重试路径的 Running
// 预标记同样发生在本函数的 r.mu 临界区内（Failed→Running 是
// planStepTransitions 既有合法边，与 admitReadySteps 的 Pending→Running
// 互补）。预标记后 backoff 期间 step 处于内存 Running：cancel 由
// sweepNonTerminalSteps 的 Running→Skipped 合法边兜底；崩溃后 DB 仍为
// Failed，resume/retry 恢复语义不变（预标记纯内存）。
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
	// 锁内预标记 Failed→Running（发现#15）。转移失败仅打日志、不 wg.Add
	// （防御：不再触发 fail 链）——返回 false 让下游照常 cascade 收敛。
	if err := step.Transition(biz.PlanStepStatusRunning); err != nil {
		r.mu.Unlock()
		r.pe.lg.Error("maybeRetryStep: Failed→Running 预标记失败，放弃自动重试",
			loggateway.Str("plan_board_id", r.board.ID),
			loggateway.Str("step_id", step.ID),
			loggateway.Err(err))
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
