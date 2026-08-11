package modelregistry

import "math"

type MicroPricing struct {
	Input      int64
	Output     int64
	CacheRead  int64
	CacheWrite int64
	Reasoning  int64
	Embedding  int64
}

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

// normalizeUSDPer1M 去除上游 models.dev 价格的 float32 加宽噪声
// （如 0.14000000059604645）：价格粒度为 micro-USD/1K，即 USD/1M 下 6 位
// 小数以外的部分恒为噪声，统一舍入到 6 位小数后再写入 config_json cost 块。
func normalizeUSDPer1M(v float64) float64 {
	if v <= 0 || math.IsNaN(v) {
		return 0
	}
	return math.Round(v*1e6) / 1e6
}

func MicroPricingFromModelCost(c *ModelCost) (CostUSDPer1M, MicroPricing) {
	if c == nil {
		return CostUSDPer1M{}, MicroPricing{}
	}
	cost := CostUSDPer1M{
		Input:      normalizeUSDPer1M(c.Input),
		Output:     normalizeUSDPer1M(c.Output),
		CacheRead:  normalizeUSDPer1M(c.CacheRead),
		CacheWrite: normalizeUSDPer1M(c.CacheWrite),
		Reasoning:  normalizeUSDPer1M(c.Reasoning),
	}
	return cost, MicroPricing{
		Input:      USDPer1MToMicroPer1K(cost.Input),
		Output:     USDPer1MToMicroPer1K(cost.Output),
		CacheRead:  USDPer1MToMicroPer1K(cost.CacheRead),
		CacheWrite: USDPer1MToMicroPer1K(cost.CacheWrite),
		Reasoning:  USDPer1MToMicroPer1K(cost.Reasoning),
	}
}

func MicroPer1KToCostUSDPer1M(m MicroPricing) CostUSDPer1M {
	return CostUSDPer1M{
		Input:      MicroPer1KToUSDPer1M(m.Input),
		Output:     MicroPer1KToUSDPer1M(m.Output),
		CacheRead:  MicroPer1KToUSDPer1M(m.CacheRead),
		CacheWrite: MicroPer1KToUSDPer1M(m.CacheWrite),
		Reasoning:  MicroPer1KToUSDPer1M(m.Reasoning),
		Embedding:  MicroPer1KToUSDPer1M(m.Embedding),
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

func CostMicroUSDFromUSDPer1M(tokens int, usdPer1M float64) int64 {
	if tokens <= 0 || usdPer1M <= 0 || math.IsNaN(usdPer1M) {
		return 0
	}
	return int64(math.Round(float64(tokens) * usdPer1M))
}
