package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
)

type providerModelPricingJSON struct {
	InputPriceMicroUSDPer1K       int64 `json:"input_price_micro_usd_per_1k"`
	OutputPriceMicroUSDPer1K      int64 `json:"output_price_micro_usd_per_1k"`
	CachedInputPriceMicroUSDPer1K int64 `json:"cached_input_price_micro_usd_per_1k"`
	ReasoningPriceMicroUSDPer1K   int64 `json:"reasoning_price_micro_usd_per_1k"`
	EmbeddingPriceMicroUSDPer1K   int64 `json:"embedding_price_micro_usd_per_1k"`
}

func (r *usageRepo) GetActiveModelPricing(ctx context.Context, providerCode, modelAPIID string) (biz.ModelPricingSnapshot, bool, error) {
	providerCode = strings.TrimSpace(providerCode)
	modelAPIID = strings.TrimSpace(modelAPIID)
	if providerCode == "" || modelAPIID == "" {
		return biz.ModelPricingSnapshot{}, false, nil
	}
	var snap biz.ModelPricingSnapshot
	err := entQueryRowScan(r.ent(), ctx,
		`SELECT input_price_micro_usd_per_1k, output_price_micro_usd_per_1k,
		        cached_input_price_micro_usd_per_1k, reasoning_price_micro_usd_per_1k, embedding_price_micro_usd_per_1k
		 FROM model_pricing_rules
		 WHERE provider_code = ? AND model_api_id = ? AND is_active = 1 AND (effective_to = '' OR effective_to IS NULL)
		 ORDER BY effective_from DESC
		 LIMIT 1`,
		[]any{providerCode, modelAPIID},
		&snap.InputPriceMicroUSDPer1K, &snap.OutputPriceMicroUSDPer1K,
		&snap.CachedInputPriceMicroUSDPer1K, &snap.ReasoningPriceMicroUSDPer1K, &snap.EmbeddingPriceMicroUSDPer1K,
	)
	if err == nil {
		if snap.InputPriceMicroUSDPer1K != 0 || snap.OutputPriceMicroUSDPer1K != 0 ||
			snap.CachedInputPriceMicroUSDPer1K != 0 || snap.ReasoningPriceMicroUSDPer1K != 0 || snap.EmbeddingPriceMicroUSDPer1K != 0 {
			return snap, true, nil
		}
	} else if err != sql.ErrNoRows {
		return biz.ModelPricingSnapshot{}, false, err
	}
	return r.pricingFromProviderModelConfig(ctx, providerCode, modelAPIID)
}

func (r *usageRepo) pricingFromProviderModelConfig(ctx context.Context, providerCode, modelAPIID string) (biz.ModelPricingSnapshot, bool, error) {
	var cfgJSON string
	err := entQueryRowScan(r.ent(), ctx,
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
	if json.Unmarshal([]byte(cfgJSON), &cfg) != nil {
		return biz.ModelPricingSnapshot{}, false, nil
	}
	snap := biz.ModelPricingSnapshot{
		InputPriceMicroUSDPer1K:       cfg.InputPriceMicroUSDPer1K,
		OutputPriceMicroUSDPer1K:      cfg.OutputPriceMicroUSDPer1K,
		CachedInputPriceMicroUSDPer1K: cfg.CachedInputPriceMicroUSDPer1K,
		ReasoningPriceMicroUSDPer1K:   cfg.ReasoningPriceMicroUSDPer1K,
		EmbeddingPriceMicroUSDPer1K:   cfg.EmbeddingPriceMicroUSDPer1K,
	}
	if snap.InputPriceMicroUSDPer1K == 0 && snap.OutputPriceMicroUSDPer1K == 0 {
		return biz.ModelPricingSnapshot{}, false, nil
	}
	return snap, true, nil
}
