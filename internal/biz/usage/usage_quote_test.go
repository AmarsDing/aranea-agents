package usage

import (
	"context"
	"testing"
)

// P2-1 (2026-08-19): QuoteTokenUsageCostMicroUSD — TeamRunStep.CostMicroUSD
// 回填用的只读报价路径，必须与落库事件共享同一计价语义（enrichPricing →
// ApplyTokenUsageCosts，含 cached 命中按缓存读取价、不计全价输入）。
func TestQuoteTokenUsageCostMicroUSD(t *testing.T) {
	t.Run("nil usecase returns 0", func(t *testing.T) {
		var u *Usecase
		if got := u.QuoteTokenUsageCostMicroUSD(context.Background(), "deepseek", "deepseek-chat", 100, 10, 0); got != 0 {
			t.Fatalf("nil usecase = %d, want 0", got)
		}
	})

	t.Run("zero tokens returns 0", func(t *testing.T) {
		u := NewUsecase(&mockUsageRepo{}, nil)
		if got := u.QuoteTokenUsageCostMicroUSD(context.Background(), "deepseek", "deepseek-chat", 0, 0, 0); got != 0 {
			t.Fatalf("zero tokens = %d, want 0", got)
		}
	})

	t.Run("unpriced model returns 0", func(t *testing.T) {
		u := NewUsecase(&mockUsageRepo{}, nil)
		if got := u.QuoteTokenUsageCostMicroUSD(context.Background(), "noprov", "nomodel", 100, 10, 0); got != 0 {
			t.Fatalf("unpriced = %d, want 0", got)
		}
	})

	t.Run("priced: cached hit billed at cache-read price only", func(t *testing.T) {
		repo := &mockUsageRepo{
			getActiveModelPricingFn: func(context.Context, string, string) (ModelPricingSnapshot, bool, error) {
				return ModelPricingSnapshot{
					InputPriceUSDPer1M:     2,  // $2 / 1M input
					OutputPriceUSDPer1M:    8,  // $8 / 1M output
					CacheReadPriceUSDPer1M: 0.5, // $0.5 / 1M cache read
				}, true, nil
			},
		}
		u := NewUsecase(repo, nil)
		// 1000 input (400 cached) + 100 output:
		//   input  = (1000-400) × 2µ$/1M   = 1200 µ$/1M → 1200 * 2 / 1M... see CostMicroUSDFromUSDPer1M
		//   cached = 400 × 0.5             = 200
		//   output = 100 × 8               = 800
		// total = 600*2 + 400*0.5 + 100*8 (micro-USD per-1M scaled) = 1200+200+800 = 2200 µ$/1M-units
		got := u.QuoteTokenUsageCostMicroUSD(context.Background(), "deepseek", "deepseek-chat", 1000, 100, 400)
		// CostMicroUSDFromUSDPer1M(tokens, usdPer1M) = round(tokens × usdPer1M) micro USD? verify via totals:
		// 600 tok × $2/1M  = 1200 micro-USD? No: 600/1e6 × 2 USD = 0.0012 USD = 1200 µUSD ✓
		// 400 tok × $0.5/1M = 200 µUSD ✓
		// 100 tok × $8/1M  = 800 µUSD ✓
		if got != 2200 {
			t.Fatalf("priced quote = %d, want 2200", got)
		}
	})
}
