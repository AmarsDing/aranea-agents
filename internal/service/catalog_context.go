package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
)

type llmCatalogContext struct {
	llm interface {
		GetByProviderAndModel(ctx context.Context, provider, model string) (biz.ProviderModel, error)
	}
}

func (c llmCatalogContext) GetModelConfigJSON(ctx context.Context, provider, model string) string {
	if c.llm == nil {
		return ""
	}
	p := strings.TrimSpace(provider)
	m := strings.TrimSpace(model)
	if p == "" || m == "" {
		return ""
	}
	row, err := c.llm.GetByProviderAndModel(ctx, p, m)
	if err != nil {
		return ""
	}
	return row.ConfigJSON
}

func (o *ChatOrchestrator) llmContextCatalog() llmCatalogContext {
	if o == nil {
		return llmCatalogContext{}
	}
	return llmCatalogContext{llm: o.td.Catalog.LLM}
}
