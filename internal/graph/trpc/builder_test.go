package graph

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type stubModel struct{}

func (stubModel) GenerateContent(ctx context.Context, req *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	ch := make(chan *trpcmodel.Response, 1)
	ch <- &trpcmodel.Response{}
	close(ch)
	return ch, nil
}

func (stubModel) Info() trpcmodel.Info { return trpcmodel.Info{Name: "stub"} }

type stubDeps struct{}

func (stubDeps) ResolveModel(context.Context, string) (trpcmodel.Model, error) {
	return stubModel{}, nil
}

func (stubDeps) ResolveTools(context.Context, []string) (map[string]trpctool.Tool, error) {
	return map[string]trpctool.Tool{}, nil
}

func TestBuildStateGraph_LLMNode(t *testing.T) {
	cfg := GraphBuildConfig{
		EntryPoint:  "llm1",
		FinishPoint: "llm1",
		StateFields: []StateFieldDef{{Name: "messages", Type: "[]any", Reducer: ReducerAppend}},
		Nodes: []biz.NodeDef{
			{ID: "llm1", Type: "llm", Instruction: "say hi", ModelName: "openai/gpt-4o-mini"},
		},
	}
	_, _, err := BuildStateGraphWithAgents(context.Background(), cfg, &GraphNodeResolverSet{Models: stubDeps{}}, nil)
	if err != nil {
		t.Fatalf("build llm graph: %v", err)
	}
}

func TestBuildStateGraph_FunctionNode(t *testing.T) {
	reg := NewRegistry()
	reg.RegisterNodeFuncInstance("fn-func", PassthroughNodeFunc("fn"))
	cfg := GraphBuildConfig{
		EntryPoint:  "fn",
		FinishPoint: "fn",
		StateFields: []StateFieldDef{{Name: "messages", Type: "[]any", Reducer: ReducerAppend}},
		Nodes: []biz.NodeDef{
			{ID: "fn", Type: "function", FuncRef: "fn-func"},
		},
	}
	_, _, err := BuildStateGraphWithRegistryAndLogger(context.Background(), cfg, reg, nil, nil)
	if err != nil {
		t.Fatalf("build function graph: %v", err)
	}
}

// stubAgentResolver resolves agent references to pre-built stub agents.
type stubAgentResolver struct {
	agents map[string]trpcagent.Agent
}

func (s stubAgentResolver) ResolveAgent(_ context.Context, ref string) (trpcagent.Agent, error) {
	if a, ok := s.agents[ref]; ok {
		return a, nil
	}
	return nil, apierror.NotFound(apierror.DomainAgent, "agent not found: "+ref)
}

// TestGraphAgent_AgentNode_ParentAgentInjection verifies that GraphAgent
// correctly injects StateKeyParentAgent into the initial state and that
// FindSubAgent resolves agent nodes by node ID (not just by Info().Name).
//
// Bug 4 scenario:
//   - Node ID is "member-1" (graph topology identity)
//   - AgentName is "key-a1" (the catalog agent_key, resolved to an LLM agent)
//   - The resolved agent's Info().Name is "key-a1"
//   - The framework's targetAgentFromState looks up by node ID "member-1"
//
// Without StateKeyParentAgent injection: "parent agent not found in state"
// Without nodeAgents map: FindSubAgent("member-1") fails because Info().Name is "key-a1"
func TestGraphAgent_AgentNode_ParentAgentInjection(t *testing.T) {
	fakeAgent := &okSubAgent{name: "key-a1"}
	resolver := stubAgentResolver{agents: map[string]trpcagent.Agent{
		"key-a1": fakeAgent,
	}}

	cfg := GraphBuildConfig{
		EntryPoint:  "member-1",
		FinishPoint: "member-1",
		StateFields: []StateFieldDef{{Name: "messages", Type: "[]any", Reducer: ReducerAppend}},
		Nodes: []biz.NodeDef{
			{ID: "member-1", Type: "agent", AgentName: "key-a1"},
		},
	}

	g, subAgents, nodeAgents, err := BuildStateGraphWithNodeAgents(
		context.Background(), cfg, &GraphNodeResolverSet{Agents: resolver}, nil,
	)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}

	graphAgent, err := NewGraphAgentWithNodeAgents(
		"test-graph", g, false, nodeAgents, subAgents...,
	)
	if err != nil {
		t.Fatalf("create graph agent: %v", err)
	}

	ch, err := graphAgent.Run(
		context.Background(),
		&trpcagent.Invocation{InvocationID: "inv-test"},
	)
	if err != nil {
		t.Fatalf("graph run: %v", err)
	}

	var done bool
	for ev := range ch {
		if ev.Done {
			done = true
		}
	}
	if !done {
		t.Fatal("graph did not complete; expected Done event")
	}
}

// TestGraphAgent_FindSubAgent_ByNodeID verifies that FindSubAgent resolves
// agents by node ID when the nodeAgents map is populated, even when the
// agent's Info().Name differs from the node ID.
func TestGraphAgent_FindSubAgent_ByNodeID(t *testing.T) {
	fakeAgent := &okSubAgent{name: "key-a1"}
	ga := &GraphAgent{
		name:      "test",
		subAgents: []trpcagent.Agent{fakeAgent},
		nodeAgents: map[string]trpcagent.Agent{
			"member-1": fakeAgent,
		},
	}

	// Lookup by node ID should succeed via nodeAgents map.
	got := ga.FindSubAgent("member-1")
	if got == nil {
		t.Fatal("FindSubAgent(member-1) returned nil; expected the fake agent")
	}
	if got.Info().Name != "key-a1" {
		t.Fatalf("FindSubAgent(member-1).Info().Name = %q; want %q", got.Info().Name, "key-a1")
	}

	// Lookup by agent_key should still work via Info().Name fallback.
	gotByKey := ga.FindSubAgent("key-a1")
	if gotByKey == nil {
		t.Fatal("FindSubAgent(key-a1) returned nil; expected the fake agent")
	}

	// Lookup by unknown name should return nil.
	if got := ga.FindSubAgent("nonexistent"); got != nil {
		t.Fatalf("FindSubAgent(nonexistent) returned non-nil: %v", got)
	}
}

// TestGraphAgent_Run_InjectsParentAgentState verifies that Run() injects
// StateKeyParentAgent into the initial state, which is required by the
// framework's targetAgentFromState function.
func TestGraphAgent_Run_InjectsParentAgentState(t *testing.T) {
	fakeAgent := &okSubAgent{name: "key-a1"}
	resolver := stubAgentResolver{agents: map[string]trpcagent.Agent{
		"key-a1": fakeAgent,
	}}

	cfg := GraphBuildConfig{
		EntryPoint:  "member-1",
		FinishPoint: "member-1",
		StateFields: []StateFieldDef{{Name: "messages", Type: "[]any", Reducer: ReducerAppend}},
		Nodes: []biz.NodeDef{
			{ID: "member-1", Type: "agent", AgentName: "key-a1"},
		},
	}

	g, subAgents, nodeAgents, err := BuildStateGraphWithNodeAgents(
		context.Background(), cfg, &GraphNodeResolverSet{Agents: resolver}, nil,
	)
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}

	graphAgent, err := NewGraphAgentWithNodeAgents(
		"test-graph", g, false, nodeAgents, subAgents...,
	)
	if err != nil {
		t.Fatalf("create graph agent: %v", err)
	}

	_, err = graphAgent.Run(
		context.Background(),
		&trpcagent.Invocation{InvocationID: "inv-test"},
	)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "parent agent not found in state") {
			t.Fatalf("Run() failed with parent agent injection bug: %v", err)
		}
		if strings.Contains(errStr, "sub-agent") && strings.Contains(errStr, "not found") {
			t.Fatalf("Run() failed with sub-agent lookup bug: %v", err)
		}
		// Other errors are acceptable for this test (e.g. agent execution issues).
	}
}
