// Code owned by package service; extracted from spirit_team.go for AS-COG-01 file-size fitness.

package service

import (
	"context"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

func (s *TeamStarter) recordTeamCompletion(ctx context.Context, team biz.Team, durationMs int64) {
	s.team.SpiritUC.RecordTeamCompletion(ctx, team, durationMs)
}

func (s *TeamStarter) scheduleDependentTeams(ctx context.Context, spiritSessionID string, completedTeam biz.Team) {
	// PlanExecutor is the forward DAG owner (lazy AssembleTeam per step).
	// Reverse-sync ScheduleDependentTeams must not activate/fail teams while
	// a dagRun is in flight — that double-starts downstream teams (reuse of
	// pending records + StartTeamTurn) and races PlanExecutor.checkDownstream.
	if s.planExecutor != nil && s.planExecutor.HasActiveRunForSession(spiritSessionID) {
		s.lg.Info("dagRun 活跃中，跳过 reverse-sync 依赖调度（由 PlanExecutor 负责）",
			loggateway.StepID("spirit.schedule_deps.skip_dag_run"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Str("completed_team_id", completedTeam.ID),
		)
		return
	}
	actions := s.team.SpiritUC.ScheduleDependentTeams(ctx, spiritSessionID, completedTeam)
	for _, action := range actions {
		if action.Action == "fail" {
			_, uerr := s.team.TeamUC.TransitionStatus(ctx, action.TeamID, biz.TeamStatusFailed)
			if uerr != nil {
				s.lg.Warn("更新团队状态为 failed 失败，依赖调度中断",
					loggateway.StepID("spirit.schedule_deps.fail_err"),
					loggateway.Str("team_id", action.TeamID),
					loggateway.Err(uerr),
				)
			} else {
				s.lg.Info("依赖调度：团队前置依赖失败，标记团队为 failed",
					loggateway.StepID("spirit.schedule_deps.dep_failed"),
					loggateway.Str("team_id", action.TeamID),
					loggateway.Str("dag_node_id", action.DagNodeID),
				)
				if s.v2EventReady() {
					// Phase 3b-D Task 10: migrated to v2 NewTeamStageFailedEvent.
					// 2026-07-04 问题 3 修复：携带 TeamName（来自 DependentTeamAction）。
					// 2026-07-04 问题 2 修复：改用 publishV2Event（seq 优先持久化）。
					// 2026-07-05 P1 #9d-2（AS-FSM-01）：用状态机校验 Running → Failed
					// 转换，修复 Version=0 Bug（状态未持久化）。
					failTsID := string(agent.NewTeamStageActivityID(action.TeamID, string(agent.RootTaskActivityIDFromCtx(ctx))))
					newStatus, newVersion, tsOK := resolveTeamStageUpdate(ctx, s.teamStageR, s.tsSM,
						failTsID, biz.TeamStageEventFail, biz.TeamStageStatusFailed, s.lg)
					if tsOK {
						ts := biz.TeamStage{
							ID:        failTsID,
							TeamID:    action.TeamID,
							TeamName:  action.TeamName,
							SessionID: spiritSessionID,
							Status:    newStatus,
							Stage:     biz.TeamStageStageFailed,
							StartedAt: time.Now().UTC(),
							Version:   newVersion,
						}
						s.publishV2Event(ctx, biz.NewTeamStageFailedEvent(ts))
					}
				}
			}
		} else if action.Action == "activate" {
			_, uerr := s.team.TeamUC.TransitionStatus(ctx, action.TeamID, biz.TeamStatusRunning)
			if uerr != nil {
				s.lg.Warn("更新团队状态失败，依赖调度中断",
					loggateway.StepID("spirit.schedule_deps.update_err"),
					loggateway.Str("team_id", action.TeamID),
					loggateway.Err(uerr),
				)
				continue
			}
			s.lg.Info("依赖调度：团队所有前置依赖已完成，激活团队",
				loggateway.StepID("spirit.schedule_deps.activated"),
				loggateway.Str("team_id", action.TeamID),
				loggateway.Str("dag_node_id", action.DagNodeID),
			)
			if s.v2EventReady() {
				// Phase 3b-D Task 10: migrated to v2 NewTeamStageUpdatedEvent.
				// 2026-07-04 问题 3 修复：携带 TeamName；Stage 从字面量 "progress"
				// （不在枚举内）改为枚举常量 TeamStageStageExecuting。
				// 2026-07-04 问题 2 修复：改用 publishV2Event（seq 优先持久化）。
				// 2026-07-05 P1 #9d-2（AS-FSM-01）：用状态机校验 Pending → Running
				// 转换，修复 Version=0 Bug（状态未持久化）。
				activateTsID := string(agent.NewTeamStageActivityID(action.TeamID, string(agent.RootTaskActivityIDFromCtx(ctx))))
				newStatus, newVersion, tsOK := resolveTeamStageUpdate(ctx, s.teamStageR, s.tsSM,
					activateTsID, biz.TeamStageEventStart, biz.TeamStageStatusRunning, s.lg)
				if tsOK {
					ts := biz.TeamStage{
						ID:        activateTsID,
						TeamID:    action.TeamID,
						TeamName:  action.TeamName,
						SessionID: spiritSessionID,
						Status:    newStatus,
						Stage:     biz.TeamStageStageExecuting,
						StartedAt: time.Now().UTC(),
						Version:   newVersion,
					}
					s.publishV2Event(ctx, biz.NewTeamStageUpdatedEvent(ts))
				}
				// P0-2：tsOK=false（状态机拒绝，当前 TeamStage 已终态）时跳过发布，
				// 不覆盖终态；依赖团队仍会被激活（teams 表状态已转换）。
			}
			taskDesc := action.TaskDescription // P0-③b: deliverable prefix + task description
			if taskDesc == "" {
				taskDesc = action.TeamName // fallback when biz did not supply a description
			}
			depTeamID := action.TeamID
			safego.Go(ctx, "spirit-schedule-deps", func() {
				startCtx := context.WithoutCancel(ctx)
				s.findAndStartTeamTurn(startCtx, depTeamID, taskDesc)
			})
		}
	}
	// Check all teams completed once after processing all actions (instead of
	// per-action) to avoid redundant DB queries.
	if len(actions) > 0 {
		s.checkAllTeamsCompleted(ctx, spiritSessionID)
	}
}

func (s *TeamStarter) findAndStartTeamTurn(ctx context.Context, teamID, taskDesc string) {
	result, err := s.sessions.Search(ctx, sessstatus.SessionSearchQuery{TeamID: teamID, Limit: 1})
	if err != nil || len(result.Items) == 0 {
		s.lg.Warn("查找团队 session 失败",
			loggateway.StepID("spirit.find_session_fail"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(err),
		)
		return
	}
	if startErr := s.StartTeamTurn(ctx, result.Items[0].ID, taskDesc); startErr != nil {
		s.lg.Warn("自动启动团队 Turn 失败",
			loggateway.StepID("spirit.auto_start_fail"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(startErr),
		)
	}
}

func (s *TeamStarter) checkAllTeamsCompleted(ctx context.Context, spiritSessionID string) {
	result := s.team.SpiritUC.CheckAllTeamsCompleted(ctx, spiritSessionID)
	if !result.AllDone {
		return
	}

	// System-push pattern: when all teams complete, synthesize results and
	// inject a message into the Spirit session to trigger the LLM to continue.
	// This replaces the LLM-polling pattern (check_progress tool) with a
	// system-initiated continuation, eliminating unnecessary tool calls and
	// token consumption.
	//
	// CAS guard: checkAllTeamsCompleted is called from both
	// HandleTeamTurnResult (goroutine) and NotifyAllTeamsCompleted (background
	// poller goroutine). LoadOrStore keyed by spiritSessionID ensures only one
	// synthesis message is injected per spirit session lifecycle, without
	// blocking concurrent spirit sessions (singleton CAS would poison across
	// sessions — first session's success would permanently disable synthesis
	// for all subsequent sessions).
	// B.10.17 断点 4 取消守卫：存在 cancelled 团队 = 用户主动中断，
	// 跳过综合与总结 turn（中断不出报告），且不发布"所有团队已完成"
	// 成功通知（中断后展示成功通知语义错误）。
	if result.CancelledTeams > 0 {
		s.lg.Info("存在已取消团队，跳过总结报告与总结 turn（用户主动中断）",
			loggateway.StepID("spirit.synthesis_skip_cancelled"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Int("cancelled_teams", result.CancelledTeams),
		)
		return
	}
	if s.turnGateway == nil {
		return
	}
	// 2026-07-27 总结重复触发修复（会话 d78029b9 总结 ×3 根因）：lazy 建团下
	// PlanExecutor 按 DAG 依赖逐 step AssembleTeam，首波团队全部终态时后续
	// step 尚无团队记录，CheckAllTeamsCompleted（只查 teams 表）把「波次中点」
	// 误判为「编排终点」；且 StartTeamTurn 每次组团都重置 CAS 守卫，导致每个
	// 波次都触发一次 synthesis。此处门控：本 session 存在活跃 dagRun 即跳过；
	// 最终总结由 dagRun 终态释放 lease 后经 NotifyAllTeamsCompleted 唯一触发。
	// 必须在 CAS guard（LoadOrStore）之前 —— 门控路径不得占用守卫名额。
	if s.planExecutor != nil && s.planExecutor.HasActiveRunForSession(spiritSessionID) {
		s.lg.Info("dagRun 活跃中（lazy 建团），跳过波次中点的 synthesis 触发",
			loggateway.StepID("spirit.synthesis_skip_dagrun_active"),
			loggateway.Str("spirit_session_id", spiritSessionID),
		)
		return
	}
	if _, alreadyTriggered := s.synthesisTriggered.LoadOrStore(spiritSessionID, true); alreadyTriggered {
		return
	}
	// 2026-08-07 重启幂等守卫：内存 CAS 进程重启即丢失，而 spirit session
	// 常驻 Running，30s 轮询会持续命中 AllDone —— 重启后首 tick 会对「早已
	// 完结」的编排重复触发 synthesis（用户看到总结报告重复回放）。此处以
	// 持久化状态兜底：父 Task 已 Completed ⟺ synthesis 已产出 → 跳过。
	// 置于 CAS 之后：跳过是永久事实，CAS 保持占用，后续 tick 由 CAS 吸收，
	// 不会每 30s 重复查库。
	if s.synthesisAlreadyProduced(ctx, spiritSessionID) {
		s.lg.Info("父 Task 已完结（synthesis 已产出），跳过重启后的重复综合触发",
			loggateway.StepID("spirit.synthesis_skip_already_done"),
			loggateway.Str("spirit_session_id", spiritSessionID),
		)
		return
	}

	// Resolve the most recent user-input Task for this spirit session so the
	// synthesis Turn attaches as a continuation Turn (ParentTaskID) instead of
	// creating a new Task. This prevents the synthesis trigger text from being
	// rendered as a user-input bubble by TaskCard (spec §3.2.1: a Task
	// corresponds to one user input; system-push Turns are continuation Turns
	// on the same Task).
	parentTaskID := s.resolveLatestUserTaskID(ctx, spiritSessionID)
	// 2026-07-25 Fix 3 诚实化：触发文本由真实完成/失败计数与失败简报动态
	// 构建 —— 存在失败团队时不得声称「所有团队已完成」，否则 LLM 会幻觉出
	// 成功报告（19:29 根因链末环）。
	var failures []biz.TeamFailureBrief
	if result.FailedTeams > 0 {
		failures = s.team.SpiritUC.ListFailedTeamBriefs(ctx, spiritSessionID)
	}
	// F7 (Phase 11)：内嵌各终态团队的交付物摘要，LLM 基于真实产出写报告，
	// 无需 read_session_history 考古。
	digests := s.team.SpiritUC.ListTeamDeliverableDigests(ctx, spiritSessionID)
	trigger := biz.BuildSynthesisSummaryTrigger(result.TotalTeams, result.CompletedTeams, result.FailedTeams, failures, digests)
	if _, err := s.turnGateway.ExecuteTurn(ctx, biz.TurnInput{
		SessionID:    spiritSessionID,
		Content:      trigger,
		ParentTaskID: parentTaskID,
		Synthesis:    true,
	}); err != nil {
		// 2026-07-27 修复3：ErrTurnMessageQueued = 成功受理，不是失败。
		// 触发文本已被 steer 注入活跃 turn 或进入排队队列；活跃 turn 结束后
		// processPendingQueue 保证排空执行（排队消息有保障）。CAS 保持占用 ——
		// 消息已受理，守卫继续阻止轮询重复注入。不得发兜底通知：总结必将
		// 产出，通知 = 双重信号。
		if isTurnMessageQueued(err) {
			// 交付物清单独立于总结报告存在（产物已落库），受理即发布，
			// 用户无需等总结 turn 排空。
			s.publishDeliverablesNotice(ctx, spiritSessionID, parentTaskID, digests)
			s.lg.Info("all teams completed: synthesis trigger accepted into active run (steer/queue)",
				loggateway.StepID("spirit.synthesis_turn_queued"),
				loggateway.Str("spirit_session_id", spiritSessionID),
				loggateway.Str("parent_task_id", parentTaskID),
			)
			return
		}
		// 真实失败（消息未受理）：释放 CAS 守卫，让 30s 轮询路径重试总结
		// turn —— 烧掉守卫 = 本轮总结永久丢失。兜底通知保留本轮 UX 终态信号。
		s.synthesisTriggered.Delete(spiritSessionID)
		s.lg.Warn("all teams completed: failed to trigger Spirit synthesis turn",
			loggateway.StepID("spirit.synthesis_turn_err"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(err),
		)
		s.publishAllTeamsCompletedFallbackNotice(ctx, spiritSessionID, parentTaskID, result.CompletedTeams, result.FailedTeams)
		return
	}
	s.lg.Info("all teams completed: triggered Spirit synthesis turn",
		loggateway.StepID("spirit.synthesis_turn_triggered"),
		loggateway.Str("spirit_session_id", spiritSessionID),
		loggateway.Str("parent_task_id", parentTaskID),
		loggateway.Int("total_teams", result.TotalTeams),
		loggateway.Int("completed_teams", result.CompletedTeams),
		loggateway.Int("failed_teams", result.FailedTeams),
	)
	// 总结 turn 完成（ExecuteTurn 阻塞至 turn 终态）：发布交付物清单通知，
	// 渲染在任务卡末尾（报告之后），供用户点击预览/下载产物。
	s.publishDeliverablesNotice(ctx, spiritSessionID, parentTaskID, digests)
}

// deliverablesNoticeType 是「全部团队完成」聚合产物清单的 orphan notice
// step 类型（2026-09-02 会话产物点击查看 P1）。前端 NoticeBlock 按此类型
// 把 Content JSON 渲染为可点击产物卡片，而非 markdown 文本。
