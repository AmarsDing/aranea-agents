package plugintrpc

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

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

// ResolveCostGuardFallbackModel returns fallback model when base model is blocked.
func ResolveCostGuardFallbackModel(model string, cfg CostGuardConfig) string {
	model = strings.TrimSpace(model)
	if toolInList(model, cfg.BlockedModels) {
		return strings.TrimSpace(cfg.FallbackModel)
	}
	return ""
}

type CostGuardPlugin struct {
	name   string
	cfg    costGuardConfig
	stats  StatsRecorder
	logger *PluginSafeLogger

	mu     sync.Mutex
	day    string
	tokens int
}

var _ trpcplugin.Plugin = (*CostGuardPlugin)(nil)

func NewCostGuardPlugin(p biz.Plugin, stats StatsRecorder, bus event.Bus) *CostGuardPlugin {
	var cfg costGuardConfig
	cfg.AdminBypass = true
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
	return &CostGuardPlugin{name: p.Key, cfg: cfg, stats: stats, logger: NewPluginSafeLogger(p.Key, bus)}
}

func (c *CostGuardPlugin) Name() string { return c.name }

func (c *CostGuardPlugin) Register(r *trpcplugin.Registry) {
	r.BeforeModel(c.beforeModel)
}

func (c *CostGuardPlugin) beforeModel(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
	if args == nil || args.Request == nil {
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	}
	model := modelNameFromContext(ctx)
	if toolInList(model, c.cfg.BlockedModels) {
		c.logger.Info("plugin.cost_guard.before_model", "status", "blocked", "model", model, "reason", "model_blocked")
		c.record(ctx, "before_model", "blocked")
		return &trpcmodel.BeforeModelResult{
			Context:        ctx,
			CustomResponse: blockedModelResponse(fmt.Sprintf("cost_guard: model %q is blocked", model)),
		}, nil
	}
	est := estimatePromptTokens(args.Request)
	if c.cfg.MaxPromptTokens > 0 && est > c.cfg.MaxPromptTokens {
		if fb := strings.TrimSpace(c.cfg.FallbackModel); fb != "" {
			patchRequestModelHint(args.Request, fb)
			c.logger.Info("plugin.cost_guard.before_model", "status", "fallback", "model", model, "fallback_model", fb, "est_tokens", est, "reason", "max_prompt_tokens_exceeded")
			c.record(ctx, "before_model", "ok")
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		c.logger.Info("plugin.cost_guard.before_model", "status", "blocked", "model", model, "est_tokens", est, "reason", "max_prompt_tokens_exceeded")
		c.record(ctx, "before_model", "blocked")
		return &trpcmodel.BeforeModelResult{
			Context:        ctx,
			CustomResponse: blockedModelResponse("cost_guard: prompt exceeds max_prompt_tokens"),
		}, nil
	}
	if c.cfg.DailyTokenBudget > 0 && !c.allowDaily(est) {
		if fb := strings.TrimSpace(c.cfg.FallbackModel); fb != "" && !toolInList(fb, c.cfg.BlockedModels) {
			patchRequestModelHint(args.Request, fb)
			c.logger.Info("plugin.cost_guard.before_model", "status", "fallback", "model", model, "fallback_model", fb, "reason", "daily_budget_exceeded")
			c.record(ctx, "before_model", "ok")
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		c.logger.Info("plugin.cost_guard.before_model", "status", "blocked", "model", model, "reason", "daily_budget_exceeded")
		c.record(ctx, "before_model", "blocked")
		return &trpcmodel.BeforeModelResult{
			Context:        ctx,
			CustomResponse: blockedModelResponse("cost_guard: daily token budget exceeded"),
		}, nil
	}
	c.logger.Info("plugin.cost_guard.before_model", "status", "ok", "model", model, "est_tokens", est)
	c.record(ctx, "before_model", "ok")
	return &trpcmodel.BeforeModelResult{Context: ctx}, nil
}

func (c *CostGuardPlugin) allowDaily(add int) bool {
	day := time.Now().UTC().Format("2006-01-02")
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.day != day {
		c.day = day
		c.tokens = 0
	}
	if c.tokens+add > c.cfg.DailyTokenBudget {
		return false
	}
	c.tokens += add
	return true
}

func estimatePromptTokens(req *trpcmodel.Request) int {
	if req == nil {
		return 0
	}
	n := 0
	for _, m := range req.Messages {
		n += len(m.Content) / 4
	}
	if n == 0 {
		return 1
	}
	return n
}

func (c *CostGuardPlugin) record(ctx context.Context, point, status string) {
	if c.stats != nil {
		c.stats.Record(ctx, c.name, point, status)
	}
}
