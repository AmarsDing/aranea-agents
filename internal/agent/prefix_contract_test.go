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
// 断言：
//   - C1：3 轮 head 区字节完全相等；
//   - C2：前轮 tail 消息序列是后轮 tail 的前缀（消息集合只 append）；
//   - F-A4：每轮链后不存在任何 intent system 消息滞留 head/conv 区
//     （intentReorder 必须搬家到 tail 且转 dynamic cue）。
func TestPrefixContract_HeadStableTailAppendOnly(t *testing.T) {
	t.Skip("Phase 0 脚手架：待「head 区逐轮 diff 缺口清单」产出后启用（79-runtime-governance.development.md 任务 0.4）")

	ag := contractTestAgent()
	chain := productCallbackChain(context.Background(), ag, TRPCBuilderDeps{}, nil)
	if chain == nil {
		t.Fatal("chain must build with zero deps")
	}
	instruction := bakedInstruction(ag)

	var history []trpcmodel.Message
	var prevHead, prevTail string
	for turn := 1; turn <= 3; turn++ {
		msgs := runBeforeModelChain(t, chain, simulateTurn(instruction, history, turn))
		head, _, tail := splitPromptZones(msgs)

		// F-A4：intent 不得滞留 head/conv——它必须是 tail 的 dynamic cue。
		for _, m := range head {
			if intent.IsIntentContextContent(m.Content) {
				t.Fatalf("turn %d: intent context leaked into head zone", turn)
			}
		}
		intentInTail := false
		for _, m := range tail {
			if intent.IsIntentContextContent(m.Content) {
				intentInTail = true
				if !isDynamicCueMessage(m) {
					t.Fatalf("turn %d: intent in tail but not converted to dynamic cue", turn)
				}
			}
		}
		if !intentInTail {
			t.Fatalf("turn %d: intent context missing from tail (reorder hook dropped it?)", turn)
		}

		headDigest, tailDigest := zoneDigest(head), zoneDigest(tail)
		if turn > 1 {
			if headDigest != prevHead {
				t.Fatalf("turn %d: C1 violated — head zone bytes changed across turns\n--- prev ---\n%s\n--- curr ---\n%s", turn, prevHead, headDigest)
			}
			if !strings.HasPrefix(tailDigest, prevTail) {
				t.Fatalf("turn %d: C2 violated — tail zone is not append-only\n--- prev tail ---\n%s\n--- curr tail ---\n%s", turn, prevTail, tailDigest)
			}
		}
		prevHead, prevTail = headDigest, tailDigest

		// 会话推进：本轮 user + assistant 应答入历史。
		history = append(history,
			trpcmodel.NewUserMessage(fmt.Sprintf("user turn %d", turn)),
			trpcmodel.NewAssistantMessage(fmt.Sprintf("assistant reply %d", turn)),
		)
	}
}

// TestPrefixContract_HeadZoneGapInventory 产出「当前 head 区逐轮 diff 缺口清单」
// （任务 0.4 第二交付物）。不断言失败——逐轮 dump 分区摘要供人工核对，
// 输出即缺口清单素材。启用主测试前，本清单中的非期望项必须全部消解或
// 在设计文档附录 A 登记为豁免。
func TestPrefixContract_HeadZoneGapInventory(t *testing.T) {
	t.Skip("Phase 0 脚手架：与主测试同步启用")

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
