package graph

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

func TestCriticLoopFinishAgentNodeIDs(t *testing.T) {
	cfg := GraphBuildConfig{
		Nodes: []biz.NodeDef{
			{ID: "gen", Type: biz.NodeTypeAgent},
			{ID: "crit", Type: biz.NodeTypeAgent},
			{ID: "llm-crit", Type: biz.NodeTypeLLM},
			{ID: "plain", Type: biz.NodeTypeAgent},
		},
		ConditionalEdges: []biz.ConditionalEdgeDef{
			{
				From:        "crit",
				CondFuncRef: biz.CriticLoopCondFuncRefForNode(0, 3, "crit"),
				PathMap:     map[string]string{"approved": biz.EndNodeID, "retry": "gen"},
			},
			{
				From:        "llm-crit",
				CondFuncRef: biz.CriticLoopCondFuncRef,
				PathMap:     map[string]string{"approved": biz.EndNodeID, "retry": "gen"},
			},
			{
				From:        "plain",
				CondFuncRef: "some_other_cond",
				PathMap:     map[string]string{"x": "gen"},
			},
		},
	}
	ids := criticLoopFinishAgentNodeIDs(cfg)
	if len(ids) != 1 {
		t.Fatalf("ids=%v want exactly 1 (agent-node critic only)", ids)
	}
	if _, ok := ids["crit"]; !ok {
		t.Fatalf("ids=%v missing %q", ids, "crit")
	}
}

func TestCriticLoopFinishAgentNodeIDs_NoCriticEdges(t *testing.T) {
	cfg := GraphBuildConfig{
		Nodes: []biz.NodeDef{{ID: "a", Type: biz.NodeTypeAgent}},
	}
	if ids := criticLoopFinishAgentNodeIDs(cfg); len(ids) != 0 {
		t.Fatalf("ids=%v want empty", ids)
	}
}

// scopedMetaKeys 返回 nodeID scoped 的三个 metadata key（测试便捷函数）。
func scopedMetaKeys(nodeID string) (rounds, last, prev string) {
	return biz.CriticLoopMetaKeysForNode(nodeID)
}

func TestCriticRoundCaptureCallback_FirstRound(t *testing.T) {
	cb := criticRoundCaptureCallbackForNode("crit")
	upd := trpcgraph.State{trpcgraph.StateKeyLastResponse: "round 1 feedback"}
	got, err := cb(context.Background(), nil, trpcgraph.State{}, upd, nil)
	if err != nil {
		t.Fatal(err)
	}
	st, ok := got.(trpcgraph.State)
	if !ok {
		t.Fatalf("result type=%T want trpcgraph.State", got)
	}
	meta, ok := st[trpcgraph.StateKeyMetadata].(map[string]any)
	if !ok {
		t.Fatalf("metadata missing in %v", st)
	}
	roundsKey, lastKey, prevKey := scopedMetaKeys("crit")
	if n := biz.CriticLoopMetaInt(meta[roundsKey]); n != 1 {
		t.Fatalf("rounds=%d want 1", n)
	}
	if s, _ := meta[lastKey].(string); s != "round 1 feedback" {
		t.Fatalf("last=%q", s)
	}
	if s, _ := meta[prevKey].(string); s != "" {
		t.Fatalf("prev=%q want empty", s)
	}
	// 不写裸 key（scoped 隔离）。
	if _, exists := meta[biz.CriticLoopRoundsMetaKey]; exists {
		t.Fatalf("bare rounds key must not be written by scoped callback")
	}
}

func TestCriticRoundCaptureCallback_SecondRoundShiftsPrev(t *testing.T) {
	cb := criticRoundCaptureCallbackForNode("crit")
	roundsKey, lastKey, prevKey := scopedMetaKeys("crit")
	state := trpcgraph.State{
		trpcgraph.StateKeyMetadata: map[string]any{
			roundsKey:   1,
			lastKey:     "round 1 feedback",
			"unrelated": "kept",
		},
	}
	upd := trpcgraph.State{trpcgraph.StateKeyLastResponse: "round 2 feedback"}
	got, err := cb(context.Background(), nil, state, upd, nil)
	if err != nil {
		t.Fatal(err)
	}
	meta := got.(trpcgraph.State)[trpcgraph.StateKeyMetadata].(map[string]any)
	if n := biz.CriticLoopMetaInt(meta[roundsKey]); n != 2 {
		t.Fatalf("rounds=%d want 2", n)
	}
	if s, _ := meta[prevKey].(string); s != "round 1 feedback" {
		t.Fatalf("prev=%q", s)
	}
	if s, _ := meta[lastKey].(string); s != "round 2 feedback" {
		t.Fatalf("last=%q", s)
	}
	if s, _ := meta["unrelated"].(string); s != "kept" {
		t.Fatalf("unrelated key clobbered: %q", s)
	}
}

func TestCriticRoundCaptureCallback_NodeIsolation(t *testing.T) {
	// 多 critic 图：crit-a 的轮次不得被 crit-b 的 callback 读取/覆盖。
	cbB := criticRoundCaptureCallbackForNode("crit-b")
	roundsA, lastA, _ := scopedMetaKeys("crit-a")
	roundsB, lastB, _ := scopedMetaKeys("crit-b")

	state := trpcgraph.State{
		trpcgraph.StateKeyMetadata: map[string]any{
			roundsA: 2,
			lastA:   "a round 2",
		},
	}
	// crit-b 第一轮：scoped 无数据，bare 也无数据 → 从 0 计。
	got, err := cbB(context.Background(), nil, state, trpcgraph.State{trpcgraph.StateKeyLastResponse: "b round 1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	meta := got.(trpcgraph.State)[trpcgraph.StateKeyMetadata].(map[string]any)
	if n := biz.CriticLoopMetaInt(meta[roundsB]); n != 1 {
		t.Fatalf("crit-b rounds=%d want 1 (isolated from crit-a)", n)
	}
	if s, _ := meta[lastB].(string); s != "b round 1" {
		t.Fatalf("crit-b last=%q", s)
	}
	// crit-a 数据原样保留。
	if n := biz.CriticLoopMetaInt(meta[roundsA]); n != 2 {
		t.Fatalf("crit-a rounds clobbered: %d", n)
	}
}

func TestCriticRoundCaptureCallback_LegacyBareKeyFallback(t *testing.T) {
	// 旧 checkpoint（升级前写入裸 key）：scoped 无数据时读裸 key 作为基数，
	// 写入只进 scoped key。
	cb := criticRoundCaptureCallbackForNode("crit")
	roundsKey, lastKey, prevKey := scopedMetaKeys("crit")
	state := trpcgraph.State{
		trpcgraph.StateKeyMetadata: map[string]any{
			biz.CriticLoopRoundsMetaKey:       2,
			biz.CriticLoopLastResponseMetaKey: "legacy round 2",
		},
	}
	upd := trpcgraph.State{trpcgraph.StateKeyLastResponse: "round 3"}
	got, err := cb(context.Background(), nil, state, upd, nil)
	if err != nil {
		t.Fatal(err)
	}
	meta := got.(trpcgraph.State)[trpcgraph.StateKeyMetadata].(map[string]any)
	if n := biz.CriticLoopMetaInt(meta[roundsKey]); n != 3 {
		t.Fatalf("rounds=%d want 3 (legacy base 2 + 1)", n)
	}
	if s, _ := meta[prevKey].(string); s != "legacy round 2" {
		t.Fatalf("prev=%q want legacy round 2", s)
	}
	if s, _ := meta[lastKey].(string); s != "round 3" {
		t.Fatalf("last=%q", s)
	}
}

func TestCriticRoundCaptureCallback_ToleratesFloat64Rounds(t *testing.T) {
	// JSON checkpoint round-trip 后 int 变 float64。
	cb := criticRoundCaptureCallbackForNode("crit")
	roundsKey, _, _ := scopedMetaKeys("crit")
	state := trpcgraph.State{
		trpcgraph.StateKeyMetadata: map[string]any{
			roundsKey: float64(2),
		},
	}
	upd := trpcgraph.State{trpcgraph.StateKeyLastResponse: "round 3"}
	got, err := cb(context.Background(), nil, state, upd, nil)
	if err != nil {
		t.Fatal(err)
	}
	meta := got.(trpcgraph.State)[trpcgraph.StateKeyMetadata].(map[string]any)
	if n := biz.CriticLoopMetaInt(meta[roundsKey]); n != 3 {
		t.Fatalf("rounds=%d want 3", n)
	}
}

func TestCriticRoundCaptureCallback_FailOpen(t *testing.T) {
	cb := criticRoundCaptureCallbackForNode("crit")
	upd := trpcgraph.State{trpcgraph.StateKeyLastResponse: "x"}

	// 节点出错：不重写结果。
	got, err := cb(context.Background(), nil, trpcgraph.State{}, upd, context.Canceled)
	if err != nil || got != nil {
		t.Fatalf("nodeErr: got=%v err=%v want nil,nil", got, err)
	}
	// 结果不是 State：不重写。
	got, err = cb(context.Background(), nil, trpcgraph.State{}, "not-a-state", nil)
	if err != nil || got != nil {
		t.Fatalf("bad result: got=%v err=%v want nil,nil", got, err)
	}
	// last_response 为空：不计轮次。
	got, err = cb(context.Background(), nil, trpcgraph.State{}, trpcgraph.State{trpcgraph.StateKeyLastResponse: "  "}, nil)
	if err != nil || got != nil {
		t.Fatalf("empty response: got=%v err=%v want nil,nil", got, err)
	}
}
