package biz

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// Domain: Orchestration — timeout registration for team execution.
func (o *SpiritOrchestration) registerTeamTimeout(ctx context.Context, cfg ParallelConfig, teamID string) {
	if cfg.TeamTimeoutSeconds <= 0 {
		return
	}
	if cfg.TeamMaxLifetimeSeconds <= 0 {

		cfg.TeamMaxLifetimeSeconds = 14400
	}

	o.timeouts.cancel(teamID)

	bgCtx := context.WithoutCancel(ctx)
	o.armTeamTimeout(bgCtx, cfg, teamID, cfg.TeamTimeout())
}

// armTeamTimeout schedules the next idle-timeout check (F1, 2026-09-03
// lbg-verify-planner 复盘): TeamTimeout is an IDLE window, not a wall-clock
// deadline. A team whose members are still progressing is re-armed for the
// remaining window instead of being killed mid-work.
func (o *SpiritOrchestration) armTeamTimeout(bgCtx context.Context, cfg ParallelConfig, teamID string, after time.Duration) {
	timer := time.AfterFunc(after, func() {

		if !o.timeouts.claim(teamID) {
			return
		}
		safego.Go(bgCtx, "spirit-team-timeout", func() {
			o.onTeamTimeout(bgCtx, cfg, teamID)
		})
	})
	o.timeouts.store(teamID, timer)
}

// onTeamTimeout decides whether the team is genuinely wedged (idle for a full
// window or beyond the absolute lifetime ceiling) or merely long-running
// (members still progressing → re-arm). The runner-level guards (first-byte/
// stall budgets, no-progress auditor, token budget) own member-level liveness;
// this timer is a backstop for a fully wedged run.
func (o *SpiritOrchestration) onTeamTimeout(bgCtx context.Context, cfg ParallelConfig, teamID string) {
	timeoutCtx, timeoutCancel := context.WithTimeout(bgCtx, cfg.TimeoutHandlerDBTimeout())
	defer timeoutCancel()
	team, err := o.teamUC.Get(timeoutCtx, teamID)
	if err != nil {

		o.lg.Warn("超时回调读取团队失败，顺延一个窗口",
			loggateway.StepID("spirit.team.timeout_get_err"),
			loggateway.Str("team_id", teamID),
			loggateway.Err(err),
		)
		o.armTeamTimeout(bgCtx, cfg, teamID, cfg.TeamTimeout())
		return
	}
	if team.Status == TeamStatusCompleted || team.Status == TeamStatusFailed || team.Status == TeamStatusCancelled || team.Status == TeamStatusPartialFailure {

		return
	}
	if team.Status == TeamStatusPending {
		o.lg.Info("团队仍待激活，忽略超时",
			loggateway.StepID("spirit.team.timeout_skip_pending"),
			loggateway.Str("team_id", teamID),
		)
		return
	}
	now := time.Now()
	lastActivity, runStartedAt, probeErr := o.probeTeamActivity(timeoutCtx, teamID)

	ceilingBase := runStartedAt
	if ceilingBase.IsZero() {
		if created, cerr := time.Parse(time.RFC3339, strings.TrimSpace(team.CreatedAt)); cerr == nil {
			ceilingBase = created
		}
	}
	if !ceilingBase.IsZero() && now.Sub(ceilingBase) > cfg.TeamMaxLifetime() {
		o.lg.Warn("团队超过最大存活时长，强制终止",
			loggateway.StepID("spirit.team.max_lifetime"),
			loggateway.Str("team_id", teamID),
			loggateway.Str("ceiling_base", ceilingBase.Format(time.RFC3339)),
		)
		o.failTeamOnTimeout(timeoutCtx, team)
		return
	}
	if probeErr != nil {

		if errors.Is(probeErr, errNoTeamRunToProbe) && teamCreatedOlderThan(team, 2*cfg.TeamTimeout(), now) {
			o.lg.Warn("团队 Running 但无运行记录且已超启动宽限，判死",
				loggateway.StepID("spirit.team.timeout_no_run"),
				loggateway.Str("team_id", teamID),
			)
			o.failTeamOnTimeout(timeoutCtx, team)
			return
		}

		strikes := o.noteProbeFailure(teamID)
		if strikes >= teamProbeMaxStrikes {
			o.lg.Warn("团队活动探测连续失败，fail-closed 判死",
				loggateway.StepID("spirit.team.timeout_probe_strikes"),
				loggateway.Str("team_id", teamID),
				loggateway.Int("strikes", strikes),
				loggateway.Err(probeErr),
			)
			o.failTeamOnTimeout(timeoutCtx, team)
			return
		}
		o.lg.Warn("团队活动探测失败，超时顺延一个窗口",
			loggateway.StepID("spirit.team.timeout_probe_err"),
			loggateway.Str("team_id", teamID),
			loggateway.Int("strikes", strikes),
			loggateway.Err(probeErr),
		)
		o.armTeamTimeout(bgCtx, cfg, teamID, cfg.TeamTimeout())
		return
	}

	o.resetProbeFailures(teamID)
	if idle := now.Sub(lastActivity); idle < cfg.TeamTimeout() {

		remaining := min(cfg.TeamTimeout()-idle, cfg.TeamTimeout())
		o.lg.Info("团队成员仍在推进，超时顺延",
			loggateway.StepID("spirit.team.timeout_extended"),
			loggateway.Str("team_id", teamID),
			loggateway.Str("last_activity", lastActivity.Format(time.RFC3339)),
			loggateway.Str("remaining", remaining.String()),
		)
		o.armTeamTimeout(bgCtx, cfg, teamID, remaining)
		return
	}
	o.lg.Warn("团队执行超时（idle 窗口内无成员活动）",
		loggateway.StepID("spirit.team.timeout"),
		loggateway.Str("team_id", teamID),
		loggateway.Str("last_activity", lastActivity.Format(time.RFC3339)),
	)
	o.failTeamOnTimeout(timeoutCtx, team)
}

// failTeamOnTimeout transitions the team to failed and notifies the service
// layer — the original timeout path, now reached only for genuinely idle or
// over-ceiling teams.
func (o *SpiritOrchestration) failTeamOnTimeout(ctx context.Context, team Team) {
	o.resetProbeFailures(team.ID)
	if _, err := o.teamUC.TransitionStatus(ctx, team.ID, TeamStatusFailed); err != nil {
		o.lg.Warn("超时后转换团队状态失败",
			loggateway.StepID("spirit.team.timeout_transition_err"),
			loggateway.Str("team_id", team.ID),
			loggateway.Err(err),
		)
		return
	}

	if o.timeoutHandler != nil && team.SpiritSessionID != "" {
		o.timeoutHandler.HandleTeamTimeout(ctx, team.SpiritSessionID, team.ID)
	}
}

// probeTeamActivity returns the team's last observed activity and the latest
// run's start time. Activity = max(latest steps_v2 started_at across the team
// session tree, latest run started_at). Steps are probed through the narrow
// SpiritStepActivityReader capability when the configured SpiritStepReader
// supports it; otherwise the probe degrades to run-start activity (bounded by
// the lifetime ceiling).
func (o *SpiritOrchestration) probeTeamActivity(ctx context.Context, teamID string) (lastActivity, runStartedAt time.Time, err error) {
	runs, err := o.teamUC.ListRuns(ctx, teamID, 1)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if len(runs) == 0 || strings.TrimSpace(runs[0].SessionID) == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: team %s", errNoTeamRunToProbe, teamID)
	}
	run := runs[0]
	runStartedAt = parseTeamRunStartedAt(run.StartedAt)
	lastActivity = runStartedAt
	var activityReader SpiritStepActivityReader
	if o.delivery != nil {
		activityReader, _ = o.delivery.stepReader.(SpiritStepActivityReader)
	}
	if activityReader == nil {
		return lastActivity, runStartedAt, nil
	}
	sessionIDs := []string{run.SessionID}
	children, childErr := o.sessionUC.ListChildSessions(ctx, run.SessionID)
	if childErr != nil {
		return time.Time{}, runStartedAt, childErr
	}
	for _, child := range children {
		if child.ID != "" {
			sessionIDs = append(sessionIDs, child.ID)
		}
	}
	latest, stepErr := activityReader.LatestStepActivityAt(ctx, sessionIDs)
	if stepErr != nil {
		return time.Time{}, runStartedAt, stepErr
	}
	if latest.After(lastActivity) {
		lastActivity = latest
	}

	if o.delivery != nil && o.delivery.heartbeatReader != nil {
		if hb, hbErr := o.delivery.heartbeatReader.LatestTeamRunHeartbeat(ctx, run.SessionID); hbErr != nil {
			o.lg.Warn("团队心跳读取失败，降级为 steps-only 探测",
				loggateway.StepID("spirit.team.heartbeat_probe_err"),
				loggateway.Str("team_id", teamID),
				loggateway.Err(hbErr),
			)
		} else if hb.After(lastActivity) {
			lastActivity = hb
		}
	}
	return lastActivity, runStartedAt, nil
}

// parseTeamRunStartedAt parses the run started_at column; empty or invalid
// yields zero (the ceiling check then skips while the idle probe still applies).
func parseTeamRunStartedAt(s string) time.Time {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}
	}
	return t
}
