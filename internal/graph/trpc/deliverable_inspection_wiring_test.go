package graph

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// TestDeliverableInspectionOptions_OnlySynthesizer P3-2：只有 synthesizer 角色
// 节点挂校验回调；worker/coordinator/无角色节点不挂。
func TestDeliverableInspectionOptions_OnlySynthesizer(t *testing.T) {
	t.Parallel()
	cases := []struct {
		role string
		want int
	}{
		{biz.RoleSynthesizer, 1},
		{biz.RoleWorker, 0},
		{biz.RoleCoordinator, 0},
		{"", 0},
	}
	for _, tc := range cases {
		n := NodeDef{NodeDef: biz.NodeDef{ID: "n1", Type: "agent", RequiredRole: tc.role}}
		if got := len(deliverableInspectionOptions(n)); got != tc.want {
			t.Errorf("role=%q: options len = %d, want %d", tc.role, got, tc.want)
		}
	}
}

// TestInspectDeliverableBeforeSynthesis_InjectsNotice P3-2：可疑产出注入
// [产出校验] 通告到 messages；回调返回 (nil, nil) 不阻断节点执行（框架语义：
// 非 nil customResult 会跳过节点）。
func TestInspectDeliverableBeforeSynthesis_InjectsNotice(t *testing.T) {
	t.Parallel()
	state := trpcgraph.State{
		biz.DeliverableStateKey: map[string]any{
			"report": "我无法完成这个任务。",
			"data":   "这是一份足够长且健康的数据产出，包含具体指标、对比分析与明确结论，可直接采信。",
		},
		trpcgraph.StateKeyMessages: []trpcmodel.Message{
			{Role: trpcmodel.RoleUser, Content: "汇总各成员产出"},
		},
	}
	res, err := inspectDeliverableBeforeSynthesis(context.Background(), nil, state)
	if err != nil {
		t.Fatalf("callback returned error: %v", err)
	}
	if res != nil {
		t.Fatalf("callback returned non-nil customResult（会跳过节点执行）: %v", res)
	}
	msgs, ok := state[trpcgraph.StateKeyMessages].([]trpcmodel.Message)
	if !ok || len(msgs) != 2 {
		t.Fatalf("messages len = %v, want 2（原消息+通告）", state[trpcgraph.StateKeyMessages])
	}
	notice := msgs[1]
	if notice.Role != trpcmodel.RoleAssistant {
		t.Errorf("notice role = %s, want assistant", notice.Role)
	}
	if !strings.Contains(notice.Content, "[产出校验]") || !strings.Contains(notice.Content, "report") {
		t.Errorf("通告内容不含校验标记或可疑 topic: %q", notice.Content)
	}
	if strings.Contains(notice.Content, "data") && strings.Contains(notice.Content, "topic \"data\"") {
		t.Errorf("健康 topic 不应被列入通告: %q", notice.Content)
	}
}

// TestInspectDeliverableBeforeSynthesis_HealthyNoop P3-2：健康产出/无
// deliverable/空 map 时不注入任何消息（advisory 不打扰）。
func TestInspectDeliverableBeforeSynthesis_HealthyNoop(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		state trpcgraph.State
	}{
		{"无 deliverable 键", trpcgraph.State{}},
		{"deliverable 类型异常", trpcgraph.State{biz.DeliverableStateKey: "not-a-map"}},
		{"空 deliverable", trpcgraph.State{biz.DeliverableStateKey: map[string]any{}}},
		{"健康产出", trpcgraph.State{biz.DeliverableStateKey: map[string]any{
			"report": "本报告系统梳理了三类主流方案并给出选型建议与风险评估，数据来源于权威渠道且经过交叉验证。",
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := inspectDeliverableBeforeSynthesis(context.Background(), nil, tc.state)
			if err != nil || res != nil {
				t.Fatalf("res=%v err=%v, want (nil, nil)", res, err)
			}
			if _, exists := tc.state[trpcgraph.StateKeyMessages]; exists {
				t.Errorf("不应注入 messages: %v", tc.state[trpcgraph.StateKeyMessages])
			}
		})
	}
}
