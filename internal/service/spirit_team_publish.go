// Code owned by package service; extracted from spirit_team.go for AS-COG-01 file-size fitness.

package service

import (
	"context"
	stderrors "errors"
	"strings"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

func (a *SpiritTeamAssembler) publishSpiritTeamAssembled(ctx context.Context, spiritSessionID string, team biz.Team, teamSession biz.Session, mode, taskDesc, topologyReason string, agentKeys []string, memberSessions map[string]string) {
	// Phase 3b-D Task 10: dual-bus scheme. eventBus (v2) is the primary bus
	// for team_stage events; bus (v1) is retained for graph_stage snapshot only.
	// 2026-07-04 问题 2 修复：放宽守卫为 v2EventReady()（seq 或 eventBus 任一可用），
	// 让 seq-only 部署也能发布 TeamStage 事件并持久化。
	if !a.v2EventReady() {
		return
	}
	// GraphStage created 由 PublishV2Board 负责；节点状态由 PlanExecutor 更新。
	// 不再在此发布 v1 team-based graph_stage 快照（与 v2 GraphStage 双真相源冲突）。

	// ADR-06 (12:33 修复)：不再过滤系统 Agent。此前 filteredKeys 把系统 Agent
	// 排除在 MemberSession created 事件与 TeamStage.Members 之外，导致 Planner
	// 显式编入团队的 __system_admin__（安装 skill 等合法任务）无状态实体：
	// Graph 成员卡缺失或卡死 running、完成事件无法触达。系统 Agent 的分配约束
	// 上移到 Allocator/Planner 层；进入定义的成员一律拥有完整 MemberSession 生命周期。

	// Build members array so the frontend TeamCard can render the member list.
	// Problem 2 fix: previously agent_name was set to agent_key (showing raw
	// kebab-case keys like "deep-researcher" in the UI). Now we batch-resolve
	// each agent_key to DisplayName via AgentReader. Lookup failures fall back
	// to the agent_key so we never block team assembly on a stale key — the
	// richer team_summary SystemNoticeEvent (publishTeamRunSummary) emitted at
	// run completion will still carry authoritative member data.
	members := make([]map[string]any, 0, len(agentKeys))
	for _, key := range agentKeys {
		displayName := key
		avatarURL := ""
		if a.agentReader != nil {
			if ag, lookupErr := a.agentReader.GetAgentByAgentKey(ctx, key); lookupErr == nil {
				if ag.DisplayName != "" {
					displayName = ag.DisplayName
				}
				// Integrate agent config (avatar/icon) so the frontend can render
				// the member's configured avatar instead of a generic initial.
				avatarURL = ag.Icon
			} else if lookupErr != nil && !stderrors.Is(lookupErr, shared.ErrNotFound) {
				a.lg.Warn("team member name lookup failed, falling back to agent_key",
					loggateway.StepID("spirit.team.member_lookup_fail"),
					loggateway.Str("agent_key", key),
					loggateway.Err(lookupErr),
				)
			}
		}
		// Use the member's individual agent session ID for frontend lazy-loading.
		// Fall back to teamSession.ID if the agent session wasn't created (e.g. depth limit).
		sessionID := teamSession.ID
		if sid, ok := memberSessions[key]; ok && sid != "" {
			sessionID = sid
		}
		members = append(members, map[string]any{
			"agent_key":  key,
			"agent_name": displayName,
			"avatar_url": avatarURL,
			"status":     biz.ActivityStatusPending,
			"session_id": sessionID,
		})
	}
	// Persist synchronously to prevent a race with the progress event
	// (StartTeamTurn publishes a "progress" event with Status=Running but no
	// members/depends_on); the 2026-07-24 race-fix note below details the
	// version-guard failure this prevents. Persist via v2 TeamStage Sequencer
	// only (activities dual-write removed 2026-07-16).
	if strings.TrimSpace(team.DisplayName) == "" {
		a.lg.Warn("publishSpiritTeamAssembled: team.DisplayName 为空，前端将显示 team_id",
			loggateway.StepID("spirit.team.empty_display_name"),
			loggateway.Str("team_id", team.ID),
		)
	}
	ts := biz.TeamStage{
		ID:        string(agent.NewTeamStageActivityID(team.ID, string(agent.RootTaskActivityIDFromCtx(ctx)))),
		TaskID:    string(agent.RootTaskActivityIDFromCtx(ctx)),
		TurnID:    event.TurnIDFromContext(ctx),
		TeamID:    team.ID,
		TeamName:  team.DisplayName,
		SessionID: spiritSessionID,
		Status:    biz.TeamStageStatusPending,
		Stage:     biz.TeamStageStageAssembled,
		DependsOn: team.DependsOn,
		Members:   buildV2Members(members),
		StartedAt: time.Now().UTC(),
		Version:   1,
	}
	// 2026-07-24 竞态修复：TeamStage created(Version=1) 必须同步落库，再发异步事件。
	// 此前仅 seq.Publish（异步持久化），而 dispatchStep 的 Version=2 Upsert 是
	// 同步直写 repos——异步 V=1 一旦落后于同步 V=2，V=2 便以 Members=nil 走
	// CREATE 路径写入 members=null，随后 V=1 被 VersionLT 守卫拒绝，members
	// 永久丢失（实发事故：team members=null → ListSpiritTeams 返回空成员 →
	// 左侧边栏成员卡片缺失）。同步 Upsert 幂等：seq 异步重放同版本事件会被
	// 版本守卫拒绝并返回现有记录，无副作用；同步失败则降级为纯异步事件兜底。
	if a.teamStageW != nil {
		if _, err := a.teamStageW.UpsertTeamStage(ctx, ts); err != nil {
			a.lg.Warn("publishSpiritTeamAssembled: 同步持久化 TeamStage 失败，降级为异步事件",
				loggateway.StepID("spirit.team.stage_sync_persist_fail"),
				loggateway.Str("team_id", team.ID),
				loggateway.Err(err),
			)
		}
	}
	a.publishV2Event(ctx, biz.NewTeamStageCreatedEvent(ts))

	// 2026-07-04 问题 4 修复：发布 v2 TeamRun + MemberSession 创建事件。
	// 原先仅发布 TeamStage 事件，前端 v2 store 的 teamRuns/memberSessions Map 为空，
	// 导致 MemberSessionPanel 无法渲染成员会话树。
	a.publishV2TeamRunAndMemberSessions(ctx, spiritSessionID, team, teamSession, agentKeys, members)
}

// buildV2Members converts the v1 members array (map[string]any, used by v1
// Activity Meta) to the v2 type-safe []biz.MemberInfo.
//
// 2026-07-04 问题 4 修复：publishSpiritTeamAssembled 在创建 v2 TeamStage 时
// 需要填充 Members 字段，但 members 数据已构建为 v1 Meta 格式（[]map[string]any）。
// 此函数负责转换，避免重复构建。
func buildV2Members(members []map[string]any) []biz.MemberInfo {
	if len(members) == 0 {
		return nil
	}
	out := make([]biz.MemberInfo, 0, len(members))
	for _, m := range members {
		mi := biz.MemberInfo{}
		if v, ok := m["agent_key"].(string); ok {
			mi.AgentKey = v
		}
		if v, ok := m["agent_name"].(string); ok {
			mi.AgentName = v
		}
		if v, ok := m["avatar_url"].(string); ok {
			mi.AvatarURL = v
		}
		if v, ok := m["session_id"].(string); ok {
			mi.ChildSessionID = v
		}
		if v, ok := m["status"].(string); ok {
			mi.Status = v
		}
		out = append(out, mi)
	}
	return out
}

// publishV2TeamRunAndMemberSessions 发布 v2 TeamRun + MemberSession 创建事件。
// 这些事件通过 v2 Sequencer 持久化到 v2 表 + 推送到 WS，前端 v2 store 收到后
// 渲染 MemberSessionPanel（团队成员会话树）。
//
// 设计参考：docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md
// §3.2.2 TeamRun / §3.2.2 MemberSession
//
// 关键关系：
//   - TeamRun.TeamStageID = TeamStage.ID（已由 publishSpiritTeamAssembled 创建）
//   - MemberSession.TeamRunID = TeamRun.ID
//   - MemberSession.TeamStageID = TeamStage.ID
//   - MemberSession.SessionID = 成员自己的 session ID（来自 memberSessions map）
//
// 注意：此处仅发布 created 事件（status=running/pending）。后续生命周期事件
// （completed/failed）由 HandleTeamTurnResult 在团队状态变更时发布。
func (a *SpiritTeamAssembler) publishV2TeamRunAndMemberSessions(
	ctx context.Context,
	spiritSessionID string,
	team biz.Team,
	teamSession biz.Session,
	agentKeys []string,
	members []map[string]any,
) {
	if a.seq == nil || spiritSessionID == "" || team.ID == "" {
		return
	}
	rootTaskID := string(agent.RootTaskActivityIDFromCtx(ctx))
	teamStageID := string(agent.NewTeamStageActivityID(team.ID, rootTaskID))
	now := time.Now().UTC()
	// 派生 TeamRun ID（基于 teamStageID 确定性派生，确保多次调用产生相同 ID）。
	teamRunID := agent.NewTeamRunV2ID(teamStageID)
	// 构建 v2 TeamRun 实体。
	tr := biz.TeamRun{
		ID:              teamRunID,
		TeamStageID:     teamStageID,
		TaskID:          rootTaskID,
		SessionID:       teamSession.ID,
		SpiritSessionID: spiritSessionID,
		DagNodeID:       team.DagNodeID,
		DependsOn:       append([]string(nil), team.DependsOn...),
		Status:          biz.TeamRunV2StatusRunning,
		StartedAt:       now,
		Version:         1,
	}
	a.seq.Publish(ctx, biz.NewTeamRunStartedEvent(tr))
	// 为每个成员发布 MemberSessionCreatedEvent。
	for i, key := range agentKeys {
		if key == "" {
			continue
		}
		// 从 members 数组中获取对应的元数据（agent_name, avatar_url, session_id）。
		var agentName, avatarURL, memberSessionID string
		if i < len(members) {
			if v, ok := members[i]["agent_name"].(string); ok {
				agentName = v
			}
			if v, ok := members[i]["avatar_url"].(string); ok {
				avatarURL = v
			}
			if v, ok := members[i]["session_id"].(string); ok {
				memberSessionID = v
			}
		}
		if memberSessionID == "" {
			memberSessionID = teamSession.ID
		}
		// 派生 MemberSession ID（基于 teamRunID + agentKey 确定性派生）。
		msID := string(agent.NewMemberSessionActivityID(teamRunID, key))
		ms := biz.MemberSession{
			ID:              msID,
			TeamRunID:       teamRunID,
			TeamStageID:     teamStageID,
			TaskID:          rootTaskID,
			SessionID:       memberSessionID,
			SpiritSessionID: spiritSessionID,
			AgentKey:        key,
			AgentName:       agentName,
			AvatarURL:       avatarURL,
			Status:          biz.MemberSessionStatusRunning,
			StartedAt:       now,
			Version:         biz.MemberSessionVersionCreated,
		}
		a.seq.Publish(ctx, biz.NewMemberSessionCreatedEvent(ms))
	}
}

// resolveMemberOutcomeStatus implements F10 (Phase 11) outcome-oriented member
// status: when the team reports completed, the member's final status is decided
// by execution EVIDENCE, not the message lifecycle (12:33: members returning
// text were shown successful even when the underlying work failed).
//
// Evidence chain (evaluated only when base=Completed):
//  1. MemberExecutionEvidence (interrupted session / failed|cancelled step)
//     → failed with the evidence reason.
//  2. Single-member DAG team: HasRealDeliverable=false/err → failed
//     ("completed must be proven" — same posture as the Fix-1 deliverable
//     gate, which treats gateErr as no-deliverable). Restricted to DAG teams
//     because non-DAG teams carry no deliverable obligation
//     (HasRealDeliverable=false by definition); restricted to single-member
//     teams because the MDC deliverable state is a shared blackboard that
//     cannot be attributed to one member.
//
// skipped (team cancelled) and failed (team failed) pass through unchanged:
// evidence never overrides a terminal member status derived from the team.
// A nil controller is tolerated (evidence unavailable → keep base status).
