package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// PlannerModelLookup is a narrow port for reading the planner model system
// setting. Satisfied by biz.SystemSettingRepo and biz.SystemSettingUsecase.
type PlannerModelLookup interface {
	GetPlannerModel(ctx context.Context) (biz.PlannerModelSetting, error)
}

// ResolvePlannerModel resolves the provider/model for plan_and_execute's
// internal LLM calls (decomposeTask, llmColdStart, AgentFactory).
//
// Resolution chain:
//  1. specify mode → admin-specified provider+model (if available in catalog)
//  2. inherit mode (or specify model unavailable) → session's provider/model
//  3. legacy fallback → first enabled model in catalog
//
// This replaces the legacy resolvePlannerProviderModel (env vars) +
// resolveFallbackProviderModelFromCatalog (first model, ignoring Enabled).
//
// The sessionProvider/sessionModel come from the Spirit session's
// DefaultProvider/DefaultModel (the effective model driving the current turn).
// Pass empty strings when session context is unavailable (e.g. AgentFactory
// at init time) — the resolver will skip inherit and use the catalog fallback.
func ResolvePlannerModel(
	ctx context.Context,
	setting biz.PlannerModelSetting,
	sessionProvider, sessionModel string,
	catalog *biz.LlmProviderModelUsecase,
	lg loggateway.Logger,
	stepID, component string,
) (provider, model string) {
	sessionProvider = strings.TrimSpace(sessionProvider)
	sessionModel = strings.TrimSpace(sessionModel)

	// 1. specify mode: use admin-specified model if available.
	if setting.Mode == biz.PlannerModelModeSpecify && setting.Provider != "" && setting.Model != "" {
		if isModelAvailable(ctx, catalog, setting.Provider, setting.Model) {
			lg.Info(component+": using specify-mode planner model",
				loggateway.StepID(stepID),
				loggateway.Str("provider", setting.Provider),
				loggateway.Str("model", setting.Model),
			)
			return setting.Provider, setting.Model
		}
		// Specified model unavailable — log warning, fall through to inherit.
		lg.Warn(component+": specified planner model unavailable, falling back to inherit",
			loggateway.StepID(stepID),
			loggateway.Str("provider", setting.Provider),
			loggateway.Str("model", setting.Model),
		)
	}

	// 2. inherit: use the session's effective provider/model.
	if sessionProvider != "" && sessionModel != "" {
		lg.Info(component+": using inherit-mode planner model from session",
			loggateway.StepID(stepID),
			loggateway.Str("provider", sessionProvider),
			loggateway.Str("model", sessionModel),
		)
		return sessionProvider, sessionModel
	}

	// 3. legacy fallback: first enabled model in catalog.
	return resolveFirstEnabledModelFromCatalog(ctx, catalog, lg, stepID, component)
}

// isModelAvailable checks whether a provider/model exists and is enabled in
// the catalog. Returns false when catalog is nil or the model is not found.
func isModelAvailable(ctx context.Context, catalog *biz.LlmProviderModelUsecase, provider, model string) bool {
	if catalog == nil {
		return false
	}
	_, err := catalog.GetByProviderAndModel(ctx, provider, model)
	return err == nil
}

// resolveFirstEnabledModelFromCatalog picks the first enabled model from the
// catalog. Fixes the legacy resolveFallbackProviderModelFromCatalog bug which
// did not filter by Enabled, causing "provider model not found" when the
// picked model was disabled.
func resolveFirstEnabledModelFromCatalog(ctx context.Context, catalog *biz.LlmProviderModelUsecase, lg loggateway.Logger, stepID, component string) (string, string) {
	if catalog == nil {
		return "", ""
	}
	models, err := catalog.List(ctx)
	if err != nil || len(models) == 0 {
		return "", ""
	}
	for _, m := range models {
		if m.Enabled && m.Provider != "" && m.Model != "" {
			lg.Info(component+": using fallback provider/model from catalog",
				loggateway.StepID(stepID),
				loggateway.Str("provider", m.Provider),
				loggateway.Str("model", m.Model),
			)
			return m.Provider, m.Model
		}
	}
	return "", ""
}
