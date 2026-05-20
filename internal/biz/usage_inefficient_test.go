package biz

import (
	"context"
	"testing"
)

func TestInefficientModels_flagsHighCostLowTPS(t *testing.T) {
	repo := &stubUsageRepo{}
	repo.topModels = []UsageBreakdownRow{{
		ProviderCode:       "openai",
		ModelAPIID:         "gpt-4",
		ModelDisplayName:   "GPT-4",
		CallCount:          10,
		TotalTokens:        100_000,
		TotalCostMicroUSD:  2_000_000,
		AvgTokensPerSecond: 2.0,
		SuccessRate:        0.95,
	}}
	uc := NewUsageUsecase(repo)
	items, err := uc.InefficientModels(context.Background(), UsageQuery{Range: "7d"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 insight, got %d", len(items))
	}
	if len(items[0].Flags) == 0 {
		t.Fatal("expected at least one flag")
	}
}
