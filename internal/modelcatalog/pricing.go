package modelcatalog

import "math"

// MicroPricing holds legacy micro-USD per 1K token prices used by Usage billing.
type MicroPricing struct {
	Input       int64
	Output      int64
	CacheRead   int64
	CacheWrite  int64
	Reasoning   int64
	Embedding   int64
}

// CostUSDPer1M is the canonical catalog pricing block (USD per 1M tokens).
type CostUSDPer1M struct {
	Input      float64 `json:"input_usd_per_1m"`
	Output     float64 `json:"output_usd_per_1m"`
	CacheRead  float64 `json:"cache_read_usd_per_1m"`
	CacheWrite float64 `json:"cache_write_usd_per_1m"`
	Reasoning  float64 `json:"reasoning_usd_per_1m"`
	Embedding  float64 `json:"embedding_usd_per_1m"`
}

func USDPer1MToMicroPer1K(v float64) int64 {
	if v <= 0 || math.IsNaN(v) {
		return 0
	}
	return int64(math.Round(v * 1000))
}

func MicroPer1KToUSDPer1M(v int64) float64 {
	if v <= 0 {
		return 0
	}
	return float64(v) / 1000
}

func MicroPricingFromModelCost(c *ModelCost) (CostUSDPer1M, MicroPricing) {
	if c == nil {
		return CostUSDPer1M{}, MicroPricing{}
	}
	cost := CostUSDPer1M{
		Input:      c.Input,
		Output:     c.Output,
		CacheRead:  c.CacheRead,
		CacheWrite: c.CacheWrite,
		Reasoning:  c.Reasoning,
	}
	return cost, MicroPricing{
		Input:      USDPer1MToMicroPer1K(c.Input),
		Output:     USDPer1MToMicroPer1K(c.Output),
		CacheRead:  USDPer1MToMicroPer1K(c.CacheRead),
		CacheWrite: USDPer1MToMicroPer1K(c.CacheWrite),
		Reasoning:  USDPer1MToMicroPer1K(c.Reasoning),
	}
}

func MicroPricingFromCostBlock(cost CostUSDPer1M) MicroPricing {
	return MicroPricing{
		Input:      USDPer1MToMicroPer1K(cost.Input),
		Output:     USDPer1MToMicroPer1K(cost.Output),
		CacheRead:  USDPer1MToMicroPer1K(cost.CacheRead),
		CacheWrite: USDPer1MToMicroPer1K(cost.CacheWrite),
		Reasoning:  USDPer1MToMicroPer1K(cost.Reasoning),
		Embedding:  USDPer1MToMicroPer1K(cost.Embedding),
	}
}

// CostMicroUSDFromUSDPer1M converts token count and USD/1M price to micro-USD total.
func CostMicroUSDFromUSDPer1M(tokens int, usdPer1M float64) int64 {
	if tokens <= 0 || usdPer1M <= 0 || math.IsNaN(usdPer1M) {
		return 0
	}
	return int64(math.Round(float64(tokens) * usdPer1M))
}
