package service

import (
	"context"

	"aranea-agents/internal/biz"
)

// TeamOrchestrator dispatches a team_run for a PlanStep and reports its
// completion via the returned channel. The channel must emit exactly one
// TeamCompleteEvent and then close.
//
// Implementations are expected to:
//  1. Build and start the team_run (creating MemberSessions, etc.)
//  2. Wait for the team_run to reach a terminal status
//  3. Send a single TeamCompleteEvent on the channel and close it
//
// Stability: evolving
type TeamOrchestrator interface {
	Orchestrate(ctx context.Context, step biz.PlanStep, ts biz.TeamStage) (<-chan biz.TeamCompleteEvent, error)
}
