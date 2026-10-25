// Code owned by package service; extracted from spirit_team.go for AS-COG-01 file-size fitness.

package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/pkg/loggateway"
)

func (s *TeamStarter) HandleTeamTurnResult(ctx context.Context, spiritSessionID, teamID, status, errMsg string, chatSessionID string) {
	if s.team.TeamUC == nil {
		return
	}
	if rtID := string(agent.RootTaskActivityIDFromCtx(ctx)); rtID == "" {
		s.lg.Warn("HandleTeamTurnResult: RootTaskActivityID 为空，v2 TeamRun/MemberSession 将无法关联到根 Task",
			loggateway.StepID("spirit.handle_team_turn_result.empty_root_task"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Str("team_id", teamID),
			loggateway.Str("status", string(status)),
		)
	}
	team, err := s.team.TeamUC.Get(ctx, teamID)
	if err != nil {
		return
	}

	// 2026-07-21 P0-2 取消竞态修复：runner 生命周期终态守卫。
	// team 已达 completed/failed/cancelled 时，迟到的 runner 回调（如取消后
	// runner 继续执行至完成，见断链 1：WithoutCancel 启动的 runner 不可取消）
	// 直接丢弃，防止：(1) team_stages_v2/team_runs_v2 终态被 fallback 覆盖；
	// (2) recordTeamCompletion 重复计数；(3) scheduleDependentTeams 错误激活
	// 依赖团队；(4) checkAllTeamsCompleted 错误触发 synthesis。
	// 与当前状态一致的回调放行（如 CancelTeam 显式发起的 cancelled 清理）。
	// D1-F2（87-orchestration-chain-hardening）：partial_failure 入守卫集合
	// ——部分失败同为终态（交付物已部分落库），异态回调（如迟到 completed）
	// 不得覆盖；同态放行由下方 CAS 幂等门吸收。
	// 注意：不能用 biz.IsTeamTerminal（仅认 Archived）；这里守卫的是 runner
	// 生命周期终态集合。
	switch biz.TeamState(team.Status) {
	case biz.TeamStateCompleted, biz.TeamStateFailed, biz.TeamStateCancelled, biz.TeamStatePartialFailure:
		if team.Status != status {
			s.lg.Warn("HandleTeamTurnResult: team 已达终态，丢弃过期回调",
				loggateway.StepID("spirit.handle_team_turn_result.stale_callback"),
				loggateway.Str("team_id", teamID),
				loggateway.Str("current_status", team.Status),
				loggateway.Str("callback_status", status),
			)
			return
		}
	}

	// 2026-07-29 F-1：standalone（Mode A，非 AutoCreated）团队走精简终态
	// pass——此前此处直接 return，standalone 团队的 teams 状态、TeamStage、
	// TeamRun、MemberSession 全部永不达终态（成员永 running）。
	if !team.AutoCreated {
		// 2026-07-29 S-1：聚合根 service 内兜底回退。standalone 团队
		// team.SpiritSessionID==""（CreateTeam 不落该字段），CancelTeam 等无
		// session 上下文的入口只能传空 spiritSessionID；此处回退查 team
		// session 作聚合根（F-3 语义：与 runner deriveSpiritSessionID 回退
		// sess.ID 一致）。团队从未运行（无 team session）时保持空——
		// publishV2TeamRunCompletion 守卫安全跳过（无成员需终态）。
		if spiritSessionID == "" {
			spiritSessionID = s.resolveStandaloneSpiritSessionID(ctx, team.ID)
		}
		s.handleStandaloneTeamTurnResult(ctx, spiritSessionID, team, status)
		return
	}

	// 2026-07-25 Fix 1+2 真实产出闸门：DAG 团队回调 completed 但从未调用
	// set_deliverable（无 graph state 交付物）时，在状态转换前把 status 翻转
	// 为 failed —— 「只提问/只说话」不是产出，不允许无交付物的成功。翻转后
	// 全链路走既有 failed 路径：team 标 failed、级联调度下游（anyDepFailed
	// 级联失败）、发布 TeamStageFailedEvent、NotifyTeamCompletion(success=false)。
	// P1-4（2026-09-03 salvage）：区分「门翻转 failed」与「runner 执行失败」——
	// 前者（无交付物/验证门拒绝）是 fail-closed 语义，不得被 salvage 救回。
	gateFlipped := false
	if status == biz.TeamStatusCompleted && team.DagNodeID != "" {
		has, gateErr := s.team.SpiritUC.HasRealDeliverable(ctx, team)
		if gateErr != nil {
			s.lg.Warn("真实交付物校验失败（infra），按无交付物处理",
				loggateway.StepID("spirit.team.deliverable_gate"),
				loggateway.Str("team_id", teamID),
				loggateway.Err(gateErr),
			)
		}
		if gateErr != nil || !has {
			status = biz.TeamStatusFailed
			errMsg = "团队未通过 set_deliverable 提交真实交付物（completed 回调无产出，已翻转为 failed）"
			s.lg.Info("DAG 团队 completed 回调无真实交付物，翻转为 failed",
				loggateway.StepID("spirit.team.deliverable_gate"),
				loggateway.Str("team_id", teamID),
				loggateway.Str("dag_node_id", team.DagNodeID),
			)
		} else {
			// F9 验证门（2026-07-28 outcome pass 步骤②）：交付物门通过后执行
			// definition verification_gates（当前唯一自动来源：skill 安装
			// tool_assertion 门）。拒绝或 infra 错误均 fail-closed 翻转 failed——
			// 「装了但不可用」不得报成功。teamOutput 传空：tool_assertion 门
			// 不读团队产出文本（事实断言而非质量评判）。
			approved, reasons, vErr := s.team.SpiritUC.ExecuteVerificationGates(ctx, teamID, "")
			switch {
			case vErr != nil:
				status = biz.TeamStatusFailed
				gateFlipped = true
				errMsg = fmt.Sprintf("验证门执行失败（fail-closed）：%v", vErr)
				s.lg.Warn("F9 验证门执行错误，翻转为 failed",
					loggateway.StepID("spirit.team.verification_gate"),
					loggateway.Str("team_id", teamID),
					loggateway.Err(vErr),
				)
			case !approved:
				status = biz.TeamStatusFailed
				gateFlipped = true
				errMsg = fmt.Sprintf("验证门拒绝：%s", strings.Join(reasons, "；"))
				s.lg.Info("F9 验证门拒绝，翻转为 failed",
					loggateway.StepID("spirit.team.verification_gate"),
					loggateway.Str("team_id", teamID),
					loggateway.Str("reasons", strings.Join(reasons, "；")),
				)
			}
		}
	}

	// F10 部分失败收敛：completed 回调且交付物门已通过（DAG 团队）时，若 ≥1
	// 成员有执行失败证据（与 resolveMemberOutcomeStatus 同源），团队状态从
	// completed 收敛为 partial_failure。调度语义保持成功（下游照常激活、
	// 可归档、可重试），仅状态展示区分「部分失败」。
	// P1-3（2026-09-03 quorum 收敛）：全体成员均带失败证据 = 团队全灭——
	// 交付物不含任何成员实质产出（仅合成者独角戏），收敛为 failed 而非
	// partial_failure。临界成员（合成者）失败已在图级=团队失败，不进此分支。
	if status == biz.TeamStatusCompleted {
		if failedN, totalN := s.countFailedEvidenceMembers(ctx, teamID); failedN > 0 {
			if totalN > 0 && failedN >= totalN {
				status = biz.TeamStatusFailed
				// 全灭翻转与交付物/验证门同为 fail-closed 语义：置 gateFlipped
				// 防止下方 P1-4 salvage（交付物存在）将其救回 partial_failure。
				gateFlipped = true
				errMsg = fmt.Sprintf("全体成员（%d/%d）均有执行失败证据，交付物无实质成员产出，收敛为 failed", failedN, totalN)
				s.lg.Info("团队完成但全体成员有失败证据，收敛为 failed（quorum）",
					loggateway.StepID("spirit.team.quorum_failed"),
					loggateway.Str("team_id", teamID),
					loggateway.Int("failed_members", failedN),
					loggateway.Int("total_members", totalN),
				)
			} else {
				status = biz.TeamStatusPartialFailure
				s.lg.Info("团队完成但存在失败成员，收敛为 partial_failure",
					loggateway.StepID("spirit.team.partial_failure"),
					loggateway.Str("team_id", teamID),
					loggateway.Int("failed_members", failedN),
					loggateway.Int("total_members", totalN),
				)
			}
		}
	}

	// P1-4（2026-09-03 salvage，LangGraph error_handler 模式的 team 级镜像）：
	// runner 报 failed（图执行失败，如 finish 合成节点重试耗尽）但图 state 中
	// 已存在成员真实交付物时，收敛为 partial_failure——「有产出不浪费」，任务
	// 导向。门翻转 failed（gateFlipped：无交付物/验证门拒绝）是 fail-closed
	// 语义，明确排除在 salvage 之外。失败信息保留在 errMsg 供下游展示。
	if status == biz.TeamStatusFailed && !gateFlipped && team.DagNodeID != "" {
		if has, salvErr := s.team.SpiritUC.HasRealDeliverable(ctx, team); salvErr == nil && has {
			s.lg.Info("团队执行失败但存在成员真实交付物，salvage 为 partial_failure",
				loggateway.StepID("spirit.team.salvage"),
				loggateway.Str("team_id", teamID),
				loggateway.Str("original_error", errMsg),
			)
			status = biz.TeamStatusPartialFailure
			errMsg = "团队未完整执行（合成/收尾环节失败），交付物为成员原始产出：" + errMsg
		}
	}

	// Cancel timeout timer for any terminal status to prevent stale callbacks.
	s.team.SpiritUC.CancelTimeoutTimer(teamID)

	durationMs := int64(0)
	teamRunID := ""
	runs, runErr := s.team.TeamUC.ListRuns(ctx, teamID, 1)
	if runErr == nil && len(runs) > 0 {
		durationMs = int64(runs[0].DurationMS)
		teamRunID = runs[0].ID
	}

	// Phase 3b-D Task 10: token usage extraction (tokenIn/tokenOut) was removed.
	// The v1 Activity.Meta carried total_token_in/total_token_out, but v2 TeamStage
	// has no Meta field. The token data is still available in the session record
	// (InputTokens/OutputTokens) and via recordTeamCompletion. Removing the
	// session Search call also eliminates a DB query per team turn completion.

	// D1-F2（87-orchestration-chain-hardening）：teams 表 CAS 转移是本轮记账的
	// 幂等键——同态重放（runner 取消后跑完补回调、超时处理器与 runner 回调
	// 竞态）现态已终态，TransitionStatus 被 FSM（同态非法）或 CAS 拒绝时，
	// recordTeamCompletion/scheduleDependentTeams 必须被门控跳过，至多生效
	// 一次；拒绝仅记 Warn，不阻断后续 TeamStage/TeamRun 终态事件。
	if status == biz.TeamStatusCompleted || status == biz.TeamStatusPartialFailure {
		transitioned := true
		if _, updateErr := s.team.TeamUC.TransitionStatus(ctx, teamID, status); updateErr != nil {
			transitioned = false
			s.lg.Warn("更新团队状态为 completed/partial_failure 失败（CAS 幂等门，跳过记账型副作用）",
				loggateway.StepID("spirit.team.completed_err"),
				loggateway.Str("team_id", teamID),
				loggateway.Str("target_status", status),
				loggateway.Err(updateErr),
			)
		}
		if transitioned {
			s.recordTeamCompletion(ctx, team, durationMs)
			s.scheduleDependentTeams(ctx, spiritSessionID, team)
		}
	} else if status == biz.TeamStatusCancelled {
		// D1-F2：破除「调用方已转换」假设——不同来源（CancelTeam 显式清理/
		// runner 取消回调/门控翻转/幂等重放）都可能以非 cancelled 现态进入
		// 本分支。现态非 cancelled 时尝试 CAS 转移（FSM 已有 pending/running→
		// cancelled 边）；BadRequest/Conflict 容忍 Warn（幂等重放/竞态取消
		// 已转换）——无论是否本侧完成转换，成员 session interrupted
		// ("user_cancelled") 与依赖调度照常执行。
		if team.Status != biz.TeamStatusCancelled {
			if _, updateErr := s.team.TeamUC.TransitionStatus(ctx, teamID, biz.TeamStatusCancelled); updateErr != nil {
				s.lg.Warn("更新团队状态为 cancelled 失败（幂等容忍，继续成员终态收敛）",
					loggateway.StepID("spirit.team.cancelled_err"),
					loggateway.Str("team_id", teamID),
					loggateway.Err(updateErr),
				)
			}
		}
		s.scheduleDependentTeams(ctx, spiritSessionID, team)
		result, searchErr := s.sessions.Search(ctx, biz.SessionSearchQuery{TeamID: teamID, Limit: biz.SpiritCancelSessionLimit})
		if searchErr == nil {
			for _, sess := range result.Items {
				if sess.Status == string(sessstatus.SessionStatusRunning) {
					if transErr := s.sessions.TransitionStatus(ctx, sess.ID, sessstatus.SessionStatusInterrupted, "user_cancelled"); transErr != nil {
						s.lg.Warn("取消团队 Session 状态转换失败",
							loggateway.StepID("spirit.cancel_session_transition_err"),
							loggateway.Str("session_id", sess.ID),
							loggateway.Err(transErr),
						)
					}
				}
			}
		}
	} else {
		// D1-F2：failed 同态重放（超时处理器与 runner 失败回调竞态）——
		// scheduleDependentTeams 同样挂在 CAS 幂等门后，至多生效一次。
		transitioned := true
		if _, updateErr := s.team.TeamUC.TransitionStatus(ctx, teamID, biz.TeamStatusFailed); updateErr != nil {
			transitioned = false
			s.lg.Warn("更新团队状态为 failed 失败（CAS 幂等门，跳过依赖级联）",
				loggateway.StepID("spirit.team.failed_err"),
				loggateway.Str("team_id", teamID),
				loggateway.Err(updateErr),
			)
		}
		if transitioned {
			// Schedule dependent teams so they can detect the failure and cascade cancel.
			s.scheduleDependentTeams(ctx, spiritSessionID, team)
		}
	}

	// 2026-07-04 问题 4 修复：将 primaryStatus 提取到 eventBus 块外，
	// 让 publishV2TeamRunCompletion 也能使用（该函数仅依赖 seq，不依赖 eventBus）。
	primaryStatus := biz.TeamStageStatusFailed
	switch status {
	case biz.TeamStatusCompleted:
		primaryStatus = biz.TeamStageStatusCompleted
	case biz.TeamStatusPartialFailure:
		primaryStatus = biz.TeamStageStatusPartialFailure
	case biz.TeamStatusCancelled:
		primaryStatus = biz.TeamStageStatusCancelled
	default:
		primaryStatus = biz.TeamStageStatusFailed
	}

	if s.v2EventReady() {
		// Phase 3b-D Task 10: migrated to v2 TeamStage events.
		// 2026-07-04 问题 2 修复：改用 publishV2Event（seq 优先持久化）。
		// 2026-07-29 F-1：终态 TeamStage 发布提取为 publishTerminalTeamStage，
		// 与 standalone 终态 pass 共用（状态机校验 + 版本解析，含 P0-3/P0-4
		// Version=0 修复与 P0-2 终态守卫）。
		s.publishTerminalTeamStage(ctx, team, spiritSessionID, primaryStatus)
		// Carry DependsOn from the team so the frontend can render DAG
		// edges and the database preserves the dependency graph across
		// status updates (assembled → progress → completed/failed/cancelled).
		dependsOn := team.DependsOn

		// BUGFIX: the progress event (Status=Running) was published AFTER the
		// primary terminal event, causing the version-guarded UpsertActivity to
		// overwrite the terminal status (completed/failed/cancelled) with
		// "Running". Skip the progress event for terminal statuses — the primary
		// event already carries the correct status, and the frontend dedup logic
		// merges meta from all events of the same team.
		// 2026-07-05 P1 #9d-2：修复 Version=0 Bug。progress 事件是状态刷新
		// （不是转换），用 TeamStageEventStart：from=Running 时状态机拒绝并降级
		// 为 Version=100，from=Pending 时正常转换 Pending → Running。
		if status == biz.TeamStatusRunning {
			progressTsID := string(agent.NewTeamStageActivityID(teamID, string(agent.RootTaskActivityIDFromCtx(ctx))))
			newStatus, newVersion, tsOK := resolveTeamStageUpdate(ctx, s.teamStageR, s.tsSM,
				progressTsID, biz.TeamStageEventStart, biz.TeamStageStatusRunning, s.lg)
			if tsOK {
				progressTs := biz.TeamStage{
					ID:        progressTsID,
					TeamID:    teamID,
					TeamName:  team.DisplayName,
					SessionID: spiritSessionID,
					Status:    newStatus,
					Stage:     biz.TeamStageStageExecuting,
					DependsOn: dependsOn,
					StartedAt: time.Now().UTC(),
					Version:   newVersion,
				}
				s.publishV2Event(ctx, biz.NewTeamStageUpdatedEvent(progressTs))
			}
		}
		// GraphStage / GraphNode 状态由 PlanExecutor 经 v2 事件维护（§3.7.5）。
		// 不再发布 v1 team-based graph_stage Activity 快照。

	}

	// 2026-07-04 问题 4 修复：发布 v2 TeamRun + MemberSession 完成事件，
	// 让前端 MemberSessionPanel 能更新成员会话状态（running → completed/failed）。
	// 移出 eventBus nil-check 块：此函数仅依赖 seq（v2 Sequencer），与 eventBus 无关。
	// 之前放在 eventBus 块内导致 eventBus=nil 但 seq!=nil 的部署场景下成员状态永远停在 running。
	s.publishV2TeamRunCompletion(ctx, spiritSessionID, team.ID, primaryStatus, status)

	s.checkAllTeamsCompleted(ctx, spiritSessionID)

	// 2026-07-04 问题 4 修复：通知 PlanExecutor team_run 已完成，
	// 让等待的 dispatchStep 能继续执行（通过 TeamOrchestrator 转发 channel 事件）。
	// 必须在 checkAllTeamsCompleted 之后调用，确保 synthesis 逻辑先执行。
	if s.planExecutor != nil && teamID != "" {
		// partial_failure 交付物门已通过，对下游 PlanExecutor 等同成功。
		success := status == biz.TeamStatusCompleted || status == biz.TeamStatusPartialFailure
		s.planExecutor.NotifyTeamCompletion(teamID, teamRunID, success, errMsg)
	}

	// Trigger auto-archive for completed/failed/cancelled teams that have
	// exceeded the configured threshold. This is the primary call site for
	// AutoArchiveCompletedTeams — it runs on every team lifecycle event so
	// no separate worker/cron is needed.
	s.team.SpiritUC.AutoArchiveCompletedTeams(ctx, spiritSessionID)
}

// publishTerminalTeamStage 发布 TeamStage 终态事件（completed/failed/cancelled）。
// 状态机校验 + 版本解析（resolveTeamStageUpdate，含 2026-07-21 P0-3/P0-4
// Version=0 修复）；tsOK=false（当前 TeamStage 已终态，过期回调）时跳过发布，
// 不覆盖终态。AutoCreated 与 standalone（F-1）终态 pass 共用。
