// Code owned by package service; extracted from spirit_team.go for AS-COG-01 file-size fitness.

package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

type SpiritTeamAssembler struct {
	spiritUC    *biz.SpiritTeamUsecase
	orchCache   *biz.OrchestrationCache
	eventBus    biz.EventBus      // v2: team_stage + notice events
	seq         rt.EventPublisher // v2 Sequencer
	teamStarter biz.TeamStarterPort
	agentReader biz.AgentReader
	lg          loggateway.Logger
	teamStageR  biz.TeamStageV2Reader
	teamStageW  biz.TeamStageV2Writer
	tsSM        *biz.TeamStageStateMachine
}

func NewSpiritTeamAssembler(
	spiritUC *biz.SpiritTeamUsecase,
	orchCache *biz.OrchestrationCache,
	eventBus biz.EventBus,
	seq rt.EventPublisher,
	teamStarter biz.TeamStarterPort,
	agentReader biz.AgentReader,
	lg loggateway.Logger,
	teamStageR biz.TeamStageV2Reader,
	teamStageW biz.TeamStageV2Writer,
) *SpiritTeamAssembler {
	return &SpiritTeamAssembler{
		spiritUC:    spiritUC,
		orchCache:   orchCache,
		eventBus:    eventBus,
		seq:         seq,
		teamStarter: teamStarter,
		agentReader: agentReader,
		lg:          lg,
		teamStageR:  teamStageR,
		teamStageW:  teamStageW,
		tsSM:        biz.NewTeamStageStateMachine(),
	}
}

func (a *SpiritTeamAssembler) AssembleTeam(ctx context.Context, params biz.SpiritTeamParams) (biz.Team, biz.Session, map[string]string, error) {
	spiritSessionID := strings.TrimSpace(params.SpiritSessionID)

	a.lg.Info("精灵团队组装开始",
		loggateway.StepID("spirit.team.assemble"),
		loggateway.Str("spirit_session_id", spiritSessionID),
		loggateway.Str("mode", params.Mode),
		loggateway.Int("agent_count", len(params.AgentKeys)),
	)

	result, err := a.spiritUC.AssembleTeam(ctx, params)
	if err != nil {
		a.lg.Error("精灵团队组装失败",
			loggateway.StepID("spirit.team.assemble_fail"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(err),
		)
		return biz.Team{}, biz.Session{}, nil, err
	}

	a.publishSpiritTeamAssembled(ctx, spiritSessionID, result.Team, result.Session, params.Mode, params.TaskDescription, params.TopologyReason, params.AgentKeys, result.MemberSessions)

	if params.AutoStart && a.teamStarter != nil && result.Team.Status == biz.TeamStatusPending && strings.TrimSpace(params.TaskDescription) != "" {
		sessionID := result.Session.ID
		taskDesc := params.TaskDescription
		safego.Go(ctx, "spirit-auto-start", func() {
			startCtx := context.WithoutCancel(ctx)
			if startErr := a.teamStarter.StartTeamTurn(startCtx, sessionID, taskDesc); startErr != nil {
				a.lg.Warn("自动启动团队 Turn 失败",
					loggateway.StepID("spirit.auto_start_fail"),
					loggateway.Str("team_id", result.Team.ID),
					loggateway.Err(startErr),
				)
			}
		})
	}

	a.lg.Info("精灵团队组装完成",
		loggateway.StepID("spirit.team.assemble_done"),
		loggateway.Str("team_id", result.Team.ID),
		loggateway.Str("session_id", result.Session.ID),
	)

	return result.Team, result.Session, result.MemberSessions, nil
}

func (a *SpiritTeamAssembler) FindTeamSessionAndStartTurn(ctx context.Context, teamID string, taskDesc string) {
	result, searchErr := a.spiritUC.SearchSessions(ctx, sessstatus.SessionSearchQuery{TeamID: teamID, Limit: 1})
	if searchErr != nil || len(result.Items) == 0 {
		a.lg.Warn("查找团队 session 失败",
			loggateway.StepID("spirit.find_session_fail"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(searchErr),
		)
		return
	}
	if startErr := a.teamStarter.StartTeamTurn(ctx, result.Items[0].ID, taskDesc); startErr != nil {
		a.lg.Warn("自动启动团队 Turn 失败",
			loggateway.StepID("spirit.auto_start_fail"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(startErr),
		)
	}
}

func (a *SpiritTeamAssembler) ListActiveTeams(ctx context.Context, spiritSessionID string) ([]biz.Team, error) {
	return a.spiritUC.ListActiveTeams(ctx, spiritSessionID)
}

// InjectUpstreamDeliverables builds the upstream deliverable prefix for a
// DAG-activated downstream team (P0-③a). Thin passthrough to the biz layer;
// returns "" when there are no dependencies or nothing to inject.
func (a *SpiritTeamAssembler) InjectUpstreamDeliverables(ctx context.Context, spiritSessionID string, dependsOn []string) string {
	if a.spiritUC == nil || len(dependsOn) == 0 {
		return ""
	}
	return a.spiritUC.InjectUpstreamDeliverables(ctx, biz.Team{
		SpiritSessionID: spiritSessionID,
		DependsOn:       dependsOn,
	})
}

// BuildTeamTurnInput composes a DAG team's first-turn input (upstream prefix
// + task description + mandatory delivery protocol, 2026-07-25 Fix 2b). Thin
// passthrough to the biz layer.
func (a *SpiritTeamAssembler) BuildTeamTurnInput(ctx context.Context, team biz.Team) string {
	if a.spiritUC == nil {
		return team.TaskDescription
	}
	return a.spiritUC.BuildTeamTurnInput(ctx, team)
}

func (a *SpiritTeamAssembler) ListAllTeams(ctx context.Context, spiritSessionID string) ([]biz.Team, error) {
	return a.spiritUC.ListAllTeams(ctx, spiritSessionID)
}

// ReadUpstreamDeliverable delegates to the biz usecase, backing the spirit's
// get_team_deliverable tool (F6, Phase 11). Together with ListAllTeams it
// makes SpiritTeamAssembler satisfy tools.SpiritTeamDeliverablePort.
func (a *SpiritTeamAssembler) ReadUpstreamDeliverable(ctx context.Context, readerSessionID, teamID string, maxChars int) (biz.UpstreamDeliverableContent, error) {
	return a.spiritUC.ReadUpstreamDeliverable(ctx, readerSessionID, teamID, maxChars)
}

func (a *SpiritTeamAssembler) GetMaxParallelTeams(ctx context.Context, spiritSessionID string) int {
	return a.spiritUC.GetMaxParallelTeams(ctx, spiritSessionID)
}

func (a *SpiritTeamAssembler) CancelTeam(ctx context.Context, teamID string, reason biz.CancelReason) error {
	team, err := a.spiritUC.GetTeam(ctx, teamID)
	if err != nil {
		return err
	}
	if err := a.spiritUC.CancelTeam(ctx, teamID, reason); err != nil {
		return err
	}
	spiritSessionID := strings.TrimSpace(team.SpiritSessionID)
	if a.v2EventReady() && spiritSessionID != "" {
		// Phase 3b-D Task 10: migrated to v2 NewTeamStageUpdatedEvent.
		// No NewTeamStageCancelledEvent factory exists; use Updated as the
		// closest semantic match (same as HandleTeamTurnResult cancelled case).
		// 2026-07-04 问题 3 修复：携带 TeamName 让前端展示团队名称而非 ID。
		// 2026-07-04 问题 2 修复：改用 publishV2Event（seq 优先持久化）。
		// 2026-07-05 P1 #9d-2（AS-FSM-01）：用状态机校验 Running → Cancelled
		// 转换，修复 Version=0 Bug（状态未持久化）。
		// S-3 后 TeamStage 按 (teamID, rootTaskID) 每轮一行；cancel 是用户动作
		// （ctx 无 RootTaskActivityID），无法重放 ID 公式——查团队最新行定位；
		// 无行（团队从未产生 stage）时降级 legacy 公式，保留旧行为。
		cancelTsID := string(agent.NewTeamStageActivityID(teamID, ""))
		if a.teamStageR != nil {
			if latest, lerr := a.teamStageR.GetLatestTeamStageByTeam(ctx, teamID); lerr == nil && latest.ID != "" {
				cancelTsID = latest.ID
			}
		}
		newStatus, newVersion, tsOK := resolveTeamStageUpdate(ctx, a.teamStageR, a.tsSM,
			cancelTsID, biz.TeamStageEventCancel, biz.TeamStageStatusCancelled, a.lg)
		if tsOK {
			ts := biz.TeamStage{
				ID:        cancelTsID,
				TeamID:    teamID,
				TeamName:  team.DisplayName,
				SessionID: spiritSessionID,
				Status:    newStatus,
				Stage:     "cancelled",
				StartedAt: time.Now().UTC(),
				Version:   newVersion,
			}
			a.publishV2Event(ctx, biz.NewTeamStageUpdatedEvent(ts))
		}
		// P0-2：tsOK=false（状态机拒绝，当前 TeamStage 已终态，如重复取消）时
		// 跳过发布，不覆盖终态。
	}
	// 2026-07-29 S-1：standalone 团队 team.SpiritSessionID=="" 也必须进
	// HandleTeamTurnResult——其 standalone 分支内部回退解析聚合根（查 team
	// session），否则 teams 已 cancelled 而 TeamRun/MemberSession 永不达
	// 终态（成员永 running）。AutoCreated 团队 spiritSessionID 恒非空，
	// 行为不变。
	if a.teamStarter != nil {
		a.teamStarter.HandleTeamTurnResult(ctx, spiritSessionID, teamID, biz.TeamStatusCancelled, "", "")
	}
	return nil
}

func (a *SpiritTeamAssembler) CheckTeamProgress(ctx context.Context, spiritSessionID string) ([]biz.TeamProgress, error) {
	return a.spiritUC.CheckTeamProgress(ctx, spiritSessionID)
}

func (a *SpiritTeamAssembler) SuggestTopology(ctx context.Context, taskDescription string) (string, bool) {
	if a.orchCache == nil {
		return "", false
	}
	topology, found := a.orchCache.SuggestTopology(taskDescription)
	if found {
		a.lg.Info("编排缓存命中，推荐拓扑",
			loggateway.StepID("spirit.suggest_topology"),
			loggateway.Str("topology", string(topology)),
		)
		// Emit cache hit event
		// Phase 3b-D Task 10: migrated to v2 NewStepCreatedEvent (Kind=StepKindNotice).
		// DATA LOSS: v2 Step has no Meta field, so task_pattern/topology from v1
		// Meta are dropped. The original v1 Activity had no Content/SpiritSessionID;
		// v2 Step also leaves these empty (no spirit session context in SuggestTopology).
		if a.v2EventReady() {
			// 2026-07-04 问题 2 修复：改用 publishV2Event（seq 优先持久化）。
			// 2026-07-21 P1-5 F3：直发 notice 补全 StartedAt/CompletedAt/Version。
			now := time.Now()
			step := biz.Step{
				ID:             uuid.NewString(),
				Kind:           biz.StepKindNotice,
				Content:        "编排缓存命中，推荐拓扑: " + string(topology),
				Status:         biz.StepStatusCompleted,
				StartedAt:      now,
				CompletedAt:    &now,
				Version:        1,
				AuthorAgentKey: "spirit-team-assembler",
			}
			a.publishV2Event(ctx, biz.NewStepCreatedEvent(step))
		}
	}
	return string(topology), found
}
