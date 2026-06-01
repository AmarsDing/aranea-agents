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
	preview, err := CompileToGraphBuildConfig(def, nil, loggateway.NewNoop())
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
		FailurePolicy: &biz.TeamFailurePolicy{
			Default: biz.FailureDefaultRetryThenBlock,
			Retry:   biz.TeamRetryPolicy{MaxAttempts: 4},
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
		FailurePolicy: &biz.TeamFailurePolicy{
			Default:      biz.FailureDefaultRetryThenBlock,
			Retry:        biz.TeamRetryPolicy{MaxAttempts: 2},
			ParallelFail: biz.ParallelFailContinue,
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

func TestGraphRuntimeE2E_buildSequentialTeamGraph(t *testing.T) {
	def := Definition{
		Mode: "sequential",
		Members: []MemberDef{
			{AgentID: "a", SortOrder: 1, Name: "Agent A"},
			{AgentID: "b", SortOrder: 2, Name: "Agent B"},
		},
		FailurePolicy: &biz.TeamFailurePolicy{
			Default: biz.FailureDefaultRetryThenBlock,
			Retry:   biz.TeamRetryPolicy{MaxAttempts: 2},
		},
	}
	cfg, err := CompileToGraphRuntimeConfig(def, func(id string) string { return "key-" + id }, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	trpcCfg := graphtrpc.GraphBuildConfig{
		EntryPoint:  cfg.EntryPoint,
		FinishPoint: cfg.FinishPoint,
		Nodes:       make([]graphtrpc.NodeDef, len(cfg.Nodes)),
		Edges:       make([]graphtrpc.EdgeDef, len(cfg.Edges)),
	}
	for i, n := range cfg.Nodes {
		trpcCfg.Nodes[i] = graphtrpc.NodeDef{
			ID: n.ID, Type: n.Type, AgentName: n.AgentName, RetryMaxAttempts: n.RetryMaxAttempts,
			Destinations: append([]string(nil), n.Destinations...),
		}
	}
	for i, e := range cfg.Edges {
		trpcCfg.Edges[i] = graphtrpc.EdgeDef{From: e.From, To: e.To}
	}
	g, agents, err := graphtrpc.BuildStateGraphWithAgents(context.Background(), trpcCfg, &graphtrpc.BuildDeps{
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
	trpcCfg := graphtrpc.GraphBuildConfig{
		EntryPoint: cfg.EntryPoint, FinishPoint: cfg.FinishPoint,
		Nodes: make([]graphtrpc.NodeDef, len(cfg.Nodes)),
		Edges: make([]graphtrpc.EdgeDef, len(cfg.Edges)),
	}
	for i, n := range cfg.Nodes {
		trpcCfg.Nodes[i] = graphtrpc.NodeDef{ID: n.ID, Type: n.Type, AgentName: n.AgentName}
	}
	for i, e := range cfg.Edges {
		trpcCfg.Edges[i] = graphtrpc.EdgeDef{From: e.From, To: e.To}
	}
	g, _, err := graphtrpc.BuildStateGraphWithAgents(context.Background(), trpcCfg, &graphtrpc.BuildDeps{Agents: stubAgentResolver{}}, nil)
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
