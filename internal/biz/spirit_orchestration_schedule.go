package biz

import (
	"context"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

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
		missingUpstream := false
		for _, depID := range t.DependsOn {
			found := false
			for j := range allTeams {
				if allTeams[j].DagNodeID != depID {
					continue
				}
				found = true
				switch allTeams[j].Status {
				case TeamStatusCompleted, TeamStatusPartialFailure:

				case TeamStatusFailed, TeamStatusCancelled:
					anyDepFailed = true
				default:
					allDepsMet = false
				}
				break
			}
			if !found {
				missingUpstream = true
				anyDepFailed = true
			}
		}
		if anyDepFailed {
			reason := "前置依赖团队执行失败"
			if missingUpstream {
				reason = "missing upstream team"
			}
			actions = append(actions, DependentTeamAction{
				TeamID:    t.ID,
				TeamName:  t.DisplayName,
				DagNodeID: t.DagNodeID,
				Action:    "fail",
				Reason:    reason,
			})
			continue
		}
		if !allDepsMet {
			continue
		}

		current, getErr := o.teamUC.Get(ctx, t.ID)
		if getErr != nil || current.Status != TeamStatusPending {
			o.lg.Info("依赖调度：团队状态已变更，跳过激活",
				loggateway.StepID("spirit.schedule_deps.stale"),
				loggateway.Str("team_id", t.ID),
				loggateway.Str("current_status", current.Status),
			)
			continue
		}

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
	// All teams are in a terminal state (completed, partial_failure, failed, cancelled, or archived).
	var teamIDs []string
	var completedTeams, failedTeams, cancelledTeams int
	for _, t := range teams {
		teamIDs = append(teamIDs, t.ID)
		switch t.Status {
		case TeamStatusCompleted, TeamStatusPartialFailure:

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

// EscalateToSpirit marks a team that has exceeded max retries as failed so
// DAG cascade and synthesis can observe the terminal failure. It does not
// inject a Spirit-session message (no turn gateway in this layer); callers
// that need user-visible escalation must publish via TeamStarter / EventBus.
// Domain: Orchestration — fail-closed on max retries.
func (o *SpiritOrchestration) EscalateToSpirit(ctx context.Context, teamID string, tracker ReworkTracker) error {
	t, err := o.teamUC.Get(ctx, teamID)
	if err != nil {
		return err
	}

	o.lg.Warn("团队达到最大重试次数，升级为失败",
		loggateway.StepID("spirit.escalate"),
		loggateway.Str("team_id", teamID),
		loggateway.Str("team_name", t.DisplayName),
		loggateway.Int("attempts", tracker.Attempt),
		loggateway.Str("last_reason", tracker.LastReason),
	)

	_, err = o.teamUC.TransitionStatus(ctx, teamID, TeamStatusFailed)
	if err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, "SPIRIT")
	}

	return nil
}

// HandleTeamRejection is a leftover rework helper (Running→Pending).
// Production PlanExecutor fail-closes verification/deliverable rejects and
// uses EvaluateDeliverableQuality + revision enqueue instead. Do not call
// this while a dagRun is active — Pending team + failed PlanStep diverge.
func (o *SpiritOrchestration) HandleTeamRejection(ctx context.Context, teamID string, tracker ReworkTracker, reason string) (*ReworkTracker, error) {
	tracker.LastReason = reason

	if !tracker.CanRetry() {
		if err := o.EscalateToSpirit(ctx, teamID, tracker); err != nil {
			return nil, err
		}
		return &tracker, nil
	}

	tracker.IncrementAttempt()

	_, transitionErr := o.teamUC.TransitionStatus(ctx, teamID, TeamStatusPending)
	if transitionErr != nil {
		o.lg.Warn("返工状态转换失败",
			loggateway.StepID("spirit.rework"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(transitionErr),
		)
		return nil, transitionErr
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
