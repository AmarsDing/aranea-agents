package biz

import (
	"context"
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
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if provider != "" && model != "" {
		return provider, model, nil
	}
	if uc != nil && uc.refineLLM != nil {
		if refine, err := uc.refineLLM.GetRefineLLM(ctx); err == nil {
			provider = strutil.FirstNonEmpty(provider, refine.Provider)
			model = strutil.FirstNonEmpty(model, refine.Model)
		}
	}
	if provider != "" && model != "" {
		return provider, model, nil
	}
	if uc != nil && uc.modelLister != nil {
		if models, err := uc.modelLister.List(ctx); err == nil {
			for _, m := range models {
				if m.Enabled && m.Provider != "" && m.Model != "" {
					provider = strutil.FirstNonEmpty(provider, m.Provider)
					model = strutil.FirstNonEmpty(model, m.Model)
					break
				}
			}
		}
	}
	return provider, model, nil
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
