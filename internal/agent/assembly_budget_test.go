package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// 包A（session-eval-20260825 A1）装配预算硬闸单测。
// 估算口径：默认 2.5 chars/token（llmcontext.EstimateTokensFromChars），
// fixture 均按 chars = tokens×2.5 标定，与 context_compression_inject_test.go 同法。

func runAssemblyBudgetHook(t *testing.T, hook interface{}, ctx context.Context, args *trpcmodel.BeforeModelArgs) {
	t.Helper()
	hookFn := hook.(interface {
		HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
	})
	if _, err := hookFn.HandleBeforeModel(ctx, args); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func assemblyTestAgent(soft, hard int) biz.Agent {
	return biz.Agent{
		AgentKey: "test_agent",
		Settings: &biz.AgentRuntimeSettings{
			AssemblyBudgetSoftTokens: soft,
			AssemblyBudgetHardTokens: hard,
		},
	}
}

func findMarker(msgs []trpcmodel.Message, marker string) bool {
	for _, m := range msgs {
		if strings.HasPrefix(m.Content, marker) {
			return true
		}
	}
	return false
}

func countMarker(msgs []trpcmodel.Message, marker string) int {
	n := 0
	for _, m := range msgs {
		if strings.HasPrefix(m.Content, marker) {
			n++
		}
	}
	return n
}

// TestAssemblyBudgetHook_NilWhenDisabled pins the zero-cost specialist path:
// hard<=0（含 settings 为 nil）时闸不注册。Spirit 在 hard=0 时走 40K/60K 默认（见 TestResolveAssemblyBudget）。
func TestAssemblyBudgetHook_NilWhenDisabled(t *testing.T) {
	if hook := newAssemblyBudgetBeforeHook(biz.Agent{}, TRPCBuilderDeps{}); hook != nil {
		t.Fatalf("settings=nil must yield nil hook")
	}
	if hook := newAssemblyBudgetBeforeHook(assemblyTestAgent(0, 0), TRPCBuilderDeps{}); hook != nil {
		t.Fatalf("hard=0 specialist must yield nil hook")
	}
}

func TestAssemblyBudgetHook_SpiritDefaultsWhenHardZero(t *testing.T) {
	hook := newAssemblyBudgetBeforeHook(biz.Agent{AgentKey: biz.SpiritAgentKey}, TRPCBuilderDeps{})
	if hook == nil {
		t.Fatal("Spirit hard=0 must install 40K/60K assembly gate")
	}
}

// TestAssemblyBudgetHook_Priority8 pins registration order: after every
// injection hook (memory 5 / knowledge 6 / cues ≤6), before the final
// compression gate (9) and the L0 snapshot (10).
func TestAssemblyBudgetHook_Priority8(t *testing.T) {
	hook := newAssemblyBudgetBeforeHook(assemblyTestAgent(40000, 60000), TRPCBuilderDeps{})
	if hook == nil {
		t.Fatalf("expected non-nil hook")
	}
	if got := hook.(interface{ Priority() int }).Priority(); got != assemblyBudgetHookPriority {
		t.Fatalf("装配预算闸必须挂在 priority %d，got %d", assemblyBudgetHookPriority, got)
	}
}

// TestAssemblyBudgetHook_BelowSoftPassthrough verifies est ≤ soft leaves the
// request untouched（无告警、无截断）。
func TestAssemblyBudgetHook_BelowSoftPassthrough(t *testing.T) {
	hook := newAssemblyBudgetBeforeHook(assemblyTestAgent(600, 1000), TRPCBuilderDeps{})
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: "hello"},
	}
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: msgs}}
	runAssemblyBudgetHook(t, hook, context.Background(), args)
	if len(args.Request.Messages) != 2 {
		t.Fatalf("below soft must pass through unchanged, got %d messages", len(args.Request.Messages))
	}
}

// TestAssemblyBudgetHook_SoftDefaultsToTwoThirds verifies soft=0 时默认
// 2/3 hard：hard=300 → soft=200，250 tok 请求越 soft 但未达 hard → 只告警不截断。
func TestAssemblyBudgetHook_SoftDefaultsToTwoThirds(t *testing.T) {
	hook := newAssemblyBudgetBeforeHook(assemblyTestAgent(0, 300), TRPCBuilderDeps{})
	inv := trpcagent.NewInvocation()
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: strings.Repeat("u", 600)}, // 240 tok → 越 200 soft、未及 300 hard
	}
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: msgs}}
	runAssemblyBudgetHook(t, hook, ctx, args)
	if !findMarker(args.Request.Messages, assemblyBudgetWarnMarker) {
		t.Fatalf("soft（默认 2/3 hard）越线必须注容量告警，messages: %v", args.Request.Messages)
	}
	// 未达 hard：原有消息一条不少（仅追加告警 cue）。
	if len(args.Request.Messages) != 3 {
		t.Fatalf("soft 档不得截断，want 3 messages, got %d", len(args.Request.Messages))
	}
}

// TestAssemblyBudgetHook_SoftWarnsOncePerTurn verifies the MemGPT 容量告警
// 每个 turn 只注一次（工具循环续轮以同一 invocation 重进 hook 不得重复加注）；
// 无 invocation 的 ctx 不注（无去重载体，防每轮重注）。
func TestAssemblyBudgetHook_SoftWarnsOncePerTurn(t *testing.T) {
	hook := newAssemblyBudgetBeforeHook(assemblyTestAgent(200, 10000), TRPCBuilderDeps{})
	inv := trpcagent.NewInvocation()
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: strings.Repeat("u", 600)}, // 240 tok > 200 soft
	}

	args1 := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: append([]trpcmodel.Message(nil), msgs...)}}
	runAssemblyBudgetHook(t, hook, ctx, args1)
	if got := countMarker(args1.Request.Messages, assemblyBudgetWarnMarker); got != 1 {
		t.Fatalf("首轮须注 1 条容量告警，got %d", got)
	}

	// 续轮（同 invocation）：告警不得重注。
	args2 := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: append([]trpcmodel.Message(nil), msgs...)}}
	runAssemblyBudgetHook(t, hook, ctx, args2)
	if got := countMarker(args2.Request.Messages, assemblyBudgetWarnMarker); got != 0 {
		t.Fatalf("同 turn 续轮不得重注告警，got %d", got)
	}

	// 无 invocation ctx：不注。
	args3 := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: append([]trpcmodel.Message(nil), msgs...)}}
	runAssemblyBudgetHook(t, hook, context.Background(), args3)
	if got := countMarker(args3.Request.Messages, assemblyBudgetWarnMarker); got != 0 {
		t.Fatalf("无 invocation 不得注告警（无去重载体），got %d", got)
	}
}

// TestAssemblyBudgetHook_HardDropsCuesByProtectionOrder 是闸的核心回归：
// hard 越线时按保护序丢尾部 cue——reply reminder 与工具目录先于 knowledge
// 与记忆牺牲；未标记 cue（protected）与静态头骨架永保。
//
// Fixture（2.5 chars/token）：head 100c=40t + conv 20c=8t + protected 100c=40t
// + reply 1250c=500t + catalog 1250c=500t + knowledge 1250c=500t + memory 625c=250t
// ≈ 1838 tok；hard=1000、target=900 → 丢 reply(500)→1338、丢 catalog(500)→838 ≤ 900 停。
func TestAssemblyBudgetHook_HardDropsCuesByProtectionOrder(t *testing.T) {
	hook := newAssemblyBudgetBeforeHook(assemblyTestAgent(600, 1000), TRPCBuilderDeps{})
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: strings.Repeat("s", 100)},
		{Role: trpcmodel.RoleUser, Content: "hi"},
		{Role: trpcmodel.RoleAssistant, Content: "ok"},
		asDynamicCue("unmarked protected cue " + strings.Repeat("p", 100)),
		asDynamicCue(replyReminderCueMarker + strings.Repeat("r", 1250)),
		asDynamicCue(toolCatalogCueMarker + strings.Repeat("t", 1250)),
		asDynamicCue(knowledgeCueMarker + strings.Repeat("k", 1250)),
		asDynamicCue(memoryInjectMarker + strings.Repeat("m", 625)),
	}
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: msgs}}
	runAssemblyBudgetHook(t, hook, context.Background(), args)

	out := args.Request.Messages
	if findMarker(out, replyReminderCueMarker) {
		t.Fatalf("reply reminder（rank 最高）必须最先被丢")
	}
	if findMarker(out, toolCatalogCueMarker) {
		t.Fatalf("工具目录必须先于 knowledge/memory 被丢")
	}
	if !findMarker(out, knowledgeCueMarker) {
		t.Fatalf("knowledge（question 检索，高相关）必须保留")
	}
	if !findMarker(out, memoryInjectMarker) {
		t.Fatalf("记忆 cue 必须保留（保底序最后）")
	}
	protected := false
	for _, m := range out {
		if strings.HasPrefix(m.Content, "unmarked protected cue") {
			protected = true
		}
	}
	if !protected {
		t.Fatalf("未标记 cue 属 protected，宁保勿丢")
	}
	if out[0].Role != trpcmodel.RoleSystem || !strings.HasPrefix(out[0].Content, "sss") {
		t.Fatalf("静态头骨架永保，got role=%s", out[0].Role)
	}
	// 本 fixture 丢两段 cue 后已达标，不得再逐历史。
	for _, m := range out {
		if strings.Contains(m.Content, "<context_truncated>") {
			t.Fatalf("cue 降级已达标，不得触发历史驱逐")
		}
	}
}

// TestAssemblyBudgetHook_HardEvictsHistoryWhenCuesInsufficient：丢光可丢
// cue 仍超 target 时，驱逐最旧历史（截断标记落点=静态头之后），头骨架不动。
//
// Fixture：head 100c=40t + 10 条 conv ×500c=200t 共 2000t + knowledge 250c=100t
// ≈ 2140 tok；hard=1000、target=900 → 丢 knowledge(100)→2040 仍超 → 逐历史。
func TestAssemblyBudgetHook_HardEvictsHistoryWhenCuesInsufficient(t *testing.T) {
	hook := newAssemblyBudgetBeforeHook(assemblyTestAgent(600, 1000), TRPCBuilderDeps{})
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: strings.Repeat("s", 100)},
	}
	for i := 0; i < 5; i++ {
		msgs = append(msgs,
			trpcmodel.Message{Role: trpcmodel.RoleUser, Content: strings.Repeat("u", 500)},
			trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: strings.Repeat("a", 500)},
		)
	}
	msgs = append(msgs, asDynamicCue(knowledgeCueMarker+strings.Repeat("k", 250)))
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: msgs}}
	runAssemblyBudgetHook(t, hook, context.Background(), args)

	out := args.Request.Messages
	if findMarker(out, knowledgeCueMarker) {
		t.Fatalf("knowledge cue 应先被丢")
	}
	hasTruncMarker := false
	for _, m := range out {
		if strings.Contains(m.Content, "<context_truncated>") {
			hasTruncMarker = true
		}
	}
	if !hasTruncMarker {
		t.Fatalf("cue 丢光仍超 target，必须驱逐最旧历史并落截断标记")
	}
	if out[0].Role != trpcmodel.RoleSystem || !strings.HasPrefix(out[0].Content, "sss") {
		t.Fatalf("历史驱逐不得触碰静态头骨架")
	}
	estAfter := analyzePromptRequest(out).EstTokens
	if estAfter > 1000 {
		t.Fatalf("截断后 est=%d 不得仍超 hard=1000（滞回 target=900）", estAfter)
	}
}

// TestAssemblyBudgetHook_HeadOverBudgetPassesThrough：静态头自身超预算且
// 无 cue 可丢、无历史可逐时放行不阻断（K3：绝不阻断模型调用），仅台账曝光。
func TestAssemblyBudgetHook_HeadOverBudgetPassesThrough(t *testing.T) {
	hook := newAssemblyBudgetBeforeHook(assemblyTestAgent(100, 200), TRPCBuilderDeps{})
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: strings.Repeat("s", 2000)}, // 头 800 tok ≫ target
		{Role: trpcmodel.RoleUser, Content: "hi"},
	}
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: msgs}}
	runAssemblyBudgetHook(t, hook, context.Background(), args)
	if args.Request.Messages[0].Role != trpcmodel.RoleSystem || len(args.Request.Messages[0].Content) != 2000 {
		t.Fatalf("头自身超预算时 Warn 放行，静态头不得被截")
	}
}

// TestClassifyAssemblyCue pins the marker→kind mapping（闸的分段分类表）。
func TestClassifyAssemblyCue(t *testing.T) {
	cases := []struct {
		content string
		want    assemblyCueKind
	}{
		{replyReminderCueMarker + "x", cueKindReplyReminder},
		{toolCatalogCueMarker + "x", cueKindToolCatalog},
		{orchBriefCueMarker + "x", cueKindOrchBrief},
		{deferredSummaryCueMarker + "x", cueKindOrchBrief},
		{workspaceSkillsCueMarker + "x", cueKindWorkspaceSkills},
		{skillGuidanceCueMarker + "x", cueKindSkillGuidance},
		{knowledgeCueMarker + "x", cueKindKnowledge},
		{memoryInjectMarker + "x", cueKindMemory},
		{assemblyBudgetWarnMarker + "x", cueKindProtected},
		{"plain cue without marker", cueKindProtected},
	}
	for _, tc := range cases {
		if got := classifyAssemblyCue(trpcmodel.Message{Content: tc.content}); got != tc.want {
			t.Errorf("classifyAssemblyCue(%q) = %s, want %s", tc.content[:30], got, tc.want)
		}
	}
}

func TestAssemblyBudgetHook_ToolsSchemaCountsTowardSoft(t *testing.T) {
	hook := newAssemblyBudgetBeforeHook(assemblyTestAgent(80, 10000), TRPCBuilderDeps{})
	inv := trpcagent.NewInvocation()
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	args := &trpcmodel.BeforeModelArgs{
		Request: &trpcmodel.Request{
			Messages: []trpcmodel.Message{
				{Role: trpcmodel.RoleSystem, Content: "sys"},
				{Role: trpcmodel.RoleUser, Content: "hi"},
			},
			Tools: map[string]trpctool.Tool{
				"huge": sizedDeclTool{name: "huge", desc: strings.Repeat("x", 2500)},
			},
		},
	}
	msgsOnly := analyzePromptRequest(args.Request.Messages).EstTokens
	if msgsOnly > 80 {
		t.Fatalf("fixture messages must be under soft, got %d", msgsOnly)
	}
	runAssemblyBudgetHook(t, hook, ctx, args)
	if !findMarker(args.Request.Messages, assemblyBudgetWarnMarker) {
		t.Fatalf("tools_schema must count toward assembly est; messages=%d schema=%d", msgsOnly, toolsSchemaEstTokens(args.Request))
	}
}
