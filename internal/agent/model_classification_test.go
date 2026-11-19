package agent

// D6 L-1 密级路由 selector 测试（需求 §7.3 验收 #1/#5）。
// 验收 #1：水位线 >= 阈值 → 实际打 local（mock 目录断言路由结果）；
// 验收 #5：解析/读库失败 → 保守 nil 保持 base。

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// ─── fakes ──────────────────────────────────────────────────────────────────

// classificationFakePolicyRepo 固定阈值策略 repo（GetPolicy 直返，无热副本干扰）。
type classificationFakePolicyRepo struct {
	threshold int
	err       error
}

func (r *classificationFakePolicyRepo) GetPolicy(context.Context) (*biz.SecopsModelPolicy, error) {
	if r.err != nil {
		return nil, r.err
	}
	return &biz.SecopsModelPolicy{LocalThreshold: r.threshold}, nil
}

func (r *classificationFakePolicyRepo) SetPolicy(context.Context, int, string) error { return nil }

// classificationTestGate 注册测试门并返回清理函数（每个用例独立门，杜绝串扰）。
func classificationTestGate(t *testing.T, ceiling int, ceilingErr error, threshold int, policyErr error) {
	t.Helper()
	policy := biz.NewSecopsModelPolicyUsecase(&classificationFakePolicyRepo{threshold: threshold, err: policyErr}, loggateway.NewNoop())
	SetDefaultClassificationGate(ClassificationGate{
		CeilingFor: func(context.Context, string) (int, error) { return ceiling, ceilingErr },
		Policy:     policy,
	})
	t.Cleanup(func() { SetDefaultClassificationGate(ClassificationGate{}) })
}

func classificationInv(sessionID string) *trpcagent.Invocation {
	inv := &trpcagent.Invocation{AgentName: "worker-key"}
	if sessionID != "" {
		inv.Session = &trpcsession.Session{ID: sessionID}
	}
	return inv
}

// classificationCatalogRow 造一行带通道类别的目录记录（cloud 打底）。
func classificationCatalogRow(prov, mod, channelClass string, inCost, outCost float64, toolCall, enabled bool) biz.ProviderModel {
	row := cascadeCatalogRow(prov, mod, inCost, outCost, toolCall, enabled)
	row.ChannelClass = channelClass
	return row
}

// classificationTestCatalog base=cloud openai + local ollama 双候选。
func classificationTestCatalog() *cascadeFakeCatalog {
	return &cascadeFakeCatalog{models: []biz.ProviderModel{
		classificationCatalogRow("openai", "gpt-4o", biz.ProviderChannelClassCloud, 5.0, 15.0, true, true),
		classificationCatalogRow("ollama", "qwen3-32b", biz.ProviderChannelClassLocal, 0.0, 0.0, true, true),
		classificationCatalogRow("ollama", "qwen3-4b", biz.ProviderChannelClassLocal, 0.0, 0.0, true, true),
	}}
}

func classificationSelector(cat *cascadeFakeCatalog) trpcagent.ModelSelector {
	return ClassificationAwareModelSelector("openai", "gpt-4o", cat, cascadeTestRT(), loggateway.NewNoop())
}

// ─── 验收 #1：水位线 >= 阈值 → 实际打 local ──────────────────────────────────

func TestClassificationAwareModelSelector_ForcesLocal(t *testing.T) {
	classificationTestGate(t, 3, nil, 3, nil) // ceiling=3 >= threshold=3（L3=秘密 强制本地）
	sel := classificationSelector(classificationTestCatalog())

	m, err := sel(context.Background(), classificationInv("sess-l3"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("ceiling >= threshold 必须路由到 local，不得保持 base")
	}
	// 两个 local 候选均无定价（0/0）→ CheapestCapableModel 跳过，兜底第一个启用 local。
	if got := m.Info().Name; got != "qwen3-32b" {
		t.Fatalf("routed model = %s, want qwen3-32b (local)", got)
	}
}

func TestClassificationAwareModelSelector_CheapestPricedLocalWins(t *testing.T) {
	classificationTestGate(t, 4, nil, 3, nil)
	cat := classificationTestCatalog()
	// 给 4b 更低定价 → 比价路径选中 4b（验证 CheapestCapableModel 分支）。
	for i := range cat.models {
		if cat.models[i].Model == "qwen3-4b" {
			cat.models[i].ConfigJSON = `{"provider_type":"openai","api_base_url":"https://api.openai.com/v1","api_key":"sk-t",` +
				`"cost":{"input_usd_per_1m":0.1,"output_usd_per_1m":0.2}}`
		}
	}
	sel := ClassificationAwareModelSelector("openai", "gpt-4o", cat, cascadeTestRT(), loggateway.NewNoop())

	m, err := sel(context.Background(), classificationInv("sess-l4"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("ceiling=4 >= threshold=3 必须路由到 local")
	}
	if got := m.Info().Name; got != "qwen3-4b" {
		t.Fatalf("routed model = %s, want qwen3-4b (cheapest priced local)", got)
	}
}

func TestClassificationAwareModelSelector_BaseAlreadyLocalKeepsBase(t *testing.T) {
	classificationTestGate(t, 5, nil, 3, nil)
	cat := classificationTestCatalog()
	sel := ClassificationAwareModelSelector("ollama", "qwen3-32b", cat, cascadeTestRT(), loggateway.NewNoop())

	m, err := sel(context.Background(), classificationInv("sess-l5"))
	if err != nil || m != nil {
		t.Fatalf("base 已是 local 必须保持 base (nil, nil)，got (%v, %v)", m, err)
	}
}

// ─── fail-closed：命中强制本地但目录无 local 候选 ────────────────────────────

func TestClassificationAwareModelSelector_NoLocalCandidateFailsClosed(t *testing.T) {
	classificationTestGate(t, 3, nil, 3, nil)
	cat := &cascadeFakeCatalog{models: []biz.ProviderModel{
		classificationCatalogRow("openai", "gpt-4o", biz.ProviderChannelClassCloud, 5.0, 15.0, true, true),
	}}
	sel := classificationSelector(cat)

	m, err := sel(context.Background(), classificationInv("sess-l3"))
	if err == nil {
		t.Fatalf("无 local 候选必须 fail-closed 返回 error（禁止回落 cloud），got model=%v", m)
	}
	if m != nil {
		t.Fatalf("fail-closed 路径不得返回模型，got %v", m.Info().Name)
	}
}

// ─── 验收 #5：保守 nil 语义全集 ──────────────────────────────────────────────

func TestClassificationAwareModelSelector_GateDisabledKeepsBase(t *testing.T) {
	SetDefaultClassificationGate(ClassificationGate{}) // 零值门
	t.Cleanup(func() { SetDefaultClassificationGate(ClassificationGate{}) })
	sel := classificationSelector(classificationTestCatalog())

	m, err := sel(context.Background(), classificationInv("sess-l3"))
	if err != nil || m != nil {
		t.Fatalf("门停用必须保持 base，got (%v, %v)", m, err)
	}
}

func TestClassificationAwareModelSelector_NoSessionKeepsBase(t *testing.T) {
	classificationTestGate(t, 5, nil, 3, nil)
	sel := classificationSelector(classificationTestCatalog())

	m, err := sel(context.Background(), classificationInv(""))
	if err != nil || m != nil {
		t.Fatalf("无会话 ID 必须保持 base（无水位线可判），got (%v, %v)", m, err)
	}
}

func TestClassificationAwareModelSelector_CeilingErrorKeepsBase(t *testing.T) {
	classificationTestGate(t, 0, errors.New("db down"), 3, nil)
	sel := classificationSelector(classificationTestCatalog())

	m, err := sel(context.Background(), classificationInv("sess-x"))
	if err != nil || m != nil {
		t.Fatalf("水位线读失败必须保守 nil 保持 base（验收 #5），got (%v, %v)", m, err)
	}
}

func TestClassificationAwareModelSelector_PolicyErrorKeepsBase(t *testing.T) {
	classificationTestGate(t, 5, nil, 3, errors.New("policy repo down"))
	sel := classificationSelector(classificationTestCatalog())

	m, err := sel(context.Background(), classificationInv("sess-x"))
	if err != nil || m != nil {
		t.Fatalf("策略读失败必须保守 nil 保持 base（验收 #5），got (%v, %v)", m, err)
	}
}

func TestClassificationAwareModelSelector_BelowThresholdKeepsBase(t *testing.T) {
	classificationTestGate(t, 2, nil, 3, nil) // L2 < 阈值3
	sel := classificationSelector(classificationTestCatalog())

	m, err := sel(context.Background(), classificationInv("sess-l2"))
	if err != nil || m != nil {
		t.Fatalf("水位线低于阈值必须保持 base，got (%v, %v)", m, err)
	}
}

func TestClassificationAwareModelSelector_UnclassifiedKeepsBase(t *testing.T) {
	classificationTestGate(t, 0, nil, 3, nil) // 未定级
	sel := classificationSelector(classificationTestCatalog())

	m, err := sel(context.Background(), classificationInv("sess-l0"))
	if err != nil || m != nil {
		t.Fatalf("未定级会话必须保持 base，got (%v, %v)", m, err)
	}
}

// ─── L-2 通道契约：selector 每步写入路由裁决（SecopsRouteState）──────────────

func TestClassificationAwareModelSelector_WritesRouteStateForcedLocal(t *testing.T) {
	classificationTestGate(t, 3, nil, 3, nil)
	sel := classificationSelector(classificationTestCatalog())

	inv := classificationInv("sess-l3")
	if _, err := sel(context.Background(), inv); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	st := SecopsRouteStateFromInvocation(inv)
	if st.Channel != biz.ProviderChannelClassLocal || !st.Forced {
		t.Fatalf("route state = %+v, want local+forced", st)
	}
	if st.Ceiling != 3 || st.Threshold != 3 {
		t.Fatalf("route state 水位线快照 = %+v", st)
	}
}

func TestClassificationAwareModelSelector_RouteStateBaseAlreadyLocal(t *testing.T) {
	classificationTestGate(t, 4, nil, 3, nil)
	cat := classificationTestCatalog()
	// base 本身即本地通道：selector 保持 base（nil），但状态须标记 forced local。
	sel := ClassificationAwareModelSelector("ollama", "qwen3-32b", cat, cascadeTestRT(), loggateway.NewNoop())
	inv := classificationInv("sess-l4")
	m, err := sel(context.Background(), inv)
	if err != nil || m != nil {
		t.Fatalf("base 已 local 必须保持 base，got (%v, %v)", m, err)
	}
	st := SecopsRouteStateFromInvocation(inv)
	if st.Channel != biz.ProviderChannelClassLocal || !st.Forced {
		t.Fatalf("route state = %+v, want local+forced (route_local_forced 审计依据)", st)
	}
}

func TestClassificationAwareModelSelector_RouteStateClearedOnKeepBase(t *testing.T) {
	classificationTestGate(t, 2, nil, 3, nil) // 低于阈值 → 保守 nil
	sel := classificationSelector(classificationTestCatalog())

	inv := classificationInv("sess-l2")
	// 预置上一步的脏状态：本步 selector 必须清零（钩子据此自解析通道）。
	inv.SetState(secopsRouteStateKey, SecopsRouteState{Channel: biz.ProviderChannelClassLocal, Forced: true, Ceiling: 5})
	if _, err := sel(context.Background(), inv); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st := SecopsRouteStateFromInvocation(inv); st != (SecopsRouteState{}) {
		t.Fatalf("keep-base 路径必须清零路由状态，got %+v", st)
	}
}
