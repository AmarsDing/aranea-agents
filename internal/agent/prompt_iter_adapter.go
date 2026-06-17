package agent

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	astructure "trpc.group/trpc-go/trpc-agent-go/agent/structure"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/workflow/promptiter"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/workflow/promptiter/engine"
)

// PromptIterAdapter bridges the framework's PromptIter engine to the project's
// biz.Refiner interface. When the PromptIter engine is configured, it runs a
// single-round evaluation-driven optimization via engine.Run(). When the
// engine is nil, it falls back to the injected Refiner for single-shot
// LLM refinement.
type PromptIterAdapter struct {
	fallback biz.Refiner
	eng      engine.Engine
	lg       loggateway.Logger
}

// compile-time check: PromptIterAdapter satisfies biz.Refiner.
var _ biz.Refiner = (*PromptIterAdapter)(nil)

// NewPromptIterAdapter creates a PromptIterAdapter.
// If eng is nil, all calls delegate to fallback.
func NewPromptIterAdapter(fallback biz.Refiner, eng engine.Engine, lg loggateway.Logger) *PromptIterAdapter {
	return &PromptIterAdapter{
		fallback: fallback,
		eng:      eng,
		lg:       lg.With(loggateway.Domain("prompt_iter_adapter")),
	}
}

// Refine refines the given text. When the PromptIter engine is configured,
// it runs a single-round optimization loop and extracts the accepted profile's
// surface override as the refined text. Otherwise, it delegates to the
// fallback PromptRefiner.
func (a *PromptIterAdapter) Refine(ctx context.Context, req biz.RefineRequest) (*biz.RefineResult, error) {
	if a.eng == nil {
		return a.fallback.Refine(ctx, req)
	}

	runReq := a.buildRunRequest(req)
	result, err := a.eng.Run(ctx, runReq)
	if err != nil {
		a.lg.Warn("promptiter engine run failed, falling back to single-shot refiner",
			loggateway.Err(err),
			loggateway.Str("scope", string(req.Scope)),
		)
		return a.fallback.Refine(ctx, req)
	}

	if result.Status != engine.RunStatusSucceeded {
		a.lg.Warn("promptiter engine run did not succeed, falling back",
			loggateway.Str("status", string(result.Status)),
			loggateway.Str("error_message", result.ErrorMessage),
			loggateway.Str("scope", string(req.Scope)),
		)
		return a.fallback.Refine(ctx, req)
	}

	refined, ok := a.extractRefinedText(result, req)
	if !ok {
		a.lg.Warn("promptiter produced no accepted profile override, falling back",
			loggateway.Str("scope", string(req.Scope)),
		)
		return a.fallback.Refine(ctx, req)
	}

	a.lg.Info("promptiter engine optimization accepted",
		loggateway.Str("scope", string(req.Scope)),
		loggateway.Int("rounds", result.CurrentRound),
	)

	return &biz.RefineResult{
		Refined:      refined,
		Diff:         biz.UnifiedDiffSimple(req.OriginalText, refined),
		TokensBefore: biz.EstimateTokenCount(req.OriginalText),
		TokensAfter:  biz.EstimateTokenCount(refined),
		Provider:     "promptiter",
		Model:        "engine",
		ModelSource:  biz.ModelSourcePromptIter,
	}, nil
}

// buildRunRequest converts a RefineRequest into an engine.RunRequest with
// a single-round, single-surface configuration.
func (a *PromptIterAdapter) buildRunRequest(req biz.RefineRequest) *engine.RunRequest {
	surfaceID := string(req.Scope)
	if req.FileName != "" {
		surfaceID = fmt.Sprintf("%s/%s", req.Scope, req.FileName)
	}

	initialProfile := &promptiter.Profile{
		Overrides: []promptiter.SurfaceOverride{
			{
				SurfaceID: surfaceID,
				Value:     surfaceValueFromText(req.OriginalText),
			},
		},
	}

	return &engine.RunRequest{
		TrainEvalSetIDs:     []string{"refine-train"},
		ValidationEvalSetIDs: []string{"refine-validation"},
		InitialProfile:      initialProfile,
		MaxRounds:           1,
		TargetSurfaceIDs:    []string{surfaceID},
	}
}

// extractRefinedText reads the accepted profile from the engine result and
// returns the surface override text for the requested scope.
func (a *PromptIterAdapter) extractRefinedText(result *engine.RunResult, req biz.RefineRequest) (string, bool) {
	if result.AcceptedProfile == nil {
		return "", false
	}

	surfaceID := string(req.Scope)
	if req.FileName != "" {
		surfaceID = fmt.Sprintf("%s/%s", req.Scope, req.FileName)
	}

	for _, override := range result.AcceptedProfile.Overrides {
		if override.SurfaceID == surfaceID && override.Value.Text != nil {
			return *override.Value.Text, true
		}
	}
	return "", false
}

// surfaceValueFromText creates an astructure.SurfaceValue from a plain text string.
func surfaceValueFromText(text string) astructure.SurfaceValue {
	return astructure.SurfaceValue{
		Text: &text,
	}
}
