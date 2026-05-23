package graph

import (
	"context"
	"fmt"
	"reflect"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcgraphcheckpoint "trpc.group/trpc-go/trpc-agent-go/graph/checkpoint/inmemory"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type ReducerType string

const (
	ReducerDefault ReducerType = "default"
	ReducerAppend  ReducerType = "append"
	ReducerCover   ReducerType = "cover"
	ReducerMerge   ReducerType = "merge"
)

type StateFieldDef struct {
	Name            string
	Type            string
	Reducer         ReducerType
	DefaultValue    any
	Required        bool
	DisableDeepCopy bool
}

type ExecutionEngineType string

const (
	EngineBSP ExecutionEngineType = "bsp"
	EngineDAG ExecutionEngineType = "dag"
)

type SubgraphDef struct {
	ID              string
	GraphID         string
	BuildConfig     GraphBuildConfig
	InputMapper     trpcgraph.SubgraphInputMapper
	OutputMapper    trpcgraph.SubgraphOutputMapper
	InterruptBefore bool
	InterruptAfter  bool
}

type NodeDef struct {
	ID                       string
	FuncRef                  string
	Func                     trpcgraph.NodeFunc
	Type                     string
	Description              string
	Instruction              string
	ModelName                string
	ToolNames                []string
	AgentName                string
	InterruptBefore          bool
	InterruptAfter           bool
	Destinations             []string
	RequiredRole             string
	AssignmentMode           string
	AssignmentStrategy       string
	ReviewerAgent            string
	ReviewRules              string
	TimeoutSeconds           int
	HeartbeatIntervalSeconds int
	EnableLeaseExtension     bool
	RetryMaxAttempts         int
	FailureAction            string
	FallbackAgent            string
	InputMapperJSON          string
	OutputMapperJSON         string
	IsolatedMessages         bool
	InputFromLastResponse    bool
	CacheEnabled             bool
	CacheTTLSeconds          int
}

type EdgeDef struct {
	From string
	To   string
}

type ConditionalEdgeDef struct {
	From        string
	CondFuncRef string
	CondFunc    any
	PathMap     map[string]string
}

type GraphBuildConfig struct {
	Nodes            []NodeDef
	Edges            []EdgeDef
	ConditionalEdges []ConditionalEdgeDef
	Subgraphs        []SubgraphDef
	StateFields      []StateFieldDef
	EntryPoint       string
	FinishPoint      string
	EnableCheckpoint bool
	ExecutionEngine  ExecutionEngineType
	InterruptBefore  []string
	InterruptAfter   []string
	FailurePolicy    *biz.TeamFailurePolicy
}

func resolveReducer(rt ReducerType) trpcgraph.StateReducer {
	switch rt {
	case ReducerAppend:
		return trpcgraph.AppendReducer
	case ReducerCover:
		return trpcgraph.CoverReducer
	case ReducerMerge:
		return trpcgraph.MergeReducer
	default:
		return trpcgraph.DefaultReducer
	}
}

func resolveFieldType(typeName string) reflect.Type {
	switch typeName {
	case "string":
		return reflect.TypeOf("")
	case "int", "integer":
		return reflect.TypeOf(0)
	case "float", "float64":
		return reflect.TypeOf(0.0)
	case "bool", "boolean":
		return reflect.TypeOf(false)
	case "[]string":
		return reflect.TypeOf([]string{})
	case "[]int":
		return reflect.TypeOf([]int{})
	case "[]any":
		return reflect.TypeOf([]any{})
	case "map":
		return reflect.TypeOf(map[string]any{})
	default:
		return nil
	}
}

func BuildStateGraph(ctx context.Context, cfg GraphBuildConfig, deps *BuildDeps) (*trpcgraph.Graph, error) {
	g, _, err := BuildStateGraphWithAgents(ctx, cfg, deps)
	return g, err
}

func BuildStateGraphWithRegistry(ctx context.Context, cfg GraphBuildConfig, reg *Registry, deps *BuildDeps) (*trpcgraph.Graph, []trpcagent.Agent, error) {
	local := GraphBuildConfig{
		Nodes:            append([]NodeDef(nil), cfg.Nodes...),
		Edges:            append([]EdgeDef(nil), cfg.Edges...),
		ConditionalEdges: append([]ConditionalEdgeDef(nil), cfg.ConditionalEdges...),
		Subgraphs:        append([]SubgraphDef(nil), cfg.Subgraphs...),
		StateFields:      append([]StateFieldDef(nil), cfg.StateFields...),
		EntryPoint:       cfg.EntryPoint,
		FinishPoint:      cfg.FinishPoint,
		EnableCheckpoint: cfg.EnableCheckpoint,
		ExecutionEngine:  cfg.ExecutionEngine,
		InterruptBefore:  append([]string(nil), cfg.InterruptBefore...),
		InterruptAfter:   append([]string(nil), cfg.InterruptAfter...),
		FailurePolicy:    cfg.FailurePolicy,
	}
	if reg != nil {
		resolved, err := reg.ResolveBuildConfig(local)
		if err != nil {
			return nil, nil, err
		}
		local = resolved
	}
	return BuildStateGraphWithAgents(ctx, local, deps)
}

func BuildStateGraphWithAgents(ctx context.Context, cfg GraphBuildConfig, deps *BuildDeps) (*trpcgraph.Graph, []trpcagent.Agent, error) {
	if len(cfg.Nodes) == 0 && len(cfg.Subgraphs) == 0 {
		return nil, nil, fmt.Errorf("graph: at least one node required")
	}
	if cfg.EntryPoint == "" {
		return nil, nil, fmt.Errorf("graph: entry point required")
	}

	var allAgents []trpcagent.Agent

	schema := trpcgraph.NewStateSchema()
	for _, sf := range cfg.StateFields {
		field := trpcgraph.StateField{
			Reducer:         resolveReducer(sf.Reducer),
			Required:        sf.Required,
			DisableDeepCopy: sf.DisableDeepCopy,
		}
		if sf.Type != "" {
			field.Type = resolveFieldType(sf.Type)
		}
		if sf.DefaultValue != nil {
			dv := sf.DefaultValue
			field.Default = func() any { return dv }
		}
		schema.AddField(sf.Name, field)
	}

	sg := trpcgraph.NewStateGraph(schema)

	for _, n := range cfg.Nodes {
		extras, err := wireNode(ctx, sg, n, deps, cfg.FailurePolicy)
		if err != nil {
			return nil, nil, err
		}
		allAgents = append(allAgents, extras...)
	}

	for _, sub := range cfg.Subgraphs {
		subGraph, subAgents, err := BuildStateGraphWithAgents(ctx, sub.BuildConfig, deps)
		if err != nil {
			return nil, nil, fmt.Errorf("graph: subgraph %q build failed: %w", sub.ID, err)
		}
		subAgent, err := NewGraphAgent(sub.ID, subGraph, sub.BuildConfig.EnableCheckpoint)
		if err != nil {
			return nil, nil, fmt.Errorf("graph: subgraph %q agent failed: %w", sub.ID, err)
		}
		allAgents = append(allAgents, subAgent)
		allAgents = append(allAgents, subAgents...)
		opts := []trpcgraph.Option{}
		if sub.InputMapper != nil {
			opts = append(opts, trpcgraph.WithSubgraphInputMapper(sub.InputMapper))
		}
		if sub.OutputMapper != nil {
			opts = append(opts, trpcgraph.WithSubgraphOutputMapper(sub.OutputMapper))
		}
		if sub.InterruptBefore {
			opts = append(opts, trpcgraph.WithInterruptBefore())
		}
		if sub.InterruptAfter {
			opts = append(opts, trpcgraph.WithInterruptAfter())
		}
		sg.AddAgentNode(sub.ID, opts...)
	}

	sg.SetEntryPoint(cfg.EntryPoint)
	if cfg.FinishPoint != "" {
		sg.SetFinishPoint(cfg.FinishPoint)
	}

	for _, e := range cfg.Edges {
		sg.AddEdge(e.From, e.To)
	}

	for _, ce := range cfg.ConditionalEdges {
		sg.AddConditionalEdges(ce.From, ce.CondFunc, ce.PathMap)
	}

	if len(cfg.InterruptBefore) > 0 {
		sg.WithInterruptBeforeNodes(cfg.InterruptBefore...)
	}
	if len(cfg.InterruptAfter) > 0 {
		sg.WithInterruptAfterNodes(cfg.InterruptAfter...)
	}

	compiled, err := sg.Compile()
	if err != nil {
		return nil, nil, err
	}
	return compiled, allAgents, nil
}

type GraphAgent struct {
	graph      *trpcgraph.Graph
	executor   *trpcgraph.Executor
	name       string
	saver      trpcgraph.CheckpointSaver
	subAgents  []trpcagent.Agent
}

var _ trpcagent.Agent = (*GraphAgent)(nil)

func NewGraphAgent(name string, g *trpcgraph.Graph, enableCheckpoint bool, subAgents ...trpcagent.Agent) (*GraphAgent, error) {
	return NewGraphAgentWithSubAgents(name, g, enableCheckpoint, nil, subAgents)
}

func NewGraphAgentWithSubAgents(name string, g *trpcgraph.Graph, enableCheckpoint bool, saver trpcgraph.CheckpointSaver, subAgents []trpcagent.Agent) (*GraphAgent, error) {
	var execOpts []trpcgraph.ExecutorOption
	if enableCheckpoint && saver == nil {
		saver = trpcgraphcheckpoint.NewSaver()
	}
	if saver != nil {
		execOpts = append(execOpts, trpcgraph.WithCheckpointSaver(saver))
	}
	exec, err := trpcgraph.NewExecutor(g, execOpts...)
	if err != nil {
		return nil, fmt.Errorf("graph agent: %w", err)
	}
	return &GraphAgent{
		graph:     g,
		executor:  exec,
		name:      name,
		saver:     saver,
		subAgents: append([]trpcagent.Agent(nil), subAgents...),
	}, nil
}

func NewGraphAgentWithSaver(name string, g *trpcgraph.Graph, saver trpcgraph.CheckpointSaver, engine ExecutionEngineType, subAgents ...trpcagent.Agent) (*GraphAgent, error) {
	var execOpts []trpcgraph.ExecutorOption
	if saver != nil {
		execOpts = append(execOpts, trpcgraph.WithCheckpointSaver(saver))
	}
	switch engine {
	case EngineDAG:
		execOpts = append(execOpts, trpcgraph.WithExecutionEngine(trpcgraph.ExecutionEngineDAG))
	default:
		execOpts = append(execOpts, trpcgraph.WithExecutionEngine(trpcgraph.ExecutionEngineBSP))
	}
	exec, err := trpcgraph.NewExecutor(g, execOpts...)
	if err != nil {
		return nil, fmt.Errorf("graph agent: %w", err)
	}
	return &GraphAgent{
		graph:     g,
		executor:  exec,
		name:      name,
		saver:     saver,
		subAgents: append([]trpcagent.Agent(nil), subAgents...),
	}, nil
}

func NewGraphAgentWithEngine(name string, g *trpcgraph.Graph, enableCheckpoint bool, engine ExecutionEngineType, subAgents ...trpcagent.Agent) (*GraphAgent, error) {
	var execOpts []trpcgraph.ExecutorOption
	var saver trpcgraph.CheckpointSaver
	if enableCheckpoint {
		saver = trpcgraphcheckpoint.NewSaver()
		execOpts = append(execOpts, trpcgraph.WithCheckpointSaver(saver))
	}
	switch engine {
	case EngineDAG:
		execOpts = append(execOpts, trpcgraph.WithExecutionEngine(trpcgraph.ExecutionEngineDAG))
	default:
		execOpts = append(execOpts, trpcgraph.WithExecutionEngine(trpcgraph.ExecutionEngineBSP))
	}
	exec, err := trpcgraph.NewExecutor(g, execOpts...)
	if err != nil {
		return nil, fmt.Errorf("graph agent: %w", err)
	}
	return &GraphAgent{
		graph:     g,
		executor:  exec,
		name:      name,
		saver:     saver,
		subAgents: append([]trpcagent.Agent(nil), subAgents...),
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
	if len(a.subAgents) == 0 {
		return nil
	}
	return append([]trpcagent.Agent(nil), a.subAgents...)
}

func (a *GraphAgent) FindSubAgent(name string) trpcagent.Agent {
	for _, sub := range a.subAgents {
		if sub == nil {
			continue
		}
		if info := sub.Info(); info.Name == name {
			return sub
		}
	}
	return nil
}

func (a *GraphAgent) Graph() *trpcgraph.Graph {
	return a.graph
}

func (a *GraphAgent) Executor() *trpcgraph.Executor {
	return a.executor
}

func (a *GraphAgent) Saver() trpcgraph.CheckpointSaver {
	return a.saver
}

func (a *GraphAgent) TimeTravel() (*trpcgraph.TimeTravel, error) {
	return a.executor.TimeTravel()
}
