package modelcatalog

import (
	"encoding/json"
	"strings"
)

// BackfillCostFromMicro ensures config_json.cost exists when only legacy micro/1K fields are set.
func BackfillCostFromMicro(configJSON string) (string, bool) {
	cfg := map[string]any{}
	if strings.TrimSpace(configJSON) != "" {
		if json.Unmarshal([]byte(configJSON), &cfg) != nil {
			return configJSON, false
		}
	}
	if cost, ok := cfg["cost"].(map[string]any); ok && len(cost) > 0 {
		if hasPositiveCost(cost) {
			return configJSON, false
		}
	}

	micro := readMicroFields(cfg)
	if micro.Input == 0 && micro.Output == 0 && micro.CacheRead == 0 && micro.Reasoning == 0 && micro.Embedding == 0 {
		return configJSON, false
	}
	cost := CostUSDPer1M{
		Input:     MicroPer1KToUSDPer1M(micro.Input),
		Output:    MicroPer1KToUSDPer1M(micro.Output),
		CacheRead: MicroPer1KToUSDPer1M(micro.CacheRead),
		Reasoning: MicroPer1KToUSDPer1M(micro.Reasoning),
		Embedding: MicroPer1KToUSDPer1M(micro.Embedding),
	}
	cfg["cost"] = cost
	b, err := json.Marshal(cfg)
	if err != nil {
		return configJSON, false
	}
	return string(b), true
}

func readMicroFields(cfg map[string]any) MicroPricing {
	return MicroPricing{
		Input:     asInt64(cfg["input_price_micro_usd_per_1k"]),
		Output:    asInt64(cfg["output_price_micro_usd_per_1k"]),
		CacheRead: asInt64(cfg["cached_input_price_micro_usd_per_1k"]),
		Reasoning: asInt64(cfg["reasoning_price_micro_usd_per_1k"]),
		Embedding: asInt64(cfg["embedding_price_micro_usd_per_1k"]),
	}
}

func hasPositiveCost(cost map[string]any) bool {
	for _, k := range []string{"input_usd_per_1m", "output_usd_per_1m", "cache_read_usd_per_1m", "reasoning_usd_per_1m", "embedding_usd_per_1m"} {
		if asFloat(cost[k]) > 0 {
			return true
		}
	}
	return false
}

func asInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	default:
		return 0
	}
}

func asFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}
