package service

import (
	"context"
	stderrors "errors"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

var (
	_ tools.SpiritTeamAssemblerPort  = (*SpiritTeamAssembler)(nil)
	_ tools.SpiritTeamQueryPort      = (*SpiritTeamAssembler)(nil)
	_ tools.SpiritTeamControllerPort = (*SpiritTeamAssembler)(nil)
	_ biz.TeamStarterPort            = (*TeamStarter)(nil)
	_ biz.TimeoutHandler             = (*TeamStarter)(nil)
	_ biz.AllTeamsCompletedNotifier  = (*TeamStarter)(nil)
)

type TeamStarter struct {
	sessions         *biz.SessionUsecase
	team             TeamOrchestrationDeps
	bus              biz.ActivityEventBus
	activityReader   biz.ActivityReader
	activityUpserter biz.ActivityUpserter
	lg               loggateway.Logger
	// turnGateway is used to inject a synthetic message into the Spirit
	// session when all teams complete, triggering the LLM to continue
	// with synthesis. This replaces the LLM-polling pattern (check_progress
	// tool) with a system-push pattern.
	turnGateway biz.TurnGateway
	// synthesisSvc is used to synthesize team results when all teams
	// complete. The synthesis output is injected as the content of the
	// turn message sent via turnGateway.
	synthesisSvc *SpiritSynthesisService
	// synthesisTriggered guards against concurrent duplicate synthesis
	// triggers per spirit session. checkAllTeamsCompleted is called from
	// both HandleTeamTurnResult (goroutine) and NotifyAllTeamsCompleted
	// (background poller goroutine). LoadOrStore ensures only one synthesis
	// message is injected per spirit session lifecycle.
	// Keyed by spiritSessionID so concurrent spirit sessions don't interfere.
	synthesisTriggered sync.Map

	// planExecutor is the v2 forward DAG scheduler that replaces the
	// reverse-sync updatePlanStepForTeam method. Injected via SetPlanExecutor
	// (post-construction) to avoid a Wire cycle: PlanExecutor's TeamOrchestrator
	// dependency is satisfied by SpiritTeamAssembler, which itself depends on
	// TeamStarter. May be nil in v1-only deployments.
	planExecutor *PlanExecutor
}

func NewTeamStarter(
	sessions *biz.SessionUsecase,
	team TeamOrchestrationDeps,
	bus biz.ActivityEventBus,
	activityReader biz.ActivityReader,
	activityUpserter biz.ActivityUpserter,
	lg loggateway.Logger,
	synthesisSvc *SpiritSynthesisService,
) *TeamStarter {
	return &TeamStarter{
		sessions:         sessions,
		team:             team,
		bus:              bus,
		activityReader:   activityReader,
		activityUpserter: activityUpserter,
		lg:               lg,
		synthesisSvc:     synthesisSvc,
	}
}

// SetTurnGateway injects the turn gateway after construction to break the
// Wire cycle: ChatService → TeamStarterPort → TurnGateway → ChatService.
// turnGateway is used in checkAllTeamsCompleted for the system-push pattern
// (synthesize results → inject message into Spirit session).
func (s *TeamStarter) SetTurnGateway(gw biz.TurnGateway) {
	s.turnGateway = gw
}

// SetPlanExecutor injects the v2 forward DAG scheduler after construction.
// This breaks a Wire cycle: PlanExecutor → TeamOrchestrator → SpiritTeamAssembler
// → TeamStarter. May be nil in v1-only deployments (the reverse-sync
// updatePlanStepForTeam method has been removed; v1 plan steps are no longer
// auto-updated by team completion — the v2 PlanExecutor handles this when wired).
func (s *TeamStarter) SetPlanExecutor(pe *PlanExecutor) {
	s.planExecutor = pe
}

func (s *TeamStarter) StartTeamTurn(ctx context.Context, sessionID string, content string) error {
	if s.team.TeamsNative == nil {
		return apierror.Internal("SPIRIT", "team runner not available")
	}
	sess, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return apierror.NotFound("SPIRIT", "team session not found")
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

	// Fetch the team so we can include team_name in team_stage events. Without
	// team_name, the frontend TeamCard's displayTeamName degrades to generic
	// status text ("assembling"). The fetch is cheap (single DB row by ID).
	var team biz.Team
	if teamID != "" {
		var updateErr error
		team, updateErr = s.team.TeamUC.TransitionStatus(ctx, teamID, biz.TeamStatusRunning)
		if updateErr != nil {
			s.lg.Warn("更新团队状态为 running 失败",
				loggateway.StepID("spirit.team.running_err"),
				loggateway.Str("team_id", teamID),
				loggateway.Err(updateErr),
			)
			// Fallback: try plain Get so we still have DisplayName for events.
			if t, getErr := s.team.TeamUC.Get(ctx, teamID); getErr == nil {
				team = t
			}
		}
	}

	if s.bus != nil && teamID != "" && spiritSessionID != "" {
		// Derive DependsOn from the team fetched above. The assembled event
		// (published by publishSpiritTeamAssembled) carries the full member
		// list and DependsOn, but the progress event is a separate publish.
		// Without DependsOn here, the WS event to the frontend doesn't carry
		// it, and the version-guarded UpsertActivity may clear the stored
		// DependsOn if the async persist races with the assembled event.
		dependsOn := team.DependsOn
		ev := biz.ActivityEvent{
			Event: biz.ActivityEventUpdated,
			Activity: biz.Activity{
				ID:               agent.TeamStageActivityID(teamID),
				Kind:             biz.ActivityKindTeamStage,
				Status:           biz.ActivityStatusRunning,
				Stage:            "progress",
				Timestamp:        time.Now().UTC(),
				ParentActivityID: agent.GraphStageActivityID(spiritSessionID),
				SpiritSessionID:  spiritSessionID,
				TeamID:           teamID,
				AgentKey:         "team-starter",
				DependsOn:        dependsOn,
				Meta: map[string]any{
					"team_id":      teamID,
					"team_name":    team.DisplayName,
					"status":       biz.TeamStatusRunning,
					"progress_pct": 0,
					"depends_on":   dependsOn,
				},
			},
			Domain: biz.ActivityDomainChat,
		}
		s.bus.Publish(ctx, ev)
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
			s.HandleTeamTurnResult(ctx, spiritSessionID, teamID, biz.TeamStatusFailed, err.Error(), "")
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
		s.HandleTeamTurnResult(ctx, spiritSessionID, teamID, biz.TeamStatusCompleted, "", "")
	}
	return nil
}

func (s *TeamStarter) HandleTeamTurnResult(ctx context.Context, spiritSessionID, teamID, status, errMsg string, chatSessionID string) {
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

	// Extract token usage from the team's session.
	// Assumption: each team is associated with exactly one session; Limit:1 is sufficient.
	var tokenIn, tokenOut int
	if sessResult, searchErr := s.sessions.Search(ctx, biz.SessionSearchQuery{TeamID: teamID, Limit: 1}); searchErr == nil && len(sessResult.Items) > 0 {
		tokenIn = sessResult.Items[0].InputTokens
		tokenOut = sessResult.Items[0].OutputTokens
	}

	if status == biz.TeamStatusCompleted {
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
		// Note: TransitionStatus is skipped here because the caller (CancelTeam)
		// has already transitioned the status to cancelled. Double-writing is
		// unnecessary and wasteful.
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
		primaryEvent := biz.ActivityEventFailed
		primaryStatus := biz.ActivityStatusFailed
		primaryStage := "failed"
		switch status {
		case biz.TeamStatusCompleted:
			primaryEvent = biz.ActivityEventCompleted
			primaryStatus = biz.ActivityStatusCompleted
			primaryStage = "completed"
		case biz.TeamStatusCancelled:
			primaryEvent = biz.ActivityEventCancelled
			primaryStatus = biz.ActivityStatusCancelled
			primaryStage = "cancelled"
		default:
			primaryEvent = biz.ActivityEventFailed
			primaryStatus = biz.ActivityStatusFailed
			primaryStage = "failed"
		}
		// Carry DependsOn from the team so the frontend can render DAG
		// edges and the database preserves the dependency graph across
		// status updates (assembled → progress → completed/failed/cancelled).
		dependsOn := team.DependsOn
		// progress_pct: 100 when completed, 0 otherwise. Without this, the
		// terminal event meta lacks progress_pct and the frontend TeamCard
		// shows 0% even after the team finishes. The assembled event sets
		// progress_pct=0; the dedup logic in useActivityTimeline.ts skips
		// 0 values when merging meta, so a non-zero value here overrides.
		progressPct := 0
		if status == biz.TeamStatusCompleted {
			progressPct = 100
		}
		meta := map[string]any{
			"team_id":         teamID,
			"team_name":       team.DisplayName,
			"status":          status,
			"duration_ms":     durationMs,
			"total_token_in":  tokenIn,
			"total_token_out": tokenOut,
			"depends_on":      dependsOn,
			"progress_pct":    progressPct,
		}
		if errMsg != "" {
			meta["error"] = errMsg
		}
		ev := biz.ActivityEvent{
			Event: primaryEvent,
			Activity: biz.Activity{
				ID:               agent.TeamStageActivityID(teamID),
				Kind:             biz.ActivityKindTeamStage,
				Status:           primaryStatus,
				Stage:            primaryStage,
				Timestamp:        time.Now().UTC(),
				ParentActivityID: agent.GraphStageActivityID(spiritSessionID),
				SpiritSessionID:  spiritSessionID,
				TeamID:           teamID,
				AgentKey:         "spirit-lifecycle",
				DependsOn:        dependsOn,
				Meta:             meta,
			},
			Domain: biz.ActivityDomainChat,
		}
		s.bus.Publish(ctx, ev)

		// BUGFIX: the progress event (Status=Running) was published AFTER the
		// primary terminal event, causing the version-guarded UpsertActivity to
		// overwrite the terminal status (completed/failed/cancelled) with
		// "Running". Skip the progress event for terminal statuses — the primary
		// event already carries the correct status, and the frontend dedup logic
		// merges meta from all events of the same team.
		if status == biz.TeamStatusRunning {
			progressEv := biz.ActivityEvent{
				Event: biz.ActivityEventUpdated,
				Activity: biz.Activity{
					ID:               agent.TeamStageActivityID(teamID),
					Kind:             biz.ActivityKindTeamStage,
					Status:           biz.ActivityStatusRunning,
					Stage:            "progress",
					Timestamp:        time.Now().UTC(),
					ParentActivityID: agent.GraphStageActivityID(spiritSessionID),
					SpiritSessionID:  spiritSessionID,
					TeamID:           teamID,
					AgentKey:         "spirit-lifecycle",
					DependsOn:        dependsOn,
					Meta: map[string]any{
						"team_id":      teamID,
						"team_name":    team.DisplayName,
						"status":       status,
						"progress_pct": 0,
						"depends_on":   dependsOn,
					},
				},
				Domain: biz.ActivityDomainChat,
			}
			s.bus.Publish(ctx, progressEv)
		}

		// B.4.4: After team status change, republish the graph_stage DAG
		// snapshot so node statuses stay in sync with team statuses.
		if allTeams, listErr := s.team.TeamUC.ListBySpiritSessionID(ctx, spiritSessionID); listErr == nil {
			publishGraphStageSnapshot(ctx, s.bus, s.lg, spiritSessionID, allTeams)
		} else {
			s.lg.Warn("graph_stage 快照刷新失败：列出团队失败",
				loggateway.StepID("spirit.graph_stage.list_fail"),
				loggateway.Str("spirit_session_id", spiritSessionID),
				loggateway.Err(listErr),
			)
		}

	}

	s.checkAllTeamsCompleted(ctx, spiritSessionID)

	// Trigger auto-archive for completed/failed/cancelled teams that have
	// exceeded the configured threshold. This is the primary call site for
	// AutoArchiveCompletedTeams — it runs on every team lifecycle event so
	// no separate worker/cron is needed.
	s.team.SpiritUC.AutoArchiveCompletedTeams(ctx, spiritSessionID)
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
					ev := biz.ActivityEvent{
						Event: biz.ActivityEventFailed,
						Activity: biz.Activity{
							ID:               agent.TeamStageActivityID(action.TeamID),
							Kind:             biz.ActivityKindTeamStage,
							Status:           biz.ActivityStatusFailed,
							Stage:            "failed",
							Timestamp:        time.Now().UTC(),
							ParentActivityID: agent.RootTaskActivityIDFromCtx(ctx),
							SpiritSessionID:  spiritSessionID,
							TeamID:           action.TeamID,
							AgentKey:         "spirit-scheduler",
							Meta: map[string]any{
								"team_id":   action.TeamID,
								"team_name": action.TeamName,
								"error":     action.Reason,
							},
						},
						Domain: biz.ActivityDomainChat,
					}
					s.bus.Publish(ctx, ev)
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
				ev := biz.ActivityEvent{
					Event: biz.ActivityEventUpdated,
					Activity: biz.Activity{
						ID:               agent.TeamStageActivityID(action.TeamID),
						Kind:             biz.ActivityKindTeamStage,
						Status:           biz.ActivityStatusRunning,
						Stage:            "progress",
						Timestamp:        time.Now().UTC(),
						ParentActivityID: agent.RootTaskActivityIDFromCtx(ctx),
						SpiritSessionID:  spiritSessionID,
						TeamID:           action.TeamID,
						AgentKey:         "spirit-scheduler",
						Meta: map[string]any{
							"team_id":   action.TeamID,
							"team_name": action.TeamName,
							"status":    biz.TeamStatusRunning,
						},
					},
					Domain: biz.ActivityDomainChat,
				}
				s.bus.Publish(ctx, ev)
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
	if s.synthesisSvc != nil && s.turnGateway != nil {
		if _, alreadyTriggered := s.synthesisTriggered.LoadOrStore(spiritSessionID, true); !alreadyTriggered {
			synthesisMsg := s.buildSynthesisMessage(ctx, spiritSessionID)
			if synthesisMsg != "" {
				if _, err := s.turnGateway.ExecuteTurn(ctx, biz.TurnInput{
					SessionID: spiritSessionID,
					Content:   synthesisMsg,
				}); err != nil {
					s.lg.Warn("all teams completed: failed to trigger Spirit synthesis turn",
						loggateway.StepID("spirit.synthesis_turn_err"),
						loggateway.Str("spirit_session_id", spiritSessionID),
						loggateway.Err(err),
					)
				} else {
					s.lg.Info("all teams completed: triggered Spirit synthesis turn",
						loggateway.StepID("spirit.synthesis_turn_triggered"),
						loggateway.Str("spirit_session_id", spiritSessionID),
						loggateway.Int("total_teams", result.TotalTeams),
						loggateway.Int("completed_teams", result.CompletedTeams),
						loggateway.Int("failed_teams", result.FailedTeams),
					)
				}
			}
		}
	}

	// Publish the completion event for frontend awareness (non-blocking).
	if s.bus != nil {
		ev := biz.ActivityEvent{
			Event: biz.ActivityEventCompleted,
			Activity: biz.Activity{
				ID:              uuid.NewString(),
				Kind:            biz.ActivityKindNotice,
				Status:          biz.ActivityStatusCompleted,
				Timestamp:       time.Now().UTC(),
				SpiritSessionID: spiritSessionID,
				AgentKey:        "team-starter",
				AgentName:       "团队编排",
				Stage:           "teams_all_completed",
				Content:         "所有团队已完成",
				Meta: map[string]any{
					"event_type":        "spirit_teams_all_completed",
					"spirit_session_id": spiritSessionID,
					"team_ids":          result.TeamIDs,
					"total_teams":       result.TotalTeams,
					"completed_teams":   result.CompletedTeams,
					"failed_teams":      result.FailedTeams,
					"total_token_in":    result.TotalTokenIn,
					"total_token_out":   result.TotalTokenOut,
					"notice_type":       "success",
				},
			},
			Domain: biz.ActivityDomainChat,
		}
		s.bus.Publish(ctx, ev)
	}
}

// buildSynthesisMessage synthesizes team results and formats them as a message
// for the Spirit session. Returns empty string if synthesis fails or produces
// no content.
func (s *TeamStarter) buildSynthesisMessage(ctx context.Context, spiritSessionID string) string {
	output, err := s.synthesisSvc.SynthesizeResults(ctx, spiritSessionID, "")
	if err != nil {
		s.lg.Warn("all teams completed: synthesis failed",
			loggateway.StepID("spirit.synthesis_fail"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(err),
		)
		return ""
	}
	if output == nil || output.Content == "" {
		s.lg.Warn("all teams completed: synthesis returned empty content",
			loggateway.StepID("spirit.synthesis_empty"),
			loggateway.Str("spirit_session_id", spiritSessionID),
		)
		return ""
	}
	return "所有团队已完成任务。以下是综合结果：\n\n" + output.Content + "\n\n请基于以上结果给出最终总结和分析。"
}

// HandleTeamTimeout implements biz.TimeoutHandler. Called when a team times out
// to trigger dependency scheduling, event publishing, and AllDone checks — the
// same lifecycle as HandleTeamTurnResult for a failed team.
func (s *TeamStarter) HandleTeamTimeout(ctx context.Context, spiritSessionID, teamID string) {
	s.HandleTeamTurnResult(ctx, spiritSessionID, teamID, biz.TeamStatusFailed, "team execution timed out", "")
}

// NotifyAllTeamsCompleted implements biz.AllTeamsCompletedNotifier. Called by the
// background poller when all teams for a spirit session have reached terminal
// state. This is the "active notification" path from the backend polling
// mechanism, supplementing the event-driven path in HandleTeamTurnResult.
func (s *TeamStarter) NotifyAllTeamsCompleted(ctx context.Context, spiritSessionID string) {
	s.checkAllTeamsCompleted(ctx, spiritSessionID)
}

type SpiritTeamAssembler struct {
	spiritUC     *biz.SpiritTeamUsecase
	orchCache    *biz.OrchestrationCache
	bus          biz.ActivityEventBus
	activityRepo biz.ActivityUpserter
	teamStarter  biz.TeamStarterPort
	agentReader  biz.AgentReader
	lg           loggateway.Logger
}

func NewSpiritTeamAssembler(
	spiritUC *biz.SpiritTeamUsecase,
	orchCache *biz.OrchestrationCache,
	bus biz.ActivityEventBus,
	activityRepo biz.ActivityUpserter,
	teamStarter biz.TeamStarterPort,
	agentReader biz.AgentReader,
	lg loggateway.Logger,
) *SpiritTeamAssembler {
	return &SpiritTeamAssembler{
		spiritUC:     spiritUC,
		orchCache:    orchCache,
		bus:          bus,
		activityRepo: activityRepo,
		teamStarter:  teamStarter,
		agentReader:  agentReader,
		lg:           lg,
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
		ev := biz.ActivityEvent{
			Event: biz.ActivityEventCancelled,
			Activity: biz.Activity{
				ID:               agent.TeamStageActivityID(teamID),
				Kind:             biz.ActivityKindTeamStage,
				Status:           biz.ActivityStatusCancelled,
				Stage:            "cancelled",
				Timestamp:        time.Now().UTC(),
				ParentActivityID: agent.RootTaskActivityIDFromCtx(ctx),
				SpiritSessionID:  spiritSessionID,
				TeamID:           teamID,
				AgentKey:         "spirit-cancel",
				Meta: map[string]any{
					"team_id":   teamID,
					"team_name": team.DisplayName,
					"status":    biz.TeamStatusCancelled,
				},
			},
			Domain: biz.ActivityDomainChat,
		}
		a.bus.Publish(ctx, ev)
	}
	if a.teamStarter != nil && spiritSessionID != "" {
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
		if a.bus != nil {
			a.bus.Publish(ctx, biz.ActivityEvent{
				Event: biz.ActivityEventCreated,
				Activity: biz.Activity{
					ID:        uuid.NewString(),
					Kind:      biz.ActivityKindNotice,
					Status:    biz.ActivityStatusCompleted,
					Timestamp: time.Now().UTC(),
					AgentKey:  "spirit-team-assembler",
					Meta: map[string]any{
						"task_pattern": biz.ExtractTaskPattern(taskDescription),
						"topology":     string(topology),
					},
				},
				Domain: biz.ActivityDomainChat,
			})
		}
	}
	return string(topology), found
}

func (a *SpiritTeamAssembler) publishSpiritTeamAssembled(ctx context.Context, spiritSessionID string, team biz.Team, teamSession biz.Session, mode, taskDesc, topologyReason string, agentKeys []string, memberSessions map[string]string) {
	if a.bus == nil {
		return
	}
	// B.4.4: Publish graph_stage snapshot BEFORE team_stage so the graph appears
	// between plan and team-card in the timeline (pure Timestamp ASC sort, design
	// B.3.3). The graph_stage Activity uses a deterministic ID per spirit session,
	// so the first publish establishes the creation Timestamp (which the frontend
	// preserves across subsequent updates — see useActivityTimeline timestamp
	// retention). Publishing graph before team_stage ensures:
	//   plan (T_plan) → graph_stage (T_graph) → team_stage (T_team)
	// If published after team_stage, the order would be plan → team → graph,
	// violating B.4.4's positional requirement.
	//
	// The team is already committed to the DB (AssembleTeam runs in a tx that
	// has committed before this method is called), so ListAllTeams will include
	// the new team as a pending node in the DAG snapshot.
	a.publishSpiritGraphStageSnapshot(ctx, spiritSessionID)

	// Build members array so the frontend TeamCard can render the member list.
	// Problem 2 fix: previously agent_name was set to agent_key (showing raw
	// kebab-case keys like "deep-researcher" in the UI). Now we batch-resolve
	// each agent_key to DisplayName via AgentReader. Lookup failures fall back
	// to the agent_key so we never block team assembly on a stale key — the
	// richer TeamSummaryActivityEvent emitted at run completion will still
	// carry authoritative member data.
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
	// members/depends_on). The Bus's async persistence goroutine for the
	// assembled event may execute after the progress event's goroutine, causing
	// the version-guarded update to be rejected and members/depends_on to be
	// lost. Synchronous persistence guarantees the assembled event is always
	// stored first, so the progress event's UpsertActivity can merge its partial
	// Meta with the existing members/depends_on.
	//
	// Issue 3 fix: If sync persistence fails, fall back to Bus async persist
	// (SequencerHandled=false). Previously the Bus event always had
	// SequencerHandled=true, meaning a sync failure = permanent data loss.
	// Now the Bus provides a fallback path.
	persistedSync := false
	if a.activityRepo != nil {
		activity := biz.Activity{
			ID:               agent.TeamStageActivityID(team.ID),
			Kind:             biz.ActivityKindTeamStage,
			Status:           biz.ActivityStatusPending,
			Stage:            "assembled",
			Timestamp:        time.Now().UTC(),
			ParentActivityID: agent.GraphStageActivityID(spiritSessionID),
			SpiritSessionID:  spiritSessionID,
			SessionID:        spiritSessionID,
			TeamID:           team.ID,
			AgentKey:         "spirit-team-assembler",
			DependsOn:        team.DependsOn,
			Meta: map[string]any{
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
				"members":         members,
			},
		}
		if _, err := a.activityRepo.UpsertActivity(ctx, activity); err != nil {
			a.lg.Warn("spirit team assembled: synchronous persist failed, falling back to Bus async",
				loggateway.StepID("spirit.team.assembled_persist"),
				loggateway.Str("team_id", team.ID),
				loggateway.Err(err),
			)
		} else {
			persistedSync = true
		}
	}

	// Issue 1 fix: Set SequencerHandled=true when sync persist succeeded so the
	// Bus skips async persist. Without this flag, bus.Publish triggers an async
	// UpsertActivity that can race with StartTeamTurn's progress event: the async
	// persist may execute AFTER the progress event and overwrite the Running
	// status back to Pending, because both use the bus's monotonic versionSeq and
	// the assembled event gets a higher version number.
	// Issue 3 fix: If sync persist failed, set SequencerHandled=false so the Bus
	// also tries to persist (fallback path).
	a.bus.Publish(ctx, biz.ActivityEvent{
		Event:            biz.ActivityEventCreated,
		SequencerHandled: persistedSync, // skip Bus persist only if sync succeeded
		Activity: biz.Activity{
			ID:               agent.TeamStageActivityID(team.ID),
			Kind:             biz.ActivityKindTeamStage,
			Status:           biz.ActivityStatusPending,
			Stage:            "assembled",
			Timestamp:        time.Now().UTC(),
			ParentActivityID: agent.GraphStageActivityID(spiritSessionID),
			SpiritSessionID:  spiritSessionID,
			SessionID:        spiritSessionID,
			TeamID:           team.ID,
			AgentKey:         "spirit-team-assembler",
			DependsOn:        team.DependsOn,
			Meta: map[string]any{
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
				"members":         members,
			},
		},
		Domain: biz.ActivityDomainChat,
	})
}

// publishSpiritGraphStageSnapshot publishes a graph_stage Activity carrying
// the current DAG snapshot (meta.nodes) for a spirit session. The Activity ID
// is derived deterministically from spiritSessionID via agent.GraphStageActivityID so every
// call for the same session updates the same GraphStageBlock on the frontend
// (no dedup logic needed — the upsert keyed by Activity.ID naturally merges).
//
// Design B.4.4 (方案A: backend aggregates DAG snapshot):
//   - Each node corresponds to one team (nodeId = team.ID, label = team.DisplayName)
//   - Node status is derived from team status (no independent node state machine)
//   - dependsOn mirrors team.DependsOn for DAG edges
//   - When team count == 1 (no DAG), frontend may hide Graph per design — still
//     publish so the data is available
//
// Called from publishSpiritTeamAssembled (after team creation) and
// HandleTeamTurnResult (after team status change) to keep the snapshot fresh.
func (a *SpiritTeamAssembler) publishSpiritGraphStageSnapshot(ctx context.Context, spiritSessionID string) {
	if a.bus == nil || spiritSessionID == "" || a.spiritUC == nil {
		return
	}
	teams, err := a.spiritUC.ListAllTeams(ctx, spiritSessionID)
	if err != nil {
		a.lg.Warn("graph_stage 快照构建失败：列出团队失败",
			loggateway.StepID("spirit.graph_stage.list_fail"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(err),
		)
		return
	}
	publishGraphStageSnapshot(ctx, a.bus, a.lg, spiritSessionID, teams)
}

// publishGraphStageSnapshot is the free-function core of
// publishSpiritGraphStageSnapshot. It accepts an already-fetched team list so
// callers that have direct access to TeamUsecase (e.g. TeamStarter) can reuse
// the logic without depending on SpiritTeamUsecase. See
// publishSpiritGraphStageSnapshot for the full design rationale.
func publishGraphStageSnapshot(ctx context.Context, bus biz.ActivityEventBus, lg loggateway.Logger, spiritSessionID string, teams []biz.Team) {
	if bus == nil || spiritSessionID == "" || len(teams) == 0 {
		return
	}
	nodes := make([]map[string]any, 0, len(teams))
	for _, t := range teams {
		nodeStatus := "pending"
		switch t.Status {
		case biz.TeamStatusRunning:
			nodeStatus = "running"
		case biz.TeamStatusCompleted:
			nodeStatus = "completed"
		case biz.TeamStatusFailed:
			nodeStatus = "failed"
		case biz.TeamStatusCancelled:
			nodeStatus = "skipped"
		case biz.TeamStatusInterrupted:
			nodeStatus = "failed"
		}
		node := map[string]any{
			"nodeId":  t.ID,
			"label":   t.DisplayName,
			"status":  nodeStatus,
			"team_id": t.ID,
		}
		if len(t.DependsOn) > 0 {
			node["dependsOn"] = t.DependsOn
		}
		nodes = append(nodes, node)
	}
	// Aggregate status: running if any team running; completed if all terminal;
	// failed if any failed; else pending.
	aggregateStatus := biz.ActivityStatusCompleted
	anyRunning := false
	anyFailed := false
	allTerminal := true
	for _, t := range teams {
		switch t.Status {
		case biz.TeamStatusRunning, biz.TeamStatusPending:
			allTerminal = false
			if t.Status == biz.TeamStatusRunning {
				anyRunning = true
			}
		case biz.TeamStatusFailed, biz.TeamStatusInterrupted:
			anyFailed = true
		}
	}
	switch {
	case anyRunning:
		aggregateStatus = biz.ActivityStatusRunning
	case anyFailed:
		aggregateStatus = biz.ActivityStatusFailed
	case allTerminal:
		aggregateStatus = biz.ActivityStatusCompleted
	default:
		aggregateStatus = biz.ActivityStatusRunning
	}
	eventType := biz.ActivityEventUpdated
	if aggregateStatus == biz.ActivityStatusCompleted {
		eventType = biz.ActivityEventCompleted
	} else if aggregateStatus == biz.ActivityStatusFailed {
		eventType = biz.ActivityEventFailed
	}
	// Deterministic Activity ID: same spiritSessionID → same ID across calls
	// → frontend upsert updates the existing GraphStageBlock instead of creating
	// a new one each snapshot.
	activityID := agent.GraphStageActivityID(spiritSessionID)
	bus.Publish(ctx, biz.ActivityEvent{
		Event: eventType,
		Activity: biz.Activity{
			ID:               activityID,
			Kind:             biz.ActivityKindGraphStage,
			Status:           aggregateStatus,
			Stage:            "team_dag_snapshot",
			Timestamp:        time.Now().UTC(),
			ParentActivityID: agent.RootTaskActivityIDFromCtx(ctx),
			SpiritSessionID:  spiritSessionID,
			SessionID:        spiritSessionID,
			AgentKey:         "spirit-graph-snapshot",
			Content:          "团队 DAG 执行进度",
			Meta: map[string]any{
				"nodes":             nodes,
				"spirit_session_id": spiritSessionID,
				"source":            "spirit-team-assembler",
				"team_count":        len(teams),
			},
		},
		Domain: biz.ActivityDomainChat,
	})
}
