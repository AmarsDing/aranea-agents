package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent/membersessionv2"
	"aranea-agents/internal/data/ent/stepv2"
	"aranea-agents/internal/data/ent/taskv2"
	"aranea-agents/internal/data/ent/teamrunv2"
	"aranea-agents/internal/data/ent/teamstagev2"
	"aranea-agents/internal/data/ent/turnv2"
	"aranea-agents/pkg/loggateway"
)

// v2RecoveryRepo implements biz.V2RecoveryRepo.
// Stability:evolving
type v2RecoveryRepo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.V2RecoveryRepo = (*v2RecoveryRepo)(nil)

// NewV2RecoveryRepo creates a new V2RecoveryRepo.
func NewV2RecoveryRepo(d *Data, lg loggateway.Logger) biz.V2RecoveryRepo {
	return &v2RecoveryRepo{data: d, lg: lg.With(loggateway.Domain("V2_RECOVERY"))}
}

// recoveryError is stamped on rows that carry an error column so operators
// can distinguish restart-recovered failures from genuine runtime errors.
const recoveryError = "orphaned by server restart: terminalized at startup"

// FailOrphanedInFlight batch-terminalizes in-flight rows in all six v2
// entity tables. All updates run in one transaction: either every orphan is
// recovered or none is, keeping cross-entity state consistent.
//
// Human-waiting statuses are deliberately excluded (see biz.V2RecoveryRepo):
//   - steps_v2.tool_blocked        — resumable via the tool-approve path
//   - team_stages_v2.waiting_human — resumable via the HITL resume path
func (r *v2RecoveryRepo) FailOrphanedInFlight(ctx context.Context, recoverAt time.Time) (biz.V2RecoveryStats, error) {
	if r == nil || r.data == nil {
		return biz.V2RecoveryStats{}, fmt.Errorf("v2 recovery repo: database not configured")
	}
	var stats biz.V2RecoveryStats
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		w := r.data.RW().Write(txCtx)

		n, err := w.TaskV2.Update().
			Where(taskv2.StatusIn(string(biz.TaskStatusPending), string(biz.TaskStatusRunning))).
			SetStatus(string(biz.TaskStatusFailed)).
			SetCompletedAt(recoverAt).
			SetUpdatedAt(recoverAt).
			AddVersion(1).
			Save(txCtx)
		if err != nil {
			return fmt.Errorf("tasks_v2: %w", err)
		}
		stats.Tasks = n

		n, err = w.TurnV2.Update().
			Where(turnv2.StatusEQ(string(biz.TurnStatusRunning))).
			SetStatus(string(biz.TurnStatusFailed)).
			SetCompletedAt(recoverAt).
			AddVersion(1).
			Save(txCtx)
		if err != nil {
			return fmt.Errorf("turns_v2: %w", err)
		}
		stats.Turns = n

		n, err = w.StepV2.Update().
			Where(stepv2.StatusIn(
				string(biz.StepStatusPending),
				string(biz.StepStatusRunning),
				string(biz.StepStatusToolRunning),
			)).
			SetStatus(string(biz.StepStatusFailed)).
			SetCompletedAt(recoverAt).
			AddVersion(1).
			Save(txCtx)
		if err != nil {
			return fmt.Errorf("steps_v2: %w", err)
		}
		stats.Steps = n

		n, err = w.TeamStageV2.Update().
			Where(teamstagev2.StatusIn(string(biz.TeamStageStatusPending), string(biz.TeamStageStatusRunning))).
			SetStatus(string(biz.TeamStageStatusFailed)).
			SetCompletedAt(recoverAt).
			AddVersion(1).
			Save(txCtx)
		if err != nil {
			return fmt.Errorf("team_stages_v2: %w", err)
		}
		stats.TeamStages = n

		n, err = w.TeamRunV2.Update().
			Where(teamrunv2.StatusIn(string(biz.TeamRunV2StatusRunning), string(biz.TeamRunV2StatusPaused))).
			SetStatus(string(biz.TeamRunV2StatusFailed)).
			SetCompletedAt(recoverAt).
			SetError(recoveryError).
			AddVersion(1).
			Save(txCtx)
		if err != nil {
			return fmt.Errorf("team_runs_v2: %w", err)
		}
		stats.TeamRuns = n

		n, err = w.MemberSessionV2.Update().
			Where(membersessionv2.StatusIn(
				string(biz.MemberSessionStatusPending),
				string(biz.MemberSessionStatusRunning),
				string(biz.MemberSessionStatusPaused),
			)).
			SetStatus(string(biz.MemberSessionStatusFailed)).
			SetFinishedAt(recoverAt).
			SetError(recoveryError).
			AddVersion(1).
			Save(txCtx)
		if err != nil {
			return fmt.Errorf("member_sessions_v2: %w", err)
		}
		stats.MemberSessions = n

		return nil
	})
	if err != nil {
		return biz.V2RecoveryStats{}, entErrToBizErr(err, "V2_RECOVERY")
	}
	if stats.Total() > 0 {
		r.lg.Info("v2 orphaned in-flight entities recovered",
			loggateway.Int("tasks", stats.Tasks),
			loggateway.Int("turns", stats.Turns),
			loggateway.Int("steps", stats.Steps),
			loggateway.Int("team_stages", stats.TeamStages),
			loggateway.Int("team_runs", stats.TeamRuns),
			loggateway.Int("member_sessions", stats.MemberSessions),
		)
	}
	return stats, nil
}
