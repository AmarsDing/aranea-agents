package graph

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	graphtrpc "aranea-agents/internal/graph/trpc"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"

	"github.com/google/uuid"
)

type trpcGraphRuntime struct {
	agent     *graphtrpc.GraphAgent
	graph     *trpcgraph.Graph
	lineageID string
	eventBus  event.Bus
	sessionID string
	graphID   string
	execID    string
}

var _ biz.GraphRuntime = (*trpcGraphRuntime)(nil)

func (r *trpcGraphRuntime) Run(ctx context.Context, initialState map[string]any) (<-chan biz.GraphRuntimeEvent, error) {
	lineageID := r.lineageID
	if lineageID == "" {
		lineageID = uuid.New().String()
		r.lineageID = lineageID
	}

	runtimeState := trpcgraph.CheckpointRef{
		LineageID:    lineageID,
		CheckpointID: "",
	}.ToRuntimeState()

	for k, v := range initialState {
		runtimeState[k] = v
	}

	inv := &trpcagent.Invocation{
		RunOptions: trpcagent.RunOptions{
			RuntimeState: runtimeState,
		},
	}

	eventCh, err := r.agent.Run(ctx, inv)
	if err != nil {
		return nil, fmt.Errorf("graph runtime run: %w", err)
	}

	out := make(chan biz.GraphRuntimeEvent, 64)
	go func() {
		defer close(out)
		for e := range eventCh {
			runtimeEvt := convertTrpcEvent(e, r.eventBus, r.sessionID, r.graphID, r.execID)
			out <- runtimeEvt
		}
	}()

	return out, nil
}

func (r *trpcGraphRuntime) Resume(ctx context.Context, lineageID string, resumeValue map[string]any) (<-chan biz.GraphRuntimeEvent, error) {
	r.lineageID = lineageID
	runtimeState := trpcgraph.CheckpointRef{
		LineageID:    lineageID,
		Namespace:    "",
		CheckpointID: "",
	}.ToRuntimeState()

	if resumeValue != nil {
		runtimeState[trpcgraph.ResumeChannel] = resumeValue
	}

	inv := &trpcagent.Invocation{
		RunOptions: trpcagent.RunOptions{
			RuntimeState: runtimeState,
		},
	}

	eventCh, err := r.agent.Run(ctx, inv)
	if err != nil {
		return nil, fmt.Errorf("graph runtime resume: %w", err)
	}

	out := make(chan biz.GraphRuntimeEvent, 64)
	go func() {
		defer close(out)
		for e := range eventCh {
			runtimeEvt := convertTrpcEvent(e, r.eventBus, r.sessionID, r.graphID, r.execID)
			out <- runtimeEvt
		}
	}()

	return out, nil
}

func (r *trpcGraphRuntime) Cancel() error {
	return nil
}

func (r *trpcGraphRuntime) TimeTravelGetState(ctx context.Context, lineageID, checkpointID, namespace string) (any, error) {
	tt, err := r.agent.TimeTravel()
	if err != nil {
		return nil, fmt.Errorf("time travel not available: %w", err)
	}
	ref := trpcgraph.CheckpointRef{
		LineageID:    lineageID,
		Namespace:    namespace,
		CheckpointID: checkpointID,
	}
	return tt.GetState(ctx, ref)
}

func (r *trpcGraphRuntime) TimeTravelHistory(ctx context.Context, lineageID, namespace string, limit int) (any, error) {
	tt, err := r.agent.TimeTravel()
	if err != nil {
		return nil, fmt.Errorf("time travel not available: %w", err)
	}
	return tt.History(ctx, lineageID, namespace, limit)
}

func (r *trpcGraphRuntime) TimeTravelEditState(ctx context.Context, lineageID, checkpointID, namespace string, patch map[string]any) (any, error) {
	tt, err := r.agent.TimeTravel()
	if err != nil {
		return nil, fmt.Errorf("time travel not available: %w", err)
	}
	base := trpcgraph.CheckpointRef{
		LineageID:    lineageID,
		Namespace:    namespace,
		CheckpointID: checkpointID,
	}
	return tt.EditState(ctx, base, patch)
}

func (r *trpcGraphRuntime) ListCheckpoints(ctx context.Context, lineageID, namespace string, limit int) (any, error) {
	tt, err := r.agent.TimeTravel()
	if err != nil {
		return nil, fmt.Errorf("time travel not available: %w", err)
	}
	return tt.History(ctx, lineageID, namespace, limit)
}

func (r *trpcGraphRuntime) GetLineageID() string {
	return r.lineageID
}

func convertTrpcEvent(e *trpcevent.Event, bus event.Bus, sessionID, graphID, execID string) biz.GraphRuntimeEvent {
	var bridge *graphtrpc.EventBridge
	if bus != nil {
		bridge = graphtrpc.NewEventBridge(bus, sessionID, graphID, execID)
	}

	if bridge != nil {
		env := bridge.ConvertEvent(e)
		if env != nil {
			bus.Publish(context.Background(), *env)
		}
	}

	runtimeEvt := biz.GraphRuntimeEvent{
		RawEvent: e,
	}

	switch e.Object {
	case trpcgraph.ObjectTypeGraphNodeStart:
		meta := graphtrpc.ExtractNodeMeta(e)
		runtimeEvt.Type = biz.DomainEventGraphNodeStart
		runtimeEvt.NodeID = meta.NodeID
	case trpcgraph.ObjectTypeGraphNodeComplete:
		meta := graphtrpc.ExtractNodeMeta(e)
		runtimeEvt.Type = biz.DomainEventGraphNodeEnd
		runtimeEvt.NodeID = meta.NodeID
	case trpcgraph.ObjectTypeGraphNodeError:
		meta := graphtrpc.ExtractNodeMeta(e)
		runtimeEvt.Type = biz.DomainEventGraphNodeError
		runtimeEvt.NodeID = meta.NodeID
		runtimeEvt.Error = meta.Error
	case trpcgraph.ObjectTypeGraphCheckpointInterrupt:
		runtimeEvt.Type = biz.DomainEventGraphInterrupt
	}

	return runtimeEvt
}

type trpcGraphBuilderFactory struct {
	registry     *graphtrpc.Registry
	saver        trpcgraph.CheckpointSaver
	eventBus     event.Bus
	agentChecker biz.AgentExistenceCheckerFunc
}

var _ biz.GraphBuilderFactory = (*trpcGraphBuilderFactory)(nil)

func NewGraphBuilderFactory(registry *graphtrpc.Registry, saver trpcgraph.CheckpointSaver, eventBus event.Bus, agentChecker biz.AgentExistenceCheckerFunc) biz.GraphBuilderFactory {
	return &trpcGraphBuilderFactory{
		registry:     registry,
		saver:        saver,
		eventBus:     eventBus,
		agentChecker: agentChecker,
	}
}

func (f *trpcGraphBuilderFactory) BuildAndRun(ctx context.Context, cfg graphtrpc.GraphBuildConfig, sessionID, graphID, execID string, initialState map[string]any) (biz.GraphRuntime, <-chan biz.GraphRuntimeEvent, error) {
	g, _, err := graphtrpc.BuildStateGraphWithRegistry(cfg, f.registry)
	if err != nil {
		return nil, nil, err
	}

	name := cfg.EntryPoint
	graphAgent, err := f.createAgent(name, g, cfg.EnableCheckpoint, cfg.ExecutionEngine)
	if err != nil {
		return nil, nil, err
	}

	runtime := &trpcGraphRuntime{
		agent:     graphAgent,
		graph:     g,
		eventBus:  f.eventBus,
		sessionID: sessionID,
		graphID:   graphID,
		execID:    execID,
	}

	eventCh, err := runtime.Run(ctx, initialState)
	if err != nil {
		return nil, nil, err
	}

	return runtime, eventCh, nil
}

func (f *trpcGraphBuilderFactory) BuildAndResume(ctx context.Context, cfg graphtrpc.GraphBuildConfig, sessionID, graphID, execID, lineageID string, resumeValue map[string]any) (biz.GraphRuntime, <-chan biz.GraphRuntimeEvent, error) {
	g, _, err := graphtrpc.BuildStateGraphWithRegistry(cfg, f.registry)
	if err != nil {
		return nil, nil, err
	}

	name := cfg.EntryPoint
	graphAgent, err := f.createAgent(name, g, cfg.EnableCheckpoint, cfg.ExecutionEngine)
	if err != nil {
		return nil, nil, err
	}

	runtime := &trpcGraphRuntime{
		agent:     graphAgent,
		graph:     g,
		lineageID: lineageID,
		eventBus:  f.eventBus,
		sessionID: sessionID,
		graphID:   graphID,
		execID:    execID,
	}

	eventCh, err := runtime.Resume(ctx, lineageID, resumeValue)
	if err != nil {
		return nil, nil, err
	}

	return runtime, eventCh, nil
}

func (f *trpcGraphBuilderFactory) Visualize(ctx context.Context, cfg graphtrpc.GraphBuildConfig) (any, error) {
	g, _, err := graphtrpc.BuildStateGraphWithRegistry(cfg, f.registry)
	if err != nil {
		return nil, fmt.Errorf("build state graph for visualization: %w", err)
	}

	dot := g.DOT()
	vg := graphtrpc.ParseDOTToVisualGraph(dot, cfg.Nodes, cfg.ConditionalEdges)

	startEnd := graphtrpc.BuildStartEndNodes()
	allNodes := make([]graphtrpc.VisualGraphNode, 0, len(vg.Nodes)+2)
	allNodes = append(allNodes, startEnd...)
	allNodes = append(allNodes, vg.Nodes...)
	vg.Nodes = allNodes

	return vg, nil
}

func (f *trpcGraphBuilderFactory) Validate(ctx context.Context, cfg graphtrpc.GraphBuildConfig) (any, error) {
	checker := graphtrpc.AgentExistenceChecker(f.agentChecker)
	return graphtrpc.ValidateGraph(&cfg, checker, f.registry), nil
}

func (f *trpcGraphBuilderFactory) ListTemplates() any {
	return graphtrpc.ListBuiltinTemplates()
}

func (f *trpcGraphBuilderFactory) GetTemplate(templateID string) (any, bool) {
	tmpl := graphtrpc.GetBuiltinTemplate(templateID)
	if tmpl == nil {
		return nil, false
	}
	return *tmpl, true
}

func (f *trpcGraphBuilderFactory) TemplateToDef(template any, name, description string) *biz.GraphDefinition {
	tmpl, ok := template.(graphtrpc.GraphTemplate)
	if !ok {
		return nil
	}
	cfg := graphtrpc.TemplateToBuildConfig(tmpl)
	return &biz.GraphDefinition{
		Name:             name,
		Description:      description,
		StateFields:      cfg.StateFields,
		Nodes:            cfg.Nodes,
		Edges:            cfg.Edges,
		ConditionalEdges: cfg.ConditionalEdges,
		EntryPoint:       cfg.EntryPoint,
		FinishPoint:      cfg.FinishPoint,
		EnableCheckpoint: cfg.EnableCheckpoint,
		ExecutionEngine:  cfg.ExecutionEngine,
	}
}

func (f *trpcGraphBuilderFactory) AgentExists(agentID string) bool {
	if f.agentChecker == nil {
		return false
	}
	return f.agentChecker(agentID)
}

func (f *trpcGraphBuilderFactory) FindNodeDef(cfg graphtrpc.GraphBuildConfig, nodeID string) *biz.NodeDefInfo {
	for i := range cfg.Nodes {
		if cfg.Nodes[i].ID == nodeID {
			return &biz.NodeDefInfo{
				RequiredRole:             cfg.Nodes[i].RequiredRole,
				AssignmentMode:           cfg.Nodes[i].AssignmentMode,
				AssignmentStrategy:       cfg.Nodes[i].AssignmentStrategy,
				ReviewerAgent:            cfg.Nodes[i].ReviewerAgent,
				ReviewRules:              cfg.Nodes[i].ReviewRules,
				TimeoutSeconds:           cfg.Nodes[i].TimeoutSeconds,
				HeartbeatIntervalSeconds: cfg.Nodes[i].HeartbeatIntervalSeconds,
				EnableLeaseExtension:     cfg.Nodes[i].EnableLeaseExtension,
			}
		}
	}
	return nil
}

func (f *trpcGraphBuilderFactory) createAgent(name string, g *trpcgraph.Graph, enableCheckpoint bool, ee graphtrpc.ExecutionEngineType) (*graphtrpc.GraphAgent, error) {
	if f.saver != nil && enableCheckpoint {
		return graphtrpc.NewGraphAgentWithSaver(name, g, f.saver, ee)
	} else if ee != "" && ee != graphtrpc.EngineBSP {
		return graphtrpc.NewGraphAgentWithEngine(name, g, enableCheckpoint, ee)
	}
	return graphtrpc.NewGraphAgent(name, g, enableCheckpoint)
}
