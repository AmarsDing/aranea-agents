package service

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/internal/event"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

var (
	_ tools.SpiritTeamAssemblerPort = (*SpiritTeamAssembler)(nil)
	_ tools.SpiritTeamQueryPort     = (*SpiritTeamAssembler)(nil)
	_ tools.SpiritTeamControllerPort = (*SpiritTeamAssembler)(nil)
	_ biz.TeamStarterPort           = (*TeamStarter)(nil)
)

type TeamStarter struct {
	sessions       *biz.SessionUsecase
	team           TeamOrchestrationDeps
	bus            event.Bus
	orchCache      *biz.OrchestrationCache
	evolutionSugg  biz.EvolutionSuggestionRepo
	lg             loggateway.Logger
}

func NewTeamStarter(sessions *biz.SessionUsecase, team TeamOrchestrationDeps, bus event.Bus, orchCache *biz.OrchestrationCache, evolutionSugg biz.EvolutionSuggestionRepo, lg loggateway.Logger) *TeamStarter {
	return &TeamStarter{sessions: sessions, team: team, bus: bus, orchCache: orchCache, evolutionSugg: evolutionSugg, lg: lg}
}

func (s *TeamStarter) StartTeamTurn(ctx context.Context, sessionID string, content string) error {
	if s.team.TeamsNative == nil {
		return kerrors.InternalServer("SPIRIT", "team runner not available")
	}
	sess, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return kerrors.NotFound("SPIRIT", "team session not found")
	}
	if sess.Status == string(sessstatus.SessionStatusRunning) {
		s.lg.Info("团队 session 已在运行中，跳过重复启动",
			loggateway.StepID("spirit.start_team_skip"),
			loggateway.Str("session_id", sessionID),
		)
		return nil
	}

	_ = s.sessions.TransitionStatus(ctx, sessionID, sessstatus.SessionStatusRunning, "")

	spiritSessionID := strings.TrimSpace(sess.ParentSessionID)
	teamID := strings.TrimSpace(sess.TeamID)

	if teamID != "" {
		if _, updateErr := s.team.TeamUC.Update(ctx, teamID, biz.Team{Status: "running"}); updateErr != nil {
			s.lg.Warn("更新团队状态为 running 失败",
				loggateway.StepID("spirit.team.running_err"),
				loggateway.Str("team_id", teamID),
				loggateway.Err(updateErr),
			)
		}
	}

	if s.bus != nil && teamID != "" && spiritSessionID != "" {
		env := event.NewEnvelope(event.EnvelopeTypeSpiritTeamProgress, "team-starter", spiritSessionID)
		env.TeamID = teamID
		env.Metadata = map[string]any{
			"team_id":      teamID,
			"status":       "running",
			"progress_pct": 0,
		}
		s.bus.Publish(ctx, env)
	}

	input := biz.TurnInput{
		SessionID: sessionID,
		Content:   content,
	}

	turnCtx := ctx
	if spiritSessionID != "" && s.team.SpiritUC != nil {
		resolvedCfg := s.team.SpiritUC.GetParallelConfig(ctx, spiritSessionID)
		if timeout := resolvedCfg.TeamTimeout(); timeout > 0 {
			var cancel context.CancelFunc
			turnCtx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
	}

	_, _, err = s.team.TeamsNative.RunTurnFromInput(turnCtx, sess, input)

	if err != nil {
		_ = s.sessions.TransitionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
		s.lg.Warn("自动启动团队 Turn 失败",
			loggateway.StepID("spirit.start_team_fail"),
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err),
		)
		if spiritSessionID != "" && teamID != "" {
			s.HandleTeamTurnResult(ctx, spiritSessionID, teamID, "failed", err.Error())
		}
		return err
	}

	_ = s.sessions.TransitionStatus(ctx, sessionID, sessstatus.SessionStatusCompleted, "")
	if spiritSessionID != "" && teamID != "" {
		s.HandleTeamTurnResult(ctx, spiritSessionID, teamID, "completed", "")
	}
	return nil
}

func (s *TeamStarter) HandleTeamTurnResult(ctx context.Context, spiritSessionID, teamID, status, errMsg string) {
	if s.team.TeamUC == nil {
		return
	}
	team, err := s.team.TeamUC.Get(ctx, teamID)
	if err != nil || !team.AutoCreated {
		return
	}

	durationMs := int64(0)
	runs, runErr := s.team.TeamUC.ListRuns(ctx, teamID, 1)
	if runErr == nil && len(runs) > 0 {
		durationMs = int64(runs[0].DurationMS)
	}

	var envType event.EnvelopeType
	if status == "completed" {
		envType = event.EnvelopeTypeSpiritTeamCompleted
		s.recordTeamCompletion(ctx, team, durationMs)
		s.scheduleDependentTeams(ctx, spiritSessionID, team)
	} else {
		envType = event.EnvelopeTypeSpiritTeamFailed
	}

	if s.bus != nil {
		env := event.NewEnvelope(envType, "spirit-lifecycle", spiritSessionID)
		env.TeamID = teamID
		meta := map[string]any{
			"team_id":     teamID,
			"team_name":   team.DisplayName,
			"status":      status,
			"duration_ms": durationMs,
		}
		if errMsg != "" {
			meta["error"] = errMsg
		}
		env.Metadata = meta
		s.bus.Publish(ctx, env)

		progressPct := 100.0
		if status == "failed" {
			progressPct = 0
		}
		progressEnv := event.NewEnvelope(event.EnvelopeTypeSpiritTeamProgress, "spirit-lifecycle", spiritSessionID)
		progressEnv.TeamID = teamID
		progressEnv.Metadata = map[string]any{
			"team_id":      teamID,
			"status":       status,
			"progress_pct": progressPct,
			"duration_ms":  durationMs,
		}
		s.bus.Publish(ctx, progressEnv)
	}

	s.checkAllTeamsCompleted(ctx, spiritSessionID)
}

func (s *TeamStarter) recordTeamCompletion(ctx context.Context, team biz.Team, durationMs int64) {
	if s.orchCache == nil || team.DagNodeID == "" {
		return
	}
	dqScore := biz.ComputeDQScore(biz.TeamSynthesisResult{
		TeamID:   team.ID,
		TeamName: team.DisplayName,
		TaskName: team.TaskDescription,
		Status:   "completed",
	}, durationMs)
	taskPattern := biz.ExtractTaskPattern(team.TaskDescription)
	topology := biz.InferTopologyFromTeam(team, s.lg)
	s.orchCache.RecordCompletion(ctx, taskPattern, topology, dqScore, 1, durationMs)
	s.lg.Info("精灵团队完成，记录 DQ Score",
		loggateway.StepID("spirit.team.completion"),
		loggateway.Str("team_id", team.ID),
		loggateway.Str("task_pattern", taskPattern),
		loggateway.Float64("dq_score", dqScore),
	)

	if dqScore < 0.5 && s.evolutionSugg != nil && team.SpiritSessionID != "" {
		altTopology, altFound := s.orchCache.SuggestBestAlternativeTopology(team.TaskDescription, topology)
		content := fmt.Sprintf("团队 %q 的 DQ Score 为 %.2f（低于阈值 0.5），当前拓扑 %s 执行效果不佳。", team.DisplayName, dqScore, topology)
		if altFound {
			content += fmt.Sprintf("建议尝试 %s 拓扑。", altTopology)
		} else {
			content += "暂无历史数据推荐替代拓扑，建议调整任务描述或减少团队数量。"
		}
		_, suggErr := s.evolutionSugg.Create(ctx, biz.EvolutionSuggestion{
			AgentID: team.SpiritSessionID,
			Type:    "orchestration_optimization",
			Title:   fmt.Sprintf("编排优化建议: %s", biz.TruncateRunes(team.TaskDescription, 40)),
			Content: content,
			Status:  "pending",
		})
		if suggErr != nil {
			s.lg.Warn("创建编排优化建议失败",
				loggateway.StepID("spirit.evolution_suggestion_err"),
				loggateway.Str("team_id", team.ID),
				loggateway.Err(suggErr),
			)
		}
	}
}

func (s *TeamStarter) scheduleDependentTeams(ctx context.Context, spiritSessionID string, completedTeam biz.Team) {
	if completedTeam.DagNodeID == "" {
		return
	}
	allTeams, err := s.team.TeamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		s.lg.Warn("查询精灵团队列表失败，跳过依赖调度",
			loggateway.StepID("spirit.schedule_deps.list_err"),
			loggateway.Err(err),
		)
		return
	}
	for i := range allTeams {
		t := &allTeams[i]
		if t.Status != "waiting_deps" {
			continue
		}
		if !containsString(t.DependsOn, completedTeam.DagNodeID) {
			continue
		}
		allDepsMet := true
		anyDepFailed := false
		for _, depID := range t.DependsOn {
			found := false
			for j := range allTeams {
				if allTeams[j].DagNodeID == depID {
					if allTeams[j].Status == "completed" {
						found = true
					} else if allTeams[j].Status == "failed" {
						anyDepFailed = true
					}
					break
				}
			}
			if !found && !anyDepFailed {
				allDepsMet = false
				break
			}
		}
		if anyDepFailed {
			_, uerr := s.team.TeamUC.Update(ctx, t.ID, biz.Team{Status: "failed"})
			if uerr != nil {
				s.lg.Warn("更新团队状态为 failed 失败，依赖调度中断",
					loggateway.StepID("spirit.schedule_deps.fail_err"),
					loggateway.Str("team_id", t.ID),
					loggateway.Err(uerr),
				)
			} else {
				s.lg.Info("依赖调度：团队前置依赖失败，标记团队为 failed",
					loggateway.StepID("spirit.schedule_deps.dep_failed"),
					loggateway.Str("team_id", t.ID),
					loggateway.Str("dag_node_id", t.DagNodeID),
				)
				if s.bus != nil {
					env := event.NewEnvelope(event.EnvelopeTypeSpiritTeamFailed, "spirit-scheduler", spiritSessionID)
					env.TeamID = t.ID
					env.Metadata = map[string]any{
						"team_id":   t.ID,
						"team_name": t.DisplayName,
						"error":     "前置依赖团队执行失败",
					}
					s.bus.Publish(ctx, env)
				}
				s.checkAllTeamsCompleted(ctx, spiritSessionID)
			}
			continue
		}
		if !allDepsMet {
			continue
		}
		current, getErr := s.team.TeamUC.Get(ctx, t.ID)
		if getErr != nil || current.Status != "waiting_deps" {
			s.lg.Info("依赖调度：团队状态已变更，跳过激活",
				loggateway.StepID("spirit.schedule_deps.stale"),
				loggateway.Str("team_id", t.ID),
				loggateway.Str("current_status", current.Status),
			)
			continue
		}
		_, uerr := s.team.TeamUC.Update(ctx, t.ID, biz.Team{Status: "active"})
		if uerr != nil {
			s.lg.Warn("更新团队状态失败，依赖调度中断",
				loggateway.StepID("spirit.schedule_deps.update_err"),
				loggateway.Str("team_id", t.ID),
				loggateway.Err(uerr),
			)
			continue
		}
		s.lg.Info("依赖调度：团队所有前置依赖已完成，激活团队",
			loggateway.StepID("spirit.schedule_deps.activated"),
			loggateway.Str("team_id", t.ID),
			loggateway.Str("dag_node_id", t.DagNodeID),
		)
		if s.bus != nil {
			env := event.NewEnvelope(event.EnvelopeTypeSpiritTeamProgress, "spirit-scheduler", spiritSessionID)
			env.TeamID = t.ID
			env.Metadata = map[string]any{
				"team_id":   t.ID,
				"team_name": t.DisplayName,
				"status":    "active",
			}
			s.bus.Publish(ctx, env)
		}
		if t.TaskDescription != "" {
			taskDesc := t.TaskDescription
			depTeamID := t.ID
			safego.Go(ctx, "spirit-schedule-deps", func() {
				startCtx := context.WithoutCancel(ctx)
				s.findAndStartTeamTurn(startCtx, depTeamID, taskDesc)
			})
		}
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
	teams, err := s.team.TeamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		s.lg.Warn("查询精灵会话团队列表失败，跳过全完成检查",
			loggateway.StepID("spirit.teams.check_all"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(err),
		)
		return
	}
	if len(teams) == 0 {
		return
	}
	for _, t := range teams {
		if t.Status == "active" || t.Status == "waiting_deps" || t.Status == "assembled" || t.Status == "running" {
			return
		}
	}
	var teamIDs []string
	for _, t := range teams {
		teamIDs = append(teamIDs, t.ID)
	}
	if s.bus != nil {
		env := event.NewEnvelope(event.EnvelopeTypeSpiritTeamsAllCompleted, "team-starter", spiritSessionID)
		env.Metadata = map[string]any{
			"spirit_session_id": spiritSessionID,
			"team_ids":          teamIDs,
		}
		s.bus.Publish(ctx, env)
	}
}

type SpiritTeamAssembler struct {
	spiritUC    *biz.SpiritTeamUsecase
	orchCache   *biz.OrchestrationCache
	bus         event.Bus
	teamStarter biz.TeamStarterPort
	lg          loggateway.Logger
}

func NewSpiritTeamAssembler(
	spiritUC *biz.SpiritTeamUsecase,
	orchCache *biz.OrchestrationCache,
	bus event.Bus,
	teamStarter biz.TeamStarterPort,
	lg loggateway.Logger,
) *SpiritTeamAssembler {
	return &SpiritTeamAssembler{
		spiritUC:    spiritUC,
		orchCache:   orchCache,
		bus:         bus,
		teamStarter: teamStarter,
		lg:          lg,
	}
}

func (a *SpiritTeamAssembler) AssembleTeam(ctx context.Context, params biz.SpiritTeamParams) (biz.Team, biz.Session, error) {
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
		return biz.Team{}, biz.Session{}, err
	}

	a.publishSpiritTeamAssembled(ctx, spiritSessionID, result.Team, result.Session, params.Mode, params.TaskDescription, params.TopologyReason)

	if params.AutoStart && a.teamStarter != nil && result.Team.Status == "active" && strings.TrimSpace(params.TaskDescription) != "" {
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

	return result.Team, result.Session, nil
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

func (a *SpiritTeamAssembler) ListAllTeams(ctx context.Context, spiritSessionID string) ([]biz.Team, error) {
	return a.spiritUC.ListAllTeams(ctx, spiritSessionID)
}

func (a *SpiritTeamAssembler) GetMaxParallelTeams(ctx context.Context, spiritSessionID string) int {
	return a.spiritUC.GetMaxParallelTeams(ctx, spiritSessionID)
}

func (a *SpiritTeamAssembler) CancelTeam(ctx context.Context, teamID string) error {
	team, err := a.spiritUC.GetTeam(ctx, teamID)
	if err != nil {
		return err
	}
	if err := a.spiritUC.CancelTeam(ctx, teamID); err != nil {
		return err
	}
	if a.bus != nil && team.SpiritSessionID != "" {
		env := event.NewEnvelope(event.EnvelopeTypeSpiritTeamProgress, "spirit-cancel", team.SpiritSessionID)
		env.TeamID = teamID
		env.Metadata = map[string]any{
			"team_id":   teamID,
			"team_name": team.DisplayName,
			"status":    "cancelled",
		}
		a.bus.Publish(ctx, env)
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
	}
	return string(topology), found
}

func (a *SpiritTeamAssembler) publishSpiritTeamAssembled(ctx context.Context, spiritSessionID string, team biz.Team, teamSession biz.Session, mode, taskDesc, topologyReason string) {
	if a.bus == nil {
		return
	}
	env := event.NewEnvelope(event.EnvelopeTypeSpiritTeamAssembled, "spirit-team-assembler", spiritSessionID)
	env.TeamID = team.ID
	env.Metadata = map[string]any{
		"team_id":         team.ID,
		"team_name":       team.DisplayName,
		"session_id":      teamSession.ID,
		"mode":            mode,
		"task_summary":    biz.TruncateRunes(taskDesc, 200),
		"dag_node_id":     team.DagNodeID,
		"depends_on":      team.DependsOn,
		"topology_reason": topologyReason,
		"duration_ms":     0,
		"total_steps":     1,
	}
	a.bus.Publish(ctx, env)
}


