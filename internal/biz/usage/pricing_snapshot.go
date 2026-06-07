package usage

import (
	"aranea-agents/internal/modelregistry"
)

// ProviderModelPricingJSON represents the JSON structure of a provider model's pricing config.
type ProviderModelPricingJSON struct {
	Cost                          ProviderCostJSON `json:"cost"`
	InputPriceMicroUSDPer1K       int64            `json:"input_price_micro_usd_per_1k"`
	OutputPriceMicroUSDPer1K      int64            `json:"output_price_micro_usd_per_1k"`
	CachedInputPriceMicroUSDPer1K int64            `json:"cached_input_price_micro_usd_per_1k"`
	ReasoningPriceMicroUSDPer1K   int64            `json:"reasoning_price_micro_usd_per_1k"`
	EmbeddingPriceMicroUSDPer1K   int64            `json:"embedding_price_micro_usd_per_1k"`
}

// ProviderCostJSON represents the cost block in a provider model's pricing config.
type ProviderCostJSON struct {
	InputUSDPer1M      float64 `json:"input_usd_per_1m"`
	OutputUSDPer1M     float64 `json:"output_usd_per_1m"`
	CacheReadUSDPer1M  float64 `json:"cache_read_usd_per_1m"`
	CacheWriteUSDPer1M float64 `json:"cache_write_usd_per_1m"`
	ReasoningUSDPer1M  float64 `json:"reasoning_usd_per_1m"`
	EmbeddingUSDPer1M  float64 `json:"embedding_usd_per_1m"`
}

// SnapshotFromUSD creates a ModelPricingSnapshot from a USD-per-1M cost block.
func SnapshotFromUSD(cost modelregistry.CostUSDPer1M) ModelPricingSnapshot {
	micro := modelregistry.MicroPricingFromCostBlock(cost)
	return ModelPricingSnapshot{
		InputPriceUSDPer1M:            cost.Input,
		OutputPriceUSDPer1M:           cost.Output,
		CacheReadPriceUSDPer1M:        cost.CacheRead,
		CacheWritePriceUSDPer1M:       cost.CacheWrite,
		ReasoningPriceUSDPer1M:        cost.Reasoning,
		EmbeddingPriceUSDPer1M:        cost.Embedding,
		InputPriceMicroUSDPer1K:       micro.Input,
		OutputPriceMicroUSDPer1K:      micro.Output,
		CachedInputPriceMicroUSDPer1K: micro.CacheRead,
		CacheWritePriceMicroUSDPer1K:  micro.CacheWrite,
		ReasoningPriceMicroUSDPer1K:   micro.Reasoning,
		EmbeddingPriceMicroUSDPer1K:   micro.Embedding,
	}
}

// SnapshotFromProviderConfig creates a ModelPricingSnapshot from a provider model's
// pricing config JSON. It prefers USD-per-1M prices and falls back to micro-per-1K.
func SnapshotFromProviderConfig(cfg ProviderModelPricingJSON) ModelPricingSnapshot {
	if cfg.Cost.InputUSDPer1M > 0 || cfg.Cost.OutputUSDPer1M > 0 || cfg.Cost.CacheReadUSDPer1M > 0 ||
		cfg.Cost.CacheWriteUSDPer1M > 0 || cfg.Cost.ReasoningUSDPer1M > 0 || cfg.Cost.EmbeddingUSDPer1M > 0 {
		return SnapshotFromUSD(modelregistry.CostUSDPer1M{
			Input:      cfg.Cost.InputUSDPer1M,
			Output:     cfg.Cost.OutputUSDPer1M,
			CacheRead:  cfg.Cost.CacheReadUSDPer1M,
			CacheWrite: cfg.Cost.CacheWriteUSDPer1M,
			Reasoning:  cfg.Cost.ReasoningUSDPer1M,
			Embedding:  cfg.Cost.EmbeddingUSDPer1M,
		})
	}
	// Legacy config_json with micro-only prices.
	if cfg.InputPriceMicroUSDPer1K == 0 && cfg.OutputPriceMicroUSDPer1K == 0 {
		return ModelPricingSnapshot{}
	}
	cost := modelregistry.CostUSDPer1M{
		Input:      modelregistry.MicroPer1KToUSDPer1M(cfg.InputPriceMicroUSDPer1K),
		Output:     modelregistry.MicroPer1KToUSDPer1M(cfg.OutputPriceMicroUSDPer1K),
		CacheRead:  modelregistry.MicroPer1KToUSDPer1M(cfg.CachedInputPriceMicroUSDPer1K),
		Reasoning:  modelregistry.MicroPer1KToUSDPer1M(cfg.ReasoningPriceMicroUSDPer1K),
		Embedding:  modelregistry.MicroPer1KToUSDPer1M(cfg.EmbeddingPriceMicroUSDPer1K),
	}
	return SnapshotFromUSD(cost)
}
