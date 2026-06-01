package plugintrpc

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestResolveCostGuardTarget_BlockedModel(t *testing.T) {
	cfg := CostGuardConfig{
		BlockedModels: []string{"gpt-4"},
		FallbackModel: "gpt-3.5-turbo",
	}
	target := ResolveCostGuardTarget("gpt-4", cfg, 100, nil)
	if target != "gpt-3.5-turbo" {
		t.Fatalf("expected fallback, got %q", target)
	}
}

func TestResolveCostGuardTarget_PromptBudget(t *testing.T) {
	cfg := CostGuardConfig{
		MaxPromptTokens: 100,
		FallbackModel:   "cheap-model",
	}
	target := ResolveCostGuardTarget("base", cfg, 500, nil)
	if target != "cheap-model" {
		t.Fatalf("expected prompt fallback, got %q", target)
	}
}

func TestResolveCostGuardTarget_DailyBudget(t *testing.T) {
	tracker := NewCostGuardBudgetTracker(loggateway.NewNoop())
	cfg := CostGuardConfig{
		DailyTokenBudget: 1000,
		FallbackModel:    "cheap-model",
	}
	tracker.TryConsume(cfg.DailyTokenBudget, 900)
	target := ResolveCostGuardTarget("base", cfg, 200, tracker)
	if target != "cheap-model" {
		t.Fatalf("expected daily budget fallback, got %q", target)
	}
}

func TestCostGuardShouldBlock_NoFallback(t *testing.T) {
	cfg := CostGuardConfig{MaxPromptTokens: 10}
	block, reason := costGuardShouldBlock("base", cfg, 100, nil)
	if !block || reason != "max_prompt_tokens" {
		t.Fatalf("expected block without fallback, got block=%v reason=%q", block, reason)
	}
}

func TestIsSkillTool(t *testing.T) {
	if !isSkillTool("skill_run") || !isSkillTool("use_skill") {
		t.Fatal("expected skill tools")
	}
	if isSkillTool("read_file") {
		t.Fatal("read_file is not a skill tool")
	}
}

func TestNormalizeRunStatus(t *testing.T) {
	if normalizeRunStatus("ok") != "success" {
		t.Fatal("ok should map to success")
	}
}

func TestShouldPersistPluginRun(t *testing.T) {
	if shouldPersistPluginRun("success") || shouldPersistPluginRun("ok") {
		t.Fatal("success should not persist run row")
	}
	if !shouldPersistPluginRun("blocked") || !shouldPersistPluginRun("error") {
		t.Fatal("blocked/error should persist")
	}
}

func TestCostGuard_BeforeModel_FallbackBypassesDailyBudget(t *testing.T) {
	tracker := NewCostGuardBudgetTracker(loggateway.NewNoop())
	cfg := CostGuardConfig{
		DailyTokenBudget: 100,
		FallbackModel:    "cheap-model",
	}
	tracker.TryConsume(cfg.DailyTokenBudget, 100)

	registry := NewCostGuardBudgetRegistry(loggateway.NewNoop())
	registry.byScope["global"] = tracker

	c := &CostGuardPlugin{
		base: basePlugin{
			name:   "cost_guard",
			stats:  &noopStatsRecorder{},
			logger: NewPluginSafeLogger("cost_guard", nil, loggateway.NewNoop()),
		},
		cfg: cfg,
		rt: &Runtime{
			budgets: registry,
			active:  []runtimeEntry{{key: "cost_guard", scope: "global", costGuard: &cfg}},
		},
	}

	ctx := contextWithModelName(context.Background(), "cheap-model")
	args := &trpcmodel.BeforeModelArgs{
		Request: &trpcmodel.Request{
			Messages: []trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: "test"}},
		},
	}

	res, err := c.beforeModel(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.CustomResponse != nil {
		t.Fatal("fallback model should bypass daily budget block, but got blocked")
	}
}

func TestCostGuard_BeforeModel_BlocksNonFallbackWhenOverBudget(t *testing.T) {
	tracker := NewCostGuardBudgetTracker(loggateway.NewNoop())
	cfg := CostGuardConfig{
		DailyTokenBudget: 100,
		FallbackModel:    "cheap-model",
	}
	tracker.TryConsume(cfg.DailyTokenBudget, 100)

	registry := NewCostGuardBudgetRegistry(loggateway.NewNoop())
	registry.byScope["global"] = tracker

	c := &CostGuardPlugin{
		base: basePlugin{
			name:   "cost_guard",
			stats:  &noopStatsRecorder{},
			logger: NewPluginSafeLogger("cost_guard", nil, loggateway.NewNoop()),
		},
		cfg: cfg,
		rt: &Runtime{
			budgets: registry,
			active:  []runtimeEntry{{key: "cost_guard", scope: "global", costGuard: &cfg}},
		},
	}

	ctx := contextWithModelName(context.Background(), "expensive-model")
	args := &trpcmodel.BeforeModelArgs{
		Request: &trpcmodel.Request{
			Messages: []trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: "test"}},
		},
	}

	res, err := c.beforeModel(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.CustomResponse == nil {
		t.Fatal("non-fallback model over budget should be blocked")
	}
}

type budgetTestCtxKey string

const modelNameCtxKey budgetTestCtxKey = "model_name"

func contextWithModelName(ctx context.Context, modelName string) context.Context {
	inv := &trpcagent.Invocation{Model: &stubModelInfo{name: modelName}}
	return trpcagent.NewInvocationContext(ctx, inv)
}

type stubModelInfo struct {
	trpcmodel.Model
	name string
}

func (m *stubModelInfo) Info() trpcmodel.Info {
	return trpcmodel.Info{Name: m.name}
}