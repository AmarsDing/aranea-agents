package service

import (
	"context"
	"errors"

	"aranea-agents/internal/biz"
)

// stubTeamOrchestrator is a no-op TeamOrchestrator used during Phase 1 wiring.
//
// PlanExecutor is wired into the dependency graph so that SetPlanExecutor can
// be called on TeamStarter (completing the structural wiring). The actual
// Orchestrate implementation — bridging to SpiritTeamAssembler to start a
// real team_run — is deferred to Phase 2. Until then, any Subscribe call
// surfaces a clear "not implemented" error instead of silently succeeding.
type stubTeamOrchestrator struct{}

// NewStubTeamOrchestrator returns the Phase 1 no-op TeamOrchestrator.
// Exported so Wire can call it as a provider.
func NewStubTeamOrchestrator() TeamOrchestrator {
	return &stubTeamOrchestrator{}
}

// Orchestrate returns a not-implemented error. The returned channel is nil
// so callers fail fast; PlanExecutor.Subscribe propagates the error.
func (s *stubTeamOrchestrator) Orchestrate(ctx context.Context, step biz.PlanStep, ts biz.TeamStage) (*OrchestrateResult, error) {
	return nil, errors.New("TeamOrchestrator not implemented in Phase 1; wiring deferred to Phase 2")
}
