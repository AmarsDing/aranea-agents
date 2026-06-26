package service

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	loggateway "aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

type SessionStatusGuard struct {
	uc           *biz.SessionUsecase
	teamUC       *biz.TeamUsecase
	orchestrator biz.TaskOrchestratorPort
	bus          biz.ActivityEventBus
	lg           loggateway.Logger
}

func NewSessionStatusGuard(uc *biz.SessionUsecase, teamUC *biz.TeamUsecase, orchestrator biz.TaskOrchestratorPort, bus biz.ActivityEventBus, lg loggateway.Logger) *SessionStatusGuard {
	return &SessionStatusGuard{uc: uc, teamUC: teamUC, orchestrator: orchestrator, bus: bus, lg: lg}
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
	recovered, err := g.teamUC.RecoverOrphanedRunningTeams(ctx)
	if err != nil {
		g.lg.Error("session status guard: failed to recover orphaned teams", loggateway.Err(err))
		return err
	}
	// Publish spirit_team_interrupted events for each recovered team
	if g.bus != nil {
		for _, team := range recovered {
			ev := biz.ActivityEvent{
				Event: biz.ActivityEventFailed,
				Activity: biz.Activity{
					ID:              uuid.NewString(),
					Kind:            biz.ActivityKindTeamStage,
					Status:          biz.ActivityStatusInterrupted,
					Stage:           "interrupted",
					Timestamp:       time.Now().UTC(),
					SpiritSessionID: team.SpiritSessionID,
					TeamID:          team.ID,
					AgentKey:        "session-status-guard",
					Meta: map[string]any{
						"team_id":          team.ID,
						"team_name":        team.DisplayName,
						"status":           biz.TeamStatusInterrupted,
						"interrupt_reason": team.InterruptReason,
					},
				},
				Domain: biz.ActivityDomainChat,
			}
			g.bus.Publish(ctx, ev)
		}
	}
	g.lg.Info("session status guard: orphaned teams recovered", loggateway.Int("count", len(teams)))
	return nil
}

// recoverInterruptedOrchestrations recovers interrupted orchestrations on startup.
func (g *SessionStatusGuard) recoverInterruptedOrchestrations(ctx context.Context) error {
	if g.orchestrator == nil {
		return nil
	}
	return g.orchestrator.RecoverAllInterrupted(ctx)
}
