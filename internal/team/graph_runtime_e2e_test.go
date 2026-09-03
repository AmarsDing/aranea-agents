package team

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type stubCatalogAgent struct{}

func (stubCatalogAgent) Info() trpcagent.Info { return trpcagent.Info{Name: "stub-agent"} }

func (stubCatalogAgent) Run(context.Context, *trpcagent.Invocation) (<-chan *trpcevent.Event, error) {
	ch := make(chan *trpcevent.Event)
	close(ch)
	return ch, nil
}

func (stubCatalogAgent) Tools() []trpctool.Tool { return nil }

func (stubCatalogAgent) SubAgents() []trpcagent.Agent { return nil }

func (stubCatalogAgent) FindSubAgent(string) trpcagent.Agent { return nil }

type stubAgentResolver struct{}

func (stubAgentResolver) ResolveAgent(context.Context, string) (trpcagent.Agent, error) {
	return stubCatalogAgent{}, nil
}

func TestCompileToGraphRuntimeConfig_adaptiveStripsTransferEdges(t *testing.T) {
	def := Definition{
		Mode: "adaptive",
		Members: []MemberDef{
			{AgentID: "a", SortOrder: 1},
			{AgentID: "b", SortOrder: 2},
			{AgentID: "c", SortOrder: 3},
		},
	}
	preview, _, err := CompileToGraphBuildConfig(def, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	var transferPreview int
	for _, e := range preview.Edges {
		if e.Kind == "transfer" {
			transferPreview++
		}
	}
	if transferPreview == 0 {
		t.Fatal("preview should include transfer overlay edges")
	}

	runtime, err := CompileToGraphRuntimeConfig(def, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range runtime.Edges {
		if e.Kind == "transfer" {
			t.Fatalf("runtime edge should not include transfer: %+v", e)
		}
	}
	if len(runtime.Nodes[0].Destinations) < 2 {
		t.Fatalf("adaptive destinations=%v", runtime.Nodes[0].Destinations)
	}
}

func TestCompileToGraphRuntimeConfig_failurePolicyRetry(t *testing.T) {
	def := Definition{
		Mode: "sequential",
		Members: []MemberDef{
			{AgentID: "a", SortOrder: 1},
			{AgentID: "b", SortOrder: 2},
		},
		FailurePolicy: &FailurePolicy{
			Default: "retry_then_block",
			Retry:   RetryPolicy{MaxAttempts: 4},
		},
	}
	cfg, err := CompileToGraphRuntimeConfig(def, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Nodes[0].RetryMaxAttempts != 4 {
		t.Fatalf("retry=%d want 4", cfg.Nodes[0].RetryMaxAttempts)
	}
}

func TestCompileToGraphRuntimeConfig_parallelFailContinue(t *testing.T) {
	def := Definition{
		Mode:               "parallel",
		SynthesizerAgentID: "synth",
		Members: []MemberDef{
			{AgentID: "w1", SortOrder: 1},
			{AgentID: "w2", SortOrder: 2},
			{AgentID: "synth", Role: "synthesizer", SortOrder: 3},
		},
		FailurePolicy: &FailurePolicy{
			Default:      "retry_then_block",
			Retry:        RetryPolicy{MaxAttempts: 2},
			ParallelFail: "continue",
		},
	}
	cfg, err := CompileToGraphRuntimeConfig(def, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"member-1", "member-2"} {
		var found bool
		for _, n := range cfg.Nodes {
			if n.ID != id {
				continue
			}
			found = true
			if n.FailureAction != biz.FailureOnFailureSkip {
				t.Fatalf("%s action=%q want skip_on_failure", id, n.FailureAction)
			}
			if n.RetryMaxAttempts != 2 {
				t.Fatalf("%s retry=%d want preserved", id, n.RetryMaxAttempts)
			}
		}
		if !found {
			t.Fatalf("missing node %s", id)
		}
	}
}

// F4（2026-09-03 lbg-verify-planner 复盘 问题4）：coordinator（dispatch）拓扑
// 下默认策略的编译语义。dispatch 拓扑中 lead(hub) 与各 worker 都是 finish
// 汇聚点的 feeder（启发式标记全部 feeder），故 lead/中间成员均标
// skip_on_failure + 保留重试——任一成员失败都被跳过、图继续，终局由 F10
// 证据收敛 partial_failure；唯 finish（末位成员=汇聚点）保持
// retry_then_block：汇聚失败无产出，团队真失败。
func TestCompileToGraphRuntimeConfig_coordinatorParallelFailContinue(t *testing.T) {
	def := Definition{
		Mode: "coordinator",
		Members: []MemberDef{
			{AgentID: "lead", Role: "synthesizer", SortOrder: 1},
			{AgentID: "w1", SortOrder: 2},
			{AgentID: "w2", SortOrder: 3},
			{AgentID: "w3", SortOrder: 4},
		},
		FailurePolicy: &FailurePolicy{
			Default:      "retry_then_block",
			Retry:        RetryPolicy{MaxAttempts: 2},
			ParallelFail: "continue",
		},
	}
	cfg, err := CompileToGraphRuntimeConfig(def, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]biz.NodeDef, len(cfg.Nodes))
	for _, n := range cfg.Nodes {
		byID[n.ID] = n
	}
	// lead + 中间成员（finish 的全部 feeder）：skip_on_failure + 重试保留。
	for _, id := range []string{"member-1", "member-2", "member-3"} {
		n, ok := byID[id]
		if !ok {
			t.Fatalf("missing node %s", id)
		}
		if n.FailureAction != biz.FailureOnFailureSkip {
			t.Fatalf("%s action=%q want skip_on_failure", id, n.FailureAction)
		}
		if n.RetryMaxAttempts != 2 {
			t.Fatalf("%s retry=%d want 2", id, n.RetryMaxAttempts)
		}
	}
	// finish（member-4 汇聚点）：retry_then_block，不得 skip（跳过即无产出）。
	n, ok := byID["member-4"]
	if !ok {
		t.Fatal("missing finish node member-4")
	}
	if n.FailureAction != biz.FailureDefaultRetryThenBlock {
		t.Fatalf("member-4 action=%q want retry_then_block", n.FailureAction)
	}
	if n.RetryMaxAttempts != 2 {
		t.Fatalf("member-4 retry=%d want 2", n.RetryMaxAttempts)
	}
	// skip 语义需要 _skipped_nodes 状态字段支撑。
	hasSkippedField := false
	for _, sf := range cfg.StateFields {
		if sf.Name == biz.SkippedNodesStateKey {
			hasSkippedField = true
			break
		}
	}
	if !hasSkippedField {
		t.Fatal("skipped_nodes state field missing for parallel_fail=continue")
	}
}

func TestGraphRuntimeE2E_buildSequentialTeamGraph(t *testing.T) {
	def := Definition{
		Mode: "sequential",
		Members: []MemberDef{
			{AgentID: "a", SortOrder: 1, Name: "Agent A"},
			{AgentID: "b", SortOrder: 2, Name: "Agent B"},
		},
		FailurePolicy: &FailurePolicy{
			Default: "retry_then_block",
			Retry:   RetryPolicy{MaxAttempts: 2},
		},
	}
	cfg, err := CompileToGraphRuntimeConfig(def, func(id string) string { return "key-" + id }, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	g, agents, err := graphtrpc.BuildStateGraphWithAgents(context.Background(), cfg, &graphtrpc.GraphNodeResolverSet{
		Agents: stubAgentResolver{},
	}, nil)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}
	if g == nil {
		t.Fatal("graph nil")
	}
	if len(agents) != 2 {
		t.Fatalf("agents=%d want 2", len(agents))
	}
	_, err = graphtrpc.NewGraphAgent("team-seq", g, false, agents...)
	if err != nil {
		t.Fatalf("graph agent: %v", err)
	}
}

func TestGraphRuntimeE2E_buildCoordinatorTeamGraph(t *testing.T) {
	def := Definition{
		Mode: "coordinator",
		Members: []MemberDef{
			{AgentID: "lead", SortOrder: 1},
			{AgentID: "w1", SortOrder: 2},
			{AgentID: "w2", SortOrder: 3},
		},
	}
	cfg, err := CompileToGraphRuntimeConfig(def, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	g, _, err := graphtrpc.BuildStateGraphWithAgents(context.Background(), cfg, &graphtrpc.GraphNodeResolverSet{Agents: stubAgentResolver{}}, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(cfg.Edges) != 4 {
		t.Fatalf("runtime edges=%d", len(cfg.Edges))
	}
	if g == nil {
		t.Fatal("nil graph")
	}
}
