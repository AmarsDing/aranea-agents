package usage

import (
	"context"
	"math"
	"strings"
	"time"

	"aranea-agents/internal/modelregistry"
	"aranea-agents/pkg/loggateway"
)

func normalizeTokenUsageEventForInsert(e TokenUsageEvent, now time.Time) TokenUsageEvent {
	if strings.TrimSpace(e.OccurredAt) != "" {
		if t, err := time.Parse(time.RFC3339, e.OccurredAt); err == nil {
			now = t.UTC()
		}
	}
	if strings.TrimSpace(e.OccurredAt) == "" {
		e.OccurredAt = now.Format(time.RFC3339)
	}
	if strings.TrimSpace(e.CreatedAt) == "" {
		e.CreatedAt = e.OccurredAt
	}
	if strings.TrimSpace(e.DateKey) == "" && len(e.OccurredAt) >= 10 {
		e.DateKey = e.OccurredAt[:10]
	}
	if strings.TrimSpace(e.HourKey) == "" && len(e.OccurredAt) >= 13 {
		e.HourKey = e.OccurredAt[:13] + ":00"
	}
	if strings.TrimSpace(e.UsageKind) == "" {
		e.UsageKind = KindUnknown
	}
	if e.CallCount <= 0 {
		e.CallCount = 1
	}
	e.Status = NormalizeStatus(e.Status)
	if e.Status == "" {
		e.Status = "success"
	}

	if e.TokensPerSecond <= 0 && e.LatencyMS > 0 && e.OutputTokens > 0 {
		e.TokensPerSecond = float64(e.OutputTokens) * 1000.0 / float64(e.LatencyMS)
	}
	if strings.TrimSpace(e.ModelCategoryJSON) == "" {
		e.ModelCategoryJSON = "[]"
	}
	if strings.TrimSpace(e.MetadataJSON) == "" {
		e.MetadataJSON = "{}"
	}
	return e
}

// enrichPricing resolves prices for the event (caller-supplied prices win;
// otherwise the active pricing snapshot is applied) and recomputes costs.
//
// The return value reports whether the event is considered PRICED: true when a
// pricing snapshot matched (even an explicitly all-zero free tier) or any
// price field was supplied — in both cases the operator made a deliberate
// pricing decision. false means no pricing source exists at all, which
// markNoPricing treats as a config gap (P2-2 review fix, 2026-08-19).
func (u *Usecase) enrichPricing(ctx context.Context, e *TokenUsageEvent) bool {
	if e == nil {
		return true
	}
	prov := strings.TrimSpace(e.ProviderCode)
	mod := strings.TrimSpace(e.ModelAPIID)
	if prov != "" && mod != "" &&
		e.InputPriceUSDPer1M == 0 && e.OutputPriceUSDPer1M == 0 &&
		e.InputPriceMicroUSDPer1K == 0 && e.OutputPriceMicroUSDPer1K == 0 {
		if snap, ok, err := u.repo.GetActiveModelPricing(ctx, prov, mod); err == nil && ok {
			applyPricingUSDToEvent(e, snap)
			ApplyTokenUsageCosts(e)
			return true
		}
	}
	ApplyTokenUsageCosts(e)
	return hasAnyPrice(e)
}

// hasAnyPrice reports whether any price field is non-zero (caller-supplied or
// snapshot-applied). Snapshot hits return early in enrichPricing and never
// reach this check, so a false here reliably means "no pricing source".
func hasAnyPrice(e *TokenUsageEvent) bool {
	return e.InputPriceUSDPer1M != 0 || e.OutputPriceUSDPer1M != 0 ||
		e.CacheReadPriceUSDPer1M != 0 || e.CacheWritePriceUSDPer1M != 0 ||
		e.ReasoningPriceUSDPer1M != 0 || e.EmbeddingPriceUSDPer1M != 0 ||
		e.InputPriceMicroUSDPer1K != 0 || e.OutputPriceMicroUSDPer1K != 0 ||
		e.CachedInputPriceMicroUSDPer1K != 0 || e.CacheWritePriceMicroUSDPer1K != 0 ||
		e.ReasoningPriceMicroUSDPer1K != 0 || e.EmbeddingPriceMicroUSDPer1K != 0
}

// markNoPricing flags events carrying billable tokens whose cost resolved to
// zero with NO pricing source at all (no active pricing snapshot, no
// caller-supplied prices): such rows are indistinguishable from legitimately
// free consumption in aggregates, so they get metadata_json[cost_status]=
// "no_pricing" plus a Warn log deduped per provider|model (P2-2, 2026-08-19).
// priced comes from enrichPricing; an explicitly all-zero snapshot (free tier)
// reports priced=true and is NOT flagged.
func (u *Usecase) markNoPricing(e *TokenUsageEvent, priced bool) {
	if e == nil {
		return
	}
	billable := e.InputTokens + e.OutputTokens + e.CachedInputTokens +
		e.CacheWriteTokens + e.ReasoningTokens + e.EmbeddingTokens
	if billable <= 0 || e.TotalCostMicroUSD > 0 || priced {
		return
	}
	e.MetadataJSON = MergeCostStatusMetadata(e.MetadataJSON, CostStatusNoPricing)
	key := strings.TrimSpace(e.ProviderCode) + "|" + strings.TrimSpace(e.ModelAPIID)
	if _, dup := u.noPricingLogged.LoadOrStore(key, struct{}{}); dup {
		return
	}
	u.lg.Warn("usage event has tokens but no pricing; cost_status=no_pricing",
		loggateway.StepID("usage.no_pricing"),
		loggateway.Str("provider", e.ProviderCode),
		loggateway.Str("model", e.ModelAPIID),
		loggateway.Str("usage_kind", e.UsageKind),
		loggateway.Int("total_tokens", billable))
}

func applyPricingUSDToEvent(e *TokenUsageEvent, snap ModelPricingSnapshot) {
	if e == nil {
		return
	}
	e.InputPriceUSDPer1M = snap.InputPriceUSDPer1M
	e.OutputPriceUSDPer1M = snap.OutputPriceUSDPer1M
	e.CacheReadPriceUSDPer1M = snap.CacheReadPriceUSDPer1M
	e.CacheWritePriceUSDPer1M = snap.CacheWritePriceUSDPer1M
	e.ReasoningPriceUSDPer1M = snap.ReasoningPriceUSDPer1M
	e.EmbeddingPriceUSDPer1M = snap.EmbeddingPriceUSDPer1M

	if snap.InputPriceUSDPer1M > 0 {
		e.InputPriceMicroUSDPer1K = modelregistry.USDPer1MToMicroPer1K(snap.InputPriceUSDPer1M)
	} else if snap.InputPriceMicroUSDPer1K > 0 {
		e.InputPriceMicroUSDPer1K = snap.InputPriceMicroUSDPer1K
	}
	if snap.OutputPriceUSDPer1M > 0 {
		e.OutputPriceMicroUSDPer1K = modelregistry.USDPer1MToMicroPer1K(snap.OutputPriceUSDPer1M)
	} else if snap.OutputPriceMicroUSDPer1K > 0 {
		e.OutputPriceMicroUSDPer1K = snap.OutputPriceMicroUSDPer1K
	}
	if snap.CacheReadPriceUSDPer1M > 0 {
		e.CachedInputPriceMicroUSDPer1K = modelregistry.USDPer1MToMicroPer1K(snap.CacheReadPriceUSDPer1M)
	} else if snap.CachedInputPriceMicroUSDPer1K > 0 {
		e.CachedInputPriceMicroUSDPer1K = snap.CachedInputPriceMicroUSDPer1K
	}
	if snap.CacheWritePriceUSDPer1M > 0 {
		e.CacheWritePriceMicroUSDPer1K = modelregistry.USDPer1MToMicroPer1K(snap.CacheWritePriceUSDPer1M)
	} else if snap.CacheWritePriceMicroUSDPer1K > 0 {
		e.CacheWritePriceMicroUSDPer1K = snap.CacheWritePriceMicroUSDPer1K
	}
	if snap.ReasoningPriceUSDPer1M > 0 {
		e.ReasoningPriceMicroUSDPer1K = modelregistry.USDPer1MToMicroPer1K(snap.ReasoningPriceUSDPer1M)
	} else if snap.ReasoningPriceMicroUSDPer1K > 0 {
		e.ReasoningPriceMicroUSDPer1K = snap.ReasoningPriceMicroUSDPer1K
	}
	if snap.EmbeddingPriceUSDPer1M > 0 {
		e.EmbeddingPriceMicroUSDPer1K = modelregistry.USDPer1MToMicroPer1K(snap.EmbeddingPriceUSDPer1M)
	} else if snap.EmbeddingPriceMicroUSDPer1K > 0 {
		e.EmbeddingPriceMicroUSDPer1K = snap.EmbeddingPriceMicroUSDPer1K
	}
}

// QuoteTokenUsageCostMicroUSD computes the total cost (micro USD) for the given
// token counts via the same pricing path used when persisting usage events
// (enrichPricing → ApplyTokenUsageCosts). Returns 0 for nil usecase, zero-token
// input, or unpriced provider/model — callers treat 0 as "no cost data", which
// matches the behavior of usage rows recorded without a pricing snapshot.
// Read-only: nothing is persisted (P2-1 TeamRunStep.CostMicroUSD 回填).
func (u *Usecase) QuoteTokenUsageCostMicroUSD(ctx context.Context, prov, mod string, inputTok, outputTok, cachedTok, cacheWriteTok int) int64 {
	if u == nil || (inputTok <= 0 && outputTok <= 0) {
		return 0
	}
	e := &TokenUsageEvent{
		ProviderCode:      strings.TrimSpace(prov),
		ModelAPIID:        strings.TrimSpace(mod),
		InputTokens:       inputTok,
		OutputTokens:      outputTok,
		CachedInputTokens: cachedTok,
		CacheWriteTokens:  cacheWriteTok,
	}
	u.enrichPricing(ctx, e)
	return e.TotalCostMicroUSD
}

// NormalizeStatus maps legacy writer values to DB canonical status.
func NormalizeStatus(status string) string {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "", "success", "ok":
		return "success"
	case "failed", "fail", "error":
		return "failed"
	case "timeout", "timed_out":
		return "timeout"
	case "cancelled", "canceled":
		return "cancelled"
	default:
		return strings.TrimSpace(status)
	}
}

// ApplyTokenUsageCosts fills per-kind costs and total from token counts and USD/1M prices (micro fallback for legacy rows).
//
// Cached-input billing semantics (OpenAI/DeepSeek): InputTokens (prompt_tokens)
// already INCLUDES the cache-hit portion. The hit portion must be billed at the
// cache-read price only, never at the full input price. Therefore:
//
//	billableInput = InputTokens - min(CachedInputTokens, InputTokens)
//	InputCost     = (InputTokens − cached − write) × inputPrice   // 新鲜输入
//	CachedCost    = min(CachedInputTokens, InputTokens) × cacheReadPrice
//	WritedCost    = min(CacheWriteTokens, InputTokens−cached) × cacheWritePrice
func ApplyTokenUsageCosts(e *TokenUsageEvent) {
	if e == nil {
		return
	}
	cachedBillable := e.CachedInputTokens
	if cachedBillable > e.InputTokens {
		cachedBillable = e.InputTokens
	}
	if cachedBillable < 0 {
		cachedBillable = 0
	}
	// F7 修复（2026-10-20 报告）：cache_write ⊂ billingInput（provider 层
	// 归一化后），Anthropic 对写入按 write 价计费——若不从 input 扣除，
	// write 分量会被按 input 价多计一次。clamp 到（input − cached）上限
	// 对历史脏数据（write 未落账时 input 不含 write）保持零值无影响。
	writeBillable := e.CacheWriteTokens
	if writeBillable < 0 {
		writeBillable = 0
	}
	if writeBillable > e.InputTokens-cachedBillable {
		writeBillable = e.InputTokens - cachedBillable
	}
	if e.InputCostMicroUSD == 0 && e.InputTokens > 0 {
		e.InputCostMicroUSD = usageCostMicro(e.InputTokens-cachedBillable-writeBillable, e.InputPriceMicroUSDPer1K, e.InputPriceUSDPer1M)
	}
	if e.OutputCostMicroUSD == 0 && e.OutputTokens > 0 {
		e.OutputCostMicroUSD = usageCostMicro(e.OutputTokens, e.OutputPriceMicroUSDPer1K, e.OutputPriceUSDPer1M)
	}
	if e.CachedInputCostMicroUSD == 0 && cachedBillable > 0 {
		e.CachedInputCostMicroUSD = usageCostMicro(cachedBillable, e.CachedInputPriceMicroUSDPer1K, e.CacheReadPriceUSDPer1M)
	}
	if e.CacheWriteCostMicroUSD == 0 && writeBillable > 0 {
		e.CacheWriteCostMicroUSD = usageCostMicro(writeBillable, e.CacheWritePriceMicroUSDPer1K, e.CacheWritePriceUSDPer1M)
	}
	if e.ReasoningCostMicroUSD == 0 && e.ReasoningTokens > 0 {
		e.ReasoningCostMicroUSD = usageCostMicro(e.ReasoningTokens, e.ReasoningPriceMicroUSDPer1K, e.ReasoningPriceUSDPer1M)
	}
	if e.EmbeddingCostMicroUSD == 0 && e.EmbeddingTokens > 0 {
		e.EmbeddingCostMicroUSD = usageCostMicro(e.EmbeddingTokens, e.EmbeddingPriceMicroUSDPer1K, e.EmbeddingPriceUSDPer1M)
	}
	if e.TotalCostMicroUSD == 0 {
		e.TotalCostMicroUSD = e.InputCostMicroUSD + e.OutputCostMicroUSD +
			e.CachedInputCostMicroUSD + e.CacheWriteCostMicroUSD + e.ReasoningCostMicroUSD + e.EmbeddingCostMicroUSD
	}
}

func usageCostMicro(tokens int, microPer1K int64, usdPer1M float64) int64 {
	if tokens <= 0 {
		return 0
	}
	if usdPer1M > 0 && !math.IsNaN(usdPer1M) {
		return modelregistry.CostMicroUSDFromUSDPer1M(tokens, usdPer1M)
	}
	if microPer1K > 0 {
		return int64(tokens) * microPer1K / usageCostMicroDivisor
	}
	return 0
}
