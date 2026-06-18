package graph

import (
	"context"
	"fmt"
	"reflect"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcgraphcheckpoint "trpc.group/trpc-go/trpc-agent-go/graph/checkpoint/inmemory"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type ReducerType = biz.ReducerType
type ExecutionEngineType = biz.ExecutionEngineType

const (
	ReducerDefault = biz.ReducerDefault
	ReducerAppend  = biz.ReducerAppend
	ReducerCover   = biz.ReducerCover
	ReducerMerge   = biz.ReducerMerge
)

const (
	EngineBSP = biz.EngineBSP
	EngineDAG = biz.EngineDAG
)

const maxSubgraphDepth = 10

type StateFieldDef = biz.StateFieldDef

type EdgeDef = biz.EdgeDef

type ConditionalEdgeDef struct {
	biz.ConditionalEdgeDef
	CondFunc any
}

type NodeDef struct {
	biz.NodeDef
	Func trpcgraph.NodeFunc
}

type SubgraphDef struct {
	biz.SubgraphDef
	InputMapper  trpcgraph.SubgraphInputMapper
	OutputMapper trpcgraph.SubgraphOutputMapper
}

// GraphBuildConfig is a type alias for biz.GraphBuildConfig.
// Function pointers (NodeFunc, CondFunc, InputMapper, OutputMapper) are
// resolved separately in resolvedBuildConfig by the Registry.
type GraphBuildConfig = biz.GraphBuildConfig

// resolvedBuildConfig holds a GraphBuildConfig with all function pointers
// resolved by the Registry. The trpc layer uses this internally for building.
type resolvedBuildConfig struct {
	cfg       GraphBuildConfig
	nodes     []NodeDef
	condEdges []ConditionalEdgeDef
	subs      []SubgraphDef
	subRbcs   map[string]*resolvedBuildConfig // subgraphID → resolved sub-config
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

func BuildStateGraph(ctx context.Context, cfg GraphBuildConfig, deps *GraphNodeResolverSet) (*trpcgraph.Graph, error) {
	g, _, err := BuildStateGraphWithAgents(ctx, cfg, nil, nil)
	return g, err
}

func BuildStateGraphWithRegistry(ctx context.Context, cfg GraphBuildConfig, reg *Registry, deps *GraphNodeResolverSet) (*trpcgraph.Graph, []trpcagent.Agent, error) {
	return BuildStateGraphWithRegistryAndLogger(ctx, cfg, reg, deps, nil)
}

func BuildStateGraphWithRegistryAndLogger(ctx context.Context, cfg GraphBuildConfig, reg *Registry, deps *GraphNodeResolverSet, lg loggateway.Logger) (*trpcgraph.Graph, []trpcagent.Agent, error) {
	// Defensive shallow copy: duplicate slices so caller's data is not mutated.
	local := cfg
	local.Nodes = append([]biz.NodeDef(nil), cfg.Nodes...)
	local.Edges = append([]EdgeDef(nil), cfg.Edges...)
	local.ConditionalEdges = append([]biz.ConditionalEdgeDef(nil), cfg.ConditionalEdges...)
	local.Subgraphs = append([]biz.SubgraphDef(nil), cfg.Subgraphs...)
	local.StateFields = append([]StateFieldDef(nil), cfg.StateFields...)
	local.InterruptBefore = append([]string(nil), cfg.InterruptBefore...)
	local.InterruptAfter = append([]string(nil), cfg.InterruptAfter...)
	var rbc *resolvedBuildConfig
	if reg != nil {
		var err error
		rbc, err = reg.ResolveBuildConfig(local)
		if err != nil {
			return nil, nil, err
		}
	} else {
		rbc = resolvedBuildConfigFromCfg(local)
	}
	return buildFromResolved(ctx, rbc, deps, lg, 0)
}

func BuildStateGraphWithAgents(ctx context.Context, cfg GraphBuildConfig, deps *GraphNodeResolverSet, lg loggateway.Logger) (*trpcgraph.Graph, []trpcagent.Agent, error) {
	rbc := resolvedBuildConfigFromCfg(cfg)
	return buildFromResolved(ctx, rbc, deps, lg, 0)
}

// resolvedBuildConfigFromCfg wraps biz-layer defs into trpc-layer defs without resolving
// function pointers. Func/CondFunc/InputMapper/OutputMapper will be nil.
func resolvedBuildConfigFromCfg(cfg GraphBuildConfig) *resolvedBuildConfig {
	rbc := &resolvedBuildConfig{cfg: cfg}
	rbc.nodes = make([]NodeDef, len(cfg.Nodes))
	for i, n := range cfg.Nodes {
		rbc.nodes[i] = NodeDef{NodeDef: n}
	}
	rbc.condEdges = make([]ConditionalEdgeDef, len(cfg.ConditionalEdges))
	for i, ce := range cfg.ConditionalEdges {
		rbc.condEdges[i] = ConditionalEdgeDef{ConditionalEdgeDef: ce}
	}
	rbc.subs = make([]SubgraphDef, len(cfg.Subgraphs))
	for i, s := range cfg.Subgraphs {
		rbc.subs[i] = SubgraphDef{SubgraphDef: s}
	}
	return rbc
}

// BuildFromResolved builds a graph from a pre-resolved config.
// This is the public entry point for callers that construct resolvedBuildConfig directly.
func BuildFromResolved(ctx context.Context, rbc *resolvedBuildConfig, deps *GraphNodeResolverSet, lg loggateway.Logger) (*trpcgraph.Graph, []trpcagent.Agent, error) {
	return buildFromResolved(ctx, rbc, deps, lg, 0)
}

func buildFromResolved(ctx context.Context, rbc *resolvedBuildConfig, deps *GraphNodeResolverSet, lg loggateway.Logger, depth int) (*trpcgraph.Graph, []trpcagent.Agent, error) {
	if depth >= maxSubgraphDepth {
		return nil, nil, apierror.BadRequest(apierror.DomainGraph, fmt.Sprintf("graph: subgraph nesting depth exceeds limit (%d)", maxSubgraphDepth))
	}
	cfg := rbc.cfg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	if len(cfg.Nodes) == 0 && len(cfg.Subgraphs) == 0 {
		return nil, nil, apierror.BadRequest(apierror.DomainGraph, "graph: at least one node required")
	}
	if cfg.EntryPoint == "" {
		return nil, nil, apierror.BadRequest(apierror.DomainGraph, "graph: entry point required")
	}

	lg.Info("graph.BuildStateGraph started",
		loggateway.StepID("graph.build.start"),
		loggateway.Str("entry_point", cfg.EntryPoint),
		loggateway.Int("nodes", len(cfg.Nodes)),
		loggateway.Int("edges", len(cfg.Edges)),
		loggateway.Bool("checkpoint", cfg.EnableCheckpoint),
	)

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

	for _, n := range rbc.nodes {
		extras, err := wireNode(ctx, sg, n, deps, lg)
		if err != nil {
			return nil, nil, err
		}
		allAgents = append(allAgents, extras...)
	}

	for _, sub := range rbc.subs {
		subRbc := rbc.subRbcs[sub.ID]
		subGraph, subAgents, err := buildFromResolved(ctx, subRbc, deps, lg, depth+1)
		if err != nil {
			return nil, nil, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("graph: subgraph %q build failed: %v", sub.ID, err))
		}
		subAgent, err := NewGraphAgent(sub.ID, subGraph, subRbc.cfg.EnableCheckpoint, subAgents...)
		if err != nil {
			return nil, nil, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("graph: subgraph %q agent failed: %v", sub.ID, err))
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

	for _, ce := range rbc.condEdges {
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
		lg.Error("graph.BuildStateGraph compile failed",
			loggateway.StepID("graph.build.compile_fail"),
			loggateway.Err(err),
		)
		return nil, nil, err
	}
	lg.Info("graph.BuildStateGraph completed",
		loggateway.StepID("graph.build.complete"),
		loggateway.Str("entry_point", cfg.EntryPoint),
	)
	return compiled, allAgents, nil
}

type GraphAgent struct {
	graph     *trpcgraph.Graph
	executor  *trpcgraph.Executor
	name      string
	saver     trpcgraph.CheckpointSaver
	subAgents []trpcagent.Agent
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
		return nil, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("graph agent: %v", err))
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
		return nil, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("graph agent: %v", err))
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
		return nil, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("graph agent: %v", err))
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
	// Merge RuntimeState into the initial state so that runtime-injected
	// values (e.g. NodeCallbacks via StateKeyNodeCallbacks) are visible to
	// the executor. This mirrors the framework's graphagent.createInitialState
	// behavior. Without this merge, RuntimeState would be ignored because
	// Execute receives a separate initialState parameter.
	initialState := trpcgraph.State{}
	if inv != nil && inv.RunOptions.RuntimeState != nil {
		for k, v := range inv.RunOptions.RuntimeState {
			initialState[k] = v
		}
	}
	return a.executor.Execute(ctx, initialState, inv)
}

func (a *GraphAgent) Tools() []trpctool.Tool {
	return nil
}

func (a *GraphAgent) Info() trpcagent.Info {
	return trpcagent.Info{
		Name:        a.name,
		Description: "图工作流代理",
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
