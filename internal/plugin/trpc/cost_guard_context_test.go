package plugintrpc

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestBudgetTrackerForContext_PerAgentIsolation(t *testing.T) {
	t.Parallel()
	rt := &Runtime{
		budgets: NewCostGuardBudgetRegistry(loggateway.NewNoop()),
		active: []runtimeEntry{
			{key: "cost_guard", scope: "global", costGuard: &CostGuardConfig{}},
		},
	}
	rt.SetAgentKeyResolver(func(_ context.Context, agentKey string) string {
		switch agentKey {
		case "agent-a":
			return "id-a"
		case "agent-b":
			return "id-b"
		default:
			return agentKey
		}
	})
	cfg := CostGuardConfig{DailyTokenBudget: 1000, FallbackModel: "cheap"}
	ctxA := contextWithAgent("agent-a")
	ctxB := contextWithAgent("agent-b")
	a := rt.BudgetTrackerForContext(ctxA)
	b := rt.BudgetTrackerForContext(ctxB)
	a.TryConsume(cfg.DailyTokenBudget, 900)
	if ResolveCostGuardTarget("base", cfg, 200, a) != "cheap" {
		t.Fatal("agent-a should hit budget")
	}
	if ResolveCostGuardTarget("base", cfg, 200, b) != "" {
		t.Fatal("agent-b should not hit budget")
	}
}

func TestRetryReflect_SkipsHighRiskConfirmTools(t *testing.T) {
	t.Parallel()
	rt := &Runtime{
		active: []runtimeEntry{
			{
				key:               "confirmation_guard",
				scope:             "global",
				confirmationGuard: &ConfirmationGuardConfig{ConfirmTools: []string{"delete_file"}},
			},
		},
	}
	rt.SetAgentKeyResolver(func(_ context.Context, _ string) string { return "agent-1" })
	p := NewRetryAndReflectPlugin(biz.Plugin{
		Key:        "retry_and_reflect",
		ConfigJSON: `{"high_risk_tools_need_confirm":true}`,
	}, nil, nil, rt, loggateway.NewNoop())
	ctx := contextWithAgent("my-agent")
	result, err := p.afterTool(ctx, &trpctool.AfterToolArgs{
		ToolName: "delete_file",
		Error:    errors.New("boom"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != nil && result.CustomResult != nil {
		t.Fatal("expected no reflection for high-risk confirm tool")
	}
}

func TestRetryReflect_SkipsCatalogConfirmTools(t *testing.T) {
	t.Parallel()
	rt := &Runtime{}
	rt.SetCatalogConfirmChecker(func(_ context.Context, _, toolName string) bool {
		return toolName == "bash"
	})
	p := NewRetryAndReflectPlugin(biz.Plugin{
		Key:        "retry_and_reflect",
		ConfigJSON: `{"high_risk_tools_need_confirm":true}`,
	}, nil, nil, rt, loggateway.NewNoop())
	ctx := contextWithAgent("my-agent")
	result, err := p.afterTool(ctx, &trpctool.AfterToolArgs{
		ToolName: "bash",
		Error:    errors.New("boom"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != nil && result.CustomResult != nil {
		t.Fatal("expected no reflection for catalog confirm tool")
	}
}

func TestToolRequiresConfirmation_CatalogAndPlugin(t *testing.T) {
	t.Parallel()
	rt := &Runtime{
		active: []runtimeEntry{
			{
				key:               "confirmation_guard",
				scope:             "global",
				confirmationGuard: &ConfirmationGuardConfig{ConfirmTools: []string{"delete_file"}},
			},
		},
	}
	rt.SetCatalogConfirmChecker(func(_ context.Context, _, toolName string) bool {
		return toolName == "bash"
	})
	ctx := contextWithAgent("agent")
	if !rt.ToolRequiresConfirmation(ctx, "delete_file", nil) {
		t.Fatal("expected plugin confirm match")
	}
	if !rt.ToolRequiresConfirmation(ctx, "bash", nil) {
		t.Fatal("expected catalog confirm match")
	}
	if rt.ToolRequiresConfirmation(ctx, "read_file", nil) {
		t.Fatal("did not expect confirm for read_file")
	}
}

func contextWithAgent(agentKey string) context.Context {
	inv := trpcagent.NewInvocation()
	inv.AgentName = agentKey
	return trpcagent.NewInvocationContext(context.Background(), inv)
}
