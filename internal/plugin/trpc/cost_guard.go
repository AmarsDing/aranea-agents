package plugintrpc

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

type costGuardConfig struct {
	DailyTokenBudget int      `json:"daily_token_budget"`
	MaxPromptTokens  int      `json:"max_prompt_tokens"`
	BlockedModels    []string `json:"blocked_models"`
	FallbackModel    string   `json:"fallback_model"`
	AdminBypass      bool     `json:"admin_bypass"`
}

type CostGuardPlugin struct {
	name  string
	cfg   costGuardConfig
	stats StatsRecorder

	mu      sync.Mutex
	day     string
	tokens  int
}

var _ trpcplugin.Plugin = (*CostGuardPlugin)(nil)

func NewCostGuardPlugin(p biz.Plugin, stats StatsRecorder) *CostGuardPlugin {
	var cfg costGuardConfig
	cfg.AdminBypass = true
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
	return &CostGuardPlugin{name: p.Key, cfg: cfg, stats: stats}
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
			c.record(ctx, "before_model", "ok")
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		c.record(ctx, "before_model", "blocked")
		return &trpcmodel.BeforeModelResult{
			Context:        ctx,
			CustomResponse: blockedModelResponse("cost_guard: prompt exceeds max_prompt_tokens"),
		}, nil
	}
	if c.cfg.DailyTokenBudget > 0 && !c.allowDaily(est) {
		if fb := strings.TrimSpace(c.cfg.FallbackModel); fb != "" && !toolInList(fb, c.cfg.BlockedModels) {
			patchRequestModelHint(args.Request, fb)
			c.record(ctx, "before_model", "ok")
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		c.record(ctx, "before_model", "blocked")
		return &trpcmodel.BeforeModelResult{
			Context:        ctx,
			CustomResponse: blockedModelResponse("cost_guard: daily token budget exceeded"),
		}, nil
	}
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
