package graph

import (
	"context"
	"fmt"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcgraphcheckpoint "trpc.group/trpc-go/trpc-agent-go/graph/checkpoint/inmemory"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type NodeDef struct {
	ID              string
	Func            trpcgraph.NodeFunc
	InterruptBefore bool
	InterruptAfter  bool
}

type EdgeDef struct {
	From string
	To   string
}

type ConditionalEdgeDef struct {
	From     string
	CondFunc any
	PathMap  map[string]string
}

type GraphBuildConfig struct {
	Nodes            []NodeDef
	Edges            []EdgeDef
	ConditionalEdges []ConditionalEdgeDef
	EntryPoint       string
	FinishPoint      string
	EnableCheckpoint bool
	InterruptBefore  []string
	InterruptAfter   []string
}

func BuildStateGraph(cfg GraphBuildConfig) (*trpcgraph.Graph, error) {
	if len(cfg.Nodes) == 0 {
		return nil, fmt.Errorf("graph: at least one node required")
	}
	if cfg.EntryPoint == "" {
		return nil, fmt.Errorf("graph: entry point required")
	}

	schema := trpcgraph.NewStateSchema()
	sg := trpcgraph.NewStateGraph(schema)

	for _, n := range cfg.Nodes {
		opts := []trpcgraph.Option{}
		if n.InterruptBefore {
			opts = append(opts, trpcgraph.WithInterruptBefore())
		}
		if n.InterruptAfter {
			opts = append(opts, trpcgraph.WithInterruptAfter())
		}
		sg.AddNode(n.ID, n.Func, opts...)
	}

	sg.AddEdge(trpcgraph.Start, cfg.EntryPoint)

	for _, e := range cfg.Edges {
		sg.AddEdge(e.From, e.To)
	}

	for _, ce := range cfg.ConditionalEdges {
		sg.AddConditionalEdges(ce.From, ce.CondFunc, ce.PathMap)
	}

	if cfg.FinishPoint != "" {
		sg.AddEdge(cfg.FinishPoint, trpcgraph.End)
	}

	if len(cfg.InterruptBefore) > 0 {
		sg.WithInterruptBeforeNodes(cfg.InterruptBefore...)
	}
	if len(cfg.InterruptAfter) > 0 {
		sg.WithInterruptAfterNodes(cfg.InterruptAfter...)
	}

	return sg.Compile()
}

type GraphAgent struct {
	graph    *trpcgraph.Graph
	executor *trpcgraph.Executor
	name     string
}

var _ trpcagent.Agent = (*GraphAgent)(nil)

func NewGraphAgent(name string, g *trpcgraph.Graph, enableCheckpoint bool) (*GraphAgent, error) {
	var execOpts []trpcgraph.ExecutorOption
	if enableCheckpoint {
		saver := trpcgraphcheckpoint.NewSaver()
		execOpts = append(execOpts, trpcgraph.WithCheckpointSaver(saver))
	}
	exec, err := trpcgraph.NewExecutor(g, execOpts...)
	if err != nil {
		return nil, fmt.Errorf("graph agent: %w", err)
	}
	return &GraphAgent{
		graph:    g,
		executor: exec,
		name:     name,
	}, nil
}

func (a *GraphAgent) Run(ctx context.Context, inv *trpcagent.Invocation) (<-chan *trpcevent.Event, error) {
	return a.executor.Execute(ctx, nil, inv)
}

func (a *GraphAgent) Tools() []trpctool.Tool {
	return nil
}

func (a *GraphAgent) Info() trpcagent.Info {
	return trpcagent.Info{
		Name:        a.name,
		Description: "graph workflow agent",
	}
}

func (a *GraphAgent) SubAgents() []trpcagent.Agent {
	return nil
}

func (a *GraphAgent) FindSubAgent(name string) trpcagent.Agent {
	return nil
}
