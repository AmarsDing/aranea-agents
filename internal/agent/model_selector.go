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
//
// trackerFor resolves the budget tracker per invocation from ctx (N-B2): the
// agent build is cached and shared across workspaces, so baking a tracker at
// build time would pin the selector to the builder's bucket while the runtime
// cost_guard plugin consumes the per-request "{workspace}:{agent}" bucket —
// split accounting. Resolving per call keeps both on the same bucket.
func PluginCostGuardSelector(
	baseProv, baseMod string,
	catalog biz.TeamModelCatalog,
	rt *provider.RoundTrip,
	cfg plugintrpc.CostGuardConfig,
	trackerFor func(ctx context.Context) *plugintrpc.CostGuardBudgetTracker,
	lg loggateway.Logger,
) trpcagent.ModelSelector {
	baseProv = strings.TrimSpace(baseProv)
	baseMod = strings.TrimSpace(baseMod)
	return func(ctx context.Context, inv *trpcagent.Invocation) (trpcmodel.Model, error) {
		est := plugintrpc.EstimateInvocationTokens(inv)
		target := plugintrpc.ResolveCostGuardTarget(baseMod, cfg, est, trackerFor(ctx))
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
// P2-1: Model cascade routing (leader/member tiered selection)
// ---------------------------------------------------------------------------

// CascadeModelSelector routes team member invocations to the cost-tier model
// while leader/planner invocations (leaderAgentKeys, matched against
// Invocation.AgentName == agent key) keep their configured high-tier model.
//
// Tier semantics (Ensemble QSP 实证 + HiClaw fallback 启示）：
//   - leader/planner/synthesizer：保持 base 模型（高档），selector 返回 nil。
//   - member/executor：路由到成本档模型——显式配置 (memberProv, memberMod)，
//     或 memberMod 为空时 auto 选取目录中最便宜的 ToolCall 可用模型。
//
// 保守语义：目标解析/构建失败、目标 == 当前 base、invocation 缺少 base 模型名
// 时一律返回 (nil, nil) 保持 base，绝不因级联配置错误阻断运行。
//
// 注意：run-level selector（RunOption）在框架内优先于 build-level selector，
// 且二者不链式——团队级联是显式管理策略，覆盖成员自身的模型路由插件。
func CascadeModelSelector(
	leaderAgentKeys []string,
	memberProv, memberMod string,
	catalog biz.TeamModelCatalog,
	rt *provider.RoundTrip,
	lg loggateway.Logger,
) trpcagent.ModelSelector {
	leaders := make(map[string]struct{}, len(leaderAgentKeys))
	for _, k := range leaderAgentKeys {
		if k = strings.TrimSpace(k); k != "" {
			leaders[k] = struct{}{}
		}
	}
	memberProv = strings.TrimSpace(memberProv)
	memberMod = strings.TrimSpace(memberMod)
	return func(ctx context.Context, inv *trpcagent.Invocation) (trpcmodel.Model, error) {
		if inv == nil {
			return nil, nil
		}
		if _, ok := leaders[inv.AgentName]; ok {
			return nil, nil // leader/planner 保持高档 base
		}
		baseName := ""
		if inv.Model != nil {
			baseName = strings.TrimSpace(inv.Model.Info().Name)
		}
		prov, mod := memberProv, memberMod
		if mod != "" && prov == "" {
			// 显式 member_model 必须配 member_provider，否则配置无效。
			lg.Warn("级联配置缺少 member_provider，保持基础模型",
				loggateway.StepID("agent.model_cascade.route"),
				loggateway.Str("member_model", mod), loggateway.Str("agent", inv.AgentName))
			return nil, nil
		}
		if mod == "" {
			// auto：目录内最便宜的 ToolCall 可用模型（成员执行需要工具能力）。
			// 不排除 base——若 base 已是最便宜档，下方 mod == baseName 的 no-op
			// 检查负责保持；排除 base 会把成员升级到次便宜模型，违背降本语义。
			if catalog == nil {
				return nil, nil
			}
			models, err := catalog.List(ctx)
			if err != nil {
				lg.Warn("级联 auto 目录查询失败", loggateway.StepID("agent.model_cascade.route"), loggateway.Err(err))
				return nil, nil
			}
			pm, ok := CheapestCapableModel(models, "")
			if !ok {
				return nil, nil
			}
			prov, mod = pm.Provider, pm.Model
		}
		if mod == baseName {
			return nil, nil // no-op：目标即当前 base
		}
		m, err := provider.TRPCModelForProviderModel(ctx, catalog, rt, prov, mod, lg)
		if err != nil {
			lg.Warn("级联目标模型构建失败，保持基础模型",
				loggateway.StepID("agent.model_cascade.route"),
				loggateway.Str("provider", prov), loggateway.Str("target", mod),
				loggateway.Str("base", baseName), loggateway.Str("agent", inv.AgentName), loggateway.Err(err))
			metrics.ModelRouterFallbackTotal.WithLabelValues("cascade_catalog").Inc()
			return nil, nil
		}
		lg.Info("级联路由成员模型", loggateway.StepID("agent.model_cascade.route"), loggateway.Phase("done"),
			loggateway.Str("provider", prov), loggateway.Str("target", mod),
			loggateway.Str("base", baseName), loggateway.Str("agent", inv.AgentName))
		return m, nil
	}
}

// CheapestCapableModel returns the enabled, ToolCall-capable model with the
// lowest input+output price (USD per 1M tokens) from the catalog list.
// excludeModel 跳过指定模型名（通常是调用方自身 base，避免 no-op 自路由）。
// 无定价信息（modelPricing 解析失败）的行不参与比价。
func CheapestCapableModel(models []biz.ProviderModel, excludeModel string) (biz.ProviderModel, bool) {
	excludeModel = strings.TrimSpace(excludeModel)
	var best biz.ProviderModel
	bestTotal := 0.0
	found := false
	for _, m := range models {
		if !m.Enabled || m.Model == excludeModel {
			continue
		}
		if !provider.CapabilitiesForProviderModel(m).ToolCall {
			continue
		}
		inCost, outCost, ok := modelPricing(m)
		if !ok {
			continue
		}
		total := inCost + outCost
		if !found || total < bestTotal {
			best, bestTotal, found = m, total, true
		}
	}
	return best, found
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
