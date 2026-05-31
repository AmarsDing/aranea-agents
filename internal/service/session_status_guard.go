package service

import (
	"context"

	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	loggateway "aranea-agents/pkg/loggateway"
)

type SessionStatusGuard struct {
	uc *biz.SessionUsecase
	lg loggateway.Logger
}

func NewSessionStatusGuard(uc *biz.SessionUsecase, lg loggateway.Logger) *SessionStatusGuard {
	return &SessionStatusGuard{uc: uc, lg: lg}
}

func (g *SessionStatusGuard) OnStartup(ctx context.Context) error {
	g.lg.Info("session status guard: recovering orphaned running sessions")
	if err := g.uc.RecoverOrphanedRunningSessions(ctx); err != nil {
		g.lg.Error("session status guard: failed to recover orphaned sessions", loggateway.Err(err))
		return err
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
