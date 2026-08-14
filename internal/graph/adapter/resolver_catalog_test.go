package adapter

import (
	"context"
	"testing"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/pkg/loggateway"
)

func TestCatalogToolResolver_NilCatalog(t *testing.T) {
	t.Parallel()
	r := NewCatalogToolResolver(nil, loggateway.NewNoop())
	if _, err := r.ResolveTools(context.Background(), []string{"web_search"}); err == nil {
		t.Fatal("nil catalog must error")
	}
	var nilR *CatalogToolResolver
	if _, err := nilR.ResolveTools(context.Background(), []string{"web_search"}); err == nil {
		t.Fatal("nil resolver must error")
	}
}

func TestCatalogFunctionResolver_NilAndEmpty(t *testing.T) {
	t.Parallel()
	var nilR *CatalogFunctionResolver
	if _, err := nilR.ResolveFunction(context.Background(), "fn"); err == nil {
		t.Fatal("nil resolver must error")
	}
	r := NewCatalogFunctionResolver(nil, loggateway.NewNoop())
	if _, err := r.ResolveFunction(context.Background(), "fn"); err == nil {
		t.Fatal("nil catalog must error")
	}
	if _, err := r.ResolveFunction(context.Background(), "  "); err == nil {
		t.Fatal("empty func_ref must error")
	}
}

func TestCatalogAgentResolver_ResolveAgent_EmptyAndMissing(t *testing.T) {
	t.Parallel()
	var nilR *CatalogAgentResolver
	if _, err := nilR.ResolveAgent(context.Background(), "a"); err == nil {
		t.Fatal("nil resolver must error")
	}
	r := NewCatalogAgentResolver(chatagent.TRPCBuilderDeps{}, loggateway.NewNoop())
	if _, err := r.ResolveAgent(context.Background(), "  "); err == nil {
		t.Fatal("empty agent ref must error")
	}
	if _, err := r.ResolveAgent(context.Background(), "no-such-agent"); err == nil {
		t.Fatal("missing agent must error (not a silent nil agent)")
	}
}
