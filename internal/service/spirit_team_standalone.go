// Code owned by package service; extracted from spirit_team.go for AS-COG-01 file-size fitness.

package service

import (
	"context"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/pkg/loggateway"
)

func (s *TeamStarter) publishTerminalTeamStage(
	ctx context.Context,
	team biz.Team,
	spiritSessionID string,
	primaryStatus biz.TeamStageStatus,
) {
	terminalTsID := string(agent.NewTeamStageActivityID(team.ID, string(agent.RootTaskActivityIDFromCtx(ctx))))
	var tsEvent biz.TeamStageEvent
	switch primaryStatus {
	case biz.TeamStageStatusCompleted:
		tsEvent = biz.TeamStageEventComplete
	case biz.TeamStageStatusPartialFailure:
		tsEvent = biz.TeamStageEventCompletePartial
	case biz.TeamStageStatusFailed:
		tsEvent = biz.TeamStageEventFail
	case biz.TeamStageStatusCancelled:
		tsEvent = biz.TeamStageEventCancel
	default:
		tsEvent = biz.TeamStageEventFail
	}
	newStatus, newVersion, tsOK := resolveTeamStageUpdate(ctx, s.teamStageR, s.tsSM,
		terminalTsID, tsEvent, primaryStatus, s.lg)
	if !tsOK {
		return
	}
	// Stage 是阶段枚举（assembled/planning/executing/completed/failed），
	// partial_failure 是 Status 维度而非阶段 → 归一到 completed 阶段。
	stage := biz.TeamStageStage(primaryStatus)
	if primaryStatus == biz.TeamStageStatusPartialFailure {
		stage = biz.TeamStageStageCompleted
	}
	ts := biz.TeamStage{
		ID:        terminalTsID,
		TeamID:    team.ID,
		TeamName:  team.DisplayName,
		SessionID: spiritSessionID,
		Status:    newStatus,
		Stage:     stage,
		DependsOn: team.DependsOn,
		StartedAt: time.Now().UTC(),
		Version:   newVersion,
	}
	// DATA LOSS: v2 TeamStage has no Meta field, so team_name/duration_ms/
	// total_token_in/total_token_out/progress_pct/error from v1 Meta are
	// dropped. The duration/token metrics are still recorded via
	// recordTeamCompletion and are available in the TeamRun DB record.
	switch primaryStatus {
	case biz.TeamStageStatusCompleted, biz.TeamStageStatusPartialFailure:
		// partial_failure 复用 CompletedEvent 工厂：事件类型仅影响路由/日志，
		// 持久化以 ts.Status（partial_failure）为准。
		s.publishV2Event(ctx, biz.NewTeamStageCompletedEvent(ts))
	case biz.TeamStageStatusFailed:
		s.publishV2Event(ctx, biz.NewTeamStageFailedEvent(ts))
	default:
		// cancelled: no NewTeamStageCancelledEvent factory exists; use
		// NewTeamStageUpdatedEvent as the closest semantic match.
		s.publishV2Event(ctx, biz.NewTeamStageUpdatedEvent(ts))
	}
}

// resolveStandaloneSpiritSessionID 回退解析 standalone（Mode A）团队的聚合
// 根：team session ID（SessionType="team" 的第一条会话）。standalone 团队
// 无 ParentSessionID/team.SpiritSessionID，聚合根即自己的 team session
// （F-3 语义，与 runner deriveSpiritSessionID 回退 sess.ID 一致）。
// 无 team session（团队从未运行）或查询失败时返回空——调用方守卫安全跳过。
func (s *TeamStarter) resolveStandaloneSpiritSessionID(ctx context.Context, teamID string) string {
	if s.sessions == nil {
		return ""
	}
	result, err := s.sessions.Search(ctx, biz.SessionSearchQuery{TeamID: teamID, Limit: 100})
	if err != nil {
		s.lg.Warn("resolveStandaloneSpiritSessionID: 查询团队会话失败，保持空聚合根",
			loggateway.StepID("spirit.standalone.resolve_spirit_err"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(err),
		)
		return ""
	}
	for _, sess := range result.Items {
		if sess.SessionType == "team" {
			return sess.ID
		}
	}
	return ""
}

// handleStandaloneTeamTurnResult 是 standalone（Mode A，非 AutoCreated）团队的
// 终态 outcome pass（2026-07-29 F-1）。此前 HandleTeamTurnResult 对非
// AutoCreated 团队直接早退，且调用点以 ParentSessionID 为空守卫——standalone
// 团队的 teams 状态、TeamStage、TeamRun、MemberSession 全部永不达终态
// （成员永 running）。
//
// 与 AutoCreated 路径的差异（spirit 编排专属职责不适用）：
//   - 无 deliverable 门 / F9 验证门：standalone 团队 DagNodeID==""，非 DAG
//     团队无交付物义务（与 resolveMemberOutcomeStatus 的单成员交付物检查
//     同一姿态）；
//   - 无 recordTeamCompletion：DQ 分/拓扑推断/进化建议是编排域职责；
//   - 无 scheduleDependentTeams / checkAllTeamsCompleted / synthesis 注入：
//     standalone 无 DAG 依赖与编排上下文；
//   - 无 PlanExecutor.NotifyTeamCompletion：standalone 团队不由 PlanExecutor
//     派发，无等待者；
//   - 无 AutoArchiveCompletedTeams：standalone 团队是用户创建的持久实体，
//     不归 spirit 编排生命周期归档。
//
// 成员终态裁决（F10 证据链 + F4 兜底）由 publishV2TeamRunCompletion 复用，
// 与 AutoCreated 路径完全一致——成员终态单写者原则对两种模式同构。
func (s *TeamStarter) handleStandaloneTeamTurnResult(
	ctx context.Context,
	spiritSessionID string,
	team biz.Team,
	status string,
) {
	teamID := team.ID
	s.team.SpiritUC.CancelTimeoutTimer(teamID)

	// F10 部分失败收敛 + P1-3 quorum（与 AutoCreated 路径同一语义）：
	// completed 且 ≥1 成员有执行失败证据 → partial_failure；全体成员均带
	// 失败证据（全灭）→ failed。
	if status == biz.TeamStatusCompleted {
		if failedN, totalN := s.countFailedEvidenceMembers(ctx, teamID); failedN > 0 {
			if totalN > 0 && failedN >= totalN {
				status = biz.TeamStatusFailed
				s.lg.Info("standalone 团队全体成员有失败证据，收敛为 failed（quorum）",
					loggateway.StepID("spirit.standalone.quorum_failed"),
					loggateway.Str("team_id", teamID),
					loggateway.Int("failed_members", failedN),
					loggateway.Int("total_members", totalN),
				)
			} else {
				status = biz.TeamStatusPartialFailure
				s.lg.Info("standalone 团队完成但存在失败成员，收敛为 partial_failure",
					loggateway.StepID("spirit.standalone.partial_failure"),
					loggateway.Str("team_id", teamID),
					loggateway.Int("failed_members", failedN),
					loggateway.Int("total_members", totalN),
				)
			}
		}
	}

	// teams 表状态转换。completed/failed 直接转；cancelled 破除「调用方已
	// 转换」假设（D1-F2，与 AutoCreated 路径同一姿态）：runner 取消回调等
	// 入口进入本 pass 时现态可能仍是 running，需补 CAS 转移（FSM 已有
	// pending/running→cancelled 边）；已 cancelled 的重放（biz.CancelTeam
	// 显式清理）跳过——BadRequest/Conflict 拒绝容忍 Warn（幂等/竞态）。
	switch status {
	case biz.TeamStatusCompleted, biz.TeamStatusPartialFailure:
		if _, updateErr := s.team.TeamUC.TransitionStatus(ctx, teamID, status); updateErr != nil {
			s.lg.Warn("standalone 团队状态转换到 completed/partial_failure 失败",
				loggateway.StepID("spirit.standalone.completed_err"),
				loggateway.Str("team_id", teamID),
				loggateway.Str("target_status", status),
				loggateway.Err(updateErr),
			)
		}
	case biz.TeamStatusFailed:
		if _, updateErr := s.team.TeamUC.TransitionStatus(ctx, teamID, biz.TeamStatusFailed); updateErr != nil {
			s.lg.Warn("standalone 团队状态转换到 failed 失败",
				loggateway.StepID("spirit.standalone.failed_err"),
				loggateway.Str("team_id", teamID),
				loggateway.Err(updateErr),
			)
		}
	case biz.TeamStatusCancelled:
		if team.Status != biz.TeamStatusCancelled {
			if _, updateErr := s.team.TeamUC.TransitionStatus(ctx, teamID, biz.TeamStatusCancelled); updateErr != nil {
				s.lg.Warn("standalone 团队状态转换到 cancelled 失败（幂等容忍）",
					loggateway.StepID("spirit.standalone.cancelled_err"),
					loggateway.Str("team_id", teamID),
					loggateway.Err(updateErr),
				)
			}
		}
	}

	primaryStatus := biz.TeamStageStatusFailed
	switch status {
	case biz.TeamStatusCompleted:
		primaryStatus = biz.TeamStageStatusCompleted
	case biz.TeamStatusPartialFailure:
		primaryStatus = biz.TeamStageStatusPartialFailure
	case biz.TeamStatusCancelled:
		primaryStatus = biz.TeamStageStatusCancelled
	}

	// 2026-07-29 S-2：cancelled 时把仍在 running 的成员 session 转换为
	// interrupted（DB session 层面），与 MemberSession v2 skipped 对齐——
	// 与 AutoCreated 路径（HandleTeamTurnResult cancelled 分支）同一姿态。
	if status == biz.TeamStatusCancelled && s.sessions != nil {
		result, searchErr := s.sessions.Search(ctx, biz.SessionSearchQuery{TeamID: teamID, Limit: biz.SpiritCancelSessionLimit})
		if searchErr == nil {
			for _, sess := range result.Items {
				if sess.Status == string(sessstatus.SessionStatusRunning) {
					if transErr := s.sessions.TransitionStatus(ctx, sess.ID, sessstatus.SessionStatusInterrupted, "user_cancelled"); transErr != nil {
						s.lg.Warn("standalone 取消团队 Session 状态转换失败",
							loggateway.StepID("spirit.standalone.cancel_session_transition_err"),
							loggateway.Str("session_id", sess.ID),
							loggateway.Err(transErr),
						)
					}
				}
			}
		}
	}

	// TeamStage 终态事件（与 AutoCreated 路径同一机制）。空聚合根守卫与
	// publishV2TeamRunCompletion 对齐：standalone 团队从未运行（无 team
	// session，回退为空）时不创建 SessionID="" 的孤立 TeamStage 记录。
	if s.v2EventReady() && spiritSessionID != "" {
		s.publishTerminalTeamStage(ctx, team, spiritSessionID, primaryStatus)
	}

	// TeamRun + MemberSession 终态 outcome pass（F10 证据 + F4 兜底）。
	s.publishV2TeamRunCompletion(ctx, spiritSessionID, teamID, primaryStatus, status)
}
