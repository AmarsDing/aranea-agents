package graph

import (
	"context"
	"testing"

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
		Nodes: []NodeDef{
			{ID: "llm1", Type: "llm", Instruction: "say hi", ModelName: "openai/gpt-4o-mini"},
		},
	}
	_, _, err := BuildStateGraphWithAgents(context.Background(), cfg, &BuildDeps{Models: stubDeps{}}, nil)
	if err != nil {
		t.Fatalf("build llm graph: %v", err)
	}
}

func TestBuildStateGraph_FunctionNode(t *testing.T) {
	cfg := GraphBuildConfig{
		EntryPoint:  "fn",
		FinishPoint: "fn",
		StateFields: []StateFieldDef{{Name: "messages", Type: "[]any", Reducer: ReducerAppend}},
		Nodes: []NodeDef{
			{ID: "fn", Type: "function", Func: PassthroughNodeFunc("fn")},
		},
	}
	_, _, err := BuildStateGraphWithAgents(context.Background(), cfg, nil, nil)
	if err != nil {
		t.Fatalf("build function graph: %v", err)
	}
}
