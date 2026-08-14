package adapter

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type listErrReader struct{ err error }

func (r listErrReader) ListProviderModels(context.Context) ([]biz.ProviderModel, error) {
	return nil, r.err
}
func (r listErrReader) SearchProviderModels(context.Context, biz.ProviderModelListQuery) (biz.ProviderModelListResult, error) {
	return biz.ProviderModelListResult{}, r.err
}
func (r listErrReader) GetProviderModel(context.Context, string) (biz.ProviderModel, error) {
	return biz.ProviderModel{}, r.err
}
func (r listErrReader) GetProviderModelByProviderAndModel(context.Context, string, string) (biz.ProviderModel, error) {
	return biz.ProviderModel{}, r.err
}

type listOKReader struct{ items []biz.ProviderModel }

func (r listOKReader) ListProviderModels(context.Context) ([]biz.ProviderModel, error) {
	return r.items, nil
}
func (r listOKReader) SearchProviderModels(context.Context, biz.ProviderModelListQuery) (biz.ProviderModelListResult, error) {
	return biz.ProviderModelListResult{Items: r.items}, nil
}
func (r listOKReader) GetProviderModel(context.Context, string) (biz.ProviderModel, error) {
	return biz.ProviderModel{}, nil
}
func (r listOKReader) GetProviderModelByProviderAndModel(context.Context, string, string) (biz.ProviderModel, error) {
	return biz.ProviderModel{}, nil
}

func TestParseModelRef_EmptyName(t *testing.T) {
	t.Parallel()
	_, _, err := parseModelRef(context.Background(), nil, "  ")
	if err == nil {
		t.Fatal("empty model name must error")
	}
}

func TestParseModelRef_Separators(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, prov, api string
	}{
		{"openai/gpt-4", "openai", "gpt-4"},
		{"openai|gpt-4", "openai", "gpt-4"},
		{"openai:gpt-4", "openai", "gpt-4"},
	}
	for _, tc := range cases {
		prov, api, err := parseModelRef(context.Background(), nil, tc.in)
		if err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if prov != tc.prov || api != tc.api {
			t.Fatalf("%s: prov=%q api=%q", tc.in, prov, api)
		}
	}
}

func TestParseModelRef_CatalogListErrorIsObservable(t *testing.T) {
	t.Parallel()
	uc := biz.NewLlmProviderModelUsecase(listErrReader{err: errors.New("catalog down")}, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	_, _, err := parseModelRef(context.Background(), uc, "gpt-4")
	if err == nil {
		t.Fatal("catalog List failure must surface, not be swallowed as empty provider")
	}
}

func TestParseModelRef_BareNameLooksUpProvider(t *testing.T) {
	t.Parallel()
	uc := biz.NewLlmProviderModelUsecase(listOKReader{items: []biz.ProviderModel{
		{Provider: "openai", Model: "gpt-4"},
	}}, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	prov, api, err := parseModelRef(context.Background(), uc, "gpt-4")
	if err != nil {
		t.Fatal(err)
	}
	if prov != "openai" || api != "gpt-4" {
		t.Fatalf("prov=%q api=%q", prov, api)
	}
}

func TestCatalogModelResolver_NilCatalog(t *testing.T) {
	t.Parallel()
	r := NewCatalogModelResolver(nil, nil, loggateway.NewNoop())
	if _, err := r.ResolveModel(context.Background(), "openai/gpt-4"); err == nil {
		t.Fatal("nil catalog must error")
	}
	var nilR *CatalogModelResolver
	if _, err := nilR.ResolveModel(context.Background(), "openai/gpt-4"); err == nil {
		t.Fatal("nil resolver must error")
	}
}
