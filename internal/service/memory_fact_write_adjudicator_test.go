package service

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
)

func TestNewMemoryFactWriteAdjudicator_NilDeps(t *testing.T) {
	if got := NewMemoryFactWriteAdjudicator(MemoryFactWriteAdjudicatorConfig{}); got != nil {
		t.Fatal("nil ModelCatalog must yield nil adjudicator")
	}
	if got := NewMemoryFactWriteAdjudicator(MemoryFactWriteAdjudicatorConfig{
		ModelCatalog: &biz.LlmProviderModelUsecase{},
	}); got != nil {
		t.Fatal("nil RoundTrip must yield nil adjudicator")
	}
	if got := NewMemoryFactWriteAdjudicator(MemoryFactWriteAdjudicatorConfig{
		ModelCatalog: &biz.LlmProviderModelUsecase{},
		RoundTrip:    &provider.RoundTrip{},
		LLMDisabled:  true,
	}); got != nil {
		t.Fatal("LLMDisabled must yield nil adjudicator")
	}
}

func TestMemoryFactWriteAdjudicator_NilReceiver(t *testing.T) {
	var a *MemoryFactWriteAdjudicator
	verdicts, err := a.AdjudicateFactWrites(context.Background(), "agent", "user", []biz.FactAdjudicationItem{{}})
	if err != nil || verdicts != nil {
		t.Fatalf("nil receiver: got (%v, %v), want (nil, nil)", verdicts, err)
	}
}

func TestMemoryFactWriteAdjudicator_EmptyBatch(t *testing.T) {
	a := NewMemoryFactWriteAdjudicator(MemoryFactWriteAdjudicatorConfig{
		ModelCatalog: &biz.LlmProviderModelUsecase{},
		RoundTrip:    &provider.RoundTrip{},
	})
	if a == nil {
		t.Fatal("adjudicator should be constructed with valid deps")
	}
	verdicts, err := a.AdjudicateFactWrites(context.Background(), "agent", "user", nil)
	if err != nil || verdicts != nil {
		t.Fatalf("empty batch: got (%v, %v), want (nil, nil)", verdicts, err)
	}
}

func TestMemoryFactWriteAdjudicator_NoModelResolved(t *testing.T) {
	// agents == nil + empty agent → resolveProviderModel returns "" →
	// ErrLLMExtractorUnavailable (pipeline then falls back to heuristic).
	a := NewMemoryFactWriteAdjudicator(MemoryFactWriteAdjudicatorConfig{
		ModelCatalog: &biz.LlmProviderModelUsecase{},
		RoundTrip:    &provider.RoundTrip{},
	})
	_, err := a.AdjudicateFactWrites(context.Background(), "", "user", []biz.FactAdjudicationItem{{
		Candidate: biz.FactWriteCandidate{Statement: "用户喜欢茶", FactKind: "preference"},
	}})
	if !errors.Is(err, biz.ErrLLMExtractorUnavailable) {
		t.Fatalf("got %v, want ErrLLMExtractorUnavailable", err)
	}
}
