package provider

import (
	"context"
	"errors"
	"strings"

	"aranea-agents/internal/biz"

	"google.golang.org/adk/model"
)

var (
	// ErrNilLlmCatalog is returned when LlmProviderModelUsecase is nil.
	ErrNilLlmCatalog = errors.New("provider: nil llm catalog usecase")
)

// ModelForProviderModel resolves the catalog row to [model.LLM] using the process registry (same as native chat).
func ModelForProviderModel(ctx context.Context, catalog *biz.LlmProviderModelUsecase, rt *RoundTrip, prov, modelAPI string) (model.LLM, error) {
	_ = ctx
	if catalog == nil {
		return nil, ErrNilLlmCatalog
	}
	pm, err := catalog.GetByProviderAndModel(ctx, strings.TrimSpace(prov), strings.TrimSpace(modelAPI))
	if err != nil {
		return nil, err
	}
	cfg, err := CatalogFromProviderModel(pm)
	if err != nil {
		return nil, err
	}
	cfg = MergeCatalogConfig(cfg, pm.ConfigJSON)
	return DefaultRegistry().Resolve(cfg, roundTripOrNilADK(rt))
}

func roundTripOrNilADK(rt *RoundTrip) *RoundTrip {
	if rt == nil {
		return &RoundTrip{}
	}
	return rt
}
