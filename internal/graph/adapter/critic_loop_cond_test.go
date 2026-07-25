package adapter

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestCriticLoopCondFunc_ToolCallApprove(t *testing.T) {
	fn := criticLoopCondFunc(0.8, 0, "", loggateway.NewNoop())
	state := trpcgraph.State{
		trpcgraph.StateKeyMessages: []trpcmodel.Message{
			{Role: trpcmodel.RoleAssistant, ToolCalls: []trpcmodel.ToolCall{
				{Function: trpcmodel.FunctionDefinitionParam{
					Name:      biz.OrchestrationControlToolName,
					Arguments: []byte(`{"action":"approve","score":0.9,"reason":"looks good"}`),
				}},
			}},
		},
	}
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "approved" {
		t.Fatalf("expected approved, got %s", result)
	}
}

func TestCriticLoopCondFunc_ToolCallRetry(t *testing.T) {
	fn := criticLoopCondFunc(0.8, 0, "", loggateway.NewNoop())
	state := trpcgraph.State{
		trpcgraph.StateKeyMessages: []trpcmodel.Message{
			{Role: trpcmodel.RoleAssistant, ToolCalls: []trpcmodel.ToolCall{
				{Function: trpcmodel.FunctionDefinitionParam{
					Name:      biz.OrchestrationControlToolName,
					Arguments: []byte(`{"action":"retry","reason":"needs improvement"}`),
				}},
			}},
		},
	}
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "retry" {
		t.Fatalf("expected retry, got %s", result)
	}
}

func TestCriticLoopCondFunc_ToolCallScoreCannotOverrideRetry(t *testing.T) {
	// F2 语义：显式 action=retry 优先，score 高于阈值也不得推翻。
	fn := criticLoopCondFunc(0.7, 0, "", loggateway.NewNoop())
	state := trpcgraph.State{
		trpcgraph.StateKeyMessages: []trpcmodel.Message{
			{Role: trpcmodel.RoleAssistant, ToolCalls: []trpcmodel.ToolCall{
				{Function: trpcmodel.FunctionDefinitionParam{
					Name:      biz.OrchestrationControlToolName,
					Arguments: []byte(`{"action":"retry","score":0.8}`),
				}},
			}},
		},
	}
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "retry" {
		t.Fatalf("expected retry (explicit action wins over score), got %s", result)
	}
}

func TestCriticLoopCondFunc_ToolCallEmptyActionScoreFallback(t *testing.T) {
	// action 为空时 score 才兜底：score >= threshold → approved。
	fn := criticLoopCondFunc(0.7, 0, "", loggateway.NewNoop())
	state := trpcgraph.State{
		trpcgraph.StateKeyMessages: []trpcmodel.Message{
			{Role: trpcmodel.RoleAssistant, ToolCalls: []trpcmodel.ToolCall{
				{Function: trpcmodel.FunctionDefinitionParam{
					Name:      biz.OrchestrationControlToolName,
					Arguments: []byte(`{"score":0.8,"reason":"no explicit action"}`),
				}},
			}},
		},
	}
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "approved" {
		t.Fatalf("expected approved (empty action, score >= threshold), got %s", result)
	}
}

func TestCriticLoopCondFunc_FallbackStringApproved(t *testing.T) {
	fn := criticLoopCondFunc(0.8, 0, "", loggateway.NewNoop())
	state := trpcgraph.State{
		trpcgraph.StateKeyMessages: []trpcmodel.Message{
			{Role: trpcmodel.RoleAssistant, Content: "I have reviewed the work and it is approved."},
		},
	}
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "approved" {
		t.Fatalf("expected approved (string fallback), got %s", result)
	}
}

func TestCriticLoopCondFunc_FallbackStringScore(t *testing.T) {
	fn := criticLoopCondFunc(0.7, 0, "", loggateway.NewNoop())
	state := trpcgraph.State{
		trpcgraph.StateKeyMessages: []trpcmodel.Message{
			{Role: trpcmodel.RoleAssistant, Content: `{"score": 0.8}`},
		},
	}
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "approved" {
		t.Fatalf("expected approved (score fallback), got %s", result)
	}
}

func TestCriticLoopCondFunc_NoMessages(t *testing.T) {
	fn := criticLoopCondFunc(0.8, 0, "", loggateway.NewNoop())
	result, err := fn(context.Background(), trpcgraph.State{})
	if err != nil {
		t.Fatal(err)
	}
	if result != "retry" {
		t.Fatalf("expected retry (no messages), got %s", result)
	}
}

func TestCriticLoopCondFunc_OtherToolCallIgnored(t *testing.T) {
	fn := criticLoopCondFunc(0.8, 0, "", loggateway.NewNoop())
	state := trpcgraph.State{
		trpcgraph.StateKeyMessages: []trpcmodel.Message{
			{Role: trpcmodel.RoleAssistant, Content: "needs work", ToolCalls: []trpcmodel.ToolCall{
				{Function: trpcmodel.FunctionDefinitionParam{
					Name:      "some_other_tool",
					Arguments: []byte(`{"action":"approve"}`),
				}},
			}},
		},
	}
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "retry" {
		t.Fatalf("expected retry (other tool call ignored, no 'approved' in content), got %s", result)
	}
}

func criticDecisionMsg(content, args string) trpcmodel.Message {
	return trpcmodel.Message{
		Role:    trpcmodel.RoleAssistant,
		Content: content,
		ToolCalls: []trpcmodel.ToolCall{
			{Function: trpcmodel.FunctionDefinitionParam{
				Name:      biz.OrchestrationControlToolName,
				Arguments: []byte(args),
			}},
		},
	}
}

func TestCriticLoopCondFunc_MaxIterationsForcedConvergence(t *testing.T) {
	// F4：结构化 retry 且达上限 → approved_forced（兜底收敛，区别于真实批准）。
	fn := criticLoopCondFunc(0.8, 2, "", loggateway.NewNoop())
	state := trpcgraph.State{
		trpcgraph.StateKeyMessages: []trpcmodel.Message{
			criticDecisionMsg("round 1 feedback", `{"action":"retry","reason":"round 1"}`),
			{Role: trpcmodel.RoleAssistant, Content: "generator output v2"},
			criticDecisionMsg("round 2 feedback", `{"action":"retry","reason":"round 2"}`),
		},
	}
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != biz.CriticLoopResultApprovedForced {
		t.Fatalf("expected approved_forced (max iterations reached), got %s", result)
	}
}

func TestCriticLoopCondFunc_MaxIterationsNotReached(t *testing.T) {
	fn := criticLoopCondFunc(0.8, 3, "", loggateway.NewNoop())
	state := trpcgraph.State{
		trpcgraph.StateKeyMessages: []trpcmodel.Message{
			criticDecisionMsg("round 1 feedback", `{"action":"retry","reason":"round 1"}`),
		},
	}
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "retry" {
		t.Fatalf("expected retry (1 < 3 rounds), got %s", result)
	}
}

func TestCriticLoopCondFunc_StructuredRetryBeatsDry(t *testing.T) {
	// F3：两轮反馈文本相同（heuristic 判 dry），但末条消息带显式
	// 结构化 retry verdict — 显式信号优先，不得被 dry 收敛推翻。
	fn := criticLoopCondFunc(0.8, 0, "", loggateway.NewNoop())
	state := trpcgraph.State{
		trpcgraph.StateKeyMessages: []trpcmodel.Message{
			criticDecisionMsg("Needs more detail in section 2.", `{"action":"retry","reason":"r1"}`),
			{Role: trpcmodel.RoleAssistant, Content: "generator output v2"},
			criticDecisionMsg("  needs more detail in section 2. ", `{"action":"retry","reason":"r2"}`),
		},
	}
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "retry" {
		t.Fatalf("expected retry (explicit structured verdict beats dry), got %s", result)
	}
}

func TestCriticLoopCondFunc_MessagesDryConvergence(t *testing.T) {
	// 无结构化裁决时（末条为纯文本），messages 路径 dry 仍生效。
	fn := criticLoopCondFunc(0.8, 0, "", loggateway.NewNoop())
	state := trpcgraph.State{
		trpcgraph.StateKeyMessages: []trpcmodel.Message{
			criticDecisionMsg("Needs more detail in section 2.", `{"action":"retry","reason":"r1"}`),
			criticDecisionMsg("  needs more detail in section 2. ", `{"action":"retry","reason":"r2"}`),
			{Role: trpcmodel.RoleAssistant, Content: "generator output v3"},
		},
	}
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "approved" {
		t.Fatalf("expected approved (messages dry without structured verdict), got %s", result)
	}
}

func TestCriticLoopCondFunc_DryNotTriggeredWhenFeedbackDiffers(t *testing.T) {
	fn := criticLoopCondFunc(0.8, 0, "", loggateway.NewNoop())
	state := trpcgraph.State{
		trpcgraph.StateKeyMessages: []trpcmodel.Message{
			criticDecisionMsg("Needs more detail in section 2.", `{"action":"retry","reason":"r1"}`),
			{Role: trpcmodel.RoleAssistant, Content: "generator output v2"},
			criticDecisionMsg("Section 2 improved, but conclusion is weak.", `{"action":"retry","reason":"r2"}`),
		},
	}
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "retry" {
		t.Fatalf("expected retry (new feedback present), got %s", result)
	}
}

func TestExtractScore(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{`{"score": 0.85}`, 0.85},
		{`[{"score": 0.9}]`, 0.9},
		{`no score here`, 0},
		{``, 0},
	}
	for _, tt := range tests {
		got := biz.ExtractScore(tt.input)
		if got != tt.expected {
			t.Errorf("ExtractScore(%q) = %f, want %f", tt.input, got, tt.expected)
		}
	}
}

// --- agent 节点 critic 路径：轮次/反馈来自 state metadata（capture callback
// 写入），评审文本来自 last_response。messages 为空。 ---

func criticMetaState(rounds int, prev, last, lastResponse string) trpcgraph.State {
	return trpcgraph.State{
		trpcgraph.StateKeyMetadata: map[string]any{
			biz.CriticLoopRoundsMetaKey:       rounds,
			biz.CriticLoopPrevResponseMetaKey: prev,
			biz.CriticLoopLastResponseMetaKey: last,
		},
		trpcgraph.StateKeyLastResponse: lastResponse,
	}
}

func TestCriticLoopCondFunc_MetadataMaxIterations(t *testing.T) {
	// agent 节点 critic：messages 为空，轮次只能来自 metadata。
	fn := criticLoopCondFunc(0, 2, "", loggateway.NewNoop())
	state := criticMetaState(2, "r1 feedback", "r2 feedback", "仍需修改：结论薄弱")
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != biz.CriticLoopResultApprovedForced {
		t.Fatalf("expected approved_forced (metadata rounds >= max), got %s", result)
	}
}

func TestCriticLoopCondFunc_GenuineApprovalAtLimitNotForced(t *testing.T) {
	// 上限当轮评审文本真实批准 → approved，不得标记 approved_forced。
	fn := criticLoopCondFunc(0, 2, "", loggateway.NewNoop())
	state := criticMetaState(2, "r1 feedback", "评审通过", "评审通过，可以发布")
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "approved" {
		t.Fatalf("expected approved (genuine approval at limit round), got %s", result)
	}
}

func TestCriticLoopCondFunc_MetadataMaxIterationsNotReached(t *testing.T) {
	fn := criticLoopCondFunc(0, 3, "", loggateway.NewNoop())
	state := criticMetaState(1, "", "r1 feedback", "仍需修改：结论薄弱")
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "retry" {
		t.Fatalf("expected retry (1 < 3), got %s", result)
	}
}

func TestCriticLoopCondFunc_MetadataDryConvergence(t *testing.T) {
	fn := criticLoopCondFunc(0, 0, "", loggateway.NewNoop())
	// 最近两轮反馈归一化后相同 → loop-until-dry 提前收敛。
	state := criticMetaState(2, " Needs  more detail ", "needs more detail", "needs more detail")
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "approved" {
		t.Fatalf("expected approved (metadata dry), got %s", result)
	}
}

func TestCriticLoopCondFunc_MetadataDryNotTriggeredWhenDiffers(t *testing.T) {
	fn := criticLoopCondFunc(0, 0, "", loggateway.NewNoop())
	state := criticMetaState(2, "needs more detail", "conclusion is weak", "conclusion is weak")
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "retry" {
		t.Fatalf("expected retry (new feedback), got %s", result)
	}
}

func TestCriticLoopCondFunc_LastResponseApprovalZH(t *testing.T) {
	fn := criticLoopCondFunc(0, 0, "", loggateway.NewNoop())
	state := criticMetaState(1, "", "评审通过，可以发布", "评审通过，可以发布")
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "approved" {
		t.Fatalf("expected approved (中文批准词), got %s", result)
	}
}

func TestCriticLoopCondFunc_ApprovalKeywordsZH(t *testing.T) {
	fn := criticLoopCondFunc(0, 0, "", loggateway.NewNoop())
	cases := []struct {
		content string
		want    string
	}{
		{"结论：通过，无需修改", "approved"},
		{"审核通过，同意发布", "approved"},
		{"不批准，仍需修改", "retry"},
		{"未通过评审", "retry"},
		{"予以驳回", "retry"},
		// 裸「通过」作介词，不得误判为批准。
		{"通过描绘更多细节可以提升质量", "retry"},
		// 组合式否定：批准词紧邻否定标记，拒绝词表枚举不到，须前缀否定拦截。
		{"不能予以通过", "retry"},
		{"不予评审通过", "retry"},
		{"难以评审通过", "retry"},
		{"无法审核通过", "retry"},
		{"并非通过评审", "retry"},
		{"尚不能批准", "retry"},
		// 正文别处的「不」不紧邻批准词，不得误拒。
		{"问题不复存在，予以通过", "approved"},
		{"不涉密，同意发布", "approved"},
		{"经多轮修改已达标，评审通过", "approved"},
		// 多次出现：第一次被否定、第二次干净，以干净命中为准。
		{"不予评审通过；修改后评审通过", "approved"},
	}
	for _, c := range cases {
		state := trpcgraph.State{trpcgraph.StateKeyLastResponse: c.content}
		got, err := fn(context.Background(), state)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("content=%q got=%s want=%s", c.content, got, c.want)
		}
	}
}

func TestCriticLoopCondFunc_LastResponsePreferredOverMessages(t *testing.T) {
	// agent 节点路径：last_response 优先于 messages 末条内容。
	fn := criticLoopCondFunc(0, 0, "", loggateway.NewNoop())
	state := trpcgraph.State{
		trpcgraph.StateKeyLastResponse: "批准",
		trpcgraph.StateKeyMessages: []trpcmodel.Message{
			{Role: trpcmodel.RoleAssistant, Content: "仍需修改"},
		},
	}
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "approved" {
		t.Fatalf("expected approved (last_response wins), got %s", result)
	}
}

// --- 节点 scoped metadata（加固1：多 critic 图按节点隔离轮次） ---

// criticScopedMetaState 构造 nodeID scoped 的 metadata（biz.CriticLoopMetaKeysForNode）。
func criticScopedMetaState(nodeID string, rounds int, prev, last, lastResponse string) trpcgraph.State {
	roundsKey, lastKey, prevKey := biz.CriticLoopMetaKeysForNode(nodeID)
	return trpcgraph.State{
		trpcgraph.StateKeyMetadata: map[string]any{
			roundsKey: rounds,
			prevKey:   prev,
			lastKey:   last,
		},
		trpcgraph.StateKeyLastResponse: lastResponse,
	}
}

func TestCriticLoopCondFunc_NodeScopedMaxIterations(t *testing.T) {
	// team 编译路径：cond func 经 ref 拿到 nodeID，只读该节点 scoped 轮次。
	fn := criticLoopCondFunc(0, 2, "member-2", loggateway.NewNoop())
	state := criticScopedMetaState("member-2", 2, "r1", "r2", "仍需修改")
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != biz.CriticLoopResultApprovedForced {
		t.Fatalf("expected approved_forced (scoped rounds >= max), got %s", result)
	}
}

func TestCriticLoopCondFunc_NodeScopedIsolation(t *testing.T) {
	// 多 critic 图：member-1 已循环 2 轮，member-2 的 cond func 不得读到。
	fn := criticLoopCondFunc(0, 2, "member-2", loggateway.NewNoop())
	state := criticScopedMetaState("member-1", 2, "r1", "r2", "仍需修改")
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "retry" {
		t.Fatalf("expected retry (other node's rounds must not leak), got %s", result)
	}
}

func TestCriticLoopCondFunc_NodeScopedDryConvergence(t *testing.T) {
	fn := criticLoopCondFunc(0, 0, "member-2", loggateway.NewNoop())
	state := criticScopedMetaState("member-2", 2, " Needs  more detail ", "needs more detail", "needs more detail")
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != "approved" {
		t.Fatalf("expected approved (scoped dry), got %s", result)
	}
}

func TestCriticLoopCondFunc_NodeScopedBareKeyFallback(t *testing.T) {
	// 旧 checkpoint（升级前写入裸 key）：scoped 无数据时回落读取一次。
	fn := criticLoopCondFunc(0, 2, "member-2", loggateway.NewNoop())
	state := criticMetaState(2, "r1", "r2", "仍需修改")
	result, err := fn(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if result != biz.CriticLoopResultApprovedForced {
		t.Fatalf("expected approved_forced (legacy bare-key rounds fallback), got %s", result)
	}
}
