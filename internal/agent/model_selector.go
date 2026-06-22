package agent

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/metrics"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// PluginCostGuardSelector switches to fallback when blocked_models or budget limits require routing.
func PluginCostGuardSelector(
	baseProv, baseMod string,
	catalog biz.TeamModelCatalog,
	rt *provider.RoundTrip,
	cfg plugintrpc.CostGuardConfig,
	tracker *plugintrpc.CostGuardBudgetTracker,
	lg loggateway.Logger,
) trpcagent.ModelSelector {
	baseProv = strings.TrimSpace(baseProv)
	baseMod = strings.TrimSpace(baseMod)
	return func(ctx context.Context, inv *trpcagent.Invocation) (trpcmodel.Model, error) {
		est := plugintrpc.EstimateInvocationTokens(inv)
		target := plugintrpc.ResolveCostGuardTarget(baseMod, cfg, est, tracker)
		if target == "" || target == baseMod {
			return nil, nil
		}
		m, err := provider.TRPCModelForProviderModel(ctx, catalog, rt, baseProv, target, lg)
		if err != nil {
			lg.Warn("费用保护回退到基础模型",
				loggateway.StepID("agent.cost_guard.fallback"),
				loggateway.Str("provider", baseProv), loggateway.Str("target", target), loggateway.Str("base", baseMod), loggateway.Err(err))
			metrics.ModelRouterFallbackTotal.WithLabelValues("cost_guard_catalog").Inc()
			return nil, nil
		}
		lg.Info("费用保护切换模型", loggateway.StepID("agent.cost_guard.switch"), loggateway.Phase("done"),
			loggateway.Str("provider", baseProv), loggateway.Str("target", target), loggateway.Str("base", baseMod), loggateway.Int("est_tokens", est))
		return m, nil
	}
}

// ChainedModelSelector tries selectors in order; first non-nil model wins.
func ChainedModelSelector(selectors ...trpcagent.ModelSelector) trpcagent.ModelSelector {
	return func(ctx context.Context, inv *trpcagent.Invocation) (trpcmodel.Model, error) {
		for _, sel := range selectors {
			if sel == nil {
				continue
			}
			m, err := sel(ctx, inv)
			if err != nil {
				return nil, err
			}
			if m != nil {
				return m, nil
			}
		}
		return nil, nil
	}
}

// PluginModelSelector returns a ModelSelector that routes to another catalog model when model_router is enabled.
func PluginModelSelector(
	baseProv, baseMod string,
	catalog biz.TeamModelCatalog,
	rt *provider.RoundTrip,
	cfg plugintrpc.ModelRouterConfig,
	lg loggateway.Logger,
) trpcagent.ModelSelector {
	baseProv = strings.TrimSpace(baseProv)
	baseMod = strings.TrimSpace(baseMod)
	return func(ctx context.Context, inv *trpcagent.Invocation) (trpcmodel.Model, error) {
		prompt := invocationPrompt(inv)
		target := plugintrpc.ResolveModelAPI(prompt, cfg)
		if target == "" || target == baseMod {
			return nil, nil
		}
		m, err := provider.TRPCModelForProviderModel(ctx, catalog, rt, baseProv, target, lg)
		if err != nil {
			lg.Warn("模型路由回退到基础模型",
				loggateway.StepID("agent.model_router.route"),
				loggateway.Str("provider", baseProv), loggateway.Str("target", target), loggateway.Str("base", baseMod), loggateway.Err(err))
			metrics.ModelRouterFallbackTotal.WithLabelValues("catalog_lookup").Inc()
			return nil, nil
		}
		return m, nil
	}
}

func invocationPrompt(inv *trpcagent.Invocation) string {
	if inv == nil {
		return ""
	}
	if c := strings.TrimSpace(inv.Message.Content); c != "" {
		return c
	}
	return lastUserPromptFromSession(inv)
}

func lastUserPromptFromSession(inv *trpcagent.Invocation) string {
	if inv == nil || inv.Session == nil {
		return ""
	}
	inv.Session.EventMu.RLock()
	defer inv.Session.EventMu.RUnlock()
	for i := len(inv.Session.Events) - 1; i >= 0; i-- {
		ev := inv.Session.Events[i]
		if strings.TrimSpace(ev.Author) != "user" {
			continue
		}
		if ev.Response != nil && len(ev.Response.Choices) > 0 {
			if t := strings.TrimSpace(ev.Response.Choices[0].Message.Content); t != "" {
				return t
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Built-in model selector strategies: cost-aware, quality-aware, latency-aware
// ---------------------------------------------------------------------------

// modelPricing extracts the input+output cost (USD per 1M tokens) from a
// provider-model's config_json. Returns (inputCost, outputCost, ok).
func modelPricing(pm biz.ProviderModel) (float64, float64, bool) {
	if pm.ConfigJSON == "" {
		return 0, 0, false
	}
	var cfg struct {
		Cost struct {
			InputUSDPer1M  float64 `json:"input_usd_per_1m"`
			OutputUSDPer1M float64 `json:"output_usd_per_1m"`
		} `json:"cost"`
		InputPriceMicroUSDPer1K  int64 `json:"input_price_micro_usd_per_1k"`
		OutputPriceMicroUSDPer1K int64 `json:"output_price_micro_usd_per_1k"`
	}
	if err := json.Unmarshal([]byte(pm.ConfigJSON), &cfg); err != nil {
		return 0, 0, false
	}
	if cfg.Cost.InputUSDPer1M > 0 || cfg.Cost.OutputUSDPer1M > 0 {
		return cfg.Cost.InputUSDPer1M, cfg.Cost.OutputUSDPer1M, true
	}
	if cfg.InputPriceMicroUSDPer1K > 0 || cfg.OutputPriceMicroUSDPer1K > 0 {
		return float64(cfg.InputPriceMicroUSDPer1K) / 1000, float64(cfg.OutputPriceMicroUSDPer1K) / 1000, true
	}
	return 0, 0, false
}

// CostAwareModelSelector selects the cheapest model that satisfies the
// invocation's requirements. It uses pricing metadata from the model catalog.
// Conservative: only switches when a clearly cheaper alternative with
// superset capabilities is found; otherwise returns nil (keep current model).
func CostAwareModelSelector(
	baseProv, baseMod string,
	catalog biz.TeamModelCatalog,
	rt *provider.RoundTrip,
	lg loggateway.Logger,
) trpcagent.ModelSelector {
	baseProv = strings.TrimSpace(baseProv)
	baseMod = strings.TrimSpace(baseMod)
	return func(ctx context.Context, inv *trpcagent.Invocation) (trpcmodel.Model, error) {
		if catalog == nil {
			return nil, nil
		}
		models, err := catalog.List(ctx)
		if err != nil {
			lg.Warn("cost-aware 选择器目录查询失败", loggateway.StepID("agent.model_selector.cost_aware"), loggateway.Err(err))
			return nil, nil
		}
		// Find current model's pricing and capabilities.
		var baseInputCost, baseOutputCost float64
		var baseCaps biz.ModelCapabilities
		baseFound := false
		for _, m := range models {
			if m.Provider == baseProv && m.Model == baseMod {
				baseInputCost, baseOutputCost, _ = modelPricing(m)
				baseCaps = provider.CapabilitiesForProviderModel(m)
				baseFound = true
				break
			}
		}
		if !baseFound || (baseInputCost == 0 && baseOutputCost == 0) {
			return nil, nil
		}
		// Find the cheapest alternative with same-or-superset capabilities.
		type candidate struct {
			model      biz.ProviderModel
			inputCost  float64
			outputCost float64
			totalCost  float64
		}
		var candidates []candidate
		for _, m := range models {
			if !m.Enabled || m.Provider != baseProv || m.Model == baseMod {
				continue
			}
			inCost, outCost, ok := modelPricing(m)
			if !ok {
				continue
			}
			caps := provider.CapabilitiesForProviderModel(m)
			// Check capability superset: alternative must support everything the base model does.
			if caps.TextOnly && !baseCaps.TextOnly {
				continue
			}
			if baseCaps.Vision && !caps.Vision {
				continue
			}
			if baseCaps.ToolCall && !caps.ToolCall {
				continue
			}
			if baseCaps.Thinking && !caps.Thinking {
				continue
			}
			total := inCost + outCost
			baseTotal := baseInputCost + baseOutputCost
			if total < baseTotal {
				candidates = append(candidates, candidate{model: m, inputCost: inCost, outputCost: outCost, totalCost: total})
			}
		}
		if len(candidates) == 0 {
			return nil, nil
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].totalCost < candidates[j].totalCost })
		best := candidates[0]
		m, err := provider.TRPCModelForProviderModel(ctx, catalog, rt, baseProv, best.model.Model, lg)
		if err != nil {
			lg.Warn("cost-aware 选择器模型构建失败", loggateway.StepID("agent.model_selector.cost_aware"), loggateway.Str("target", best.model.Model), loggateway.Err(err))
			return nil, nil
		}
		lg.Info("cost-aware 切换模型", loggateway.StepID("agent.model_selector.cost_aware"), loggateway.Phase("done"),
			loggateway.Str("provider", baseProv), loggateway.Str("base", baseMod), loggateway.Str("target", best.model.Model),
			loggateway.Float64("base_total_cost", baseInputCost+baseOutputCost), loggateway.Float64("target_total_cost", best.totalCost))
		return m, nil
	}
}

// qualityScore computes a simple heuristic quality score from model capabilities.
// More capabilities = higher quality potential.
func qualityScore(caps biz.ModelCapabilities) int {
	score := 0
	if caps.Thinking {
		score += 4
	}
	if caps.Vision {
		score += 2
	}
	if caps.ToolCall {
		score += 2
	}
	if caps.Cache {
		score += 1
	}
	if caps.Text && !caps.TextOnly {
		score += 1
	}
	return score
}

// QualityAwareModelSelector selects the highest quality model for the task
// type. It uses capability metadata (thinking, vision, etc.) to pick the
// best model. Conservative: only switches when a clearly better option exists.
func QualityAwareModelSelector(
	baseProv, baseMod string,
	catalog biz.TeamModelCatalog,
	rt *provider.RoundTrip,
	lg loggateway.Logger,
) trpcagent.ModelSelector {
	baseProv = strings.TrimSpace(baseProv)
	baseMod = strings.TrimSpace(baseMod)
	return func(ctx context.Context, inv *trpcagent.Invocation) (trpcmodel.Model, error) {
		if catalog == nil {
			return nil, nil
		}
		models, err := catalog.List(ctx)
		if err != nil {
			lg.Warn("quality-aware 选择器目录查询失败", loggateway.StepID("agent.model_selector.quality_aware"), loggateway.Err(err))
			return nil, nil
		}
		var baseScore int
		baseFound := false
		for _, m := range models {
			if m.Provider == baseProv && m.Model == baseMod {
				baseScore = qualityScore(provider.CapabilitiesForProviderModel(m))
				baseFound = true
				break
			}
		}
		if !baseFound {
			return nil, nil
		}
		// Find the highest-quality alternative (strictly better score).
		type candidate struct {
			model biz.ProviderModel
			score int
		}
		var best *candidate
		for _, m := range models {
			if !m.Enabled || m.Provider != baseProv || m.Model == baseMod {
				continue
			}
			caps := provider.CapabilitiesForProviderModel(m)
			score := qualityScore(caps)
			if score > baseScore && (best == nil || score > best.score) {
				best = &candidate{model: m, score: score}
			}
		}
		if best == nil {
			return nil, nil
		}
		m, err := provider.TRPCModelForProviderModel(ctx, catalog, rt, baseProv, best.model.Model, lg)
		if err != nil {
			lg.Warn("quality-aware 选择器模型构建失败", loggateway.StepID("agent.model_selector.quality_aware"), loggateway.Str("target", best.model.Model), loggateway.Err(err))
			return nil, nil
		}
		lg.Info("quality-aware 切换模型", loggateway.StepID("agent.model_selector.quality_aware"), loggateway.Phase("done"),
			loggateway.Str("provider", baseProv), loggateway.Str("base", baseMod), loggateway.Str("target", best.model.Model),
			loggateway.Int("base_score", baseScore), loggateway.Int("target_score", best.score))
		return m, nil
	}
}

// latencyHeuristic estimates relative latency from model name heuristics.
// Smaller/faster models (mini, flash, lite, small) get lower scores (faster).
// Models with "thinking" capability are assumed slower.
func latencyHeuristic(pm biz.ProviderModel) int {
	name := strings.ToLower(pm.Model)
	heuristic := 5 // default middle
	if strings.Contains(name, "mini") || strings.Contains(name, "flash") ||
		strings.Contains(name, "lite") || strings.Contains(name, "small") {
		heuristic = 1
	} else if strings.Contains(name, "pro") || strings.Contains(name, "opus") ||
		strings.Contains(name, "max") || strings.Contains(name, "ultra") {
		heuristic = 9
	}
	caps := provider.CapabilitiesForProviderModel(pm)
	if caps.Thinking {
		heuristic += 2
	}
	return heuristic
}

// LatencyAwareModelSelector selects the fastest responding model.
// It uses model name heuristics and capability metadata to estimate latency.
// Conservative: only switches when a clearly faster alternative exists.
func LatencyAwareModelSelector(
	baseProv, baseMod string,
	catalog biz.TeamModelCatalog,
	rt *provider.RoundTrip,
	lg loggateway.Logger,
) trpcagent.ModelSelector {
	baseProv = strings.TrimSpace(baseProv)
	baseMod = strings.TrimSpace(baseMod)
	return func(ctx context.Context, inv *trpcagent.Invocation) (trpcmodel.Model, error) {
		if catalog == nil {
			return nil, nil
		}
		models, err := catalog.List(ctx)
		if err != nil {
			lg.Warn("latency-aware 选择器目录查询失败", loggateway.StepID("agent.model_selector.latency_aware"), loggateway.Err(err))
			return nil, nil
		}
		var baseLatency int
		baseFound := false
		for _, m := range models {
			if m.Provider == baseProv && m.Model == baseMod {
				baseLatency = latencyHeuristic(m)
				baseFound = true
				break
			}
		}
		if !baseFound {
			return nil, nil
		}
		// Derive baseCaps once outside the loop.
		var baseCaps biz.ModelCapabilities
		for _, bm := range models {
			if bm.Provider == baseProv && bm.Model == baseMod {
				baseCaps = provider.CapabilitiesForProviderModel(bm)
				break
			}
		}
		// Find the fastest alternative with same-or-superset capabilities.
		type candidate struct {
			model        biz.ProviderModel
			latencyScore int
		}
		var best *candidate
		for _, m := range models {
			if !m.Enabled || m.Provider != baseProv || m.Model == baseMod {
				continue
			}
			lat := latencyHeuristic(m)
			if lat >= baseLatency {
				continue // not faster
			}
			caps := provider.CapabilitiesForProviderModel(m)
			if baseCaps.Vision && !caps.Vision {
				continue
			}
			if baseCaps.ToolCall && !caps.ToolCall {
				continue
			}
			if best == nil || lat < best.latencyScore {
				best = &candidate{model: m, latencyScore: lat}
			}
		}
		if best == nil {
			return nil, nil
		}
		m, err := provider.TRPCModelForProviderModel(ctx, catalog, rt, baseProv, best.model.Model, lg)
		if err != nil {
			lg.Warn("latency-aware 选择器模型构建失败", loggateway.StepID("agent.model_selector.latency_aware"), loggateway.Str("target", best.model.Model), loggateway.Err(err))
			return nil, nil
		}
		lg.Info("latency-aware 切换模型", loggateway.StepID("agent.model_selector.latency_aware"), loggateway.Phase("done"),
			loggateway.Str("provider", baseProv), loggateway.Str("base", baseMod), loggateway.Str("target", best.model.Model),
			loggateway.Int("base_latency_score", baseLatency), loggateway.Int("target_latency_score", best.latencyScore))
		return m, nil
	}
}
