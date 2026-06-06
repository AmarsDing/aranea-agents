package service

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	kerrors "github.com/go-kratos/kratos/v2/errors"
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

// GenerateDraft generates a draft skill body improvement using LLM.
// If LLM is unavailable, it falls back to rule-based suggestions.
func (s *SkillCuratorService) GenerateDraft(ctx context.Context, suggestionID string) (string, error) {
	suggestion, err := s.uc.GetEvolutionSuggestion(ctx, suggestionID)
	if err != nil {
		return "", err
	}
	if suggestion == nil {
		return "", kerrors.NotFound("SKILL_CURATOR", fmt.Sprintf("suggestion not found: %s", suggestionID))
	}

	// For v1, use a rule-based approach as the primary generator.
	// LLM integration will be added in a future iteration.
	draft := s.generateRuleBasedDraft(suggestion)

	// Update the suggestion with the draft
	if err := s.uc.UpdateSuggestionDraftBody(ctx, suggestionID, draft); err != nil {
		s.lg.Warn("SkillCurator: failed to update draft body",
			loggateway.StepID("skill_curator.generate"),
			loggateway.Str("suggestion_id", suggestionID),
			loggateway.Err(err))
	}

	// Update lifecycle status to draft (indicates draft has been generated)
	if lcErr := s.uc.UpdateSuggestionLifecycleStatus(ctx, suggestionID, biz.EvoLifecycleDraft); lcErr != nil {
		s.lg.Warn("SkillCurator: failed to update lifecycle status",
			loggateway.StepID("skill_curator.generate"),
			loggateway.Str("suggestion_id", suggestionID),
			loggateway.Err(lcErr))
	}

	return draft, nil
}

func (s *SkillCuratorService) generateRuleBasedDraft(suggestion *biz.SkillEvolutionSuggestion) string {
	switch suggestion.Type {
	case biz.EvoSuggestionFixFailure:
		return fmt.Sprintf("# Skill Evolution Draft (fix_failure)\n\n"+
			"Original skill: %s\n"+
			"Trigger reason: %s\n\n"+
			"## Suggested improvements:\n"+
			"1. Add error handling for common failure patterns\n"+
			"2. Improve parameter validation\n"+
			"3. Add retry logic for transient failures\n",
			suggestion.SkillID, suggestion.TriggerReason)
	case biz.EvoSuggestionBoostEfficiency:
		return fmt.Sprintf("# Skill Evolution Draft (boost_efficiency)\n\n"+
			"Original skill: %s\n"+
			"Trigger reason: %s\n\n"+
			"## Suggested improvements:\n"+
			"1. Optimize prompt to reduce token usage\n"+
			"2. Cache frequently used results\n"+
			"3. Reduce unnecessary tool calls\n",
			suggestion.SkillID, suggestion.TriggerReason)
	case biz.EvoSuggestionMergeDuplicate:
		return fmt.Sprintf("# Skill Evolution Draft (merge_duplicate)\n\n"+
			"Original skill: %s\n"+
			"Trigger reason: %s\n\n"+
			"## Suggested improvements:\n"+
			"1. Consolidate overlapping functionality\n"+
			"2. Unify parameter interfaces\n"+
			"3. Merge description and instructions\n",
			suggestion.SkillID, suggestion.TriggerReason)
	default:
		return fmt.Sprintf("# Skill Evolution Draft\n\nOriginal skill: %s\nTrigger: %s\n",
			suggestion.SkillID, suggestion.TriggerReason)
	}
}
