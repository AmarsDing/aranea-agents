// Code owned by package service; extracted from spirit_team.go for AS-COG-01 file-size fitness.

package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func resolveMemberOutcomeStatus(
	ctx context.Context,
	ctrl biz.SpiritTeamController,
	team biz.Team,
	base biz.MemberSessionStatus,
	memberSessionID string,
	singleMember bool,
) (biz.MemberSessionStatus, string) {
	if base != biz.MemberSessionStatusCompleted || ctrl == nil {
		return base, ""
	}
	if failed, reason := ctrl.MemberExecutionEvidence(ctx, memberSessionID); failed {
		return biz.MemberSessionStatusFailed, reason
	}
	if singleMember && team.DagNodeID != "" {
		has, err := ctrl.HasRealDeliverable(ctx, team)
		if err != nil {
			return biz.MemberSessionStatusFailed, "交付物校验失败（infra），无法证明成员产出: " + err.Error()
		}
		if !has {
			return biz.MemberSessionStatusFailed, "成员未通过 set_deliverable 提交真实交付物（团队 completed 回调无产出）"
		}
	}
	return base, ""
}

// countFailedEvidenceMembers 统计团队下有执行失败证据（F10
// MemberExecutionEvidence）的成员数与成员总数，用于 partial_failure 收敛与
// P1-3 quorum 判定（全灭→failed）。与 publishV2TeamRunCompletion 的逐成员
// 覆盖同源同口径（SessionType=agent 且 MemberAgentKey 非空的成员会话）。
// 查询失败按 (0,0) 处理（fail-open：不阻断 completed 收敛，quorum 规则
// failedN>0 才生效，(0,0) 天然放行，与 F10 证据覆盖的降级姿态一致）。
func (s *TeamStarter) countFailedEvidenceMembers(ctx context.Context, teamID string) (failed int, total int) {
	if s.team.SpiritUC == nil || s.sessions == nil || teamID == "" {
		return 0, 0
	}
	result, err := s.sessions.Search(ctx, biz.SessionSearchQuery{TeamID: teamID, Limit: 100})
	if err != nil {
		s.lg.Warn("countFailedEvidenceMembers: 查询团队成员会话失败，按 0 处理",
			loggateway.StepID("spirit.team.partial_failure.probe_err"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(err),
		)
		return 0, 0
	}
	for _, sess := range result.Items {
		if sess.SessionType != "agent" || sess.MemberAgentKey == "" {
			continue
		}
		total++
		if failedEv, _ := s.team.SpiritUC.MemberExecutionEvidence(ctx, sess.ID); failedEv {
			failed++
		}
	}
	return failed, total
}

// publishV2TeamRunCompletion 发布 v2 TeamRun 完成事件 + MemberSession 更新事件。
// 由 HandleTeamTurnResult 在团队状态变为 completed/failed/cancelled 时调用。
//
// 关键设计：
//   - 使用与 publishV2TeamRunAndMemberSessions 相同的确定性派生 ID 公式，确保
//     同一 TeamRun 的 created/completed 事件 ID 一致（v2 store 的 mapKey 替换而非新增）
//   - 查询团队成员会话（SessionType=agent, MemberAgentKey 非空）来发布每个成员的
//     MemberSessionUpdatedEvent
//   - 失败原因（errMsg）不在此处传入：HandleTeamTurnResult 已通过 v1 recordTeamCompletion
//     记录；v2 TeamRun.Error 字段由 RepoSet 从 v1 同步或后续补丁填充
//
// 参数：
//   - primaryStatus: v2 TeamStage 状态（Completed/Failed/Cancelled）
//   - originalStatus: v1 biz.TeamStatus 原始状态字符串
func (s *TeamStarter) publishV2TeamRunCompletion(
	ctx context.Context,
	spiritSessionID, teamID string,
	primaryStatus biz.TeamStageStatus,
	originalStatus string,
) {
	if s.seq == nil || spiritSessionID == "" || teamID == "" {
		return
	}
	// 映射 TeamStageStatus → TeamRunV2Status
	teamRunStatus := biz.TeamRunV2StatusFailed
	memberStatus := biz.MemberSessionStatusFailed
	switch primaryStatus {
	case biz.TeamStageStatusCompleted:
		teamRunStatus = biz.TeamRunV2StatusCompleted
		memberStatus = biz.MemberSessionStatusCompleted
	case biz.TeamStageStatusPartialFailure:
		// 成员 base 仍取 completed，由 resolveMemberOutcomeStatus 按 F10 证据
		// 逐成员覆盖为 failed——与 completed 路径同一机制。
		teamRunStatus = biz.TeamRunV2StatusPartialFailure
		memberStatus = biz.MemberSessionStatusCompleted
	case biz.TeamStageStatusCancelled:
		teamRunStatus = biz.TeamRunV2StatusCancelled
		memberStatus = biz.MemberSessionStatusSkipped
	case biz.TeamStageStatusFailed:
		teamRunStatus = biz.TeamRunV2StatusFailed
		memberStatus = biz.MemberSessionStatusFailed
	default:
		// Unknown status; skip publishing to avoid data corruption.
		return
	}

	rootTaskID := string(agent.RootTaskActivityIDFromCtx(ctx))
	teamStageID := string(agent.NewTeamStageActivityID(teamID, rootTaskID))
	teamRunID := agent.NewTeamRunV2ID(teamStageID)
	now := time.Now().UTC()

	// 获取 team 实体（用于 DependsOn 等字段）。
	team, teamErr := s.team.TeamUC.Get(ctx, teamID)
	if teamErr != nil {
		s.lg.Warn("publishV2TeamRunCompletion: 获取 team 失败，跳过 DependsOn 字段",
			loggateway.StepID("spirit.v2.team_run_completion.team_fetch_fail"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(teamErr),
		)
	}
	var dependsOn []string
	if team.ID != "" {
		dependsOn = append([]string(nil), team.DependsOn...)
	}

	// 发布 TeamRun 完成事件。
	// 2026-07-05 P1 #9e（AS-FSM-01）：用 TeamRunV2StateMachine 校验 Running → terminal
	// 转换，并修复 Version=2 硬编码 Bug（改为 current.Version+1）。
	// 2026-07-21 P0-2 取消竞态修复 3（TeamRun 终态权威）：拆分两类失败——
	// 读取失败（记录不存在/DB 错误）降级为原行为（Version=2），保证主流程不中断；
	// 状态机拒绝（当前 TeamRun 已达终态）说明本回调是迟到的过期回调（如取消后
	// runner 继续执行至完成），必须跳过整个发布（TeamRun + MemberSession），
	// 防止 team_runs_v2 终态被覆盖（cancelled → completed）。
	var trEvent biz.TeamRunV2Event
	switch teamRunStatus {
	case biz.TeamRunV2StatusCompleted:
		trEvent = biz.TeamRunV2EventComplete
	case biz.TeamRunV2StatusPartialFailure:
		trEvent = biz.TeamRunV2EventCompletePartial
	case biz.TeamRunV2StatusFailed:
		trEvent = biz.TeamRunV2EventFail
	case biz.TeamRunV2StatusCancelled:
		trEvent = biz.TeamRunV2EventCancel
	default:
		return
	}
	newStatus := teamRunStatus
	newVersion := int64(2) // 降级默认值（与原硬编码一致）
	// 2026-09-04 终态覆盖修复：原实现把 SessionID 留空、StartedAt 取 now，
	// 注释声称"RepoSet 保持空避免覆盖"，但 UpsertTeamRun 无条件全字段 SET——
	// 早建行（组团时创建）的 session_id 被抹空、真实 started_at 被改成结束
	// 时刻（DB 实证：v2 行 session_id 全空、started_at 聚集于 dagRun 结束）。
	// 这里回读现有行（状态机校验本就需要），用其 SessionID/StartedAt 回填；
	// 行不存在（直接团队会话路径，v2 行由本事件首建）才维持空/now 兜底。
	currentTR, getErr := s.teamRunR.GetTeamRun(ctx, teamRunID)
	if getErr != nil {
		s.lg.Warn("publishV2TeamRunCompletion: failed to load current TeamRun, fallback to Version=2",
			loggateway.Str("team_run_id", teamRunID),
			loggateway.Err(getErr))
	} else {
		ns, smErr := s.trSM.Transition(currentTR.Status, trEvent)
		if smErr != nil {
			// 状态机拒绝：当前 TeamRun 已达终态。终态权威，跳过发布。
			s.lg.Warn("publishV2TeamRunCompletion: transition rejected by state machine, skip publish",
				loggateway.Str("team_run_id", teamRunID),
				loggateway.Str("from_status", string(currentTR.Status)),
				loggateway.Str("event", string(trEvent)),
				loggateway.Err(smErr))
			return
		}
		newStatus = ns
		newVersion = currentTR.Version + 1
	}
	tr := biz.TeamRun{
		ID:              teamRunID,
		TeamStageID:     teamStageID,
		TaskID:          rootTaskID,
		SessionID:       currentTR.SessionID, // 回填早建行值；行不存在时为零值（首建场景）
		SpiritSessionID: spiritSessionID,
		DagNodeID:       team.DagNodeID,
		DependsOn:       dependsOn,
		Status:          newStatus,
		StartedAt:       currentTR.StartedAt, // 回填真实开始时刻；零值时由下方兜底
		CompletedAt:     &now,
		Version:         newVersion,
	}
	if tr.StartedAt.IsZero() {
		tr.StartedAt = now // 行不存在（首建）或早建行缺 StartedAt 时的兜底
	}
	switch newStatus {
	case biz.TeamRunV2StatusCompleted, biz.TeamRunV2StatusPartialFailure:
		// partial_failure 复用 CompletedEvent 工厂：持久化以 tr.Status 为准。
		s.seq.Publish(ctx, biz.NewTeamRunCompletedEvent(tr))
	case biz.TeamRunV2StatusFailed:
		s.seq.Publish(ctx, biz.NewTeamRunFailedEvent(tr))
	case biz.TeamRunV2StatusCancelled:
		// 87-D4-2：cancelled 终态改用专用 kind team_run.cancelled——此前复用
		// team_run.failed + Status=cancelled，CriticalJournal 按 kind 回放时
		// 无法区分取消与失败。持久化行为不变（event_router 两个 kind 都路由
		// UpsertTeamRun，RepoSet.persistTeamRun 以 tr.Status 为准）。
		s.seq.Publish(ctx, biz.NewTeamRunCancelledEvent(tr))
	default:
		// 防御性 return：上方 trEvent 开关已对未知态 return，不可达。
		return
	}

	// 查询团队成员会话，为每个成员发布 MemberSessionUpdatedEvent。
	// SessionType="agent" 的会话即为成员会话；MemberAgentKey 标识成员的 agent key。
	result, searchErr := s.sessions.Search(ctx, biz.SessionSearchQuery{TeamID: teamID, Limit: 100})
	if searchErr != nil {
		s.lg.Warn("publishV2TeamRunCompletion: 查询团队成员会话失败，仅发布 TeamRun 完成事件",
			loggateway.StepID("spirit.v2.team_run_completion.search_fail"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(searchErr),
		)
		return
	}
	if len(result.Items) == 0 {
		s.lg.Warn("publishV2TeamRunCompletion: 查询团队成员会话返回 0 条记录",
			loggateway.StepID("spirit.v2.team_run_completion.search_empty"),
			loggateway.Str("team_id", teamID),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Int("total", result.Total),
		)
	} else {
		types := make([]string, 0, len(result.Items))
		for _, sess := range result.Items {
			types = append(types, sess.SessionType+":"+sess.MemberAgentKey)
		}
		s.lg.Info("publishV2TeamRunCompletion: 查询到会话记录",
			loggateway.StepID("spirit.v2.team_run_completion.search_result"),
			loggateway.Str("team_id", teamID),
			loggateway.Int("total", result.Total),
			loggateway.Int("items", len(result.Items)),
			loggateway.Str("session_types", strings.Join(types, ",")),
		)
	}
	memberCount := 0
	publishedKeys := make(map[string]bool, len(result.Items))
	// publishedAgentIDs 按 agent_id 维度去重（2026-08-08 幽灵成员修复）：定义
	// 快照里成员常缺 agent_key 只有 agent_id，兜底块会以 hex agent_id 作为 key
	// 补发，与真实成员（按 MemberAgentKey 登记）命名空间不交叉 → 同一成员被
	// 发布两次（真实 + hex 幽灵）。登记真实成员的 agent_id 让兜底块能识别
	// 「该成员已有真实 agent session」，跳过重复补发。
	publishedAgentIDs := make(map[string]bool, len(result.Items))
	// F10 (Phase 11) 结果导向成员状态：预统计可归属成员数——交付物证据仅
	// 用于单成员团队（MDC state 是团队共享黑板，多成员无法按成员归因）。
	eligibleMembers := 0
	for _, sess := range result.Items {
		if sess.SessionType == "agent" && sess.MemberAgentKey != "" {
			eligibleMembers++
		}
	}
	singleMember := eligibleMembers == 1
	for _, sess := range result.Items {
		if sess.SessionType != "agent" {
			continue
		}
		agentKey := sess.MemberAgentKey
		if agentKey == "" {
			continue
		}
		memberCount++
		publishedKeys[agentKey] = true
		if id := strings.TrimSpace(sess.AgentID); id != "" {
			publishedAgentIDs[id] = true
		}
		msID := string(agent.NewMemberSessionActivityID(teamRunID, agentKey))
		s.lg.Info("publishV2TeamRunCompletion: 派生 MemberSession ID",
			loggateway.StepID("spirit.v2.team_run_completion.msid"),
			loggateway.Str("team_id", teamID),
			loggateway.Str("team_run_id", teamRunID),
			loggateway.Str("member_session_id", msID),
			loggateway.Str("agent_key", agentKey),
			loggateway.Str("member_session_db_id", sess.ID),
		)
		// F10：团队 completed 时以执行结果证据覆盖成员状态（详见
		// resolveMemberOutcomeStatus 文档注释）。
		msStatus, outcomeReason := resolveMemberOutcomeStatus(ctx, s.team.SpiritUC, team, memberStatus, sess.ID, singleMember)
		if outcomeReason != "" {
			s.lg.Warn("publishV2TeamRunCompletion: F10 结果证据覆盖成员状态",
				loggateway.StepID("spirit.v2.team_run_completion.member_outcome"),
				loggateway.Str("team_id", teamID),
				loggateway.Str("agent_key", agentKey),
				loggateway.Str("member_session_db_id", sess.ID),
				loggateway.Str("base_status", string(memberStatus)),
				loggateway.Str("outcome_status", string(msStatus)),
				loggateway.Str("reason", outcomeReason),
			)
		}
		// 2026-08-08 问题4a：成员执行窗口取 step 流聚合（最早 StartedAt →
		// 最晚活动证据），而非发布时刻——否则 StartedAt≈FinishedAt≈now，
		// 前端耗时恒为 0。无 step 证据（ok=false）回退发布时刻。
		startedAt := now
		finishedAt := now
		if s.team.SpiritUC != nil {
			if wStart, wEnd, ok := s.team.SpiritUC.MemberExecutionWindow(ctx, sess.ID); ok {
				startedAt = wStart
				if !wEnd.IsZero() {
					finishedAt = wEnd
				}
			}
		}
		ms := biz.MemberSession{
			ID:              msID,
			TeamRunID:       teamRunID,
			TeamStageID:     teamStageID,
			TaskID:          rootTaskID,
			SessionID:       sess.ID,
			SpiritSessionID: spiritSessionID,
			AgentKey:        agentKey,
			Status:          msStatus,
			StartedAt:       startedAt,
			FinishedAt:      &finishedAt,
			// 2026-07-28 单写者重设计：终态事件携带 outcome 权威版本带
			// （biz.MemberSessionVersion*），service 是成员终态唯一写者。
			Version: biz.MemberSessionVersionOutcome,
			Error:   outcomeReason,
		}
		s.seq.Publish(ctx, biz.NewMemberSessionUpdatedEvent(ms))
	}
	// F4 (12:33 修复) 兜底：团队定义中的成员若无 agent session（创建失败、深度
	// 超限等），session 搜索覆盖不到 → 永远收不到 updated 事件 → MemberSession
	// 卡 running。以团队主 session 补发终态事件，保证定义成员状态必达终态。
	if team.ID != "" && strings.TrimSpace(team.DefinitionJSON) != "" {
		var defProbe struct {
			Members []struct {
				AgentKey string `json:"agent_key"`
				AgentID  string `json:"agent_id"`
				Name     string `json:"name"`
			} `json:"members"`
		}
		if uerr := json.Unmarshal([]byte(team.DefinitionJSON), &defProbe); uerr == nil {
			fallbackSessionID := ""
			for _, sess := range result.Items {
				if sess.SessionType == "team" {
					fallbackSessionID = sess.ID
					break
				}
			}
			// 团队主 session 未检索到时放弃兜底：空 SessionID 会覆盖 created
			// 事件已写入的 session_id（updated 走全字段 update）。
			if fallbackSessionID == "" {
				defProbe.Members = nil
			}
			for _, m := range defProbe.Members {
				key := strings.TrimSpace(m.AgentKey)
				if key == "" {
					key = strings.TrimSpace(m.AgentID)
				}
				// 幽灵成员守卫：除按 key 去重外，还要按 agent_id 去重——定义
				// 快照缺 agent_key 时 key 退化为 hex agent_id，与真实成员的
				// agent_key 不交叉，单靠 publishedKeys 无法识别重复。
				if key == "" || publishedKeys[key] || publishedAgentIDs[strings.TrimSpace(m.AgentID)] {
					continue
				}
				publishedKeys[key] = true
				memberCount++
				name := strings.TrimSpace(m.Name)
				if name == "" {
					name = key
				}
				msID := string(agent.NewMemberSessionActivityID(teamRunID, key))
				ms := biz.MemberSession{
					ID:              msID,
					TeamRunID:       teamRunID,
					TeamStageID:     teamStageID,
					TaskID:          rootTaskID,
					SessionID:       fallbackSessionID,
					SpiritSessionID: spiritSessionID,
					AgentKey:        key,
					AgentName:       name,
					Status:          memberStatus,
					StartedAt:       now,
					FinishedAt:      &now,
					// 2026-07-28 单写者重设计：兜底终态同样携带 outcome 权威
					// 版本带（biz.MemberSessionVersion*）。
					Version: biz.MemberSessionVersionOutcome,
				}
				s.lg.Warn("publishV2TeamRunCompletion: 定义成员无 agent session，兜底补发终态事件",
					loggateway.StepID("spirit.v2.team_run_completion.fallback"),
					loggateway.Str("team_id", teamID),
					loggateway.Str("agent_key", key),
					loggateway.Str("fallback_session_id", fallbackSessionID),
				)
				s.seq.Publish(ctx, biz.NewMemberSessionUpdatedEvent(ms))
			}
		}
	}
	// 2026-07-04 问题 C1 修复：记录发布的 MemberSession 完成事件数量。
	s.lg.Info("publishV2TeamRunCompletion: 已发布 MemberSession 完成事件",
		loggateway.StepID("spirit.v2.team_run_completion.done"),
		loggateway.Str("team_id", teamID),
		loggateway.Str("team_run_id", teamRunID),
		loggateway.Str("member_status", string(memberStatus)),
		loggateway.Int("member_count", memberCount),
		loggateway.Str("root_task_id", rootTaskID),
	)
}
