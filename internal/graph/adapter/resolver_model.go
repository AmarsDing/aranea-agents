package adapter

import (
	"context"
	"net/http"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"

	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// CatalogModelResolver resolves models via LlmProviderModelUsecase.
type CatalogModelResolver struct {
	ModelCatalog *biz.LlmProviderModelUsecase
	RT           *provider.RoundTrip
	lg           loggateway.Logger
}

var _ graphtrpc.ModelResolver = (*CatalogModelResolver)(nil)

func NewCatalogModelResolver(catalog *biz.LlmProviderModelUsecase, rt *provider.RoundTrip, lg loggateway.Logger) *CatalogModelResolver {
	if rt == nil {
		rt = &provider.RoundTrip{HTTP: &http.Client{Timeout: 120 * time.Second}}
	}
	return &CatalogModelResolver{ModelCatalog: catalog, RT: rt, lg: lg}
}

func (r *CatalogModelResolver) ResolveModel(ctx context.Context, modelName string) (trpcmodel.Model, error) {
	if r == nil || r.ModelCatalog == nil {
		return nil, apierror.Internal(apierror.DomainGraph, "graph: model catalog not configured")
	}
	prov, api, err := parseModelRef(ctx, r.ModelCatalog, modelName)
	if err != nil {
		return nil, err
	}
	return provider.TRPCModelForProviderModel(ctx, r.ModelCatalog, r.RT, prov, api, r.lg)
}

func parseModelRef(ctx context.Context, catalog *biz.LlmProviderModelUsecase, modelName string) (prov, api string, err error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return "", "", apierror.BadRequest(apierror.DomainGraph, "graph: model_name is required for LLM nodes")
	}
	for _, sep := range []string{"/", "|", ":"} {
		if i := strings.Index(modelName, sep); i > 0 {
			return strings.TrimSpace(modelName[:i]), strings.TrimSpace(modelName[i+1:]), nil
		}
	}
	api = modelName
	if catalog == nil {
		return "", api, apierror.Internal(apierror.DomainGraph, "graph: model catalog not configured")
	}
	rows, listErr := catalog.List(ctx)
	if listErr != nil {
		return "", "", listErr
	}
	for _, row := range rows {
		if strings.EqualFold(strings.TrimSpace(row.Model), api) {
			return strings.TrimSpace(row.Provider), api, nil
		}
	}
	return "", api, nil
}
