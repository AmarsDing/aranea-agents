package data

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/usage"
	"aranea-agents/internal/modelregistry"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

func (r *usageRepo) GetActiveModelPricing(ctx context.Context, providerCode, modelAPIID string) (biz.ModelPricingSnapshot, bool, error) {
	providerCode = strings.TrimSpace(providerCode)
	modelAPIID = strings.TrimSpace(modelAPIID)
	if providerCode == "" || modelAPIID == "" {
		return biz.ModelPricingSnapshot{}, false, nil
	}
	var inputUSD, outputUSD, cacheUSD, cacheWriteUSD, reasonUSD, embedUSD float64
	var inputMicro, outputMicro, cacheMicro, cacheWriteMicro, reasonMicro, embedMicro int64
	err := entQueryRowScan(r.data.RW().Read(ctx), ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT input_price_micro_usd_per_1k, output_price_micro_usd_per_1k,
		        cached_input_price_micro_usd_per_1k, COALESCE(cache_write_price_micro_usd_per_1k, 0),
		        reasoning_price_micro_usd_per_1k, embedding_price_micro_usd_per_1k,
		        COALESCE(input_price_usd_per_1m, 0), COALESCE(output_price_usd_per_1m, 0),
		        COALESCE(cached_input_price_usd_per_1m, 0), COALESCE(cache_write_price_usd_per_1m, 0),
		        COALESCE(reasoning_price_usd_per_1m, 0), COALESCE(embedding_price_usd_per_1m, 0)
		 FROM model_pricing_rules
		 WHERE provider_code = ? AND model_api_id = ? AND is_active = TRUE AND (effective_to = '' OR effective_to IS NULL)
		 ORDER BY effective_from DESC
		 LIMIT 1`),
		[]any{providerCode, modelAPIID},
		&inputMicro, &outputMicro, &cacheMicro, &cacheWriteMicro, &reasonMicro, &embedMicro,
		&inputUSD, &outputUSD, &cacheUSD, &cacheWriteUSD, &reasonUSD, &embedUSD,
	)
	if err == nil {
		if inputUSD > 0 || outputUSD > 0 || cacheUSD > 0 || cacheWriteUSD > 0 || reasonUSD > 0 || embedUSD > 0 {
			return usage.SnapshotFromUSD(modelregistry.CostUSDPer1M{
				Input: inputUSD, Output: outputUSD, CacheRead: cacheUSD, CacheWrite: cacheWriteUSD,
				Reasoning: reasonUSD, Embedding: embedUSD,
			}), true, nil
		}
		if inputMicro != 0 || outputMicro != 0 || cacheMicro != 0 || cacheWriteMicro != 0 || reasonMicro != 0 || embedMicro != 0 {
			return usage.SnapshotFromUSD(modelregistry.CostUSDPer1M{
				Input:      modelregistry.MicroPer1KToUSDPer1M(inputMicro),
				Output:     modelregistry.MicroPer1KToUSDPer1M(outputMicro),
				CacheRead:  modelregistry.MicroPer1KToUSDPer1M(cacheMicro),
				CacheWrite: modelregistry.MicroPer1KToUSDPer1M(cacheWriteMicro),
				Reasoning:  modelregistry.MicroPer1KToUSDPer1M(reasonMicro),
				Embedding:  modelregistry.MicroPer1KToUSDPer1M(embedMicro),
			}), true, nil
		}
	} else if ae, ok := apierror.From(err); !ok || ae.Code != apierror.CodeNotFound {
		return biz.ModelPricingSnapshot{}, false, err
	}
	return r.pricingFromProviderModelConfig(ctx, providerCode, modelAPIID)
}

func (r *usageRepo) pricingFromProviderModelConfig(ctx context.Context, providerCode, modelAPIID string) (biz.ModelPricingSnapshot, bool, error) {
	var cfgJSON string
	err := entQueryRowScan(r.data.RW().Read(ctx), ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT config_json FROM llm_provider_models
		 WHERE provider = ? AND model = ? AND deleted_at = '' AND enabled = 1
		 ORDER BY sort_order ASC, created_at DESC LIMIT 1`),
		[]any{providerCode, modelAPIID},
		&cfgJSON,
	)
	if err != nil {
		if ae, ok := apierror.From(err); ok && ae.Code == apierror.CodeNotFound {
			return biz.ModelPricingSnapshot{}, false, nil
		}
		return biz.ModelPricingSnapshot{}, false, err
	}
	var cfg usage.ProviderModelPricingJSON
	if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
		r.data.lg.Warn("unmarshal provider model pricing config failed", loggateway.StepID("data.usage_pricing"), loggateway.Err(err))
		return biz.ModelPricingSnapshot{}, false, nil
	}
	snap := usage.SnapshotFromProviderConfig(cfg)
	if snap.InputPriceUSDPer1M == 0 && snap.OutputPriceUSDPer1M == 0 &&
		snap.CacheReadPriceUSDPer1M == 0 && snap.CacheWritePriceUSDPer1M == 0 {
		return biz.ModelPricingSnapshot{}, false, nil
	}
	return snap, true, nil
}
