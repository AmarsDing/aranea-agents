package biz

import "context"

// TeamCompletionCheckerAdapter adapts SpiritTeamUsecase to the TeamCompletionChecker interface
// defined in the agent package. This adapter breaks the circular dependency between
// the agent and biz packages.
type TeamCompletionCheckerAdapter struct {
	uc *SpiritTeamUsecase
}

// NewTeamCompletionCheckerAdapter creates a new adapter instance.
func NewTeamCompletionCheckerAdapter(uc *SpiritTeamUsecase) *TeamCompletionCheckerAdapter {
	if uc == nil {
		return nil
	}
	return &TeamCompletionCheckerAdapter{uc: uc}
}

// CheckAllTeamsCompleted delegates to the underlying SpiritTeamUsecase.
func (a *TeamCompletionCheckerAdapter) CheckAllTeamsCompleted(ctx context.Context, spiritSessionID string) AllTeamsCompletedResult {
	if a == nil || a.uc == nil {
		return AllTeamsCompletedResult{}
	}
	return a.uc.CheckAllTeamsCompleted(ctx, spiritSessionID)
}
