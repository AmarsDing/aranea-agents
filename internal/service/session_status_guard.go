package service

import (
	"context"

	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	loggateway "aranea-agents/pkg/loggateway"
)

type SessionStatusGuard struct {
	uc              *biz.SessionUsecase
	teamUC          *biz.TeamUsecase
	orchestrator    biz.TaskOrchestratorPort
	lg              loggateway.Logger
}

func NewSessionStatusGuard(uc *biz.SessionUsecase, teamUC *biz.TeamUsecase, orchestrator biz.TaskOrchestratorPort, lg loggateway.Logger) *SessionStatusGuard {
	return &SessionStatusGuard{uc: uc, teamUC: teamUC, orchestrator: orchestrator, lg: lg}
}

func (g *SessionStatusGuard) OnStartup(ctx context.Context) error {
	g.lg.Info("session status guard: recovering orphaned running sessions")
	if err := g.uc.RecoverOrphanedRunningSessions(ctx); err != nil {
		g.lg.Error("session status guard: failed to recover orphaned sessions", loggateway.Err(err))
		return err
	}
	if err := g.recoverOrphanedRunningTeams(ctx); err != nil {
		g.lg.Error("session status guard: failed to recover orphaned teams", loggateway.Err(err))
		// Non-fatal: session recovery already succeeded; log and continue.
	}
	if err := g.recoverInterruptedOrchestrations(ctx); err != nil {
		g.lg.Error("session status guard: failed to recover interrupted orchestrations", loggateway.Err(err))
		// Non-fatal: session and team recovery already succeeded; log and continue.
	}
	return nil
}

func (g *SessionStatusGuard) OnShutdown(ctx context.Context) error {
	g.lg.Info("session status guard: transitioning running sessions to interrupted on shutdown")
	if err := g.uc.BatchTransitionInterrupted(ctx, sessstatus.StatusReasonServerShutdown); err != nil {
		g.lg.Error("session status guard: failed to transition sessions on shutdown", loggateway.Err(err))
		return err
	}
	return nil
}

// recoverOrphanedRunningTeams transitions running teams to interrupted status
// and running team runs to failed status on startup.
func (g *SessionStatusGuard) recoverOrphanedRunningTeams(ctx context.Context) error {
	if g.teamUC == nil {
		return nil
	}
	teams, err := g.teamUC.ListTeamsByStatus(ctx, biz.TeamStatusRunning)
	if err != nil {
		return err
	}
	if len(teams) == 0 {
		return nil
	}
	g.lg.Info("session status guard: recovering orphaned running teams", loggateway.Int("count", len(teams)))
	var failedCount int
	for _, t := range teams {
		t.Status = biz.TeamStatusInterrupted
		if _, err := g.teamUC.Update(ctx, t.ID, t); err != nil {
			failedCount++
			g.lg.Warn("session status guard: failed to transition team to interrupted",
				loggateway.Str("team_id", t.ID),
				loggateway.Err(err),
			)
			continue
		}
		g.lg.Info("session status guard: team transitioned to interrupted",
			loggateway.Str("team_id", t.ID),
		)
		// Transition running TeamRuns for this team to failed.
		runs, err := g.teamUC.ListRuns(ctx, t.ID, 10)
		if err != nil {
			g.lg.Warn("session status guard: failed to list team runs",
				loggateway.Str("team_id", t.ID),
				loggateway.Err(err),
			)
			continue
		}
		for _, run := range runs {
			if run.Status != biz.TeamRunStatusRunning {
				continue
			}
			run.Status = biz.TeamRunStatusFailed
			if err := g.teamUC.UpdateRun(ctx, run); err != nil {
				g.lg.Warn("session status guard: failed to transition team run to failed",
					loggateway.Str("team_run_id", run.ID),
					loggateway.Err(err),
				)
				continue
			}
			g.lg.Info("session status guard: team run transitioned to failed",
				loggateway.Str("team_run_id", run.ID),
			)
		}
	}
	if failedCount > 0 {
		g.lg.Warn("session status guard: some teams failed to recover",
			loggateway.Int("total", len(teams)),
			loggateway.Int("failed", failedCount),
		)
	}
	return nil
}

// recoverInterruptedOrchestrations recovers interrupted orchestrations on startup.
func (g *SessionStatusGuard) recoverInterruptedOrchestrations(ctx context.Context) error {
	if g.orchestrator == nil {
		return nil
	}
	return g.orchestrator.RecoverAllInterrupted(ctx)
}
