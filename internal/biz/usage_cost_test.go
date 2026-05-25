package biz

import "testing"

func TestNormalizeUsageStatus(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"ok", "success"},
		{"success", "success"},
		{"error", "failed"},
		{"failed", "failed"},
		{"timeout", "timeout"},
		{"cancelled", "cancelled"},
	}
	for _, tc := range tests {
		if got := NormalizeUsageStatus(tc.in); got != tc.want {
			t.Fatalf("NormalizeUsageStatus(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestApplyTokenUsageCosts(t *testing.T) {
	e := &TokenUsageEvent{
		InputTokens:        2000,
		OutputTokens:       1000,
		InputPriceUSDPer1M: 3,
		OutputPriceUSDPer1M: 6,
	}
	ApplyTokenUsageCosts(e)
	if e.InputCostMicroUSD != 6000 {
		t.Fatalf("input cost = %d, want 6000", e.InputCostMicroUSD)
	}
	if e.OutputCostMicroUSD != 6000 {
		t.Fatalf("output cost = %d, want 6000", e.OutputCostMicroUSD)
	}
	if e.TotalCostMicroUSD != 12000 {
		t.Fatalf("total cost = %d, want 12000", e.TotalCostMicroUSD)
	}
}

func TestApplyTokenUsageCostsLegacyMicro(t *testing.T) {
	e := &TokenUsageEvent{
		InputTokens:             2000,
		OutputTokens:            1000,
		InputPriceMicroUSDPer1K: 3000,
		OutputPriceMicroUSDPer1K: 6000,
	}
	ApplyTokenUsageCosts(e)
	if e.TotalCostMicroUSD != 12000 {
		t.Fatalf("total cost = %d, want 12000", e.TotalCostMicroUSD)
	}
}
