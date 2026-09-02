package biz

import (
	"context"

	"aranea-agents/pkg/loggateway"
)

// RecoverOrphanedRunningTeams transitions all running teams to interrupted
// and their running runs to failed. Called on server startup to clean up
// stale state from a previous crash.
func (u *TeamUsecase) RecoverOrphanedRunningTeams(ctx context.Context) ([]Team, error) {
	return u.RecoverOrphanedRunningTeamsEx(ctx, nil)
}

// RecoverOrphanedRunningTeamsEx is the startup-resume-aware variant
// (83-长时运行韧性): a running team whose active run was already
// crash-resumed from its graph checkpoint (marker hit) is alive again — the
// whole team is skipped (no interrupted transition, no run kill). marker=nil
// degrades to the legacy kill-all behavior.
func (u *TeamUsecase) RecoverOrphanedRunningTeamsEx(ctx context.Context, marker TeamRunStartupResumeMarker) ([]Team, error) {
	teams, err := u.ListTeamsByStatus(ctx, TeamStatusRunning)
	if err != nil {
		return nil, err
	}
	if len(teams) == 0 {
		return nil, nil
	}
	var recovered []Team
	for i := range teams {
		if marker != nil && u.teamHasStartupResumedRun(ctx, teams[i].ID, marker) {
			u.lg.Info("recover orphaned teams: skip team with crash-resumed run",
				loggateway.Str("team_id", teams[i].ID),
			)
			continue
		}
		team, err := u.TransitionStatusWithReason(ctx, teams[i].ID, TeamStatusInterrupted, "服务器重启")
		if err != nil {
			u.lg.Warn("recover orphaned teams: failed to transition team to interrupted",
				loggateway.Str("team_id", teams[i].ID),
				loggateway.Err(err),
			)
			continue
		}
		recovered = append(recovered, team)
		orphanRecoveryMaxRuns := 10
		runs, err := u.ListRuns(ctx, teams[i].ID, orphanRecoveryMaxRuns)
		if err != nil {
			u.lg.Warn("recover orphaned teams: failed to list team runs",
				loggateway.Str("team_id", teams[i].ID),
				loggateway.Err(err),
			)
			continue
		}
		for _, run := range runs {
			// Only orphan pending/running runs. waiting_human/paused runs are
			// owned by the graph-session HITL recovery channel (RecoverSessions +
			// completion watch with HITL SLA timeout); force-failing them here
			// would silently discard human verdicts after restart. Terminal runs
			// are skipped to avoid Warn noise on every startup.
			var target string
			switch run.Status {
			case TeamRunStatusPending:
				target = TeamRunStatusCancelled
			case TeamRunStatusRunning:
				target = TeamRunStatusFailed
			default:
				continue
			}
			if _, tErr := u.TransitionRunStatus(ctx, run.ID, target); tErr != nil {
				u.lg.Warn("recover orphaned teams: failed to transition team run to terminal status",
					loggateway.Str("team_run_id", run.ID),
					loggateway.Str("target_status", target),
					loggateway.Err(tErr),
				)
			}
		}
	}
	return recovered, nil
}

// teamHasStartupResumedRun reports whether any running run of the team was
// crash-resumed during this process's startup reconcile (83-长时运行韧性).
// Such a team is alive again and must not be interrupted.
func (u *TeamUsecase) teamHasStartupResumedRun(ctx context.Context, teamID string, marker TeamRunStartupResumeMarker) bool {
	runs, err := u.ListRuns(ctx, teamID, 10)
	if err != nil {
		u.lg.Warn("recover orphaned teams: failed to list team runs for marker check",
			loggateway.Str("team_id", teamID),
			loggateway.Err(err),
		)
		return false
	}
	for _, run := range runs {
		if run.Status == TeamRunStatusRunning && marker.WasStartupResumed(run.ID) {
			return true
		}
	}
	return false
}
