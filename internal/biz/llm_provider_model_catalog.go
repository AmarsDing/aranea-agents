package biz

import "context"

// OnCatalogChange registers a hook invoked after successful Create/Update/Delete.
// Used to ResetBuild the skill-evolution service when models are hot-added (SKL-02).
func (u *LlmProviderModelUsecase) OnCatalogChange(fn func()) {
	if u == nil || fn == nil {
		return
	}
	u.catalogChangeMu.Lock()
	defer u.catalogChangeMu.Unlock()
	u.catalogChangeHooks = append(u.catalogChangeHooks, fn)
}

func (u *LlmProviderModelUsecase) notifyCatalogChange() {
	if u == nil {
		return
	}
	u.catalogChangeMu.Lock()
	hooks := append([]func(){}, u.catalogChangeHooks...)
	u.catalogChangeMu.Unlock()
	for _, fn := range hooks {
		fn()
	}
}

// RevealCredentials returns decrypted credentials for admin edit UI (never logged).
func (u *LlmProviderModelUsecase) RevealCredentials(ctx context.Context, id string) (ProviderCredentialsReveal, error) {
	id, err := requireNonEmpty(id, "LLM_PROVIDER_MODEL", "id")
	if err != nil {
		return ProviderCredentialsReveal{}, err
	}
	m, err := u.reader.GetProviderModel(ctx, id)
	if err != nil {
		return ProviderCredentialsReveal{}, err
	}
	return u.crypto.RevealCredentialsFromConfig(ctx, m.ConfigJSON)
}

