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
	bus          biz.EventBus // Phase 3b-D: v2 EventBus (originally biz.ActivityEventBus)
	v2Recovery   biz.V2RecoveryRepo
	lg           loggateway.Logger
}

func NewSessionStatusGuard(uc *biz.SessionUsecase, teamUC *biz.TeamUsecase, orchestrator biz.TaskOrchestratorPort, bus biz.EventBus, v2Recovery biz.V2RecoveryRepo, lg loggateway.Logger) *SessionStatusGuard {
	return &SessionStatusGuard{uc: uc, teamUC: teamUC, orchestrator: orchestrator, bus: bus, v2Recovery: v2Recovery, lg: lg}
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
	if err := g.recoverOrphanedV2Entities(ctx); err != nil {
		g.lg.Error("session status guard: failed to recover orphaned v2 entities", loggateway.Err(err))
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
	// 2026-07-21 P1-5 修复：Kratos 在调用 Stop 钩子前已取消 server ctx，
	// 直接沿用会导致 BatchTransitionInterrupted 首个 DB 调用即报
	// "context canceled"，running sessions 永远不会被终态化。脱离取消信号
	// （保留 ctx values）并加上界超时，保证清理真正执行完成。
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := g.uc.BatchTransitionInterrupted(shutdownCtx, sessstatus.StatusReasonServerShutdown); err != nil {
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
			ts := biz.TeamStage{
				ID:        uuid.NewString(),
				TeamID:    team.ID,
				TeamName:  team.DisplayName,
				SessionID: team.SpiritSessionID,
				Status:    biz.TeamStageStatusFailed,
				Stage:     biz.TeamStageStageFailed,
				StartedAt: time.Now().UTC(),
				Version:   1,
			}
			g.bus.Publish(ctx, biz.NewTeamStageFailedEvent(ts))
			g.bus.Publish(ctx, biz.NewSystemNoticeEvent(team.SpiritSessionID, "spirit_team_interrupted", "", map[string]any{
				"team_id":          team.ID,
				"team_name":        team.DisplayName,
				"status":           biz.TeamStatusInterrupted,
				"interrupt_reason": team.InterruptReason,
			}))
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

// recoverOrphanedV2Entities terminalizes v2 entities (tasks/turns/steps/
// team_stages/team_runs/member_sessions) left in-flight by a process restart.
// 2026-07-21 P1-5 修复：此前 v2 实体无恢复机制，重启后永远卡在
// running/tool_running，activities 时间线显示"仍在执行"的僵尸状态。
func (g *SessionStatusGuard) recoverOrphanedV2Entities(ctx context.Context) error {
	if g.v2Recovery == nil {
		return nil
	}
	stats, err := g.v2Recovery.FailOrphanedInFlight(ctx, time.Now().UTC())
	if err != nil {
		return err
	}
	if stats.Total() > 0 {
		g.lg.Info("session status guard: orphaned v2 entities recovered",
			loggateway.Int("total", stats.Total()),
			loggateway.Int("tasks", stats.Tasks),
			loggateway.Int("turns", stats.Turns),
			loggateway.Int("steps", stats.Steps),
			loggateway.Int("team_stages", stats.TeamStages),
			loggateway.Int("team_runs", stats.TeamRuns),
			loggateway.Int("member_sessions", stats.MemberSessions),
		)
	}
	return nil
}
