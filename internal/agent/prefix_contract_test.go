package agent

// prefix_contract_test.go — R1 Cache-First 装配契约测试（79-runtime-governance.design.md §2.3）。
//
// 契约三条（§2.1）：
//   C1 head 稳定：head/system 区 3 轮字节完全相等；
//   C2 tail 只增：动态内容一律 append 式 tail cue，禁止 insert 到历史消息之前
//      （断言放宽为"消息集合只 append"——前轮 tail 消息序列是后轮 tail 的前缀）；
//   C3 顺序固化：callback_chain.go 注册序即契约（附录 A.1 #1-#22 golden）。
//
// 模拟框架 content processor 行为（附录 A.2 / F-A4）：测试自行构造 system
// block（instruction 已烘焙 static cue，与生产 WithInstruction 一致），并把
// per-turn intent JSON 插在 system block 之后——契约要求 intentReorder（#22）
// 将其搬家到 tail 并转 user-role dynamic cue。
//
// Phase 0 任务 0.4：本文件为脚手架，断言先 skip；启用前需产出
// 「当前 head 区逐轮 diff 缺口清单」（见 TestPrefixContract_HeadZoneGapInventory）。

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// contractTestAgent 返回一个依赖最小化的固定 agent：
//   - SystemPromptMode=complete（触发 static/dynamic runtime cue 注册）；
//   - 工具/子代理/记忆/压缩/快照全关——零 DB 依赖即可跑通装配链，
//     head 区仅由 instruction（ baked static cue ）构成，纯净可断言。
func contractTestAgent() biz.Agent {
	return biz.Agent{
		ID:               "contract-agent",
		AgentKey:         "contract_agent",
		DisplayName:      "Contract Test Agent",
		SystemPromptMode: "complete",
		Settings: &biz.AgentRuntimeSettings{
			AgentID:            "contract-agent",
			L0SnapshotMode:     "off", // 与 ContextCompactionEnabled=false 一起关掉压缩 hook
			ReplyReminderEnabled: false,
		},
	}
}

// bakedInstruction 模拟生产 WithInstruction 的产物：base system + static cue
// 已烘焙为单条 system 消息（staticRuntimeCueAlreadyPresent 幂等跳过的前提）。
func bakedInstruction(ag biz.Agent) string {
	cue := StaticRuntimeCapabilityCue(context.Background(), Deps{}, ag)
	return "You are Contract Test Agent.\n\n" + cue
}

// intentContextMsg 模拟框架 content processor 每轮注入的 intent JSON 消息
// （system-role，插在 system block 之后、会话历史之前——见附录 A.2）。
func intentContextMsg(turn int) trpcmodel.Message {
	return trpcmodel.NewSystemMessage(fmt.Sprintf(
		"Derived intent (align your plan and tools to this JSON):\n{\"refined_goal\":\"turn-%d-goal\"}", turn))
}

// runBeforeModelChain 按链序执行全部 BeforeModel hook（顺序 = NewChain 排序后
// 的 entries 序，即附录 A.1 golden 序），返回最终请求消息。
func runBeforeModelChain(t *testing.T, chain *callbacks.Chain, msgs []trpcmodel.Message) []trpcmodel.Message {
	t.Helper()
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: msgs}}
	ctx := context.Background()
	for _, cb := range chain.Entries() {
		h, ok := cb.(callbacks.BeforeModelHook)
		if !ok {
			continue
		}
		res, err := h.HandleBeforeModel(ctx, args)
		if err != nil {
			t.Fatalf("before-model hook failed: %v", err)
		}
		if res != nil && res.Context != nil {
			ctx = res.Context
		}
	}
	return args.Request.Messages
}

// zoneDigest 把一段消息序列序列化为字节视图（role + 哨兵 + 内容），
// 用于跨轮 byte-diff。动态 cue 以 ToolName 哨兵区分于普通 user 消息。
func zoneDigest(msgs []trpcmodel.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		fmt.Fprintf(&b, "%s\x00%s\x00%s\x00%d\n", m.Role, m.ToolName, m.Content, len(m.ContentParts))
	}
	return b.String()
}

// simulateTurn 构造第 turn 轮（1 起）的链前请求：
// system block（baked）→ intent context（框架注入点）→ 会话历史 → 本轮 user。
func simulateTurn(instruction string, history []trpcmodel.Message, turn int) []trpcmodel.Message {
	msgs := []trpcmodel.Message{
		trpcmodel.NewSystemMessage(instruction),
		intentContextMsg(turn),
	}
	msgs = append(msgs, history...)
	msgs = append(msgs, trpcmodel.NewUserMessage(fmt.Sprintf("user turn %d", turn)))
	return msgs
}

// TestPrefixContract_HeadStableTailAppendOnly 是 R1 契约主测试。
//
// 场景（§2.3）：固定 agent + 固定 3 轮会话；第 2 轮触发一次记忆召回变化、
// 一次 intent 变化（本脚手架下记忆依赖全关，以 intent 逐轮变化 + 历史增长
// 覆盖动态面；记忆召回变化在缺口清单中列为待 mock 项）。
//
// 0.4 实证修正（2026-08-25）：tail 区在框架"每轮重建请求"模型下不跨轮累积——
// 动态 cue（intent/memory recall）是 per-turn 新鲜注入的瞬态内容，不进会话
// 历史。因此 §2.3 字面断言"前轮 tail 为后轮前缀"无可断言对象；C2 的可执行
// 不变量修正为：
//   - C2a conv 只增：前轮 conv 消息序列是后轮 conv 的前缀（历史 append-only，
//     禁止 insert 到历史消息之前）；
//   - C2b 动态零滞留：head/conv 区内不出现任何 dynamic cue 或 intent 内容。
//
// 断言：
//   - C1：3 轮 head 区字节完全相等；
//   - C2a/C2b：如上；
//   - F-A4：每轮链后 intent 必须是 tail 区的 user-role dynamic cue。
func TestPrefixContract_HeadStableTailAppendOnly(t *testing.T) {
	ag := contractTestAgent()
	chain := productCallbackChain(context.Background(), ag, TRPCBuilderDeps{}, nil)
	if chain == nil {
		t.Fatal("chain must build with zero deps")
	}
	instruction := bakedInstruction(ag)

	var history []trpcmodel.Message
	var prevHead, prevConv string
	for turn := 1; turn <= 3; turn++ {
		msgs := runBeforeModelChain(t, chain, simulateTurn(instruction, history, turn))
		head, conv, tail := splitPromptZones(msgs)

		// C2b + F-A4：head/conv 区内禁止任何动态内容滞留——intent 与
		// dynamic cue 必须全部落在 tail。
		for _, m := range head {
			if intent.IsIntentContextContent(m.Content) {
				t.Fatalf("turn %d: C2b violated — intent context leaked into head zone", turn)
			}
			if isDynamicCueMessage(m) {
				t.Fatalf("turn %d: C2b violated — dynamic cue leaked into head zone", turn)
			}
		}
		for _, m := range conv {
			if intent.IsIntentContextContent(m.Content) {
				t.Fatalf("turn %d: C2b violated — intent context leaked into conv zone", turn)
			}
			if isDynamicCueMessage(m) {
				t.Fatalf("turn %d: C2b violated — dynamic cue leaked into conv zone", turn)
			}
		}
		intentInTail := false
		for _, m := range tail {
			if intent.IsIntentContextContent(m.Content) {
				intentInTail = true
				if !isDynamicCueMessage(m) {
					t.Fatalf("turn %d: F-A4 violated — intent in tail but not converted to dynamic cue", turn)
				}
			}
		}
		if !intentInTail {
			t.Fatalf("turn %d: F-A4 violated — intent context missing from tail (reorder hook dropped it?)", turn)
		}

		headDigest, convDigest := zoneDigest(head), zoneDigest(conv)
		if turn > 1 {
			if headDigest != prevHead {
				t.Fatalf("turn %d: C1 violated — head zone bytes changed across turns\n--- prev ---\n%s\n--- curr ---\n%s", turn, prevHead, headDigest)
			}
			if !strings.HasPrefix(convDigest, prevConv) {
				t.Fatalf("turn %d: C2a violated — conv zone is not append-only\n--- prev conv ---\n%s\n--- curr conv ---\n%s", turn, prevConv, convDigest)
			}
		}
		prevHead, prevConv = headDigest, convDigest

		// 会话推进：本轮 user + assistant 应答入历史。
		history = append(history,
			trpcmodel.NewUserMessage(fmt.Sprintf("user turn %d", turn)),
			trpcmodel.NewAssistantMessage(fmt.Sprintf("assistant reply %d", turn)),
		)
	}
}

// hookFingerprint 是 C3 执行序 golden 的断言单元：(派生名, Layer, Priority)。
// 派生名来自 BeforeModelHookFunc.Name()（handler 闭包的外层函数名），
// 与附录 A.1 主表的 hook 行一一对应。
type hookFingerprint struct {
	name     string
	layer    callbacks.SystemLayer
	priority int
}

// beforeModelFingerprints 按链执行序提取全部 BeforeModel hook 的指纹。
func beforeModelFingerprints(chain *callbacks.Chain) []hookFingerprint {
	var out []hookFingerprint
	for _, cb := range chain.Entries() {
		if _, ok := cb.(callbacks.BeforeModelHook); !ok {
			continue
		}
		fp := hookFingerprint{layer: callbacks.LayerDynamic, priority: cb.Priority()}
		if lc, ok := cb.(callbacks.LayeredCallback); ok {
			fp.layer = lc.Layer()
		}
		if n, ok := cb.(interface{ Name() string }); ok {
			fp.name = n.Name()
		}
		out = append(out, fp)
	}
	return out
}

func layerName(l callbacks.SystemLayer) string {
	switch l {
	case callbacks.LayerStatic:
		return "Static"
	case callbacks.LayerSemiStatic:
		return "SemiStatic"
	default:
		return "Dynamic"
	}
}

// TestPrefixContract_HookExecutionOrderGolden 是 C3（顺序固化）断言：
// 零依赖装配下 BeforeModel hook 的执行序必须与 golden 完全一致——
// golden 即附录 A.1「代码实序」在零依赖注册子集上的投影（全量 22 hook 的
// mock 依赖扩展属 A.5 G-0.4-2 遗留）。
//
// golden 覆盖三类回归：
//   - Layer/Priority 变更（指纹立即失配）；
//   - 注册条件变更（hook 集合增删失配）；
//   - 同 (Layer,Priority) 组内注册序交换（派生名序失配，如 Dynamic-0 计量组）。
//
// 变更 callback_chain.go 注册链或任何 hook 的 layer/priority 时，必须同步
// 更新本 golden 与设计文档附录 A.1（C3 契约）。
func TestPrefixContract_HookExecutionOrderGolden(t *testing.T) {
	ag := contractTestAgent()
	chain := productCallbackChain(context.Background(), ag, TRPCBuilderDeps{}, nil)
	if chain == nil {
		t.Fatal("chain must build with zero deps")
	}

	golden := []hookFingerprint{
		// A.1 #1：head 唯一产品写入点；F-A8——LayerStatic 优先于 Dynamic prio 0。
		{"agent.newStaticRuntimeCueBeforeHook.func1", callbacks.LayerStatic, 4},
		// A.1 #2：落 tail，SemiStatic 仅控执行序（F-A5 已澄清）。
		{"agent.newDynamicRuntimeCueBeforeHook.func1", callbacks.LayerSemiStatic, 4},
		// A.1 #4-#7：Dynamic-0 计量组，组内保注册序（lifecycleMetrics 最早注册）。
		{"agent.productChainLifecycleMetrics.func3", callbacks.LayerDynamic, 0},
		{"agent.newContextBudgetToolsBeforeHook.func1", callbacks.LayerDynamic, 0},
		{"agent.newContextBudgetHistoryBeforeHook.func1", callbacks.LayerDynamic, 0},
		{"agent.newContextBudgetStaticPrefixBeforeHook.func1", callbacks.LayerDynamic, 0},
		// A.1 #10：L0 尺寸闸——注册由 Settings 门控（默认开），nil gate 在 hook 内判空。
		{"agent.newToolResultGateBeforeHook.func1", callbacks.LayerDynamic, 3},
		// A.1 #10a：N2 硬约束（session-eval-20260829-r2）——next_action 待执行
		// 指令的强制 cue；无条件注册（无 pending 时一次 state 读早退）。
		{"agent.newNextActionCueBeforeHook.func1", callbacks.LayerDynamic, 3},
		// A.1 #13：GenerationConfig 标记驱动，无消息改写。
		{"agent.newVoiceFastPathBeforeHook.func1", callbacks.LayerDynamic, 4},
		// A.1 #17：session phase brief（Dynamic-5 组零依赖下唯一注册者）。
		{"agent.newOrchestrationBriefBeforeHook.func1", callbacks.LayerDynamic, 5},
		// Wave 2：连续失败工具结果去重（priority 7，始终注册）。
		{"agent.newFailedToolResultDedupBeforeHook.func1", callbacks.LayerDynamic, 7},
		// Wave 2：named agent 默认装配闸 64K/96K（priority 8）。
		{"agent.newAssemblyBudgetBeforeHook.func1", callbacks.LayerDynamic, 8},
		// A.1 #21：快照 hook 无条件注册，持久化由 env/L0 状态内部判空。
		{"agent.newPromptSnapshotBeforeHook.func1", callbacks.LayerDynamic, 10},
		// A.1 #22：框架 intent 注入纠正点，恒最后执行。
		{"agent.newIntentReorderBeforeHook.func1", callbacks.LayerDynamic, 100},
	}

	actual := beforeModelFingerprints(chain)
	if len(actual) != len(golden) {
		var b strings.Builder
		for i, fp := range actual {
			fmt.Fprintf(&b, "  actual[%d] %s layer=%s prio=%d\n", i, fp.name, layerName(fp.layer), fp.priority)
		}
		t.Fatalf("C3 violated — before-model hook count = %d, want %d\n%s", len(actual), len(golden), b.String())
	}
	for i, want := range golden {
		got := actual[i]
		if got != want {
			t.Fatalf("C3 violated — hook[%d] = %s (layer=%s prio=%d), want %s (layer=%s prio=%d)",
				i, got.name, layerName(got.layer), got.priority,
				want.name, layerName(want.layer), want.priority)
		}
	}
}

// TestPrefixContract_HeadZoneGapInventory 产出「当前 head 区逐轮 diff 缺口清单」
// （任务 0.4 第二交付物）。不断言失败——逐轮 dump 分区摘要供人工核对，
// 输出即缺口清单素材。清单正式版落设计文档附录 A.5；新增 hook 或分区边界
// 变更时重跑本测试比对摘要。
func TestPrefixContract_HeadZoneGapInventory(t *testing.T) {
	ag := contractTestAgent()
	chain := productCallbackChain(context.Background(), ag, TRPCBuilderDeps{}, nil)
	if chain == nil {
		t.Fatal("chain must build with zero deps")
	}
	instruction := bakedInstruction(ag)

	var history []trpcmodel.Message
	for turn := 1; turn <= 3; turn++ {
		msgs := runBeforeModelChain(t, chain, simulateTurn(instruction, history, turn))
		head, conv, tail := splitPromptZones(msgs)
		t.Logf("turn %d: head=%d msgs / conv=%d msgs / tail=%d msgs", turn, len(head), len(conv), len(tail))
		for i, m := range head {
			t.Logf("  head[%d] role=%s sentinel=%q chars=%d", i, m.Role, m.ToolName, messageCharLen(m))
		}
		for i, m := range tail {
			t.Logf("  tail[%d] role=%s sentinel=%q chars=%d", i, m.Role, m.ToolName, messageCharLen(m))
		}
		history = append(history,
			trpcmodel.NewUserMessage(fmt.Sprintf("user turn %d", turn)),
			trpcmodel.NewAssistantMessage(fmt.Sprintf("assistant reply %d", turn)),
		)
	}
}
