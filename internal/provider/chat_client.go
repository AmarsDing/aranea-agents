package provider

import (
	"context"
	"iter"
	"strings"

	"aranea-agents/internal/biz"

)

// CatalogClient wires biz catalog rows to model.LLM + LLMRequest execution.
type CatalogClient struct {
	Registry *Registry
	RT       *RoundTrip
}

// NewCatalogClient returns a client using DefaultRegistry().
func NewCatalogClient(rt *RoundTrip) *CatalogClient {
	return &CatalogClient{
		Registry: DefaultRegistry(),
		RT:       rt,
	}
}

// LLM resolves model.LLM for a catalog row (connection fields from ConfigJSON).
func (c *CatalogClient) LLM(ctx context.Context, pm biz.ProviderModel) (LLM, error) {
	cfg, err := CatalogFromProviderModel(pm)
	if err != nil {
		return nil, err
	}
	cfg = MergeCatalogConfig(cfg, pm.ConfigJSON)
	return c.Registry.Resolve(cfg, roundOrNil(c.RT))
}

// MergeCatalogIntoRequest fills Model on LLMRequest when empty.
func MergeCatalogIntoRequest(cfg CatalogConfig, req *LLMRequest) {
	if req == nil {
		return
	}
	if strings.TrimSpace(req.Model) == "" && strings.TrimSpace(cfg.ModelAPI) != "" {
		req.Model = cfg.ModelAPI
	}
}

// GenerateContent invokes model.LLM.GenerateContent after resolving the backend and merging catalog model id.
func (c *CatalogClient) GenerateContent(ctx context.Context, pm biz.ProviderModel, req *LLMRequest, stream bool) iter.Seq2[*LLMResponse, error] {
	cfg, err := CatalogFromProviderModel(pm)
	if err != nil {
		return errSeq(err)
	}
	cfg = MergeCatalogConfig(cfg, pm.ConfigJSON)
	llmImpl, err := c.Registry.Resolve(cfg, roundOrNil(c.RT))
	if err != nil {
		return errSeq(err)
	}
	MergeCatalogIntoRequest(cfg, req)
	return llmImpl.GenerateContent(ctx, req, stream)
}

func errSeq(err error) iter.Seq2[*LLMResponse, error] {
	return func(yield func(*LLMResponse, error) bool) {
		yield(nil, err)
	}
}
