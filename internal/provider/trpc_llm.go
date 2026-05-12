package provider

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcopenai "trpc.group/trpc-go/trpc-agent-go/model/openai"
)

func TRPCModelForProviderModel(ctx context.Context, catalog *biz.LlmProviderModelUsecase, rt *RoundTrip, prov, modelAPI string) (trpcmodel.Model, error) {
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
	return trpcModelFromCatalogConfig(cfg, rt)
}

func trpcModelFromCatalogConfig(cfg CatalogConfig, rt *RoundTrip) (trpcmodel.Model, error) {
	name := strings.TrimSpace(cfg.ModelAPI)
	if name == "" {
		return nil, ErrNilLlmCatalog
	}
	opts := []trpcopenai.Option{}
	if baseURL := strings.TrimSpace(cfg.BaseURL); baseURL != "" {
		opts = append(opts, trpcopenai.WithBaseURL(baseURL))
	}
	if apiKey := strings.TrimSpace(cfg.APIKey); apiKey != "" {
		opts = append(opts, trpcopenai.WithAPIKey(apiKey))
	}
	if rt != nil && rt.HTTP != nil && rt.HTTP.Transport != nil {
		opts = append(opts, trpcopenai.WithHTTPClientOptions(trpcopenai.WithHTTPClientTransport(rt.HTTP.Transport)))
	}
	return trpcopenai.New(name, opts...), nil
}
