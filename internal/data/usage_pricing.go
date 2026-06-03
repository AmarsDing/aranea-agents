package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/modelregistry"
	"aranea-agents/pkg/loggateway"
)

type providerModelPricingJSON struct {
	Cost                          providerCostJSON `json:"cost"`
	InputPriceMicroUSDPer1K       int64            `json:"input_price_micro_usd_per_1k"`
	OutputPriceMicroUSDPer1K      int64            `json:"output_price_micro_usd_per_1k"`
	CachedInputPriceMicroUSDPer1K int64            `json:"cached_input_price_micro_usd_per_1k"`
	ReasoningPriceMicroUSDPer1K   int64            `json:"reasoning_price_micro_usd_per_1k"`
	EmbeddingPriceMicroUSDPer1K   int64            `json:"embedding_price_micro_usd_per_1k"`
}

type providerCostJSON struct {
	InputUSDPer1M      float64 `json:"input_usd_per_1m"`
	OutputUSDPer1M     float64 `json:"output_usd_per_1m"`
	CacheReadUSDPer1M  float64 `json:"cache_read_usd_per_1m"`
	CacheWriteUSDPer1M float64 `json:"cache_write_usd_per_1m"`
	ReasoningUSDPer1M  float64 `json:"reasoning_usd_per_1m"`
	EmbeddingUSDPer1M  float64 `json:"embedding_usd_per_1m"`
}

func snapshotFromUSD(cost modelregistry.CostUSDPer1M) biz.ModelPricingSnapshot {
	micro := modelregistry.MicroPricingFromCostBlock(cost)
	return biz.ModelPricingSnapshot{
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

func snapshotFromProviderConfig(cfg providerModelPricingJSON) biz.ModelPricingSnapshot {
	if cfg.Cost.InputUSDPer1M > 0 || cfg.Cost.OutputUSDPer1M > 0 || cfg.Cost.CacheReadUSDPer1M > 0 ||
		cfg.Cost.CacheWriteUSDPer1M > 0 || cfg.Cost.ReasoningUSDPer1M > 0 || cfg.Cost.EmbeddingUSDPer1M > 0 {
		return snapshotFromUSD(modelregistry.CostUSDPer1M{
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
		return biz.ModelPricingSnapshot{}
	}
	cost := modelregistry.CostUSDPer1M{
		Input:      modelregistry.MicroPer1KToUSDPer1M(cfg.InputPriceMicroUSDPer1K),
		Output:     modelregistry.MicroPer1KToUSDPer1M(cfg.OutputPriceMicroUSDPer1K),
		CacheRead:  modelregistry.MicroPer1KToUSDPer1M(cfg.CachedInputPriceMicroUSDPer1K),
		Reasoning:  modelregistry.MicroPer1KToUSDPer1M(cfg.ReasoningPriceMicroUSDPer1K),
		Embedding:  modelregistry.MicroPer1KToUSDPer1M(cfg.EmbeddingPriceMicroUSDPer1K),
	}
	return snapshotFromUSD(cost)
}

func (r *usageRepo) GetActiveModelPricing(ctx context.Context, providerCode, modelAPIID string) (biz.ModelPricingSnapshot, bool, error) {
	providerCode = strings.TrimSpace(providerCode)
	modelAPIID = strings.TrimSpace(modelAPIID)
	if providerCode == "" || modelAPIID == "" {
		return biz.ModelPricingSnapshot{}, false, nil
	}
	var inputUSD, outputUSD, cacheUSD, cacheWriteUSD, reasonUSD, embedUSD float64
	var inputMicro, outputMicro, cacheMicro, cacheWriteMicro, reasonMicro, embedMicro int64
	err := entQueryRowScan(r.readClient(ctx), ctx,
		`SELECT input_price_micro_usd_per_1k, output_price_micro_usd_per_1k,
		        cached_input_price_micro_usd_per_1k, COALESCE(cache_write_price_micro_usd_per_1k, 0),
		        reasoning_price_micro_usd_per_1k, embedding_price_micro_usd_per_1k,
		        COALESCE(input_price_usd_per_1m, 0), COALESCE(output_price_usd_per_1m, 0),
		        COALESCE(cached_input_price_usd_per_1m, 0), COALESCE(cache_write_price_usd_per_1m, 0),
		        COALESCE(reasoning_price_usd_per_1m, 0), COALESCE(embedding_price_usd_per_1m, 0)
		 FROM model_pricing_rules
		 WHERE provider_code = ? AND model_api_id = ? AND is_active = 1 AND (effective_to = '' OR effective_to IS NULL)
		 ORDER BY effective_from DESC
		 LIMIT 1`,
		[]any{providerCode, modelAPIID},
		&inputMicro, &outputMicro, &cacheMicro, &cacheWriteMicro, &reasonMicro, &embedMicro,
		&inputUSD, &outputUSD, &cacheUSD, &cacheWriteUSD, &reasonUSD, &embedUSD,
	)
	if err == nil {
		if inputUSD > 0 || outputUSD > 0 || cacheUSD > 0 || cacheWriteUSD > 0 || reasonUSD > 0 || embedUSD > 0 {
			return snapshotFromUSD(modelregistry.CostUSDPer1M{
				Input: inputUSD, Output: outputUSD, CacheRead: cacheUSD, CacheWrite: cacheWriteUSD,
				Reasoning: reasonUSD, Embedding: embedUSD,
			}), true, nil
		}
		if inputMicro != 0 || outputMicro != 0 || cacheMicro != 0 || cacheWriteMicro != 0 || reasonMicro != 0 || embedMicro != 0 {
			return snapshotFromUSD(modelregistry.CostUSDPer1M{
				Input:      modelregistry.MicroPer1KToUSDPer1M(inputMicro),
				Output:     modelregistry.MicroPer1KToUSDPer1M(outputMicro),
				CacheRead:  modelregistry.MicroPer1KToUSDPer1M(cacheMicro),
				CacheWrite: modelregistry.MicroPer1KToUSDPer1M(cacheWriteMicro),
				Reasoning:  modelregistry.MicroPer1KToUSDPer1M(reasonMicro),
				Embedding:  modelregistry.MicroPer1KToUSDPer1M(embedMicro),
			}), true, nil
		}
	} else if err != sql.ErrNoRows {
		return biz.ModelPricingSnapshot{}, false, err
	}
	return r.pricingFromProviderModelConfig(ctx, providerCode, modelAPIID)
}

func (r *usageRepo) pricingFromProviderModelConfig(ctx context.Context, providerCode, modelAPIID string) (biz.ModelPricingSnapshot, bool, error) {
	var cfgJSON string
	err := entQueryRowScan(r.readClient(ctx), ctx,
		`SELECT config_json FROM llm_provider_models
		 WHERE provider = ? AND model = ? AND deleted_at = '' AND enabled = 1
		 ORDER BY sort_order ASC, created_at DESC LIMIT 1`,
		[]any{providerCode, modelAPIID},
		&cfgJSON,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return biz.ModelPricingSnapshot{}, false, nil
		}
		return biz.ModelPricingSnapshot{}, false, err
	}
	var cfg providerModelPricingJSON
	if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
		r.data.lg.Warn("unmarshal provider model pricing config failed", loggateway.StepID("data.usage_pricing"), loggateway.Err(err))
		return biz.ModelPricingSnapshot{}, false, nil
	}
	snap := snapshotFromProviderConfig(cfg)
	if snap.InputPriceUSDPer1M == 0 && snap.OutputPriceUSDPer1M == 0 &&
		snap.CacheReadPriceUSDPer1M == 0 && snap.CacheWritePriceUSDPer1M == 0 {
		return biz.ModelPricingSnapshot{}, false, nil
	}
	return snap, true, nil
}
