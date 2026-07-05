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
	rt "aranea-agents/internal/runtime"
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

// TeamStarter manages team turn lifecycle.
//
// Phase 3b-D Task 10: dual-bus scheme. `bus` (v1 ActivityEventBus) is retained
// for graph_stage snapshot events (no v2 EventKind equivalent exists). `eventBus`
// (v2 EventBus) is used for team_stage and notice events. Once a v2
// graph_stage event kind is introduced, `bus` can be removed.
//
// Phase 2: `seq` (v2 Sequencer) routes graph_stage snapshots through the
// unified v2 publish entry (FIFO + retry). `bus` is retained as v1 fallback
// when neither seq nor eventBus is available.
type TeamStarter struct {
	sessions         *biz.SessionUsecase
	team             TeamOrchestrationDeps
	bus              biz.ActivityEventBus // v1: graph_stage snapshot fallback (no v2 equivalent yet)
	eventBus         biz.EventBus         // v2: team_stage + notice events
	seq              rt.EventPublisher    // Phase 2: v2 Sequencer for graph_stage snapshot (FIFO + retry)
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
	// taskV2Reader is used by resolveLatestUserTaskID to look up the most
	// recent user-input Task for the spirit session, so the system-push
	// synthesis Turn can be attached to that existing Task (ParentTaskID)
	// instead of creating a new Task. Design: 2026-07-02-llm-activity-
	// ordering-design §3.2.1.
	taskV2Reader biz.TaskV2Reader
}

func NewTeamStarter(
	sessions *biz.SessionUsecase,
	team TeamOrchestrationDeps,
	bus biz.ActivityEventBus,
	eventBus biz.EventBus,
	seq rt.EventPublisher,
	activityReader biz.ActivityReader,
	activityUpserter biz.ActivityUpserter,
	lg loggateway.Logger,
	synthesisSvc *SpiritSynthesisService,
	taskV2Reader biz.TaskV2Reader,
) *TeamStarter {
	return &TeamStarter{
		sessions:         sessions,
		team:             team,
		bus:              bus,
		eventBus:         eventBus,
		seq:              seq,
		activityReader:   activityReader,
		activityUpserter: activityUpserter,
		lg:               lg,
		synthesisSvc:     synthesisSvc,
		taskV2Reader:     taskV2Reader,
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

// publishV2Event publishes a v2 event via seq (DB persist + WS) when available,
// falling back to eventBus (WS only) otherwise.
// 2026-07-04 问题 2 修复：之前所有 TeamStage/Step 事件用 eventBus.Publish，
// 不经过 sequencer.persistAction，刷新后数据丢失。改为优先用 seq.Publish
// 确保落库；v1-only 部署（seq=nil）保留 eventBus 兜底。
func (s *TeamStarter) publishV2Event(ctx context.Context, e biz.Event) {
	if s.seq != nil {
		s.seq.Publish(ctx, e)
		return
	}
	if s.eventBus != nil {
		s.eventBus.Publish(ctx, e)
	}
}

// publishV2EventAssembler is the SpiritTeamAssembler counterpart of
// publishV2Event. Same semantics: seq first (persist + WS), eventBus fallback.
func (a *SpiritTeamAssembler) publishV2Event(ctx context.Context, e biz.Event) {
	if a.seq != nil {
		a.seq.Publish(ctx, e)
		return
	}
	if a.eventBus != nil {
		a.eventBus.Publish(ctx, e)
	}
}

// v2EventReady returns true if either seq or eventBus is wired.
// Used to guard TeamStage/Step publish blocks (replacing bare eventBus nil-check).
func (s *TeamStarter) v2EventReady() bool {
	return s.seq != nil || s.eventBus != nil
}

// v2EventReady is the SpiritTeamAssembler counterpart.
func (a *SpiritTeamAssembler) v2EventReady() bool {
	return a.seq != nil || a.eventBus != nil
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

	if s.v2EventReady() && teamID != "" && spiritSessionID != "" {
		// Phase 3b-D Task 10: migrated to v2 NewTeamStageUpdatedEvent.
		// DependsOn is carried by the TeamStage entity (type-safe).
		// 2026-07-04 问题 3 修复：携带 TeamName 让前端展示团队名称而非 ID；
		// 同时把 Stage 从字面量 "progress"（不在枚举内）改为枚举常量
		// TeamStageStageExecuting。
		// 2026-07-04 问题 2 修复：改用 publishV2Event（seq 优先持久化）。
		dependsOn := team.DependsOn
		ts := biz.TeamStage{
			ID:        string(agent.NewTeamStageActivityID(teamID)),
			TeamID:    teamID,
			TeamName:  team.DisplayName,
			SessionID: spiritSessionID,
			Status:    biz.TeamStageStatusRunning,
			Stage:     biz.TeamStageStageExecuting,
			DependsOn: dependsOn,
			StartedAt: time.Now().UTC(),
		}
		s.publishV2Event(ctx, biz.NewTeamStageUpdatedEvent(ts))
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
	if rtID := string(agent.RootTaskActivityIDFromCtx(ctx)); rtID == "" {
		s.lg.Warn("HandleTeamTurnResult: RootTaskActivityID 为空，v2 TeamRun/MemberSession 将无法关联到根 Task",
			loggateway.StepID("spirit.handle_team_turn_result.empty_root_task"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Str("team_id", teamID),
			loggateway.Str("status", string(status)),
		)
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

	// Phase 3b-D Task 10: token usage extraction (tokenIn/tokenOut) was removed.
	// The v1 Activity.Meta carried total_token_in/total_token_out, but v2 TeamStage
	// has no Meta field. The token data is still available in the session record
	// (InputTokens/OutputTokens) and via recordTeamCompletion. Removing the
	// session Search call also eliminates a DB query per team turn completion.

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

	// 2026-07-04 问题 4 修复：将 primaryStatus 提取到 eventBus 块外，
	// 让 publishV2TeamRunCompletion 也能使用（该函数仅依赖 seq，不依赖 eventBus）。
	primaryStatus := biz.TeamStageStatusFailed
	switch status {
	case biz.TeamStatusCompleted:
		primaryStatus = biz.TeamStageStatusCompleted
	case biz.TeamStatusCancelled:
		primaryStatus = biz.TeamStageStatusCancelled
	default:
		primaryStatus = biz.TeamStageStatusFailed
	}

	if s.v2EventReady() {
		// Phase 3b-D Task 10: migrated to v2 TeamStage events.
		// Map v1 status → v2 TeamStageStatus + factory function.
		// 2026-07-04 问题 2 修复：改用 publishV2Event（seq 优先持久化）。
		primaryStage := "failed"
		switch status {
		case biz.TeamStatusCompleted:
			primaryStage = "completed"
		case biz.TeamStatusCancelled:
			primaryStage = "cancelled"
		default:
			primaryStage = "failed"
		}
		// Carry DependsOn from the team so the frontend can render DAG
		// edges and the database preserves the dependency graph across
		// status updates (assembled → progress → completed/failed/cancelled).
		// 2026-07-04 问题 3 修复：携带 TeamName 让前端展示团队名称而非 ID。
		dependsOn := team.DependsOn
		ts := biz.TeamStage{
			ID:        string(agent.NewTeamStageActivityID(teamID)),
			TeamID:    teamID,
			TeamName:  team.DisplayName,
			SessionID: spiritSessionID,
			Status:    primaryStatus,
			Stage:     biz.TeamStageStage(primaryStage),
			DependsOn: dependsOn,
			StartedAt: time.Now().UTC(),
		}
		// DATA LOSS: v2 TeamStage has no Meta field, so team_name/duration_ms/
		// total_token_in/total_token_out/progress_pct/error from v1 Meta are
		// dropped. The duration/token metrics are still recorded via
		// recordTeamCompletion above and are available in the TeamRun DB record.
		switch primaryStatus {
		case biz.TeamStageStatusCompleted:
			s.publishV2Event(ctx, biz.NewTeamStageCompletedEvent(ts))
		case biz.TeamStageStatusFailed:
			s.publishV2Event(ctx, biz.NewTeamStageFailedEvent(ts))
		default:
			// cancelled: no NewTeamStageCancelledEvent factory exists; use
			// NewTeamStageUpdatedEvent as the closest semantic match.
			s.publishV2Event(ctx, biz.NewTeamStageUpdatedEvent(ts))
		}

		// BUGFIX: the progress event (Status=Running) was published AFTER the
		// primary terminal event, causing the version-guarded UpsertActivity to
		// overwrite the terminal status (completed/failed/cancelled) with
		// "Running". Skip the progress event for terminal statuses — the primary
		// event already carries the correct status, and the frontend dedup logic
		// merges meta from all events of the same team.
		if status == biz.TeamStatusRunning {
			progressTs := biz.TeamStage{
				ID:        string(agent.NewTeamStageActivityID(teamID)),
				TeamID:    teamID,
				TeamName:  team.DisplayName,
				SessionID: spiritSessionID,
				Status:    biz.TeamStageStatusRunning,
				Stage:     biz.TeamStageStageExecuting,
				DependsOn: dependsOn,
				StartedAt: time.Now().UTC(),
			}
			s.publishV2Event(ctx, biz.NewTeamStageUpdatedEvent(progressTs))
		}

		// B.4.4: After team status change, republish the graph_stage DAG
		// snapshot so node statuses stay in sync with team statuses.
		// Phase 3b-D Task 10: graph_stage has no v2 EventKind equivalent;
		// stays on v1 bus. TODO: migrate once EventKindGraphStage* is added.
		if allTeams, listErr := s.team.TeamUC.ListBySpiritSessionID(ctx, spiritSessionID); listErr == nil {
			publishGraphStageSnapshot(ctx, s.seq, s.eventBus, s.bus, s.lg, spiritSessionID, allTeams)
		} else {
			s.lg.Warn("graph_stage 快照刷新失败：列出团队失败",
				loggateway.StepID("spirit.graph_stage.list_fail"),
				loggateway.Str("spirit_session_id", spiritSessionID),
				loggateway.Err(listErr),
			)
		}

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
		success := status == biz.TeamStatusCompleted
		s.planExecutor.NotifyTeamCompletion(teamID, success, errMsg)
	}

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
				if s.v2EventReady() {
					// Phase 3b-D Task 10: migrated to v2 NewTeamStageFailedEvent.
					// 2026-07-04 问题 3 修复：携带 TeamName（来自 DependentTeamAction）。
					// 2026-07-04 问题 2 修复：改用 publishV2Event（seq 优先持久化）。
					ts := biz.TeamStage{
						ID:        string(agent.NewTeamStageActivityID(action.TeamID)),
						TeamID:    action.TeamID,
						TeamName:  action.TeamName,
						SessionID: spiritSessionID,
						Status:    biz.TeamStageStatusFailed,
						Stage:     biz.TeamStageStageFailed,
						StartedAt: time.Now().UTC(),
					}
					s.publishV2Event(ctx, biz.NewTeamStageFailedEvent(ts))
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
				ts := biz.TeamStage{
					ID:        string(agent.NewTeamStageActivityID(action.TeamID)),
					TeamID:    action.TeamID,
					TeamName:  action.TeamName,
					SessionID: spiritSessionID,
					Status:    biz.TeamStageStatusRunning,
					Stage:     biz.TeamStageStageExecuting,
					StartedAt: time.Now().UTC(),
				}
				s.publishV2Event(ctx, biz.NewTeamStageUpdatedEvent(ts))
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
				// Resolve the most recent user-input Task for this spirit
				// session so the synthesis Turn attaches as a continuation
				// Turn (ParentTaskID) instead of creating a new Task. This
				// prevents the synthesis trigger text from being rendered as
				// a user-input bubble by TaskCard (spec §3.2.1: a Task
				// corresponds to one user input; system-push Turns are
				// continuation Turns on the same Task).
				parentTaskID := s.resolveLatestUserTaskID(ctx, spiritSessionID)
				if _, err := s.turnGateway.ExecuteTurn(ctx, biz.TurnInput{
					SessionID:    spiritSessionID,
					Content:      synthesisMsg,
					ParentTaskID: parentTaskID,
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
						loggateway.Str("parent_task_id", parentTaskID),
						loggateway.Int("total_teams", result.TotalTeams),
						loggateway.Int("completed_teams", result.CompletedTeams),
						loggateway.Int("failed_teams", result.FailedTeams),
					)
				}
			}
		}
	}

	// Publish the completion event for frontend awareness (non-blocking).
	// Phase 3b-D Task 10: migrated to v2 NewStepCreatedEvent (Kind=StepKindNotice).
	// DATA LOSS: v2 Step has no Meta field, so team_ids/total_teams/completed_teams/
	// failed_teams/total_token_in/total_token_out from v1 Meta are dropped.
	// The notice_type is preserved as Step.NoticeType, and the message as Step.Content.
	if s.v2EventReady() {
		// 2026-07-04 问题 2 修复：改用 publishV2Event（seq 优先持久化）。
		step := biz.Step{
			ID:              uuid.NewString(),
			SessionID:       spiritSessionID,
			SpiritSessionID: spiritSessionID,
			Kind:            biz.StepKindNotice,
			NoticeType:      "success",
			Content:         "所有团队已完成",
			Status:          biz.StepStatusCompleted,
			AuthorAgentKey:  "team-starter",
		}
		s.publishV2Event(ctx, biz.NewStepCreatedEvent(step))
	}
}

// buildSynthesisMessage triggers synthesis and returns a short system-trigger
// prompt for the Spirit LLM. Returns empty string if synthesis fails or
// produces no content.
//
// Design rationale (spec 2026-07-02-llm-activity-ordering-design §3.5.2 step 6
// + §3.5.5 反馈机制表): the synthesis content is delivered to the frontend
// via the synthesis_completed event → SynthesisResultCard. We do NOT inject
// the synthesis content as a user message because:
//
//	(a) TaskCard would misrender the synthesis as user input (the bug being
//	    fixed here), and
//	(b) it would poison SpiritTeamUsecase.GetSpiritQuery on subsequent
//	    syntheses — GetSpiritQuery returns the most recent user message,
//	    which would become the synthesis text itself, recursively corrupting
//	    the synthesis template's {{query}} substitution.
//
// Instead, return a short system-trigger prompt so Spirit LLM generates a
// final summary based on team replies already in the session context.
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
	return "所有团队已完成，请基于已有上下文给出最终总结和分析。"
}

// resolveLatestUserTaskID returns the most recent Task ID for the given spirit
// session, so the system-push synthesis Turn can attach to it as a continuation
// Turn (ParentTaskID). Returns empty string if no Task exists or the reader is
// not wired (v1-only deployments) — in that case the synthesis Turn creates a
// new Task as before (legacy behaviour).
//
// 2026-07-04 问题 5 修复：v2 Sequencer 异步持久化 Task，当所有团队快速完成时
// （例如单团队秒级返回），ListTasksBySession 可能查不到记录，导致 parentTaskID
// 为空，synthesis Turn 在 projector.OnTurnStart 中走"创建新 Task"分支，把
// synthesis 触发文本"所有团队已完成…"渲染成新的用户输入气泡（重复执行）。
// 解决方案：DB 查不到时，从 ctx 中取 RootTaskActivityID（由 chat_orchestrator
// 在 chat_orchestrator_turn.go:401 通过 ContextWithRootTaskActivityID 注入）
// 作为 fallback。RootTaskActivityID 是根 Task 的 ID，与 team 阶段的 ctx 链
// 一脉相承，是可靠的内存态真相源。
func (s *TeamStarter) resolveLatestUserTaskID(ctx context.Context, spiritSessionID string) string {
	if s.taskV2Reader != nil {
		tasks, err := s.taskV2Reader.ListTasksBySession(ctx, spiritSessionID)
		if err != nil {
			s.lg.Warn("resolveLatestUserTaskID: ListTasksBySession failed",
				loggateway.StepID("spirit.synthesis.parent_task_lookup_err"),
				loggateway.Str("spirit_session_id", spiritSessionID),
				loggateway.Err(err),
			)
			// fall through to ctx fallback
		} else if len(tasks) > 0 {
			// tasks are returned in ascending Seq order; the last one is the most
			// recent user-input Task.
			return tasks[len(tasks)-1].ID
		}
	}
	// Fallback: use RootTaskActivityID from ctx (set by chat_orchestrator).
	// This covers the timing gap where the v2 Task hasn't been persisted yet
	// when checkAllTeamsCompleted runs, as well as v1-only deployments where
	// taskV2Reader is nil. RootTaskActivityID is the canonical root task ID
	// for the current spirit session turn chain.
	if rtID := string(agent.RootTaskActivityIDFromCtx(ctx)); rtID != "" {
		s.lg.Info("resolveLatestUserTaskID: fell back to ctx RootTaskActivityID",
			loggateway.StepID("spirit.synthesis.parent_task_ctx_fallback"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Str("root_task_id", rtID),
		)
		return rtID
	}
	return ""
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

// SpiritTeamAssembler assembles spirit teams.
//
// Phase 3b-D Task 10: dual-bus scheme. `bus` (v1) for graph_stage snapshots,
// `eventBus` (v2) for team_stage + notice events.
type SpiritTeamAssembler struct {
	spiritUC     *biz.SpiritTeamUsecase
	orchCache    *biz.OrchestrationCache
	bus          biz.ActivityEventBus // v1: graph_stage snapshot fallback (no v2 equivalent yet)
	eventBus     biz.EventBus         // v2: team_stage + notice events
	seq          rt.EventPublisher    // Phase 2: v2 Sequencer for graph_stage snapshot (FIFO + retry)
	activityRepo biz.ActivityUpserter
	teamStarter  biz.TeamStarterPort
	agentReader  biz.AgentReader
	lg           loggateway.Logger
}

func NewSpiritTeamAssembler(
	spiritUC *biz.SpiritTeamUsecase,
	orchCache *biz.OrchestrationCache,
	bus biz.ActivityEventBus,
	eventBus biz.EventBus,
	seq rt.EventPublisher,
	activityRepo biz.ActivityUpserter,
	teamStarter biz.TeamStarterPort,
	agentReader biz.AgentReader,
	lg loggateway.Logger,
) *SpiritTeamAssembler {
	return &SpiritTeamAssembler{
		spiritUC:     spiritUC,
		orchCache:    orchCache,
		bus:          bus,
		eventBus:     eventBus,
		seq:          seq,
		activityRepo: activityRepo,
		teamStarter:  teamStarter,
		agentReader:  agentReader,
		lg:           lg,
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
	if a.v2EventReady() && spiritSessionID != "" {
		// Phase 3b-D Task 10: migrated to v2 NewTeamStageUpdatedEvent.
		// No NewTeamStageCancelledEvent factory exists; use Updated as the
		// closest semantic match (same as HandleTeamTurnResult cancelled case).
		// 2026-07-04 问题 3 修复：携带 TeamName 让前端展示团队名称而非 ID。
		// 2026-07-04 问题 2 修复：改用 publishV2Event（seq 优先持久化）。
		ts := biz.TeamStage{
			ID:        string(agent.NewTeamStageActivityID(teamID)),
			TeamID:    teamID,
			TeamName:  team.DisplayName,
			SessionID: spiritSessionID,
			Status:    biz.TeamStageStatusCancelled,
			Stage:     "cancelled",
			StartedAt: time.Now().UTC(),
		}
		a.publishV2Event(ctx, biz.NewTeamStageUpdatedEvent(ts))
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
		// Phase 3b-D Task 10: migrated to v2 NewStepCreatedEvent (Kind=StepKindNotice).
		// DATA LOSS: v2 Step has no Meta field, so task_pattern/topology from v1
		// Meta are dropped. The original v1 Activity had no Content/SpiritSessionID;
		// v2 Step also leaves these empty (no spirit session context in SuggestTopology).
		if a.v2EventReady() {
			// 2026-07-04 问题 2 修复：改用 publishV2Event（seq 优先持久化）。
			step := biz.Step{
				ID:             uuid.NewString(),
				Kind:           biz.StepKindNotice,
				Content:        "编排缓存命中，推荐拓扑: " + string(topology),
				Status:         biz.StepStatusCompleted,
				AuthorAgentKey: "spirit-team-assembler",
			}
			a.publishV2Event(ctx, biz.NewStepCreatedEvent(step))
		}
	}
	return string(topology), found
}

func (a *SpiritTeamAssembler) publishSpiritTeamAssembled(ctx context.Context, spiritSessionID string, team biz.Team, teamSession biz.Session, mode, taskDesc, topologyReason string, agentKeys []string, memberSessions map[string]string) {
	// Phase 3b-D Task 10: dual-bus scheme. eventBus (v2) is the primary bus
	// for team_stage events; bus (v1) is retained for graph_stage snapshot only.
	// 2026-07-04 问题 2 修复：放宽守卫为 v2EventReady()（seq 或 eventBus 任一可用），
	// 让 seq-only 部署也能发布 TeamStage 事件并持久化。
	if !a.v2EventReady() {
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
	//
	// Phase 3b-D Task 10: graph_stage has no v2 EventKind equivalent; stays on
	// v1 bus. TODO: migrate once EventKindGraphStage* is added.
	a.publishSpiritGraphStageSnapshot(ctx, spiritSessionID)

	// 2026-07-04 问题 5 修复：在构建 members 数组和发布 v2 MemberSession
	// created 事件之前过滤系统 Agent。之前仅 AssembleTeam 过滤了系统 Agent
	// （不创建 DB session），但 publishSpiritTeamAssembled 仍用原始 agentKeys
	// 发布 MemberSession created 事件，导致系统 Agent 的 MemberSession 记录
	// 永远停在 running 状态（无 DB session → publishV2TeamRunCompletion 搜不到
	// → 不发布 updated 事件）。
	filteredKeys := make([]string, 0, len(agentKeys))
	for _, k := range agentKeys {
		if biz.IsSystemAgentKey(k) {
			continue
		}
		filteredKeys = append(filteredKeys, k)
	}
	agentKeys = filteredKeys

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
			ID:               string(agent.NewTeamStageActivityID(team.ID)),
			Kind:             biz.ActivityKindTeamStage,
			Status:           biz.ActivityStatusPending,
			Stage:            "assembled",
			Timestamp:        time.Now().UTC(),
			ParentActivityID: string(agent.NewGraphStageActivityID(spiritSessionID)),
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

	// Phase 3b-D Task 10: migrated to v2 NewTeamStageCreatedEvent.
	// The sync persist (a.activityRepo.UpsertActivity above) is RETAINED unchanged.
	// DEVIATION: the v1 SequencerHandled flag (skip Bus async persist when sync
	// succeeded) is DROPPED — v2 EventBus has no equivalent. The v2 EventBus may
	// attempt async persist after the sync persist, but the version-guarded
	// UpsertActivity makes the async attempt a no-op if sync already wrote the
	// record. The `persistedSync` variable is now unused for the bus call but
	// retained for the sync persist log above.
	// 2026-07-04 问题 4 修复：设置 TaskID + Members，让前端 TeamStagePanel 能直接
	// 显示 Team ID 和成员列表（之前 Members 为 null，前端无法渲染 member-chip）。
	// Members 数据从上面构建的 members 数组（v1 Meta 格式）转换为 v2 类型安全的 []MemberInfo。
	// 2026-07-04 问题 3 修复：携带 TeamName 让前端展示团队名称而非 ID。
	// 2026-07-04 问题 C3 修复：team.DisplayName 为空时记录 Warn 日志，便于定位。
	if strings.TrimSpace(team.DisplayName) == "" {
		a.lg.Warn("publishSpiritTeamAssembled: team.DisplayName 为空，前端将显示 team_id",
			loggateway.StepID("spirit.team.empty_display_name"),
			loggateway.Str("team_id", team.ID),
		)
	}
	ts := biz.TeamStage{
		ID:        string(agent.NewTeamStageActivityID(team.ID)),
		TaskID:    string(agent.RootTaskActivityIDFromCtx(ctx)),
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
	_ = persistedSync // retained for sync persist logging; no longer used by bus
	// 2026-07-04 问题 2 修复：改用 publishV2Event（seq 优先持久化）。
	a.publishV2Event(ctx, biz.NewTeamStageCreatedEvent(ts))

	// 2026-07-04 问题 4 修复：发布 v2 TeamRun + MemberSession 创建事件。
	// 原先仅发布 TeamStage 事件，前端 v2 store 的 teamRuns/memberSessions Map 为空，
	// 导致 MemberSessionPanel 无法渲染成员会话树。
	a.publishV2TeamRunAndMemberSessions(ctx, spiritSessionID, team, teamSession, agentKeys, members)
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
//
// Phase 2: graph_stage snapshots are routed through the v2 Sequencer (FIFO +
// retry) via ActivityBridgeEvent wrapping. The v1 ActivityEventBus is retained
// as fallback only when neither seq nor eventBus is available.
func (a *SpiritTeamAssembler) publishSpiritGraphStageSnapshot(ctx context.Context, spiritSessionID string) {
	if (a.seq == nil && a.eventBus == nil && a.bus == nil) || spiritSessionID == "" || a.spiritUC == nil {
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
	publishGraphStageSnapshot(ctx, a.seq, a.eventBus, a.bus, a.lg, spiritSessionID, teams)
}

// publishGraphStageSnapshot is the free-function core of
// publishSpiritGraphStageSnapshot. It accepts an already-fetched team list so
// callers that have direct access to TeamUsecase (e.g. TeamStarter) can reuse
// the logic without depending on SpiritTeamUsecase. See
// publishSpiritGraphStageSnapshot for the full design rationale.
//
// Phase 2: graph_stage snapshots are routed through the v2 Sequencer (preferred)
// → v2 EventBus (fallback) → v1 ActivityEventBus (last resort). The v1
// ActivityEvent is wrapped in ActivityBridgeEvent so v2 consumers (front-end
// AgentCard, Kanban) receive the payload unchanged.
func publishGraphStageSnapshot(ctx context.Context, seq rt.EventPublisher, eventBus biz.EventBus, bus biz.ActivityEventBus, lg loggateway.Logger, spiritSessionID string, teams []biz.Team) {
	if (seq == nil && eventBus == nil && bus == nil) || spiritSessionID == "" || len(teams) == 0 {
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
	activityID := string(agent.NewGraphStageActivityID(spiritSessionID))
	ev := biz.ActivityEvent{
		Event: eventType,
		Activity: biz.Activity{
			ID:               activityID,
			Kind:             biz.ActivityKindGraphStage,
			Status:           aggregateStatus,
			Stage:            "team_dag_snapshot",
			Timestamp:        time.Now().UTC(),
			ParentActivityID: string(agent.RootTaskActivityIDFromCtx(ctx)),
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
	}
	// Phase 2: route through v2 Sequencer (FIFO + retry) when available;
	// fall back to v2 EventBus; last resort v1 ActivityEventBus.
	switch {
	case seq != nil:
		seq.Publish(ctx, biz.NewActivityBridgeEvent(ev))
	case eventBus != nil:
		eventBus.Publish(ctx, biz.NewActivityBridgeEvent(ev))
	case bus != nil:
		bus.Publish(ctx, ev)
	}
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
	teamStageID := string(agent.NewTeamStageActivityID(team.ID))
	now := time.Now().UTC()
	// 派生 TeamRun ID（基于 teamStageID 确定性派生，确保多次调用产生相同 ID）。
	teamRunID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.team_run.v2:"+teamStageID)).String()
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
		msID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.member_session.v2:"+teamRunID+":"+key)).String()
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
			Version:         1,
		}
		a.seq.Publish(ctx, biz.NewMemberSessionCreatedEvent(ms))
	}
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
	teamStageID := string(agent.NewTeamStageActivityID(teamID))
	teamRunID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.team_run.v2:"+teamStageID)).String()
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
	tr := biz.TeamRun{
		ID:              teamRunID,
		TeamStageID:     teamStageID,
		TaskID:          rootTaskID,
		SessionID:       "", // RepoSet 不依赖此字段做合并；保持空避免覆盖
		SpiritSessionID: spiritSessionID,
		DagNodeID:       team.DagNodeID,
		DependsOn:       dependsOn,
		Status:          teamRunStatus,
		StartedAt:       now, // 此处用 now 是 RepoSet 的 fallback；真实 StartedAt 已在 created 事件中持久化
		CompletedAt:     &now,
		Version:         2, // Version > 1 以通过 VersionLT 守卫覆盖 created 记录
	}
	switch teamRunStatus {
	case biz.TeamRunV2StatusCompleted:
		s.seq.Publish(ctx, biz.NewTeamRunCompletedEvent(tr))
	case biz.TeamRunV2StatusFailed:
		s.seq.Publish(ctx, biz.NewTeamRunFailedEvent(tr))
	default:
		// 2026-07-04 问题 4 修复：cancelled 状态改用 NewTeamRunFailedEvent（语义更接近）。
		// 原来用 NewTeamRunStartedEvent 作为 placeholder 语义错误（Started 表示开始而非取消）。
		// event_router.go 中 FailedEvent 与 StartedEvent 都路由到 UpsertTeamRun，
		// RepoSet.persistTeamRun 根据 tr.Status 字段（Cancelled）更新实体，与事件类型无关。
		// 使用 FailedEvent 让日志和事件流更易诊断：cancelled 是一种非成功终止。
		s.seq.Publish(ctx, biz.NewTeamRunFailedEvent(tr))
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
	for _, sess := range result.Items {
		if sess.SessionType != "agent" {
			continue
		}
		agentKey := sess.MemberAgentKey
		if agentKey == "" {
			continue
		}
		memberCount++
		msID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.member_session.v2:"+teamRunID+":"+agentKey)).String()
		s.lg.Info("publishV2TeamRunCompletion: 派生 MemberSession ID",
			loggateway.StepID("spirit.v2.team_run_completion.msid"),
			loggateway.Str("team_id", teamID),
			loggateway.Str("team_run_id", teamRunID),
			loggateway.Str("member_session_id", msID),
			loggateway.Str("agent_key", agentKey),
			loggateway.Str("member_session_db_id", sess.ID),
		)
		ms := biz.MemberSession{
			ID:              msID,
			TeamRunID:       teamRunID,
			TeamStageID:     teamStageID,
			TaskID:          rootTaskID,
			SessionID:       sess.ID,
			SpiritSessionID: spiritSessionID,
			AgentKey:        agentKey,
			Status:          memberStatus,
			StartedAt:       now,
			FinishedAt:      &now,
			Version:         2,
		}
		s.seq.Publish(ctx, biz.NewMemberSessionUpdatedEvent(ms))
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
