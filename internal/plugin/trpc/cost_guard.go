package plugintrpc

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

type costGuardConfig struct {
	DailyTokenBudget int      `json:"daily_token_budget"`
	MaxPromptTokens  int      `json:"max_prompt_tokens"`
	BlockedModels    []string `json:"blocked_models"`
	FallbackModel    string   `json:"fallback_model"`
	AdminBypass      bool     `json:"admin_bypass"`
}

// CostGuardConfig is the product configuration for cost_guard plugin routing.
type CostGuardConfig = costGuardConfig

// ResolveCostGuardFallbackModel returns fallback model when base model is blocked (legacy helper).
func ResolveCostGuardFallbackModel(model string, cfg CostGuardConfig) string {
	return ResolveCostGuardTarget(model, cfg, 1, nil)
}

type CostGuardPlugin struct {
	name   string
	cfg    costGuardConfig
	stats  StatsRecorder
	logger *PluginSafeLogger
	rt     *Runtime
}

var _ trpcplugin.Plugin = (*CostGuardPlugin)(nil)

func NewCostGuardPlugin(p biz.Plugin, stats StatsRecorder, bus event.Bus, rt *Runtime) *CostGuardPlugin {
	var cfg costGuardConfig
	cfg.AdminBypass = true
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
	return &CostGuardPlugin{name: p.Key, cfg: cfg, stats: stats, logger: NewPluginSafeLogger(p.Key, bus), rt: rt}
}

func (c *CostGuardPlugin) Name() string { return c.name }

func (c *CostGuardPlugin) Register(r *trpcplugin.Registry) {
	r.BeforeModel(c.beforeModel)
}

func (c *CostGuardPlugin) budget(ctx context.Context) *CostGuardBudgetTracker {
	if c == nil || c.rt == nil {
		return NewCostGuardBudgetTracker()
	}
	return c.rt.BudgetTrackerForContext(ctx)
}

func (c *CostGuardPlugin) beforeModel(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
	if args == nil || args.Request == nil {
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	}
	budget := c.budget(ctx)
	model := modelNameFromContext(ctx)
	est := estimateRequestTokens(args.Request)
	if block, reason := costGuardShouldBlock(model, c.cfg, est, budget); block {
		c.logger.Info("plugin.cost_guard.before_model", "status", "blocked", "model", model, "est_tokens", est, "reason", reason)
		c.record(ctx, "before_model", "blocked")
		return &trpcmodel.BeforeModelResult{
			Context:        ctx,
			CustomResponse: blockedModelResponse("cost_guard: " + reason),
		}, nil
	}
	// TPM-P1-03: when ModelSelector already routed this call to the configured
	// fallback model (because base model would exceed budget), do NOT hard-block
	// on TryConsume — the fallback path is the agreed escape valve. We still
	// account for the spend so downstream telemetry stays accurate.
	if c.cfg.DailyTokenBudget > 0 && !budget.TryConsume(c.cfg.DailyTokenBudget, est) {
		fallback := strings.TrimSpace(c.cfg.FallbackModel)
		if fallback != "" && strings.EqualFold(model, fallback) {
			budget.AddOverBudget(est)
			c.logger.Warn("plugin.cost_guard.before_model",
				"status", "over_budget_allowed",
				"model", model,
				"reason", "fallback_bypass_daily_budget",
				"est_tokens", est)
			c.record(ctx, "before_model", "over_budget_allowed")
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		c.logger.Info("plugin.cost_guard.before_model", "status", "blocked", "model", model, "reason", "daily_budget_exceeded")
		c.record(ctx, "before_model", "blocked")
		return &trpcmodel.BeforeModelResult{
			Context:        ctx,
			CustomResponse: blockedModelResponse("cost_guard: daily token budget exceeded"),
		}, nil
	}
	c.logger.Info("plugin.cost_guard.before_model", "status", "success", "model", model, "est_tokens", est)
	c.record(ctx, "before_model", "success")
	return &trpcmodel.BeforeModelResult{Context: ctx}, nil
}

func (c *CostGuardPlugin) record(ctx context.Context, point, status string) {
	if c.stats != nil {
		c.stats.Record(ctx, c.name, point, status)
	}
}
