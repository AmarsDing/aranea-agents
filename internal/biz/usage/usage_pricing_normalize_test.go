package usage

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// H25（orca 计价前模型 ID 归一化同款）：上报 ID 的网关前缀/日期戳不应导致计价 miss。
func TestNormalizeModelLookupKey(t *testing.T) {
	cases := []struct{ in, want string }{
		{"openai/gpt-4o", "gpt-4o"},
		{"vendor/sub/claude-sonnet-4-5", "claude-sonnet-4-5"},
		{"claude-sonnet-4-5-20250929", "claude-sonnet-4-5"},
		{"gpt-4o-20240806", "gpt-4o"},
		{"claude-sonnet-4-5", "claude-sonnet-4-5"}, // 无装饰不动
		{"model-12345", "model-12345"},             // 5 位数字非日期戳
		{"model-123456789", "model-123456789"},     // 9 位数字非日期戳
		{"x-model-2025", "x-model-2025"},           // 4 位年份
		{"gpt-4o/", "gpt-4o/"},                     // 尾斜杠剥不出段名，保守不动
		{"", ""},
		{"  openai/gpt-4o  ", "gpt-4o"},
	}
	for _, tc := range cases {
		if got := normalizeModelLookupKey(tc.in); got != tc.want {
			t.Fatalf("normalizeModelLookupKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func pricingRule(price float64) ModelPricingSnapshot {
	return ModelPricingSnapshot{InputPriceUSDPer1M: price, OutputPriceUSDPer1M: price * 2}
}

// 精确键查不到时按归一键重查并命中。
func TestEnrichPricing_FallsBackToNormalizedKey(t *testing.T) {
	snap := pricingRule(2.5)
	repo := &mockUsageRepo{
		getActiveModelPricingFn: func(_ context.Context, _, mod string) (ModelPricingSnapshot, bool, error) {
			if mod == "gpt-4o" {
				return snap, true, nil
			}
			return ModelPricingSnapshot{}, false, nil
		},
	}
	u := NewUsecase(repo, loggateway.NewNoop())

	e := &TokenUsageEvent{ProviderCode: "openai", ModelAPIID: "openai/gpt-4o", InputTokens: 1000}
	if !u.enrichPricing(context.Background(), e) {
		t.Fatal("normalized fallback should mark event priced")
	}
	if e.InputPriceUSDPer1M != snap.InputPriceUSDPer1M {
		t.Fatalf("expected normalized snapshot price %v, got %v", snap.InputPriceUSDPer1M, e.InputPriceUSDPer1M)
	}
	if e.ModelAPIID != "openai/gpt-4o" {
		t.Fatalf("raw model id must stay untouched (attribution), got %q", e.ModelAPIID)
	}
}

// 日期戳后缀同样归一命中。
func TestEnrichPricing_FallsBackStripsDateStamp(t *testing.T) {
	snap := pricingRule(3.0)
	repo := &mockUsageRepo{
		getActiveModelPricingFn: func(_ context.Context, _, mod string) (ModelPricingSnapshot, bool, error) {
			if mod == "claude-sonnet-4-5" {
				return snap, true, nil
			}
			return ModelPricingSnapshot{}, false, nil
		},
	}
	u := NewUsecase(repo, loggateway.NewNoop())

	e := &TokenUsageEvent{ProviderCode: "anthropic", ModelAPIID: "claude-sonnet-4-5-20250929", InputTokens: 1000}
	if !u.enrichPricing(context.Background(), e) {
		t.Fatal("date-stamped id should match canonical pricing rule")
	}
}

// 归一后仍查不到 → 保持未定价，绝不猜价。
func TestEnrichPricing_UnknownStaysUnpriced(t *testing.T) {
	repo := &mockUsageRepo{
		getActiveModelPricingFn: func(context.Context, string, string) (ModelPricingSnapshot, bool, error) {
			return ModelPricingSnapshot{}, false, nil
		},
	}
	u := NewUsecase(repo, loggateway.NewNoop())

	e := &TokenUsageEvent{ProviderCode: "vendor", ModelAPIID: "vendor/never-heard-of-it-20250101", InputTokens: 1000}
	if u.enrichPricing(context.Background(), e) {
		t.Fatal("unknown model must stay unpriced (no guessing)")
	}
	if hasAnyPrice(e) {
		t.Fatal("no price field may be fabricated for unknown models")
	}
}
