package plugintrpc

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

type CostGuardConfig struct {
	DailyTokenBudget int      `json:"daily_token_budget"`
	MaxPromptTokens  int      `json:"max_prompt_tokens"`
	BlockedModels    []string `json:"blocked_models"`
	FallbackModel    string   `json:"fallback_model"`
}

type CostGuardPlugin struct {
	base basePlugin
	cfg  CostGuardConfig
	rt   *Runtime
}

var _ trpcplugin.Plugin = (*CostGuardPlugin)(nil)

func NewCostGuardPlugin(p biz.Plugin, stats StatsRecorder, monitorBus contract.MonitorBus, rt *Runtime, lg loggateway.Logger) *CostGuardPlugin {
	var cfg CostGuardConfig
	parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg, lg)
	return &CostGuardPlugin{base: newBasePlugin(p.Key, stats, monitorBus, lg), cfg: cfg, rt: rt}
}

func (c *CostGuardPlugin) Name() string { return c.base.Name() }

func (c *CostGuardPlugin) Register(r *trpcplugin.Registry) {
	r.BeforeModel(c.beforeModel)
}

func (c *CostGuardPlugin) budget(ctx context.Context) *CostGuardBudgetTracker {
	if c == nil || c.rt == nil {
		lg := c.base.lg
		if lg == nil {
			lg = loggateway.NewNoop()
		}
		return NewCostGuardBudgetTracker(lg)
	}
	return c.rt.BudgetTrackerForContext(ctx)
}

func (c *CostGuardPlugin) beforeModel(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
	if args == nil || args.Request == nil {
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	}
	budget := c.budget(ctx)
	model := modelNameFromContext(ctx)
	est := estimatePromptTokens(args.Request)
	if block, reason := costGuardShouldBlock(model, c.cfg, est, budget); block {
		// TPM-P1-03: Fallback bypass — only for daily_budget reason.
		// daily_budget is a soft limit: the fallback model is the agreed escape
		// valve when the base model would exceed budget, so we allow it through
		// and account for the spend via AddOverBudget. We do NOT bypass for
		// max_prompt_tokens (hard limit — API would reject) or blocked_model
		// (explicit admin block — must never be circumvented).
		fallback := strings.TrimSpace(c.cfg.FallbackModel)
		if fallback != "" && strings.EqualFold(model, fallback) && reason == "daily_budget" {
			budget.AddOverBudget(est)
			c.base.logger.Warn("plugin.cost_guard.before_model",
				"status", "over_budget_allowed",
				"model", model,
				"reason", "fallback_bypass_daily_budget",
				"est_tokens", est)
			c.base.record(ctx, "before_model", "over_budget_allowed")
			// N-03: Emit notice Activity for fallback bypass.
			c.emitNotice(ctx, "cost_guard_fallback",
				fmt.Sprintf("日预算已达上限，已切换至备用模型 %s", model))
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		c.base.logger.Info("plugin.cost_guard.before_model", "status", "blocked", "model", model, "est_tokens", est, "reason", reason)
		c.base.recordEvent(ctx, "before_model", "blocked",
			fmt.Sprintf("模型 %s 调用被阻止（原因：%s，预估 prompt tokens=%d）", model, reason, est))
		// N-03: Emit notice Activity for blocked model.
		c.emitNotice(ctx, "cost_guard_blocked",
			fmt.Sprintf("模型 %s 已被阻止（原因：%s）", model, reason))
		return &trpcmodel.BeforeModelResult{
			Context:        ctx,
			CustomResponse: blockedModelResponse("cost_guard: " + reason),
		}, nil
	}
	// TryConsume is the actual token spend step (costGuardShouldBlock above only
	// checks via WouldExceed — a read-only probe). When costGuardShouldBlock
	// returns false because a fallback is available, the routing layer should have
	// already redirected the request to the fallback model; if it hasn't (e.g.
	// current model is still the base model), TryConsume acts as a safety net to
	// block the over-budget base model from executing.
	if c.cfg.DailyTokenBudget > 0 && !budget.TryConsume(c.cfg.DailyTokenBudget, est) {
		c.base.logger.Info("plugin.cost_guard.before_model", "status", "blocked", "model", model, "reason", "daily_budget_exceeded")
		c.base.recordEvent(ctx, "before_model", "blocked",
			fmt.Sprintf("模型 %s 调用被阻止（原因：daily_budget_exceeded，日预算 %d tokens 已用尽）", model, c.cfg.DailyTokenBudget))
		c.emitNotice(ctx, "cost_guard_budget_exceeded",
			fmt.Sprintf("日 token 预算已用尽，模型 %s 调用被阻止", model))
		return &trpcmodel.BeforeModelResult{
			Context:        ctx,
			CustomResponse: blockedModelResponse("cost_guard: daily token budget exceeded"),
		}, nil
	}
	c.base.logger.Info("plugin.cost_guard.before_model", "status", "success", "model", model, "est_tokens", est)
	c.base.record(ctx, "before_model", "success")
	return &trpcmodel.BeforeModelResult{Context: ctx}, nil
}

// emitNotice emits a notice Activity via the ActivityEmitter in ctx.
// It is a best-effort operation: errors are logged but do not affect the
// cost guard's blocking decision.
func (c *CostGuardPlugin) emitNotice(ctx context.Context, noticeType, content string) {
	if emitter := biz.ActivityEmitterFromContext(ctx); emitter != nil {
		if err := emitter.EmitNotice(ctx, content, noticeType); err != nil {
			c.base.lg.Warn("EmitNotice failed",
				loggateway.StepID("plugin.cost_guard"),
				loggateway.Err(err))
		}
	}
}
