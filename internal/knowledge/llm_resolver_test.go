package knowledge

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// stubSysGetter / stubCatalogLister 复用 markdown_organizer_test.go 中的定义。

func TestResolveVisionLLM_PrefersVisionCapableCatalogModel(t *testing.T) {
	catalog := stubCatalogLister{models: []biz.ProviderModel{
		{Provider: "openai", Model: "gpt-4o-mini", Enabled: true, Capabilities: biz.ModelCapabilities{Text: true}},
		{Provider: "openai", Model: "gpt-4o", Enabled: true, Capabilities: biz.ModelCapabilities{Text: true, Vision: true}},
	}}
	provider, model, err := ResolveVisionLLM(context.Background(), nil, catalog, "vision extract", loggateway.NewNoop())
	if err != nil {
		t.Fatalf("ResolveVisionLLM error: %v", err)
	}
	if provider != "openai" || model != "gpt-4o" {
		t.Errorf("got %s/%s, want openai/gpt-4o (vision-capable)", provider, model)
	}
}

func TestResolveVisionLLM_SkipsDisabledVisionModel(t *testing.T) {
	catalog := stubCatalogLister{models: []biz.ProviderModel{
		{Provider: "openai", Model: "gpt-4o", Enabled: false, Capabilities: biz.ModelCapabilities{Vision: true}},
	}}
	_, _, err := ResolveVisionLLM(context.Background(), nil, catalog, "vision extract", loggateway.NewNoop())
	if err == nil {
		t.Fatal("expected error when only vision model is disabled")
	}
}

func TestResolveVisionLLM_FallsBackToRefineLLM(t *testing.T) {
	sys := stubSysGetter{s: biz.SystemSetting{
		DefaultRefineLLM: biz.RefineLLMSetting{Provider: "openai", Model: "gpt-4o"},
	}}
	catalog := stubCatalogLister{models: []biz.ProviderModel{
		{Provider: "openai", Model: "gpt-4o-mini", Enabled: true, Capabilities: biz.ModelCapabilities{Text: true}},
	}}
	provider, model, err := ResolveVisionLLM(context.Background(), sys, catalog, "vision extract", loggateway.NewNoop())
	if err != nil {
		t.Fatalf("ResolveVisionLLM error: %v", err)
	}
	if provider != "openai" || model != "gpt-4o" {
		t.Errorf("got %s/%s, want refine-LLM fallback openai/gpt-4o", provider, model)
	}
}

func TestResolveVisionLLM_NoVisionModel(t *testing.T) {
	_, _, err := ResolveVisionLLM(context.Background(), nil, stubCatalogLister{}, "vision extract", loggateway.NewNoop())
	if err == nil {
		t.Fatal("expected explicit error when no vision model available (NFR-12)")
	}
}
