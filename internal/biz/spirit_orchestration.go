package biz

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz/session"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// teamTimeoutRegistry tracks pending team timeout timers so they can be
// cancelled when a team reaches a terminal state before the timeout fires.
// Extracted from a raw sync.Map (AS-COG-01: sync.Map should be a named type).
type teamTimeoutRegistry struct {
	timers sync.Map // teamID → *time.Timer
}

func (r *teamTimeoutRegistry) store(teamID string, t *time.Timer) {
	r.timers.Store(teamID, t)
}

// claim removes the pending timer entry. Returns false when the team already
// completed (CancelTimeoutTimer won the race) so the timeout must not fire.
func (r *teamTimeoutRegistry) claim(teamID string) bool {
	_, loaded := r.timers.LoadAndDelete(teamID)
	return loaded
}

func (r *teamTimeoutRegistry) cancel(teamID string) {
	if v, ok := r.timers.LoadAndDelete(teamID); ok {
		if t, ok := v.(*time.Timer); ok {
			t.Stop()
		}
	}
}

func (r *teamTimeoutRegistry) stopAll() {
	r.timers.Range(func(key, value any) bool {
		r.timers.Delete(key)
		if t, ok := value.(*time.Timer); ok {
			t.Stop()
		}
		return true
	})
}

// SpiritOrchestration owns DAG scheduling, timeouts, completion, and team
// run-state for Spirit (DEV-09).
type SpiritOrchestration struct {
	teamUC             SpiritTeamAssembler
	sessionUC          SpiritSessionAccessor
	agentUC            SpiritAgentResolver
	orchCache          *OrchestrationCache
	evolutionSugg      EvolutionSuggestionCreator
	timeoutHandler     TimeoutHandler
	timeoutOnce        sync.Once
	timeouts           *teamTimeoutRegistry
	completionNotifier AllTeamsCompletedNotifier
	pollCtx            context.Context
	pollCancel         context.CancelFunc
	delivery           *SpiritDelivery
	lg                 loggateway.Logger
}

// SetTimeoutHandler injects the service-layer timeout handler.
// Called after construction to break the circular dependency:
// SpiritTeamUsecase → TimeoutHandler → TeamStarter → SpiritTeamController → SpiritTeamUsecase.
// This is a justified exception like L4GraphUsecase.SetCascade.
// Uses sync.Once to ensure the handler is set exactly once.
func (o *SpiritOrchestration) SetTimeoutHandler(h TimeoutHandler) {
	o.timeoutOnce.Do(func() {
		o.timeoutHandler = h
	})
}

// SetAllTeamsCompletedNotifier injects the service-layer completion notifier.
// Called by the background poller when all teams for a spirit session reach
// terminal state. This is the "active notification" path.
func (o *SpiritOrchestration) SetAllTeamsCompletedNotifier(n AllTeamsCompletedNotifier) {
	o.completionNotifier = n
}

// StartBackgroundPolling starts a background goroutine that periodically
// checks all active spirit sessions for team completion. This supplements
// the event-driven path (HandleTeamTurnResult) with a moderate-frequency
// backup to catch cases where completion events are missed.
//
// Default interval: 30 seconds. This is backend logic and does not generate
// frontend-visible activity events.
func (o *SpiritOrchestration) StartBackgroundPolling(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	o.pollCtx, o.pollCancel = context.WithCancel(ctx)
	safego.Go(o.pollCtx, "spirit-team-completion-poller", func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-o.pollCtx.Done():
				return
			case <-ticker.C:
				o.pollTeamCompletions(o.pollCtx)
			}
		}
	})
	o.lg.Info("spirit team completion poller started",
		loggateway.StepID("spirit.poller.started"),
		loggateway.Str("interval", interval.String()),
	)
}

// pollTeamCompletions scans all running sessions and checks if all their
// teams have reached terminal state. When all done, it notifies the service
// layer via completionNotifier.
//
// This is a moderate-frequency backup for the event-driven path
// (HandleTeamTurnResult → checkAllTeamsCompleted). The polling itself does
// not generate frontend-visible activity events.
func (o *SpiritOrchestration) pollTeamCompletions(ctx context.Context) {
	sessions, err := o.sessionUC.Search(ctx, SessionSearchQuery{
		Status: string(session.SessionStatusRunning),
		Limit:  100,
	})
	if err != nil {
		o.lg.Warn("spirit poller: failed to search running sessions",
			loggateway.StepID("spirit.poller.search_err"),
			loggateway.Err(err),
		)
		return
	}
	for _, sess := range sessions.Items {
		if sess.ID == "" {
			continue
		}
		result := o.CheckAllTeamsCompleted(ctx, sess.ID)
		if !result.AllDone {
			continue
		}
		o.lg.Info("spirit poller: all teams completed for session",
			loggateway.StepID("spirit.poller.all_done"),
			loggateway.Str("spirit_session_id", sess.ID),
			loggateway.Int("total_teams", result.TotalTeams),
		)
		if o.completionNotifier != nil {
			o.completionNotifier.NotifyAllTeamsCompleted(ctx, sess.ID)
		}
	}
}

// Domain: Orchestration — timeout registration for team execution.
func (o *SpiritOrchestration) registerTeamTimeout(ctx context.Context, cfg ParallelConfig, teamID string) {
	if cfg.TeamTimeoutSeconds <= 0 {
		return
	}
	// Use WithoutCancel to preserve trace/log context while detaching from request lifecycle.
	bgCtx := context.WithoutCancel(ctx)
	timer := time.AfterFunc(cfg.TeamTimeout(), func() {
		// If CancelTimeoutTimer already removed this entry, the team completed
		// normally and we should not interfere.
		if !o.timeouts.claim(teamID) {
			return
		}
		safego.Go(bgCtx, "spirit-team-timeout", func() {
			timeoutCtx, timeoutCancel := context.WithTimeout(bgCtx, cfg.TimeoutHandlerDBTimeout())
			defer timeoutCancel()
			team, err := o.teamUC.Get(timeoutCtx, teamID)
			if err != nil {
				return
			}
			if team.Status == TeamStatusCompleted || team.Status == TeamStatusFailed || team.Status == TeamStatusCancelled {
				return
			}
			o.lg.Warn("团队执行超时",
				loggateway.StepID("spirit.team.timeout"),
				loggateway.Str("team_id", teamID),
			)
			if _, err := o.teamUC.TransitionStatus(timeoutCtx, teamID, TeamStatusFailed); err != nil {
				o.lg.Warn("超时后转换团队状态失败",
					loggateway.StepID("spirit.team.timeout_transition_err"),
					loggateway.Str("team_id", teamID),
					loggateway.Err(err),
				)
				return
			}
			// Notify service layer to handle dependency scheduling, event
			// publishing, and AllDone checks — same lifecycle as a normal
			// team failure.
			if o.timeoutHandler != nil && team.SpiritSessionID != "" {
				o.timeoutHandler.HandleTeamTimeout(timeoutCtx, team.SpiritSessionID, teamID)
			}
		})
	})
	o.timeouts.store(teamID, timer)
}

func (o *SpiritOrchestration) BuildCascadeBlockedResults(ctx context.Context, teams []Team) []TeamSynthesisResult {
	var results []TeamSynthesisResult
	for i := range teams {
		t := teams[i]
		summary, keyFindings, extractErr := o.delivery.ExtractTeamOutput(ctx, t.ID)
		if extractErr != nil {
			o.lg.Warn("提取团队输出失败",
				loggateway.StepID("spirit.extract_output_err"),
				loggateway.Str("team_id", t.ID),
				loggateway.Err(extractErr),
			)
		}
		result := TeamSynthesisResult{
			TeamID:      t.ID,
			TeamName:    t.DisplayName,
			TaskName:    t.TaskDescription,
			Status:      t.Status,
			Summary:     summary,
			KeyFindings: keyFindings,
		}
		if t.Status == TeamStatusFailed {
			result.Summary = "[执行失败] " + result.Summary
		}
		results = append(results, result)
	}
	failedDagNodes := make(map[string]string)
	for i := range teams {
		if teams[i].Status == TeamStatusFailed && teams[i].DagNodeID != "" {
			failedDagNodes[teams[i].DagNodeID] = teams[i].DisplayName
		}
	}
	for i := range teams {
		if teams[i].Status != TeamStatusPending {
			continue
		}
		for _, depID := range teams[i].DependsOn {
			if failedName, ok := failedDagNodes[depID]; ok {
				results = append(results, TeamSynthesisResult{
					TeamID:   teams[i].ID,
					TeamName: teams[i].DisplayName,
					TaskName: teams[i].TaskDescription,
					Status:   TeamStatusBlocked,
					Summary:  fmt.Sprintf("被失败团队 %s 阻塞", failedName),
				})
				break
			}
		}
	}
	return results
}

func (o *SpiritOrchestration) GetMaxParallelTeams(ctx context.Context, spiritSessionID string) int {
	cfg := o.resolveParallelConfig(ctx, spiritSessionID)
	return cfg.MaxConcurrentTeams
}

func (o *SpiritOrchestration) GetParallelConfig(ctx context.Context, spiritSessionID string) ParallelConfig {
	return o.resolveParallelConfig(ctx, spiritSessionID)
}

func (o *SpiritOrchestration) resolveParallelConfig(ctx context.Context, spiritSessionID string) ParallelConfig {
	if o.agentUC == nil {
		return DefaultParallelConfig()
	}
	agents, err := o.agentUC.List(ctx, AgentListQuery{Keyword: SpiritAgentKey, Limit: SpiritAgentQueryLimit})
	if err != nil {
		o.lg.Error("查询精灵 Agent 失败，使用默认并行配置（用户自定义配置将失效）",
			loggateway.StepID("spirit.parallel_config"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(err),
		)
		return DefaultParallelConfig()
	}
	if len(agents.Items) == 0 {
		o.lg.Error("精灵 Agent 不存在，使用默认并行配置（用户自定义配置将失效）",
			loggateway.StepID("spirit.parallel_config"),
			loggateway.Str("spirit_session_id", spiritSessionID),
		)
		return DefaultParallelConfig()
	}
	ag := agents.Items[0]
	// Read parallel_config from ConfigJSON (stored as a top-level key).
	// Previously attempted to read from MetadataJSON, but that field has no DB column.
	return ParseParallelConfig(ag.ConfigJSON, o.lg)
}

// Domain: Orchestration — cancel team and its timeout timer.
// reason 是 P2-6 取消原因（空 = user_cancel，保持向后兼容）。
func (o *SpiritOrchestration) CancelTeam(ctx context.Context, teamID string, reason CancelReason) error {
	if strings.TrimSpace(teamID) == "" {
		return apierror.BadRequest("SPIRIT", "team_id is required")
	}
	if reason == "" {
		reason = CancelReasonUser
	}
	o.CancelTimeoutTimer(teamID)
	_, err := o.teamUC.TransitionStatus(ctx, teamID, TeamStatusCancelled)
	if err != nil {
		return err
	}
	return nil
}

// Domain: Orchestration — auto-archive completed/failed teams past threshold.
func (o *SpiritOrchestration) AutoArchiveCompletedTeams(ctx context.Context, spiritSessionID string) {
	cfg := o.resolveParallelConfig(ctx, spiritSessionID)
	if cfg.AutoArchiveSeconds <= 0 {
		return
	}
	teams, err := o.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		o.lg.Warn("查询精灵团队列表失败，跳过自动归档",
			loggateway.StepID("spirit.auto_archive.list_err"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(err),
		)
		return
	}
	threshold := time.Now().Add(-cfg.AutoArchiveAfter())
	var archiveIDs []string
	for _, t := range teams {
		if t.Status != TeamStatusCompleted && t.Status != TeamStatusFailed && t.Status != TeamStatusCancelled {
			continue
		}
		updatedAt, parseErr := parseTimeFlexible(t.UpdatedAt)
		if parseErr != nil {
			o.lg.Warn("解析团队更新时间失败，使用兜底策略（视为可归档）",
				loggateway.StepID("spirit.auto_archive.parse_err"),
				loggateway.Str("team_id", t.ID),
				loggateway.Str("updated_at", t.UpdatedAt),
				loggateway.Err(parseErr),
			)
			// Fallback: if we can't parse the time, treat the team as eligible
			// for archiving rather than silently skipping it forever.
			updatedAt = time.Now().Add(-cfg.AutoArchiveAfter())
		}
		if updatedAt.Before(threshold) {
			archiveIDs = append(archiveIDs, t.ID)
		}
	}
	if len(archiveIDs) == 0 {
		return
	}
	archived, archiveErr := o.teamUC.BatchArchiveTeams(ctx, archiveIDs)
	if archiveErr != nil {
		o.lg.Warn("批量归档团队失败",
			loggateway.StepID("spirit.auto_archive.batch_err"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(archiveErr),
		)
		return
	}
	if archived > 0 {
		o.lg.Info("批量归档团队完成",
			loggateway.StepID("spirit.auto_archive.batch_done"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Int("archived_count", archived),
		)
	}
}

// CancelTimeoutTimer stops the timeout timer for a team if one is pending.
// Should be called when a team reaches a terminal state (completed/failed/cancelled)
// to prevent the timeout callback from firing unnecessarily.
func (o *SpiritOrchestration) CancelTimeoutTimer(teamID string) {
	o.timeouts.cancel(teamID)
}

// Stop cancels all pending timeout timers and the background polling goroutine.
// Call during application shutdown to prevent callbacks from firing after the
// server has stopped.
func (o *SpiritOrchestration) Stop() {
	if o.pollCancel != nil {
		o.pollCancel()
	}
	o.timeouts.stopAll()
}

func (o *SpiritOrchestration) CheckTeamProgress(ctx context.Context, spiritSessionID string) ([]TeamProgress, error) {
	spiritSessionID = strings.TrimSpace(spiritSessionID)
	if spiritSessionID == "" {
		return nil, apierror.BadRequest("SPIRIT", "spirit_session_id is required")
	}
	teams, err := o.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		return nil, err
	}
	out := make([]TeamProgress, 0, len(teams))
	for i := range teams {
		tp := TeamProgress{
			TeamID:   teams[i].ID,
			TeamName: teams[i].DisplayName,
			Status:   teams[i].Status,
		}
		switch teams[i].Status {
		case TeamStatusCompleted:
			tp.ProgressPct = 100
			tp.CurrentStep = "已完成"
		case TeamStatusFailed:
			tp.ProgressPct = 0
			tp.CurrentStep = "执行失败"
		case TeamStatusCancelled:
			tp.ProgressPct = 0
			tp.CurrentStep = "已取消"
		case TeamStatusPending:
			tp.ProgressPct = 0
			tp.CurrentStep = "等待执行"
		default:
		}
		runs, runErr := o.teamUC.ListRuns(ctx, teams[i].ID, SpiritRecentRunCount)
		if runErr == nil && len(runs) > 0 {
			latestRun := runs[0]
			tp.DurationMs = int64(latestRun.DurationMS)
			if IsTeamStatusActive(tp.Status) {
				completedRuns := 0
				for _, r := range runs {
					if r.Status == TeamRunStatusSuccess {
						completedRuns++
					}
				}
				if len(runs) > 0 {
					tp.ProgressPct = float64(completedRuns) / float64(len(runs)) * 100
				}
				if tp.ProgressPct >= 100 {
					tp.ProgressPct = 99
				}
				tp.CurrentStep = fmt.Sprintf("执行中 (已完成 %d/%d 轮)", completedRuns, len(runs))
			}
		}
		out = append(out, tp)
	}
	return out, nil
}

// RecordTeamCompletion records DQ Score, infers topology, and creates evolution suggestions
// for a completed team. Returns the computed DQ Score and inferred topology.
// Domain: Orchestration — record DQ score and create evolution suggestions on team completion.
func (o *SpiritOrchestration) RecordTeamCompletion(ctx context.Context, team Team, durationMs int64) (dqScore float64, topology TopologyType) {
	// Cancel timeout timer since team has completed.
	o.CancelTimeoutTimer(team.ID)

	// P0-②: persist deliverable output BEFORE any downstream scheduling reads
	// it. Callers guarantee RecordTeamCompletion runs before
	// ScheduleDependentTeams / PlanExecutor.NotifyTeamCompletion.
	if team.DagNodeID != "" {
		if werr := o.delivery.WriteDeliverablesToSession(ctx, team.ID); werr != nil {
			if errors.Is(werr, ErrNoRealDeliverable) {
				// 2026-07-25 Fix 1 双保险：service 闸门已把无交付物团队翻转
				// 为 failed（正常不会走到这里）；静默跳过，不产生 Warn 噪音。
				o.lg.Info("团队无真实交付物，跳过交付物落库",
					loggateway.StepID("spirit.team.completion.deliverables"),
					loggateway.Str("team_id", team.ID),
				)
			} else {
				o.lg.Warn("交付物落库失败（下游团队将无注入输入）",
					loggateway.StepID("spirit.team.completion.deliverables"),
					loggateway.Str("team_id", team.ID),
					loggateway.Err(werr),
				)
			}
		}
	}

	if o.orchCache == nil || team.DagNodeID == "" {
		return 0, ""
	}
	dqScore = ComputeDQScore(TeamSynthesisResult{
		TeamID:   team.ID,
		TeamName: team.DisplayName,
		TaskName: team.TaskDescription,
		// RecordTeamCompletion always records for a completed team; the "completed"
		// status is intentional — DQ Score is only meaningful for successful executions.
		Status: TeamStatusCompleted,
	}, durationMs)
	taskPattern := ExtractTaskPattern(team.TaskDescription)
	topology = InferTopologyFromTeam(team, o.lg)
	o.orchCache.RecordCompletion(ctx, taskPattern, topology, dqScore, 1, durationMs)
	o.lg.Info("精灵团队完成，记录 DQ Score",
		loggateway.StepID("spirit.team.completion"),
		loggateway.Str("team_id", team.ID),
		loggateway.Str("task_pattern", taskPattern),
		loggateway.Float64("dq_score", dqScore),
	)

	if dqScore < DQEvolutionThreshold && o.evolutionSugg != nil && team.SpiritSessionID != "" {
		altTopology, altFound := o.orchCache.SuggestBestAlternativeTopology(team.TaskDescription, topology)
		content := fmt.Sprintf("团队 %q 的 DQ Score 为 %.2f（低于阈值 %.1f），当前拓扑 %s 执行效果不佳。", team.DisplayName, dqScore, DQEvolutionThreshold, topology)
		if altFound {
			content += fmt.Sprintf("建议尝试 %s 拓扑。", altTopology)
		} else {
			content += "暂无历史数据推荐替代拓扑，建议调整任务描述或减少团队数量。"
		}
		_, suggErr := o.evolutionSugg.CreateSuggestion(ctx, EvolutionSuggestion{
			AgentID: team.SpiritSessionID,
			Type:    "orchestration_optimization",
			Title:   fmt.Sprintf("编排优化建议: %s", TruncateRunes(team.TaskDescription, MaxSuggestionTitleLen)),
			Content: content,
			Status:  "pending",
		})
		if suggErr != nil {
			o.lg.Warn("创建编排优化建议失败",
				loggateway.StepID("spirit.evolution_suggestion_err"),
				loggateway.Str("team_id", team.ID),
				loggateway.Err(suggErr),
			)
		}
	}
	return dqScore, topology
}

// ScheduleDependentTeams resolves DAG dependencies after a team completes.
// It returns a list of actions to take (activate or fail dependent teams).
// The caller (Service layer) is responsible for executing the actions
// (starting runners, publishing events, etc.).
// Domain: Orchestration — DAG dependency resolution and scheduling.
func (o *SpiritOrchestration) ScheduleDependentTeams(ctx context.Context, spiritSessionID string, completedTeam Team) []DependentTeamAction {
	if completedTeam.DagNodeID == "" {
		return nil
	}
	allTeams, err := o.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		o.lg.Warn("查询精灵团队列表失败，跳过依赖调度",
			loggateway.StepID("spirit.schedule_deps.list_err"),
			loggateway.Err(err),
		)
		return nil
	}

	var actions []DependentTeamAction
	for i := range allTeams {
		t := &allTeams[i]
		if t.Status != TeamStatusPending {
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
					if allTeams[j].Status == TeamStatusCompleted {
						found = true
					} else if allTeams[j].Status == TeamStatusFailed || allTeams[j].Status == TeamStatusCancelled {
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
			actions = append(actions, DependentTeamAction{
				TeamID:    t.ID,
				TeamName:  t.DisplayName,
				DagNodeID: t.DagNodeID,
				Action:    "fail",
				Reason:    "前置依赖团队执行失败",
			})
			continue
		}
		if !allDepsMet {
			continue
		}
		// Re-check current status to avoid stale data.
		current, getErr := o.teamUC.Get(ctx, t.ID)
		if getErr != nil || current.Status != TeamStatusPending {
			o.lg.Info("依赖调度：团队状态已变更，跳过激活",
				loggateway.StepID("spirit.schedule_deps.stale"),
				loggateway.Str("team_id", t.ID),
				loggateway.Str("current_status", current.Status),
			)
			continue
		}
		// P0-③b + 2026-07-25 Fix 2b: build the downstream team's first-turn
		// input through the single composer — upstream-deliverable prefix +
		// its own task description + mandatory delivery protocol suffix.
		taskDesc := o.delivery.BuildTeamTurnInput(ctx, *t)
		actions = append(actions, DependentTeamAction{
			TeamID:          t.ID,
			TeamName:        t.DisplayName,
			DagNodeID:       t.DagNodeID,
			Action:          "activate",
			TaskDescription: taskDesc,
		})
	}
	return actions
}

// CheckAllTeamsCompleted checks whether all teams for a spirit session are in a terminal state.
// Returns a result indicating if all teams are done and the list of team IDs.
// A team is considered "done" if it is in completed, failed, or cancelled state.
// Domain: Orchestration — check if all teams reached terminal state.
func (o *SpiritOrchestration) CheckAllTeamsCompleted(ctx context.Context, spiritSessionID string) AllTeamsCompletedResult {
	teams, err := o.teamUC.ListBySpiritSessionID(ctx, spiritSessionID)
	if err != nil {
		o.lg.Warn("查询精灵会话团队列表失败，跳过全完成检查",
			loggateway.StepID("spirit.teams.check_all"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(err),
		)
		return AllTeamsCompletedResult{}
	}
	if len(teams) == 0 {
		return AllTeamsCompletedResult{}
	}
	for _, t := range teams {
		switch t.Status {
		case TeamStatusPending, TeamStatusRunning, TeamStatusInterrupted:
			return AllTeamsCompletedResult{}
		}
	}
	// All teams are in a terminal state (completed, failed, cancelled, or archived).
	var teamIDs []string
	var completedTeams, failedTeams, cancelledTeams int
	for _, t := range teams {
		teamIDs = append(teamIDs, t.ID)
		switch t.Status {
		case TeamStatusCompleted:
			completedTeams++
		case TeamStatusFailed:
			failedTeams++
		case TeamStatusCancelled:
			failedTeams++
			cancelledTeams++
		}
	}
	// Aggregate token usage from child sessions of the spirit session.
	var totalTokenIn, totalTokenOut int
	childSessions, sessErr := o.sessionUC.ListChildSessions(ctx, spiritSessionID)
	if sessErr != nil {
		o.lg.Warn("查询精灵会话子 session 失败，跳过 token 聚合",
			loggateway.StepID("spirit.teams.token_agg_err"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(sessErr),
		)
	} else {
		teamIDSet := make(map[string]struct{}, len(teamIDs))
		for _, id := range teamIDs {
			teamIDSet[id] = struct{}{}
		}
		for _, s := range childSessions {
			if _, ok := teamIDSet[s.TeamID]; ok {
				totalTokenIn += s.InputTokens
				totalTokenOut += s.OutputTokens
			}
		}
	}
	return AllTeamsCompletedResult{
		AllDone:        true,
		TeamIDs:        teamIDs,
		TotalTeams:     len(teams),
		CompletedTeams: completedTeams,
		FailedTeams:    failedTeams,
		CancelledTeams: cancelledTeams,
		TotalTokenIn:   totalTokenIn,
		TotalTokenOut:  totalTokenOut,
	}
}

// ---------------------------------------------------------------------------
// XC-05: Escalation on Max Retries
// ---------------------------------------------------------------------------

// EscalateToSpirit escalates a team that has exceeded max retries to the
// Spirit assistant. This creates a system message in the Spirit session
// notifying the user that human intervention may be needed.
// Domain: Orchestration — escalation to Spirit assistant on max retries.
func (o *SpiritOrchestration) EscalateToSpirit(ctx context.Context, teamID string, tracker ReworkTracker) error {
	t, err := o.teamUC.Get(ctx, teamID)
	if err != nil {
		return err
	}

	o.lg.Warn("团队达到最大重试次数，升级到 Spirit 助手",
		loggateway.StepID("spirit.escalate"),
		loggateway.Str("team_id", teamID),
		loggateway.Str("team_name", t.DisplayName),
		loggateway.Int("attempts", tracker.Attempt),
		loggateway.Str("last_reason", tracker.LastReason),
	)

	// Transition team to failed status with escalation reason
	_, err = o.teamUC.TransitionStatus(ctx, teamID, TeamStatusFailed)
	if err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, "SPIRIT")
	}

	return nil
}

// HandleTeamRejection handles a team rejection by a verification gate.
// If the team can retry, it marks the team for rework and transitions
// its status back to pending for re-execution; otherwise it escalates
// to the Spirit assistant.
// Domain: Orchestration — handle verification gate rejection with retry/escalation logic.
// Note: The Running → Pending transition (TeamEventRework) was added in B-02 fix
// to support the rework flow. Before the fix, this transition was illegal and
// would silently fail.
func (o *SpiritOrchestration) HandleTeamRejection(ctx context.Context, teamID string, tracker ReworkTracker, reason string) (*ReworkTracker, error) {
	tracker.LastReason = reason

	if !tracker.CanRetry() {
		if err := o.EscalateToSpirit(ctx, teamID, tracker); err != nil {
			return nil, err
		}
		return &tracker, nil
	}

	tracker.IncrementAttempt()

	// Mark team for rework: transition back to pending status
	// so the DAG scheduler can re-execute it.
	_, transitionErr := o.teamUC.TransitionStatus(ctx, teamID, TeamStatusPending)
	if transitionErr != nil {
		o.lg.Warn("返工状态转换失败",
			loggateway.StepID("spirit.rework"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(transitionErr),
		)
	}

	o.lg.Info("团队被拒绝，准备重试",
		loggateway.StepID("spirit.rework"),
		loggateway.Str("team_id", teamID),
		loggateway.Int("attempt", tracker.Attempt),
		loggateway.Int("max_retries", tracker.MaxRetries),
		loggateway.Str("reason", reason),
	)
	return &tracker, nil
}

// parseTimeFlexible tries multiple time formats to parse a timestamp string.
// This handles the case where Ent may output timestamps in formats other than
// strict RFC3339 (e.g., "2026-06-08 12:34:56.789+08:00").
func parseTimeFlexible(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, apierror.BadRequest("SPIRIT", "empty timestamp")
	}
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999-07:00",
		"2006-01-02 15:04:05.999 -0700",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02T15:04:05-07:00",
		"2006-01-02 15:04:05Z07:00",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, apierror.BadRequest("SPIRIT", "unable to parse timestamp: %s", s)
}

// containsString checks if a string slice contains a given string.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
