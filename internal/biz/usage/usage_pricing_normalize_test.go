package usage

import (
	"context"
	"testing"
	"time"

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
		{"model-00000001", "model-00000001"},       // 8 位数字但年段 0000，非日期（C7）
		{"x-model-2025", "x-model-2025"},           // 4 位年份
		{"thing-18991231", "thing-18991231"},       // 年段 1899 之外，不剥（C7）
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

// C9：查价结果（hit 与 miss 同缓存）进程内记忆化——热路径同 key 不再每事件回源。
func TestEnrichPricing_CachesLookupResult(t *testing.T) {
	calls := 0
	snap := pricingRule(2.5)
	repo := &mockUsageRepo{
		getActiveModelPricingFn: func(_ context.Context, _, mod string) (ModelPricingSnapshot, bool, error) {
			calls++
			if mod == "gpt-4o" {
				return snap, true, nil
			}
			return ModelPricingSnapshot{}, false, nil
		},
	}
	u := NewUsecase(repo, loggateway.NewNoop())
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		e := &TokenUsageEvent{ProviderCode: "openai", ModelAPIID: "openai/gpt-4o", InputTokens: 1000}
		if !u.enrichPricing(ctx, e) {
			t.Fatal("normalized fallback should mark event priced")
		}
	}
	if calls != 2 {
		t.Fatalf("expected 2 repo calls total (exact miss + normalized hit, then cached), got %d", calls)
	}
}

// miss 同样缓存；TTL 过期后回源；repo 错误不缓存（下次立即重试）。
func TestEnrichPricing_CacheExpiryAndErrorBypass(t *testing.T) {
	calls := 0
	now := time.Now().UTC()
	repo := &mockUsageRepo{
		getActiveModelPricingFn: func(context.Context, string, string) (ModelPricingSnapshot, bool, error) {
			calls++
			return ModelPricingSnapshot{}, false, nil
		},
	}
	u := NewUsecase(repo, loggateway.NewNoop())
	u.now = func() time.Time { return now }
	ctx := context.Background()
	mk := func() *TokenUsageEvent {
		return &TokenUsageEvent{ProviderCode: "v", ModelAPIID: "v/never-heard-20250101", InputTokens: 1000}
	}

	_ = u.enrichPricing(ctx, mk())
	_ = u.enrichPricing(ctx, mk())
	if calls != 2 {
		t.Fatalf("negative result must be cached: expected 2 calls, got %d", calls)
	}
	now = now.Add(2 * time.Minute)
	_ = u.enrichPricing(ctx, mk())
	if calls != 4 {
		t.Fatalf("TTL expiry must re-query: expected 4 calls, got %d", calls)
	}

	errCalls := 0
	errRepo := &mockUsageRepo{
		getActiveModelPricingFn: func(context.Context, string, string) (ModelPricingSnapshot, bool, error) {
			errCalls++
			return ModelPricingSnapshot{}, false, context.DeadlineExceeded
		},
	}
	u2 := NewUsecase(errRepo, loggateway.NewNoop())
	_ = u2.enrichPricing(ctx, mk())
	_ = u2.enrichPricing(ctx, mk())
	// repo 错误不缓存：每次尝试立即回源；且精确键报错时归一回退不级联（1 call/次）。
	if errCalls != 2 {
		t.Fatalf("repo errors must not be cached nor cascade to normalized key: expected 2 calls, got %d", errCalls)
	}
}
