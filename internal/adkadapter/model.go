package adkadapter

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"

	"google.golang.org/adk/model"
)

// ModelForProviderModel resolves the catalog row to [model.LLM] using the same registry as native chat.
func ModelForProviderModel(ctx context.Context, catalog *biz.LlmProviderModelUsecase, rt *provider.RoundTrip, prov, modelAPI string) (model.LLM, error) {
	_ = ctx
	if catalog == nil {
		return nil, ErrNilCatalog
	}
	pm, err := catalog.GetByProviderAndModel(ctx, strings.TrimSpace(prov), strings.TrimSpace(modelAPI))
	if err != nil {
		return nil, err
	}
	cfg, err := provider.CatalogFromProviderModel(pm)
	if err != nil {
		return nil, err
	}
	cfg = provider.MergeCatalogConfig(cfg, pm.ConfigJSON)
	return provider.DefaultRegistry().Resolve(cfg, roundTripOrNil(rt))
}

func roundTripOrNil(rt *provider.RoundTrip) *provider.RoundTrip {
	if rt == nil {
		return &provider.RoundTrip{}
	}
	return rt
}
