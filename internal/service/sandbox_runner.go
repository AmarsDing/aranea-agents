package service

import (
	"context"
	"encoding/json"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// SandboxRunner validates skill evolution suggestions in an isolated environment.
type SandboxRunner struct {
	uc *biz.SkillIntelligenceUsecase
	lg loggateway.Logger
}

func NewSandboxRunner(uc *biz.SkillIntelligenceUsecase, lg loggateway.Logger) *SandboxRunner {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SandboxRunner{uc: uc, lg: lg}
}

// ValidateSuggestion runs sandbox validation on an evolution suggestion.
// For v1, this performs rule-based validation only (no actual skill execution).
func (s *SandboxRunner) ValidateSuggestion(ctx context.Context, suggestionID string) (bool, json.RawMessage, error) {
	suggestion, err := s.uc.GetEvolutionSuggestion(ctx, suggestionID)
	if err != nil {
		return false, nil, err
	}
	if suggestion == nil {
		return false, nil, kerrors.NotFound("SANDBOX_RUNNER", fmt.Sprintf("suggestion not found: %s", suggestionID))
	}

	// Update lifecycle status to validating
	if lcErr := s.uc.UpdateSuggestionLifecycleStatus(ctx, suggestionID, biz.EvoLifecycleValidating); lcErr != nil {
		s.lg.Warn("SandboxRunner: failed to set validating status",
			loggateway.StepID("sandbox.validate"),
			loggateway.Str("suggestion_id", suggestionID),
			loggateway.Err(lcErr))
	}

	result := s.ruleBasedValidation(suggestion)
	resultJSON, _ := json.Marshal(result)

	// Update the suggestion with sandbox result
	if err := s.uc.UpdateSuggestionSandboxResult(ctx, suggestionID, result.Passed, resultJSON); err != nil {
		s.lg.Warn("SandboxRunner: failed to update sandbox result",
			loggateway.StepID("sandbox.validate"),
			loggateway.Str("suggestion_id", suggestionID),
			loggateway.Err(err))
	}

	// Update lifecycle status based on validation result
	lifecycleStatus := biz.EvoLifecycleDraft
	if result.Passed {
		lifecycleStatus = biz.EvoLifecycleReady
	}
	if lcErr := s.uc.UpdateSuggestionLifecycleStatus(ctx, suggestionID, lifecycleStatus); lcErr != nil {
		s.lg.Warn("SandboxRunner: failed to update lifecycle status",
			loggateway.StepID("sandbox.validate"),
			loggateway.Str("suggestion_id", suggestionID),
			loggateway.Err(lcErr))
	}

	return result.Passed, resultJSON, nil
}

type sandboxValidationResult struct {
	Passed  bool    `json:"passed"`
	Checks  []check `json:"checks"`
	Message string  `json:"message"`
}

type check struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

func (s *SandboxRunner) ruleBasedValidation(suggestion *biz.SkillEvolutionSuggestion) *sandboxValidationResult {
	result := &sandboxValidationResult{}

	// Check 1: Draft body is not empty
	c1 := check{Name: "draft_body_not_empty", Passed: suggestion.DraftSkillBody != "", Message: "Draft body must not be empty"}
	result.Checks = append(result.Checks, c1)

	// Check 2: Draft body length is reasonable (< 10000 chars)
	c2 := check{Name: "draft_body_length", Passed: len(suggestion.DraftSkillBody) < 10000, Message: "Draft body length must be under 10000 chars"}
	result.Checks = append(result.Checks, c2)

	// Check 3: Skill ID is valid
	c3 := check{Name: "skill_id_valid", Passed: suggestion.SkillID != "", Message: "Skill ID must be valid"}
	result.Checks = append(result.Checks, c3)

	// Overall pass if all checks pass
	result.Passed = c1.Passed && c2.Passed && c3.Passed
	if result.Passed {
		result.Message = "All validation checks passed"
	} else {
		result.Message = "Some validation checks failed"
	}

	return result
}
