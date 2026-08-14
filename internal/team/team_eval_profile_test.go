package team

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ─── fakes ──────────────────────────────────────────────────────────────────

type evalFakeCatalog struct{ models []biz.ProviderModel }

func (c *evalFakeCatalog) List(context.Context) ([]biz.ProviderModel, error) { return c.models, nil }

func (c *evalFakeCatalog) GetByProviderAndModel(_ context.Context, prov, mod string) (biz.ProviderModel, error) {
	for _, pm := range c.models {
		if pm.Provider == prov && pm.Model == mod {
			return pm, nil
		}
	}
	return biz.ProviderModel{}, biz.ErrProviderModelNotFound
}

func evalCatalogRow(prov, mod string) biz.ProviderModel {
	return biz.ProviderModel{
		Provider:             prov,
		Model:                mod,
		Enabled:              true,
		ConfigJSON:           `{"provider_type":"openai","api_base_url":"https://api.openai.com/v1","api_key":"sk-test"}`,
		CapabilitiesExplicit: true,
		Capabilities:         biz.ModelCapabilities{Text: true, ToolCall: true},
	}
}

type evalFakeTool struct{ name string }

func (t evalFakeTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{Name: t.name}
}

// evalStubAgents resolves any agent ID to the fixed key "leader-key"
// (cascade leader resolution path: ReadDeps.Agents.GetAgentByID).
type evalStubAgents struct{ biz.AgentRepository }

func (evalStubAgents) GetAgentByID(_ context.Context, id string) (biz.Agent, error) {
	return biz.Agent{ID: id, AgentKey: "leader-key"}, nil
}

func evalTestRunner(catalog biz.TeamModelCatalog) *Runner {
	return &Runner{
		lg: loggateway.NewNoop(),
		td: rt.TurnDeps{ReadDeps: rt.TurnReadDeps{LLM: catalog, Agents: evalStubAgents{}}},
	}
}

// hasFlowStep reports whether a flow_log entry with the given step ID was captured.
func (b *captureFlowBus) hasFlowStep(stepID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ev := range b.evs {
		if ev.Type != contract.MonitorEventTypeFlowLog {
			continue
		}
		if step, _ := ev.Metadata["step_id"].(string); step == stepID {
			return true
		}
	}
	return false
}

// ─── Definition 解析（ADR-E D1）──────────────────────────────────────────────

func TestParseDefinition_EvalProfile(t *testing.T) {
	raw := `{
		"mode": "pipeline",
		"members": [{"agent_id": "a1"}],
		"eval_profile": {
			"provider": "openai",
			"model": "gpt-4o",
			"tool_allowlist": ["search_docs", "get_deliverable"],
			"extra_model_fields": {"seed": 42, "temperature": 0}
		}
	}`
	def, err := ParseDefinition(raw)
	if err != nil {
		t.Fatal(err)
	}
	ep := def.EvalProfile
	if ep == nil {
		t.Fatal("EvalProfile = nil, want parsed")
	}
	if ep.Provider != "openai" || ep.Model != "gpt-4o" {
		t.Fatalf("pin = %s/%s, want openai/gpt-4o", ep.Provider, ep.Model)
	}
	if len(ep.ToolAllowlist) != 2 || ep.ToolAllowlist[0] != "search_docs" {
		t.Fatalf("allowlist = %v", ep.ToolAllowlist)
	}
	if ep.ExtraModelFields["seed"] != float64(42) {
		t.Fatalf("seed = %v, want 42", ep.ExtraModelFields["seed"])
	}
}

func TestParseDefinition_EvalProfileAbsent(t *testing.T) {
	def, err := ParseDefinition(`{"members":[{"agent_id":"a1"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if def.EvalProfile != nil {
		t.Fatalf("EvalProfile = %+v, want nil", def.EvalProfile)
	}
}

// ─── run-level option 装配（ADR-E D2）────────────────────────────────────────

// 无 eval_profile → 无 option（生产路径零开销）。
func TestEvalProfileRunOptions_Absent(t *testing.T) {
	r := evalTestRunner(&evalFakeCatalog{models: []biz.ProviderModel{evalCatalogRow("openai", "gpt-4o")}})
	def := Definition{Mode: "pipeline"}
	if opts := r.evalProfileRunOptions(context.Background(), def, "sess-1"); len(opts) != 0 {
		t.Fatalf("opts = %d, want 0", len(opts))
	}
}

// 全空 profile（"eval_profile": {}）→ 无 option。
func TestEvalProfileRunOptions_EmptyProfile(t *testing.T) {
	r := evalTestRunner(nil)
	def := Definition{Mode: "pipeline", EvalProfile: &EvalProfileDef{}}
	if opts := r.evalProfileRunOptions(context.Background(), def, "sess-1"); len(opts) != 0 {
		t.Fatalf("opts = %d, want 0 for empty profile", len(opts))
	}
}

// 全配置 → 模型钉住 + 生成参数 + 工具白名单三件套齐备且行为正确。
func TestEvalProfileRunOptions_Full(t *testing.T) {
	catalog := &evalFakeCatalog{models: []biz.ProviderModel{evalCatalogRow("openai", "gpt-4o")}}
	r := evalTestRunner(catalog)
	def := Definition{Mode: "pipeline", EvalProfile: &EvalProfileDef{
		Provider:         "openai",
		Model:            "gpt-4o",
		ToolAllowlist:    []string{"search_docs"},
		ExtraModelFields: map[string]any{"seed": 42, "temperature": 0},
	}}
	opts := r.evalProfileRunOptions(context.Background(), def, "sess-1")
	if len(opts) != 3 {
		t.Fatalf("opts = %d, want 3 (selector + extra fields + tool filter)", len(opts))
	}
	ro := trpcagent.NewRunOptions(opts...)
	if ro.ModelSelector == nil {
		t.Fatal("ModelSelector = nil, want pinned selector")
	}
	m, err := ro.ModelSelector(context.Background(), &trpcagent.Invocation{AgentName: "any-member"})
	if err != nil || m == nil {
		t.Fatalf("pinned selector: got (%v, %v)", m, err)
	}
	if got := m.Info().Name; got != "gpt-4o" {
		t.Fatalf("pinned model = %q, want gpt-4o", got)
	}
	if ro.ModelRequestExtraFields["seed"] != 42 {
		t.Fatalf("seed = %v, want 42", ro.ModelRequestExtraFields["seed"])
	}
	if ro.ToolFilter == nil {
		t.Fatal("ToolFilter = nil, want allowlist filter")
	}
	if !ro.ToolFilter(context.Background(), evalFakeTool{name: "search_docs"}) {
		t.Fatal("allowlisted tool search_docs rejected")
	}
	if ro.ToolFilter(context.Background(), evalFakeTool{name: "delete_everything"}) {
		t.Fatal("non-allowlisted tool delete_everything passed")
	}
}

// 仅工具白名单 → 只装 filter（模型/参数子项独立生效）。
func TestEvalProfileRunOptions_ToolAllowlistOnly(t *testing.T) {
	r := evalTestRunner(nil)
	def := Definition{Mode: "pipeline", EvalProfile: &EvalProfileDef{ToolAllowlist: []string{"search_docs"}}}
	opts := r.evalProfileRunOptions(context.Background(), def, "sess-1")
	if len(opts) != 1 {
		t.Fatalf("opts = %d, want 1 (tool filter only)", len(opts))
	}
	ro := trpcagent.NewRunOptions(opts...)
	if ro.ToolFilter == nil || ro.ModelSelector != nil || len(ro.ModelRequestExtraFields) != 0 {
		t.Fatalf("unexpected options: selector=%v extra=%v", ro.ModelSelector != nil, ro.ModelRequestExtraFields)
	}
}

// 优先级（ADR-E D2-2）：eval_profile 与 model_cascade 同时配置时 eval 胜出。
// 行为判别：cascade 对 leader（synthesizer）返回 nil 保持 base，eval 钉住
// 对任意 AgentName 返回固定模型。
func TestModelGovernanceRunOptions_EvalWinsOverCascade(t *testing.T) {
	catalog := &evalFakeCatalog{models: []biz.ProviderModel{
		evalCatalogRow("openai", "gpt-4o"),
		evalCatalogRow("openai", "gpt-4o-mini"),
	}}
	r := evalTestRunner(catalog)
	def := Definition{
		Mode:               "pipeline",
		SynthesizerAgentID: "leader-agent",
		ModelCascade:       &ModelCascadeDef{MemberProvider: "openai", MemberModel: "gpt-4o-mini"},
		EvalProfile:        &EvalProfileDef{Provider: "openai", Model: "gpt-4o"},
	}
	opts := r.modelGovernanceRunOptions(context.Background(), def, "sess-1")
	ro := trpcagent.NewRunOptions(opts...)
	if ro.ModelSelector == nil {
		t.Fatal("ModelSelector = nil")
	}
	m, err := ro.ModelSelector(context.Background(), &trpcagent.Invocation{AgentName: "leader-key"})
	if err != nil || m == nil {
		t.Fatalf("leader invocation: got (%v, %v) — cascade 语义（nil）说明 eval 未胜出", m, err)
	}
	if got := m.Info().Name; got != "gpt-4o" {
		t.Fatalf("leader model = %q, want pinned gpt-4o（cascade 会给 leader 保持 base）", got)
	}
}

// cascade-only 时维持既有行为（回归保护）。
func TestModelGovernanceRunOptions_CascadeOnly(t *testing.T) {
	catalog := &evalFakeCatalog{models: []biz.ProviderModel{
		evalCatalogRow("openai", "gpt-4o"),
		evalCatalogRow("openai", "gpt-4o-mini"),
	}}
	r := evalTestRunner(catalog)
	def := Definition{
		Mode:               "pipeline",
		SynthesizerAgentID: "leader-agent",
		ModelCascade:       &ModelCascadeDef{MemberProvider: "openai", MemberModel: "gpt-4o-mini"},
	}
	opts := r.modelGovernanceRunOptions(context.Background(), def, "sess-1")
	ro := trpcagent.NewRunOptions(opts...)
	if ro.ModelSelector == nil {
		t.Fatal("ModelSelector = nil, want cascade selector")
	}
	m, err := ro.ModelSelector(context.Background(), &trpcagent.Invocation{AgentName: "leader-key"})
	if err != nil || m != nil {
		t.Fatalf("cascade-only leader: got (%v, %v), want (nil, nil) 保持 base", m, err)
	}
}

// 两者皆无 → 无 option。
func TestModelGovernanceRunOptions_None(t *testing.T) {
	r := evalTestRunner(nil)
	if opts := r.modelGovernanceRunOptions(context.Background(), Definition{Mode: "pipeline"}, "sess-1"); len(opts) != 0 {
		t.Fatalf("opts = %d, want 0", len(opts))
	}
}

// FlowLog 审计（ADR-E D3）：profile 生效必须留可审计轨迹。
func TestEvalProfileRunOptions_EmitsFlowLog(t *testing.T) {
	catalog := &evalFakeCatalog{models: []biz.ProviderModel{evalCatalogRow("openai", "gpt-4o")}}
	r := evalTestRunner(catalog)
	def := Definition{Mode: "pipeline", EvalProfile: &EvalProfileDef{
		Provider: "openai", Model: "gpt-4o",
		ToolAllowlist:    []string{"search_docs"},
		ExtraModelFields: map[string]any{"seed": 42},
	}}
	monBus := &captureFlowBus{}
	em := event.NewTraceEmitter(&event.Infra{MonitorEventBus: monBus}, event.TraceContext{
		TraceID: "tr-eval", SessionID: "sess-1", RunID: "run-1", Domain: event.TraceDomainTeam,
	}, nil)
	ctx := event.WithTraceEmitter(context.Background(), em)

	r.evalProfileRunOptions(ctx, def, "sess-1")

	if !monBus.hasFlowStep("team.eval_profile.applied") {
		t.Fatalf("expected flow_log step team.eval_profile.applied, got %+v", monBus.evs)
	}
}
