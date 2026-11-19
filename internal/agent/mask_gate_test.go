package agent

// D6 L-2 脱敏闸钩子测试（验收 #3：secondCheck 命中 fail-closed、引擎不可用
// fail-closed；#4 的 shadow/灰度半边在 biz 引擎测试；本文件覆盖钩子语义）。

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

type fakeMaskGate struct {
	eng      *biz.MaskEngine
	auditErr error
	audits   []string
	details  []map[string]any
}

func (f *fakeMaskGate) Engine(context.Context) *biz.MaskEngine { return f.eng }

func (f *fakeMaskGate) RecordMaskAudit(_ context.Context, action, _ string, detail map[string]any, _ string) error {
	if f.auditErr != nil {
		return f.auditErr
	}
	f.audits = append(f.audits, action)
	f.details = append(f.details, detail)
	return nil
}

// maskTestModel 是 Info() 驱动的模型替身（钩子通道解析用）。
type maskTestModel struct{ name string }

func (m *maskTestModel) GenerateContent(context.Context, *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	return nil, nil
}

func (m *maskTestModel) Info() trpcmodel.Info { return trpcmodel.Info{Name: m.name} }

func maskGateRules() []biz.MaskRule {
	return []biz.MaskRule{
		{ID: "mr_01_idcard", Name: "身份证号", Entity: "身份证号", Pattern: `\b\d{17}[\dXx]\b`, Operator: biz.MaskOperatorPlaceholder, Enabled: true, RolloutPct: 100},
		{ID: "mr_02_phone", Name: "手机号", Entity: "手机号", Pattern: `\b1[3-9]\d{9}\b`, Operator: biz.MaskOperatorPlaceholder, Enabled: true, RolloutPct: 100},
	}
}

func newMaskTestEngineForHook(t *testing.T, rules ...biz.MaskRule) *biz.MaskEngine {
	t.Helper()
	eng, err := biz.NewMaskEngine(rules)
	if err != nil {
		t.Fatalf("NewMaskEngine: %v", err)
	}
	return eng
}

func registerMaskGate(t *testing.T, gate SecopsMaskGate) {
	t.Helper()
	SetDefaultSecopsMaskGate(gate)
	t.Cleanup(func() { SetDefaultSecopsMaskGate(nil) })
}

func maskHookDeps(cc decision.Collector, cat biz.TeamModelCatalog) TRPCBuilderDeps {
	return TRPCBuilderDeps{
		TRPCToolAssemblyDeps: TRPCToolAssemblyDeps{DecisionCollector: cc},
		TRPCModelCatalogDeps: TRPCModelCatalogDeps{ModelCatalog: cat},
		TRPCExtensionDeps:    TRPCExtensionDeps{LG: loggateway.NewNoop()},
	}
}

func runMaskHook(t *testing.T, deps TRPCBuilderDeps, inv *trpcagent.Invocation, msgs ...trpcmodel.Message) (*trpcmodel.BeforeModelResult, error) {
	t.Helper()
	hook := newMaskGateBeforeHook(deps)
	if hook == nil {
		t.Fatal("mask gate hook must be registered while the gate is wired")
	}
	ctx := context.Background()
	if inv != nil {
		ctx = trpcagent.NewInvocationContext(ctx, inv)
	}
	fn := hook.(interface {
		HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
	})
	return fn.HandleBeforeModel(ctx, &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: msgs}})
}

func maskInv(sessionID, modelName string, st SecopsRouteState) *trpcagent.Invocation {
	inv := &trpcagent.Invocation{AgentName: "worker-key"}
	if sessionID != "" {
		inv.Session = &trpcsession.Session{ID: sessionID}
	}
	if modelName != "" {
		inv.Model = &maskTestModel{name: modelName}
	}
	if st != (SecopsRouteState{}) {
		inv.SetState(secopsRouteStateKey, st)
	}
	return inv
}

func TestMaskGateHook_NilGateNotRegistered(t *testing.T) {
	SetDefaultSecopsMaskGate(nil)
	if hook := newMaskGateBeforeHook(maskHookDeps(nil, nil)); hook != nil {
		t.Fatal("gate 未装配时钩子必须不注册（行为回到 V2）")
	}
}

func TestMaskGateHook_ForcedLocalPassesThroughWithAudit(t *testing.T) {
	gate := &fakeMaskGate{eng: newMaskTestEngineForHook(t, maskGateRules()...)}
	registerMaskGate(t, gate)
	cc := &captureDecisionCollector{}
	inv := maskInv("sess-l3", "qwen3-32b", SecopsRouteState{
		Channel: biz.ProviderChannelClassLocal, Forced: true, Ceiling: 3, Threshold: 3,
	})
	msgs := []trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: "我的手机号是13812345678"}}
	if _, err := runMaskHook(t, maskHookDeps(cc, nil), inv, msgs...); err != nil {
		t.Fatalf("forced local 不得报错: %v", err)
	}
	if got := gate.audits; len(got) != 1 || got[0] != biz.SecopsMaskActionRouteLocalForced {
		t.Fatalf("audits = %v, want [route_local_forced]", got)
	}
	if len(cc.recs) != 1 || cc.recs[0].Metadata["trigger_rule"] != decision.TriggerRouteLocalForced {
		t.Fatalf("decision records = %+v, want route_local_forced", cc.recs)
	}
}

func TestMaskGateHook_LocalNotForcedSilent(t *testing.T) {
	gate := &fakeMaskGate{eng: newMaskTestEngineForHook(t, maskGateRules()...)}
	registerMaskGate(t, gate)
	inv := maskInv("sess-1", "qwen3-32b", SecopsRouteState{Channel: biz.ProviderChannelClassLocal})
	if _, err := runMaskHook(t, maskHookDeps(nil, nil), inv, trpcmodel.Message{Role: trpcmodel.RoleUser, Content: "13812345678"}); err != nil {
		t.Fatalf("本地通道不得报错: %v", err)
	}
	if len(gate.audits) != 0 {
		t.Fatalf("本地通道非强制不得产审计，got %v", gate.audits)
	}
}

// 核验 #3 通过态：云通道 + 规则覆盖 → 内容脱敏、cloud_send 审计、不报错。
func TestMaskGateHook_CloudMasksAndSends(t *testing.T) {
	gate := &fakeMaskGate{eng: newMaskTestEngineForHook(t, maskGateRules()...)}
	registerMaskGate(t, gate)
	cc := &captureDecisionCollector{}
	inv := maskInv("sess-c", "gpt-4o", SecopsRouteState{})
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleUser, Content: "联系 13812345678 或 13900001111"},
	}
	if _, err := runMaskHook(t, maskHookDeps(cc, nil), inv, msgs...); err != nil {
		t.Fatalf("脱敏通过不得报错: %v", err)
	}
	if strings.Contains(msgs[0].Content, "13812345678") || strings.Contains(msgs[0].Content, "13900001111") {
		t.Fatalf("原文未脱敏: %q", msgs[0].Content)
	}
	if !strings.Contains(msgs[0].Content, "<手机号_1>") || !strings.Contains(msgs[0].Content, "<手机号_2>") {
		t.Fatalf("占位符缺失: %q", msgs[0].Content)
	}
	if len(gate.audits) != 1 || gate.audits[0] != biz.SecopsMaskActionCloudSend {
		t.Fatalf("audits = %v, want [cloud_send]", gate.audits)
	}
	if len(cc.recs) != 1 || cc.recs[0].Metadata["trigger_rule"] != decision.TriggerCloudSend {
		t.Fatalf("decision records = %+v", cc.recs)
	}
	if got := gate.details[0]["masked_entities"]; got != 2 {
		t.Fatalf("masked_entities = %v, want 2", got)
	}
}

// 核验 #3 fail-closed：未被规则覆盖的 PII 命中 secondCheck → 拒绝上云。
func TestMaskGateHook_RejectUnmaskedPII(t *testing.T) {
	gate := &fakeMaskGate{eng: newMaskTestEngineForHook(t, maskGateRules()...)} // 无邮箱规则
	registerMaskGate(t, gate)
	cc := &captureDecisionCollector{}
	inv := maskInv("sess-c", "gpt-4o", SecopsRouteState{})
	msgs := []trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: "发到 ops@example.com"}}
	_, err := runMaskHook(t, maskHookDeps(cc, nil), inv, msgs...)
	if err == nil {
		t.Fatal("secondCheck 命中必须 fail-closed 报错（禁止上云）")
	}
	if strings.Contains(err.Error(), "ops@example.com") {
		t.Fatalf("错误信息不得携带 PII 原文: %v", err)
	}
	if len(gate.audits) != 1 || gate.audits[0] != biz.SecopsMaskActionReject {
		t.Fatalf("audits = %v, want [mask_reject]", gate.audits)
	}
	// 审计 detail 只允许计数/类别，绝不携带 findings 原文。
	detail := gate.details[0]
	if _, ok := detail["kinds"]; !ok {
		t.Fatalf("detail 缺 kinds: %v", detail)
	}
	for _, v := range detail {
		if s, ok := v.(string); ok && strings.Contains(s, "example.com") {
			t.Fatalf("审计 detail 泄露 PII: %v", detail)
		}
	}
	for _, v := range detail["kinds"].([]string) {
		if strings.Contains(v, "@") {
			t.Fatalf("kinds 不得含原文: %v", detail["kinds"])
		}
	}
	if len(cc.recs) != 1 || cc.recs[0].Outcome != "blocked" {
		t.Fatalf("decision record = %+v, want blocked", cc.recs)
	}
}

// 核验 #3 fail-closed：引擎不可用 → 拒绝上云。
func TestMaskGateHook_EngineUnavailableFailsClosed(t *testing.T) {
	gate := &fakeMaskGate{eng: nil}
	registerMaskGate(t, gate)
	inv := maskInv("sess-c", "gpt-4o", SecopsRouteState{})
	_, err := runMaskHook(t, maskHookDeps(nil, nil), inv, trpcmodel.Message{Role: trpcmodel.RoleUser, Content: "hello"})
	if err == nil {
		t.Fatal("引擎不可用必须 fail-closed 报错")
	}
	if len(gate.audits) != 1 || gate.audits[0] != biz.SecopsMaskActionEngineUnavailable {
		t.Fatalf("audits = %v, want [engine_unavailable]", gate.audits)
	}
}

// wb F3：审计失败即拒（即使脱敏本身通过）。
func TestMaskGateHook_AuditFailureBlocksSend(t *testing.T) {
	gate := &fakeMaskGate{
		eng:      newMaskTestEngineForHook(t, maskGateRules()...),
		auditErr: errors.New("audit down"),
	}
	registerMaskGate(t, gate)
	inv := maskInv("sess-c", "gpt-4o", SecopsRouteState{})
	_, err := runMaskHook(t, maskHookDeps(nil, nil), inv, trpcmodel.Message{Role: trpcmodel.RoleUser, Content: "联系 13812345678"})
	if err == nil || !strings.Contains(err.Error(), "audit persist failed") {
		t.Fatalf("审计失败必须拦截，err = %v", err)
	}
}

// 未裁决通道：按模型目录解析为 local → 放行不脱敏。
func TestMaskGateHook_ResolvesLocalViaCatalog(t *testing.T) {
	gate := &fakeMaskGate{eng: newMaskTestEngineForHook(t)}
	registerMaskGate(t, gate)
	cat := classificationTestCatalog() // ollama/qwen3-32b = local, openai/gpt-4o = cloud
	inv := maskInv("sess-1", "qwen3-32b", SecopsRouteState{})
	msgs := []trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: "13812345678"}}
	if _, err := runMaskHook(t, maskHookDeps(nil, cat), inv, msgs...); err != nil {
		t.Fatalf("目录解析为本地不得报错: %v", err)
	}
	if msgs[0].Content != "13812345678" {
		t.Fatalf("本地通道不得改写内容: %q", msgs[0].Content)
	}
	if len(gate.audits) != 0 {
		t.Fatalf("本地通道不得产审计: %v", gate.audits)
	}
}

// 未裁决通道：目录解析为 cloud → 走脱敏闸；目录里没有的模型 → 保守按云。
func TestMaskGateHook_ResolvesCloudViaCatalogAndUnknown(t *testing.T) {
	gate := &fakeMaskGate{eng: newMaskTestEngineForHook(t, maskGateRules()...)}
	registerMaskGate(t, gate)
	cat := classificationTestCatalog()
	for _, name := range []string{"gpt-4o", "not-in-catalog"} {
		inv := maskInv("sess-"+name, name, SecopsRouteState{})
		msgs := []trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: "联系 13812345678"}}
		if _, err := runMaskHook(t, maskHookDeps(nil, cat), inv, msgs...); err != nil {
			t.Fatalf("%s: 云通道脱敏通过不得报错: %v", name, err)
		}
		if !strings.Contains(msgs[0].Content, "<手机号_1>") {
			t.Fatalf("%s: 云/未知通道必须脱敏，got %q", name, msgs[0].Content)
		}
	}
}

// 多模态：文本 part 脱敏，非文本 part 原样保留（F6 面外，W4 登记）。
func TestMaskGateHook_ContentPartsTextMaskedImagesKept(t *testing.T) {
	gate := &fakeMaskGate{eng: newMaskTestEngineForHook(t, maskGateRules()...)}
	registerMaskGate(t, gate)
	inv := maskInv("sess-p", "gpt-4o", SecopsRouteState{})
	text := "证件 13812345678"
	img := &trpcmodel.Image{URL: "https://example.com/a.png"}
	msgs := []trpcmodel.Message{{
		Role: trpcmodel.RoleUser,
		ContentParts: []trpcmodel.ContentPart{
			{Type: trpcmodel.ContentTypeText, Text: &text},
			{Type: trpcmodel.ContentTypeImage, Image: img},
		},
	}}
	if _, err := runMaskHook(t, maskHookDeps(nil, nil), inv, msgs...); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	parts := msgs[0].ContentParts
	if parts[0].Text == nil || strings.Contains(*parts[0].Text, "13812345678") || !strings.Contains(*parts[0].Text, "<手机号_1>") {
		t.Fatalf("文本 part 未脱敏: %v", parts[0].Text)
	}
	if parts[1].Image == nil || parts[1].Image.URL != "https://example.com/a.png" {
		t.Fatal("非文本 part 不得被改写")
	}
}
