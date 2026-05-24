package adapter

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/internal/provider"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// CatalogModelResolver resolves models via LlmProviderModelUsecase.
type CatalogModelResolver struct {
	Catalog *biz.LlmProviderModelUsecase
	RT      *provider.RoundTrip
}

var _ graphtrpc.ModelResolver = (*CatalogModelResolver)(nil)

func NewCatalogModelResolver(catalog *biz.LlmProviderModelUsecase, rt *provider.RoundTrip) *CatalogModelResolver {
	if rt == nil {
		rt = &provider.RoundTrip{HTTP: &http.Client{Timeout: 120 * time.Second}}
	}
	return &CatalogModelResolver{Catalog: catalog, RT: rt}
}

func (r *CatalogModelResolver) ResolveModel(ctx context.Context, modelName string) (trpcmodel.Model, error) {
	if r == nil || r.Catalog == nil {
		return nil, fmt.Errorf("graph: model catalog not configured")
	}
	prov, api, err := parseModelRef(ctx, r.Catalog, modelName)
	if err != nil {
		return nil, err
	}
	return provider.TRPCModelForProviderModel(ctx, r.Catalog, r.RT, prov, api)
}

func parseModelRef(ctx context.Context, catalog *biz.LlmProviderModelUsecase, modelName string) (prov, api string, err error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return "", "", fmt.Errorf("graph: model_name is required for LLM nodes")
	}
	for _, sep := range []string{"/", "|", ":"} {
		if i := strings.Index(modelName, sep); i > 0 {
			return strings.TrimSpace(modelName[:i]), strings.TrimSpace(modelName[i+1:]), nil
		}
	}
	api = modelName
	rows, listErr := catalog.List(ctx)
	if listErr != nil {
		return "", api, nil
	}
	for _, row := range rows {
		if strings.EqualFold(strings.TrimSpace(row.Model), api) {
			return strings.TrimSpace(row.Provider), api, nil
		}
	}
	return "", api, nil
}
