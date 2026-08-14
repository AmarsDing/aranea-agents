package agent

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// ─── fakes ──────────────────────────────────────────────────────────────────

type cascadeFakeCatalog struct {
	models []biz.ProviderModel
}

func (c *cascadeFakeCatalog) List(context.Context) ([]biz.ProviderModel, error) {
	return c.models, nil
}

func (c *cascadeFakeCatalog) GetByProviderAndModel(_ context.Context, prov, mod string) (biz.ProviderModel, error) {
	for _, pm := range c.models {
		if pm.Provider == prov && pm.Model == mod {
			return pm, nil
		}
	}
	return biz.ProviderModel{}, biz.ErrProviderModelNotFound
}

type cascadeFakeModel struct{ name string }

func (m *cascadeFakeModel) GenerateContent(context.Context, *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	return nil, nil
}

func (m *cascadeFakeModel) Info() trpcmodel.Info { return trpcmodel.Info{Name: m.name} }

func cascadeCatalogRow(prov, mod string, inCost, outCost float64, toolCall, enabled bool) biz.ProviderModel {
	cfg := `{"provider_type":"openai","api_base_url":"https://api.openai.com/v1","api_key":"sk-test",` +
		`"cost":{"input_usd_per_1m":` + jsonFloat(inCost) + `,"output_usd_per_1m":` + jsonFloat(outCost) + `}}`
	return biz.ProviderModel{
		Provider:             prov,
		Model:                mod,
		Enabled:              enabled,
		ConfigJSON:           cfg,
		CapabilitiesExplicit: true,
		Capabilities:         biz.ModelCapabilities{Text: true, ToolCall: toolCall},
	}
}

func jsonFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func cascadeTestCatalog() *cascadeFakeCatalog {
	return &cascadeFakeCatalog{models: []biz.ProviderModel{
		cascadeCatalogRow("openai", "gpt-4o", 5.0, 15.0, true, true),
		cascadeCatalogRow("openai", "gpt-4o-mini", 0.15, 0.6, true, true),
		// 更便宜但无 ToolCall——auto 模式必须跳过（成员需要工具能力）。
		cascadeCatalogRow("openai", "gpt-3.5-turbo", 0.1, 0.2, false, true),
		// 禁用的超低价模型——必须跳过。
		cascadeCatalogRow("openai", "disabled-mini", 0.01, 0.01, true, false),
	}}
}

func cascadeTestRT() *provider.RoundTrip {
	return &provider.RoundTrip{HTTP: &http.Client{Timeout: 5 * time.Second}}
}

// ─── CheapestCapableModel（纯函数） ──────────────────────────────────────────

func TestCheapestCapableModel_PicksMinTotalCostWithToolCall(t *testing.T) {
	cat := cascadeTestCatalog()
	pm, ok := CheapestCapableModel(cat.models, "")
	if !ok {
		t.Fatal("expected a candidate")
	}
	if pm.Model != "gpt-4o-mini" {
		t.Fatalf("model = %s, want gpt-4o-mini (gpt-3.5 lacks ToolCall, disabled-mini disabled)", pm.Model)
	}
}

func TestCheapestCapableModel_ExcludesGivenModel(t *testing.T) {
	cat := cascadeTestCatalog()
	pm, ok := CheapestCapableModel(cat.models, "gpt-4o-mini")
	if !ok {
		t.Fatal("expected a candidate")
	}
	if pm.Model != "gpt-4o" {
		t.Fatalf("model = %s, want gpt-4o (mini excluded)", pm.Model)
	}
}

func TestCheapestCapableModel_NoCandidate(t *testing.T) {
	cat := &cascadeFakeCatalog{models: []biz.ProviderModel{
		cascadeCatalogRow("openai", "gpt-3.5-turbo", 0.1, 0.2, false, true), // no ToolCall
	}}
	if _, ok := CheapestCapableModel(cat.models, ""); ok {
		t.Fatal("expected ok=false when no ToolCall-capable enabled model exists")
	}
}

// ─── CascadeModelSelector ───────────────────────────────────────────────────

func TestCascadeModelSelector_NilInvocation(t *testing.T) {
	sel := CascadeModelSelector(nil, "openai", "gpt-4o-mini", cascadeTestCatalog(), cascadeTestRT(), loggateway.NewNoop())
	m, err := sel(context.Background(), nil)
	if err != nil || m != nil {
		t.Fatalf("nil invocation must keep base (nil, nil), got (%v, %v)", m, err)
	}
}

func TestCascadeModelSelector_LeaderKeepsBase(t *testing.T) {
	sel := CascadeModelSelector([]string{"leader-key"}, "openai", "gpt-4o-mini", cascadeTestCatalog(), cascadeTestRT(), loggateway.NewNoop())
	inv := &trpcagent.Invocation{AgentName: "leader-key", Model: &cascadeFakeModel{name: "gpt-4o"}}
	m, err := sel(context.Background(), inv)
	if err != nil || m != nil {
		t.Fatalf("leader must keep base model, got (%v, %v)", m, err)
	}
}

func TestCascadeModelSelector_MemberRoutesToExplicitTarget(t *testing.T) {
	sel := CascadeModelSelector([]string{"leader-key"}, "openai", "gpt-4o-mini", cascadeTestCatalog(), cascadeTestRT(), loggateway.NewNoop())
	inv := &trpcagent.Invocation{AgentName: "worker-key", Model: &cascadeFakeModel{name: "gpt-4o"}}
	m, err := sel(context.Background(), inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("member must be routed to cascade target")
	}
	if got := m.Info().Name; got != "gpt-4o-mini" {
		t.Fatalf("routed model = %s, want gpt-4o-mini", got)
	}
}

func TestCascadeModelSelector_MemberTargetSameAsBaseKeepsBase(t *testing.T) {
	sel := CascadeModelSelector(nil, "openai", "gpt-4o-mini", cascadeTestCatalog(), cascadeTestRT(), loggateway.NewNoop())
	inv := &trpcagent.Invocation{AgentName: "worker-key", Model: &cascadeFakeModel{name: "gpt-4o-mini"}}
	m, err := sel(context.Background(), inv)
	if err != nil || m != nil {
		t.Fatalf("target == base must be a no-op, got (%v, %v)", m, err)
	}
}

func TestCascadeModelSelector_ExplicitTargetMissingFallsBackToBase(t *testing.T) {
	sel := CascadeModelSelector(nil, "openai", "no-such-model", cascadeTestCatalog(), cascadeTestRT(), loggateway.NewNoop())
	inv := &trpcagent.Invocation{AgentName: "worker-key", Model: &cascadeFakeModel{name: "gpt-4o"}}
	m, err := sel(context.Background(), inv)
	if err != nil || m != nil {
		t.Fatalf("missing target must fall back to base (nil, nil), got (%v, %v)", m, err)
	}
}

func TestCascadeModelSelector_ExplicitTargetWithoutProviderFallsBackToBase(t *testing.T) {
	sel := CascadeModelSelector(nil, "", "gpt-4o-mini", cascadeTestCatalog(), cascadeTestRT(), loggateway.NewNoop())
	inv := &trpcagent.Invocation{AgentName: "worker-key", Model: &cascadeFakeModel{name: "gpt-4o"}}
	m, err := sel(context.Background(), inv)
	if err != nil || m != nil {
		t.Fatalf("explicit model without provider is invalid config, must keep base, got (%v, %v)", m, err)
	}
}

func TestCascadeModelSelector_AutoPicksCheapestToolCallModel(t *testing.T) {
	sel := CascadeModelSelector(nil, "", "", cascadeTestCatalog(), cascadeTestRT(), loggateway.NewNoop())
	inv := &trpcagent.Invocation{AgentName: "worker-key", Model: &cascadeFakeModel{name: "gpt-4o"}}
	m, err := sel(context.Background(), inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("auto cascade must route member to cheapest ToolCall model")
	}
	if got := m.Info().Name; got != "gpt-4o-mini" {
		t.Fatalf("routed model = %s, want gpt-4o-mini", got)
	}
}

func TestCascadeModelSelector_AutoExcludesInvocationBase(t *testing.T) {
	sel := CascadeModelSelector(nil, "", "", cascadeTestCatalog(), cascadeTestRT(), loggateway.NewNoop())
	// 成员的 base 已经是最便宜档——auto 不得把它路由到自身（no-op），保持 base。
	inv := &trpcagent.Invocation{AgentName: "worker-key", Model: &cascadeFakeModel{name: "gpt-4o-mini"}}
	m, err := sel(context.Background(), inv)
	if err != nil || m != nil {
		t.Fatalf("auto target == base must be a no-op, got (%v, %v)", m, err)
	}
}
