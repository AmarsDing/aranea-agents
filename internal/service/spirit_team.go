package service

import (
	"context"
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
	_ tools.SpiritTeamAssemblerPort  = (*SpiritTeamAssembler)(nil)
	_ tools.SpiritTeamQueryPort      = (*SpiritTeamAssembler)(nil)
	_ tools.SpiritTeamControllerPort = (*SpiritTeamAssembler)(nil)
	_ biz.TeamStarterPort            = (*TeamStarter)(nil)
	_ biz.TimeoutHandler             = (*TeamStarter)(nil)
)

type TeamStarter struct {
	sessions *biz.SessionUsecase
	team     TeamOrchestrationDeps
	bus      event.Bus
	lg       loggateway.Logger
}

func NewTeamStarter(sessions *biz.SessionUsecase, team TeamOrchestrationDeps, bus event.Bus, lg loggateway.Logger) *TeamStarter {
	return &TeamStarter{sessions: sessions, team: team, bus: bus, lg: lg}
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

	if err := s.sessions.TransitionStatus(ctx, sessionID, sessstatus.SessionStatusRunning, ""); err != nil {
		s.lg.Warn("团队 Session 状态转换到 running 失败",
			loggateway.StepID("spirit.transition_running_err"),
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err),
		)
	}

	spiritSessionID := strings.TrimSpace(sess.ParentSessionID)
	teamID := strings.TrimSpace(sess.TeamID)

	if teamID != "" {
		if _, updateErr := s.team.TeamUC.TransitionStatus(ctx, teamID, biz.TeamStatusRunning); updateErr != nil {
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
			"status":       biz.TeamStatusRunning,
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
		if transErr := s.sessions.TransitionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError); transErr != nil {
			s.lg.Warn("团队 Session 状态转换到 interrupted 失败",
				loggateway.StepID("spirit.transition_interrupted_err"),
				loggateway.Str("session_id", sessionID),
				loggateway.Err(transErr),
			)
		}
		s.lg.Warn("自动启动团队 Turn 失败",
			loggateway.StepID("spirit.start_team_fail"),
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err),
		)
		if spiritSessionID != "" && teamID != "" {
			s.HandleTeamTurnResult(ctx, spiritSessionID, teamID, biz.TeamStatusFailed, err.Error())
		}
		return err
	}

	if err := s.sessions.TransitionStatus(ctx, sessionID, sessstatus.SessionStatusCompleted, ""); err != nil {
		s.lg.Warn("团队 Session 状态转换到 completed 失败",
			loggateway.StepID("spirit.transition_completed_err"),
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err),
		)
	}
	if spiritSessionID != "" && teamID != "" {
		s.HandleTeamTurnResult(ctx, spiritSessionID, teamID, biz.TeamStatusCompleted, "")
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

	// Cancel timeout timer for any terminal status to prevent stale callbacks.
	s.team.SpiritUC.CancelTimeoutTimer(teamID)

	durationMs := int64(0)
	runs, runErr := s.team.TeamUC.ListRuns(ctx, teamID, 1)
	if runErr == nil && len(runs) > 0 {
		durationMs = int64(runs[0].DurationMS)
	}

	var envType event.EnvelopeType
	if status == biz.TeamStatusCompleted {
		envType = event.EnvelopeTypeSpiritTeamCompleted
		if _, updateErr := s.team.TeamUC.TransitionStatus(ctx, teamID, biz.TeamStatusCompleted); updateErr != nil {
			s.lg.Warn("更新团队状态为 completed 失败",
				loggateway.StepID("spirit.team.completed_err"),
				loggateway.Str("team_id", teamID),
				loggateway.Err(updateErr),
			)
		}
		s.recordTeamCompletion(ctx, team, durationMs)
		s.scheduleDependentTeams(ctx, spiritSessionID, team)
	} else if status == biz.TeamStatusCancelled {
		envType = event.EnvelopeTypeSpiritTeamFailed
		// Note: TransitionStatus is skipped here because the caller (CancelTeam)
		// has already transitioned the status to cancelled. Double-writing is
		// unnecessary and wasteful.
		s.scheduleDependentTeams(ctx, spiritSessionID, team)
		result, searchErr := s.sessions.Search(ctx, biz.SessionSearchQuery{TeamID: teamID, Limit: 10})
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
		envType = event.EnvelopeTypeSpiritTeamFailed
		if _, updateErr := s.team.TeamUC.TransitionStatus(ctx, teamID, biz.TeamStatusFailed); updateErr != nil {
			s.lg.Warn("更新团队状态为 failed 失败",
				loggateway.StepID("spirit.team.failed_err"),
				loggateway.Str("team_id", teamID),
				loggateway.Err(updateErr),
			)
		}
		// Schedule dependent teams so they can detect the failure and cascade cancel.
		s.scheduleDependentTeams(ctx, spiritSessionID, team)
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
		if status == biz.TeamStatusFailed || status == biz.TeamStatusCancelled {
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
	s.team.SpiritUC.RecordTeamCompletion(ctx, team, durationMs)
}

func (s *TeamStarter) scheduleDependentTeams(ctx context.Context, spiritSessionID string, completedTeam biz.Team) {
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
				if s.bus != nil {
					env := event.NewEnvelope(event.EnvelopeTypeSpiritTeamFailed, "spirit-scheduler", spiritSessionID)
					env.TeamID = action.TeamID
					env.Metadata = map[string]any{
						"team_id":   action.TeamID,
						"team_name": action.TeamName,
						"error":     action.Reason,
					}
					s.bus.Publish(ctx, env)
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
			if s.bus != nil {
				env := event.NewEnvelope(event.EnvelopeTypeSpiritTeamProgress, "spirit-scheduler", spiritSessionID)
				env.TeamID = action.TeamID
				env.Metadata = map[string]any{
					"team_id":   action.TeamID,
					"team_name": action.TeamName,
					"status":    biz.TeamStatusRunning,
				}
				s.bus.Publish(ctx, env)
			}
			taskDesc := action.TeamName // DagNodeID is used as task description fallback
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
	if s.bus != nil {
		env := event.NewEnvelope(event.EnvelopeTypeSpiritTeamsAllCompleted, "team-starter", spiritSessionID)
		env.Metadata = map[string]any{
			"spirit_session_id": spiritSessionID,
			"team_ids":          result.TeamIDs,
		}
		s.bus.Publish(ctx, env)
	}
}

// HandleTeamTimeout implements biz.TimeoutHandler. Called when a team times out
// to trigger dependency scheduling, event publishing, and AllDone checks — the
// same lifecycle as HandleTeamTurnResult for a failed team.
func (s *TeamStarter) HandleTeamTimeout(ctx context.Context, spiritSessionID, teamID string) {
	s.HandleTeamTurnResult(ctx, spiritSessionID, teamID, biz.TeamStatusFailed, "team execution timed out")
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
	spiritSessionID := strings.TrimSpace(team.SpiritSessionID)
	if a.bus != nil && spiritSessionID != "" {
		env := event.NewEnvelope(event.EnvelopeTypeSpiritTeamProgress, "spirit-cancel", spiritSessionID)
		env.TeamID = teamID
		env.Metadata = map[string]any{
			"team_id":   teamID,
			"team_name": team.DisplayName,
			"status":    biz.TeamStatusCancelled,
		}
		a.bus.Publish(ctx, env)
	}
	if a.teamStarter != nil && spiritSessionID != "" {
		a.teamStarter.HandleTeamTurnResult(ctx, spiritSessionID, teamID, biz.TeamStatusCancelled, "")
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


