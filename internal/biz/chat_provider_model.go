package biz

import (
	"context"
	"errors"
	"strings"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"
)

// RefineLLMLookup resolves the platform-default refine LLM configuration.
// Narrow port over SystemSettingRepo so ChatUsecase depends only on the
// method it actually calls (CS-B4).
type RefineLLMLookup interface {
	GetRefineLLM(ctx context.Context) (RefineLLMSetting, error)
}

// ResolveProviderModel resolves the effective provider and model using the
// fallback chain: explicit input → RefineLLM configuration → first enabled
// model in the LLM catalog.
//
// This is the biz-layer canonical replacement for the legacy service-layer
// resolveProviderModelFallback (BA4). Nil-safe: when dependencies are not
// wired, returns the input values unchanged.
func (uc *ChatUsecase) ResolveProviderModel(ctx context.Context, provider, model string) (string, string, error) {
	if uc == nil {
		return strings.TrimSpace(provider), strings.TrimSpace(model), nil
	}
	p, m := ResolveProviderModelWithFallback(ctx, uc.modelLister, uc.refineLLM, uc.lg, provider, model)
	return p, m, nil
}

// ResolveProviderModelWithFallback resolves the effective provider and model
// using the fallback chain: explicit input → RefineLLM configuration → first
// enabled model in the LLM catalog.
//
// Catalog validation: an explicitly provided provider/model pair that does
// not exist in the catalog (ErrProviderModelNotFound) is treated as absent
// and falls through to the fallback chain. This keeps the observation path
// (context window resolution) aligned with the execution path, which falls
// back to the system default model at agent build time. Catalog query
// failures other than NotFound (e.g. DB errors) are non-blocking: the
// original values are returned unchanged.
//
// Shared by ChatUsecase (single-agent path) and the team Runner (team path)
// so both paths resolve the same effective model. Nil-safe: nil catalog or
// nil refine lookup simply disables that fallback stage.
func ResolveProviderModelWithFallback(ctx context.Context, catalog TeamModelCatalog, refine RefineLLMLookup, lg loggateway.Logger, provider, model string) (string, string) {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if provider != "" && model != "" && catalog != nil {
		if _, err := catalog.GetByProviderAndModel(ctx, provider, model); err == nil {
			return provider, model
		} else if !errors.Is(err, ErrProviderModelNotFound) {
			return provider, model
		}
		if lg != nil {
			lg.Warn("配置模型不在模型目录中，回退解析",
				loggateway.Str("orig_provider", provider),
				loggateway.Str("orig_model", model))
		}
		provider, model = "", ""
	}
	// Caller-explicit sides (survivors of the pair validation above). M7
	// (2026-10-18): a mixed-source pair must never ship — when only one side
	// was given and the refine stage completes the other, the composed pair
	// is re-validated as a unit; on a miss the refine-filled side is dropped
	// and resolution falls to a single catalog row matching the explicit
	// side. Previously the pair sailed through unchecked (provider A +
	// provider B's model) and failed at call time with a nonexistent
	// endpoint.
	explicitProvider, explicitModel := provider, model
	refineProvider, refineModel := "", ""
	if refine != nil {
		if setting, err := refine.GetRefineLLM(ctx); err == nil {
			refineProvider, refineModel = setting.Provider, setting.Model
			provider = strutil.FirstNonEmpty(provider, refineProvider)
			model = strutil.FirstNonEmpty(model, refineModel)
		}
	}
	if provider != "" && model != "" && catalog != nil {
		pureRefinePair := refineProvider != "" && refineModel != "" &&
			provider == refineProvider && model == refineModel
		// The pure RefineLLM pair is operator configuration and is trusted
		// as-is (it may intentionally live outside the tenant catalog);
		// anything else is a mixed-source pair and is unit-validated (M7).
		if !pureRefinePair {
			if _, err := catalog.GetByProviderAndModel(ctx, provider, model); err == nil {
				return provider, model
			} else if !errors.Is(err, ErrProviderModelNotFound) {
				return provider, model // infra errors stay non-blocking
			}
			if lg != nil {
				lg.Warn("provider/model 组合不在模型目录中，回退到目录默认",
					loggateway.Str("orig_provider", provider),
					loggateway.Str("orig_model", model))
			}
			provider, model = explicitProvider, explicitModel
		}
	}
	if provider != "" && model != "" {
		return provider, model
	}
	if catalog != nil {
		if models, err := catalog.List(ctx); err == nil {
			// M7: complete the pair atomically from ONE catalog row — the
			// row must match the surviving explicit side (never fill the
			// missing side from an unrelated provider's row).
			for _, m := range models {
				if !m.Enabled || m.Provider == "" || m.Model == "" {
					continue
				}
				if provider != "" && m.Provider != provider {
					continue
				}
				if model != "" && m.Model != model {
					continue
				}
				provider, model = m.Provider, m.Model
				break
			}
		}
	}
	return provider, model
}

// SyncSessionProviderModel updates the session's DefaultProvider/DefaultModel
// when they differ from the resolved values.
//
// This is the biz-layer canonical replacement for the legacy service-layer
// syncSessionProviderModel (BA4). Nil-safe: when the session updater is not
// wired or the values already match, the call is a no-op.
func (uc *ChatUsecase) SyncSessionProviderModel(ctx context.Context, sessionID string, sess Session, provider, model string) error {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if provider == "" || model == "" {
		return nil
	}
	if sess.DefaultProvider == provider && sess.DefaultModel == model {
		return nil
	}
	if uc == nil || uc.sessionUpdater == nil {
		return nil
	}
	p, m := provider, model
	if _, err := uc.sessionUpdater.Update(ctx, sessionID, SessionUpdateFields{
		DefaultProvider: &p,
		DefaultModel:    &m,
	}); err != nil {
		uc.lg.Warn("sync session provider model failed",
			loggateway.Err(err),
			loggateway.Str("session_id", sessionID))
		return err
	}
	return nil
}

// SetRefineLLMLookup wires the refine LLM lookup dependency. Nil-safe.
func (uc *ChatUsecase) SetRefineLLMLookup(r RefineLLMLookup) {
	if uc == nil {
		return
	}
	uc.refineLLM = r
}

// SetModelLister wires the enabled model lister dependency. Nil-safe.
func (uc *ChatUsecase) SetModelLister(l TeamModelCatalog) {
	if uc == nil {
		return
	}
	uc.modelLister = l
}

// SetSessionUpdater wires the session updater dependency. Nil-safe.
func (uc *ChatUsecase) SetSessionUpdater(u SessionCRUDPort) {
	if uc == nil {
		return
	}
	uc.sessionUpdater = u
}
