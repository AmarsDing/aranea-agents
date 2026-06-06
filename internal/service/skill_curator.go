package service

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// SkillCuratorService handles LLM-driven skill evolution draft generation.
// It operates in the service layer because it needs to invoke trpc-agent-go
// which is forbidden in the biz layer.
type SkillCuratorService struct {
	uc *biz.SkillIntelligenceUsecase
	lg loggateway.Logger
}

func NewSkillCuratorService(uc *biz.SkillIntelligenceUsecase, lg loggateway.Logger) *SkillCuratorService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SkillCuratorService{uc: uc, lg: lg}
}

// RunCuratorFlow executes the full Curator Agent semi-automatic evolution
// pipeline: trigger detection → draft generation → sandbox verification.
// Delegates to biz.SkillIntelligenceUsecase.RunCuratorFlow.
func (s *SkillCuratorService) RunCuratorFlow(ctx context.Context, skillID string) (*biz.SkillEvolutionSuggestion, error) {
	return s.uc.RunCuratorFlow(ctx, skillID)
}

// GenerateDraft generates a draft skill body improvement for an existing
// evolution suggestion. Delegates to biz layer for draft generation logic.
func (s *SkillCuratorService) GenerateDraft(ctx context.Context, suggestionID string) (string, error) {
	return s.uc.GenerateDraftForSuggestion(ctx, suggestionID)
}
