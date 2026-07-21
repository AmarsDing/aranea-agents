package biz

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/loggateway"
)

type stubModelCatalog struct {
	models  []ProviderModel
	getErr  error
	listErr error
}

func (s *stubModelCatalog) GetByProviderAndModel(_ context.Context, provider, model string) (ProviderModel, error) {
	if s.getErr != nil {
		return ProviderModel{}, s.getErr
	}
	for _, m := range s.models {
		if m.Provider == provider && m.Model == model {
			return m, nil
		}
	}
	return ProviderModel{}, ErrProviderModelNotFound
}

func (s *stubModelCatalog) List(context.Context) ([]ProviderModel, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.models, nil
}

type stubRefineLLMLookup struct {
	setting RefineLLMSetting
	err     error
}

func (s *stubRefineLLMLookup) GetRefineLLM(context.Context) (RefineLLMSetting, error) {
	return s.setting, s.err
}

func newResolveTestUsecase(catalog TeamModelCatalog, refine RefineLLMLookup) *ChatUsecase {
	uc := NewChatUsecase(nil, nil, nil, nil, nil, loggateway.NewNoop())
	uc.SetModelLister(catalog)
	uc.SetRefineLLMLookup(refine)
	return uc
}

func TestResolveProviderModel_InCatalog_KeepsOriginal(t *testing.T) {
	catalog := &stubModelCatalog{models: []ProviderModel{
		{Provider: "deepseek", Model: "deepseek-v4-flash", Enabled: true},
	}}
	uc := newResolveTestUsecase(catalog, nil)

	prov, mod, err := uc.ResolveProviderModel(context.Background(), "deepseek", "deepseek-v4-flash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prov != "deepseek" || mod != "deepseek-v4-flash" {
		t.Fatalf("expected original values, got %s/%s", prov, mod)
	}
}

func TestResolveProviderModel_NotInCatalog_FallsBackToRefineLLM(t *testing.T) {
	catalog := &stubModelCatalog{models: []ProviderModel{
		{Provider: "deepseek", Model: "deepseek-v4-flash", Enabled: true},
	}}
	refine := &stubRefineLLMLookup{setting: RefineLLMSetting{Provider: "deepseek", Model: "deepseek-v4-flash"}}
	uc := newResolveTestUsecase(catalog, refine)

	prov, mod, err := uc.ResolveProviderModel(context.Background(), "deepseek", "deepseek-chat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prov != "deepseek" || mod != "deepseek-v4-flash" {
		t.Fatalf("expected fallback to refine LLM deepseek/deepseek-v4-flash, got %s/%s", prov, mod)
	}
}

func TestResolveProviderModel_NotInCatalog_FallsBackToFirstEnabled(t *testing.T) {
	catalog := &stubModelCatalog{models: []ProviderModel{
		{Provider: "openai", Model: "gpt-4o-mini", Enabled: true},
	}}
	uc := newResolveTestUsecase(catalog, nil)

	prov, mod, err := uc.ResolveProviderModel(context.Background(), "deepseek", "deepseek-chat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prov != "openai" || mod != "gpt-4o-mini" {
		t.Fatalf("expected fallback to first enabled model, got %s/%s", prov, mod)
	}
}

func TestResolveProviderModel_NotInCatalog_NoFallback_ReturnsEmpty(t *testing.T) {
	catalog := &stubModelCatalog{models: []ProviderModel{
		{Provider: "openai", Model: "gpt-4o-mini", Enabled: false},
	}}
	uc := newResolveTestUsecase(catalog, nil)

	prov, mod, err := uc.ResolveProviderModel(context.Background(), "deepseek", "deepseek-chat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prov != "" || mod != "" {
		t.Fatalf("expected empty values when catalog miss and no fallback available, got %s/%s", prov, mod)
	}
}

func TestResolveProviderModel_CatalogDBError_KeepsOriginal(t *testing.T) {
	catalog := &stubModelCatalog{getErr: errors.New("db connection lost")}
	uc := newResolveTestUsecase(catalog, nil)

	prov, mod, err := uc.ResolveProviderModel(context.Background(), "deepseek", "deepseek-chat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prov != "deepseek" || mod != "deepseek-chat" {
		t.Fatalf("expected original values on catalog DB error (non-blocking), got %s/%s", prov, mod)
	}
}

func TestResolveProviderModel_NilCatalog_KeepsOriginal(t *testing.T) {
	uc := newResolveTestUsecase(nil, nil)

	prov, mod, err := uc.ResolveProviderModel(context.Background(), "deepseek", "deepseek-chat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prov != "deepseek" || mod != "deepseek-chat" {
		t.Fatalf("expected original values with nil catalog, got %s/%s", prov, mod)
	}
}

func TestResolveProviderModel_EmptyInput_UsesFallbackChain(t *testing.T) {
	catalog := &stubModelCatalog{models: []ProviderModel{
		{Provider: "openai", Model: "gpt-4o-mini", Enabled: true},
	}}
	refine := &stubRefineLLMLookup{setting: RefineLLMSetting{Provider: "deepseek", Model: "deepseek-v4-flash"}}
	uc := newResolveTestUsecase(catalog, refine)

	prov, mod, err := uc.ResolveProviderModel(context.Background(), "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prov != "deepseek" || mod != "deepseek-v4-flash" {
		t.Fatalf("expected refine LLM for empty input, got %s/%s", prov, mod)
	}
}

// ---------------------------------------------------------------------------
// Package-level ResolveProviderModelWithFallback: shared by ChatUsecase
// (single-agent path) and team Runner (team path) so that observation and
// execution paths resolve the same model.
// ---------------------------------------------------------------------------

func TestResolveProviderModelWithFallback_InCatalog_KeepsOriginal(t *testing.T) {
	catalog := &stubModelCatalog{models: []ProviderModel{
		{Provider: "deepseek", Model: "deepseek-v4-flash", Enabled: true},
	}}
	prov, mod := ResolveProviderModelWithFallback(context.Background(), catalog, nil, loggateway.NewNoop(), "deepseek", "deepseek-v4-flash")
	if prov != "deepseek" || mod != "deepseek-v4-flash" {
		t.Fatalf("expected original values, got %s/%s", prov, mod)
	}
}

func TestResolveProviderModelWithFallback_NotInCatalog_FallsBackToRefineLLM(t *testing.T) {
	catalog := &stubModelCatalog{models: []ProviderModel{
		{Provider: "deepseek", Model: "deepseek-v4-flash", Enabled: true},
	}}
	refine := &stubRefineLLMLookup{setting: RefineLLMSetting{Provider: "deepseek", Model: "deepseek-v4-flash"}}
	prov, mod := ResolveProviderModelWithFallback(context.Background(), catalog, refine, loggateway.NewNoop(), "deepseek", "deepseek-chat")
	if prov != "deepseek" || mod != "deepseek-v4-flash" {
		t.Fatalf("expected fallback to refine LLM deepseek/deepseek-v4-flash, got %s/%s", prov, mod)
	}
}

func TestResolveProviderModelWithFallback_NotInCatalog_FallsBackToFirstEnabled(t *testing.T) {
	catalog := &stubModelCatalog{models: []ProviderModel{
		{Provider: "openai", Model: "gpt-4o-mini", Enabled: true},
	}}
	prov, mod := ResolveProviderModelWithFallback(context.Background(), catalog, nil, loggateway.NewNoop(), "deepseek", "deepseek-chat")
	if prov != "openai" || mod != "gpt-4o-mini" {
		t.Fatalf("expected fallback to first enabled model, got %s/%s", prov, mod)
	}
}

func TestResolveProviderModelWithFallback_CatalogDBError_KeepsOriginal(t *testing.T) {
	catalog := &stubModelCatalog{getErr: errors.New("db connection lost")}
	prov, mod := ResolveProviderModelWithFallback(context.Background(), catalog, nil, loggateway.NewNoop(), "deepseek", "deepseek-chat")
	if prov != "deepseek" || mod != "deepseek-chat" {
		t.Fatalf("expected original values on catalog DB error (non-blocking), got %s/%s", prov, mod)
	}
}

func TestResolveProviderModelWithFallback_NilDeps_KeepsOriginal(t *testing.T) {
	prov, mod := ResolveProviderModelWithFallback(context.Background(), nil, nil, loggateway.NewNoop(), "deepseek", "deepseek-chat")
	if prov != "deepseek" || mod != "deepseek-chat" {
		t.Fatalf("expected original values with nil deps, got %s/%s", prov, mod)
	}
}
