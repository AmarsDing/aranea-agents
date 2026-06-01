package adapter

import (
	"context"
	"fmt"
	"sync"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

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
	lg        loggateway.Logger

	cancelMu  sync.Mutex
	runCancel context.CancelFunc
}

var _ biz.GraphRuntime = (*trpcGraphRuntime)(nil)

func (r *trpcGraphRuntime) setRunCancel(cancel context.CancelFunc) {
	r.cancelMu.Lock()
	r.runCancel = cancel
	r.cancelMu.Unlock()
}

func (r *trpcGraphRuntime) clearRunCancel() {
	r.cancelMu.Lock()
	r.runCancel = nil
	r.cancelMu.Unlock()
}

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

	runCtx, cancel := context.WithCancel(ctx)
	r.setRunCancel(cancel)

	eventCh, err := r.agent.Run(runCtx, inv)
	if err != nil {
		r.clearRunCancel()
		r.lg.Error("graph runtime run failed",
			loggateway.StepID("system.graph.runtime_run_fail"),
			loggateway.Str("session_id", r.sessionID),
			loggateway.Str("graph_id", r.graphID),
			loggateway.Str("execution_id", r.execID),
			loggateway.Err(err),
		)
		return nil, kerrors.InternalServer("GRAPH", fmt.Sprintf("graph runtime run: %v", err))
	}

	out := make(chan biz.GraphRuntimeEvent, 64)
	safego.Go(ctx, "graph-event-bridge", func() {
		defer func() {
			r.clearRunCancel()
			close(out)
		}()
		for e := range eventCh {
			runtimeEvt := convertTrpcEvent(e, r.eventBus, r.sessionID, r.graphID, r.execID, r.lg)
			out <- runtimeEvt
		}
	})

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

	runCtx, cancel := context.WithCancel(ctx)
	r.setRunCancel(cancel)

	eventCh, err := r.agent.Run(runCtx, inv)
	if err != nil {
		r.clearRunCancel()
		r.lg.Error("graph runtime resume failed",
			loggateway.StepID("system.graph.runtime_resume_fail"),
			loggateway.Str("session_id", r.sessionID),
			loggateway.Str("graph_id", r.graphID),
			loggateway.Str("execution_id", r.execID),
			loggateway.Err(err),
		)
		return nil, kerrors.InternalServer("GRAPH", fmt.Sprintf("graph runtime resume: %v", err))
	}

	out := make(chan biz.GraphRuntimeEvent, 64)
	safego.Go(ctx, "graph-resume-bridge", func() {
		defer func() {
			r.clearRunCancel()
			close(out)
		}()
		for e := range eventCh {
			runtimeEvt := convertTrpcEvent(e, r.eventBus, r.sessionID, r.graphID, r.execID, r.lg)
			out <- runtimeEvt
		}
	})

	return out, nil
}

func (r *trpcGraphRuntime) Cancel() error {
	r.cancelMu.Lock()
	cancel := r.runCancel
	r.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (r *trpcGraphRuntime) TimeTravelGetState(ctx context.Context, lineageID, checkpointID, namespace string) (any, error) {
	tt, err := r.agent.TimeTravel()
	if err != nil {
		return nil, kerrors.InternalServer("GRAPH", fmt.Sprintf("time travel not available: %v", err))
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
		return nil, kerrors.InternalServer("GRAPH", fmt.Sprintf("time travel not available: %v", err))
	}
	return tt.History(ctx, lineageID, namespace, limit)
}

func (r *trpcGraphRuntime) TimeTravelEditState(ctx context.Context, lineageID, checkpointID, namespace string, patch map[string]any) (any, error) {
	tt, err := r.agent.TimeTravel()
	if err != nil {
		return nil, kerrors.InternalServer("GRAPH", fmt.Sprintf("time travel not available: %v", err))
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
		return nil, kerrors.InternalServer("GRAPH", fmt.Sprintf("time travel not available: %v", err))
	}
	return tt.History(ctx, lineageID, namespace, limit)
}

func (r *trpcGraphRuntime) GetLineageID() string {
	return r.lineageID
}

func convertTrpcEvent(e *trpcevent.Event, bus event.Bus, sessionID, graphID, execID string, lg loggateway.Logger) biz.GraphRuntimeEvent {
	var bridge *graphtrpc.EventBridge
	if bus != nil {
		bridge = graphtrpc.NewEventBridge(bus, sessionID, graphID, execID, nil)
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
		meta := graphtrpc.ExtractNodeMeta(e, lg)
		runtimeEvt.Type = biz.DomainEventGraphNodeStart
		runtimeEvt.NodeID = meta.NodeID
		runtimeEvt.StepNumber = meta.StepNumber
	case trpcgraph.ObjectTypeGraphNodeComplete:
		meta := graphtrpc.ExtractNodeMeta(e, lg)
		runtimeEvt.Type = biz.DomainEventGraphNodeEnd
		runtimeEvt.NodeID = meta.NodeID
		runtimeEvt.StepNumber = meta.StepNumber
	case trpcgraph.ObjectTypeGraphNodeError:
		meta := graphtrpc.ExtractNodeMeta(e, lg)
		runtimeEvt.Type = biz.DomainEventGraphNodeError
		runtimeEvt.NodeID = meta.NodeID
		runtimeEvt.Error = meta.Error
		runtimeEvt.StepNumber = meta.StepNumber
	case trpcgraph.ObjectTypeGraphCheckpointInterrupt:
		meta := graphtrpc.ExtractNodeMeta(e, lg)
		runtimeEvt.Type = biz.DomainEventGraphInterrupt
		runtimeEvt.NodeID = meta.NodeID
		runtimeEvt.StepNumber = meta.StepNumber
	}

	return runtimeEvt
}

type trpcGraphBuilderFactory struct {
	registry     *graphtrpc.Registry
	saver        trpcgraph.CheckpointSaver
	eventBus     event.Bus
	agentChecker biz.AgentExistenceCheckerFunc
	resolvers    graphtrpc.GraphNodeResolverSet
	lg           loggateway.Logger
}

var _ biz.GraphBuilderFactory = (*trpcGraphBuilderFactory)(nil)

func NewGraphBuilderFactory(
	registry *graphtrpc.Registry,
	saver trpcgraph.CheckpointSaver,
	eventBus event.Bus,
	agentChecker biz.AgentExistenceCheckerFunc,
	resolvers graphtrpc.GraphNodeResolverSet,
	lg loggateway.Logger,
) biz.GraphBuilderFactory {
	RegisterCriticLoopCondFunc(registry, DefaultCriticLoopThreshold)
	return &trpcGraphBuilderFactory{
		registry:     registry,
		saver:        saver,
		eventBus:     eventBus,
		agentChecker: agentChecker,
		resolvers:    resolvers,
		lg:           lg,
	}
}

func bizCfgToTrpc(cfg biz.GraphBuildConfig) graphtrpc.GraphBuildConfig {
	nodes := make([]graphtrpc.NodeDef, len(cfg.Nodes))
	for i, n := range cfg.Nodes {
		nodes[i] = graphtrpc.NodeDef{
			ID: n.ID, FuncRef: n.FuncRef, Type: n.Type, Description: n.Description,
			Instruction: n.Instruction, ModelName: n.ModelName, ToolNames: n.ToolNames,
			AgentName: n.AgentName, InterruptBefore: n.InterruptBefore, InterruptAfter: n.InterruptAfter,
			Destinations: n.Destinations, RequiredRole: n.RequiredRole, AssignmentMode: n.AssignmentMode,
			AssignmentStrategy: n.AssignmentStrategy, ReviewerAgent: n.ReviewerAgent, ReviewRules: n.ReviewRules,
			TimeoutSeconds: n.TimeoutSeconds, HeartbeatIntervalSeconds: n.HeartbeatIntervalSeconds,
			EnableLeaseExtension: n.EnableLeaseExtension,
			RetryMaxAttempts:     n.RetryMaxAttempts, FailureAction: n.FailureAction, FallbackAgent: n.FallbackAgent,
			InputMapperJSON: n.InputMapperJSON, OutputMapperJSON: n.OutputMapperJSON,
			IsolatedMessages: n.IsolatedMessages, InputFromLastResponse: n.InputFromLastResponse,
			CacheEnabled: n.CacheEnabled, CacheTTLSeconds: n.CacheTTLSeconds,
		}
	}
	edges := make([]graphtrpc.EdgeDef, len(cfg.Edges))
	for i, e := range cfg.Edges {
		edges[i] = graphtrpc.EdgeDef{From: e.From, To: e.To}
	}
	condEdges := make([]graphtrpc.ConditionalEdgeDef, len(cfg.ConditionalEdges))
	for i, ce := range cfg.ConditionalEdges {
		condEdges[i] = graphtrpc.ConditionalEdgeDef{From: ce.From, CondFuncRef: ce.CondFuncRef, PathMap: ce.PathMap}
	}
	stateFields := make([]graphtrpc.StateFieldDef, len(cfg.StateFields))
	for i, sf := range cfg.StateFields {
		stateFields[i] = graphtrpc.StateFieldDef{
			Name: sf.Name, Type: sf.Type, Reducer: graphtrpc.ReducerType(sf.Reducer),
			DefaultValue: sf.DefaultValue, Required: sf.Required, DisableDeepCopy: sf.DisableDeepCopy,
		}
	}
	subgraphs := make([]graphtrpc.SubgraphDef, len(cfg.Subgraphs))
	for i, s := range cfg.Subgraphs {
		subgraphs[i] = graphtrpc.SubgraphDef{
			ID: s.ID, GraphID: s.GraphID, BuildConfig: bizCfgToTrpc(s.BuildConfig),
			InterruptBefore: s.InterruptBefore, InterruptAfter: s.InterruptAfter,
		}
	}
	return graphtrpc.GraphBuildConfig{
		Nodes: nodes, Edges: edges, ConditionalEdges: condEdges, Subgraphs: subgraphs,
		StateFields: stateFields, EntryPoint: cfg.EntryPoint, FinishPoint: cfg.FinishPoint,
		EnableCheckpoint: cfg.EnableCheckpoint, ExecutionEngine: graphtrpc.ExecutionEngineType(cfg.ExecutionEngine),
		InterruptBefore: cfg.InterruptBefore, InterruptAfter: cfg.InterruptAfter,
		FailurePolicy: cfg.FailurePolicy,
	}
}

func trpcCfgToBiz(cfg graphtrpc.GraphBuildConfig) biz.GraphBuildConfig {
	nodes := make([]biz.NodeDef, len(cfg.Nodes))
	for i, n := range cfg.Nodes {
		nodes[i] = biz.NodeDef{
			ID: n.ID, FuncRef: n.FuncRef, Type: n.Type, Description: n.Description,
			Instruction: n.Instruction, ModelName: n.ModelName, ToolNames: n.ToolNames,
			AgentName: n.AgentName, InterruptBefore: n.InterruptBefore, InterruptAfter: n.InterruptAfter,
			Destinations: n.Destinations, RequiredRole: n.RequiredRole, AssignmentMode: n.AssignmentMode,
			AssignmentStrategy: n.AssignmentStrategy, ReviewerAgent: n.ReviewerAgent, ReviewRules: n.ReviewRules,
			TimeoutSeconds: n.TimeoutSeconds, HeartbeatIntervalSeconds: n.HeartbeatIntervalSeconds,
			EnableLeaseExtension: n.EnableLeaseExtension,
			RetryMaxAttempts:     n.RetryMaxAttempts, FailureAction: n.FailureAction, FallbackAgent: n.FallbackAgent,
			InputMapperJSON: n.InputMapperJSON, OutputMapperJSON: n.OutputMapperJSON,
			IsolatedMessages: n.IsolatedMessages, InputFromLastResponse: n.InputFromLastResponse,
			CacheEnabled: n.CacheEnabled, CacheTTLSeconds: n.CacheTTLSeconds,
		}
	}
	edges := make([]biz.EdgeDef, len(cfg.Edges))
	for i, e := range cfg.Edges {
		edges[i] = biz.EdgeDef{From: e.From, To: e.To}
	}
	condEdges := make([]biz.ConditionalEdgeDef, len(cfg.ConditionalEdges))
	for i, ce := range cfg.ConditionalEdges {
		condEdges[i] = biz.ConditionalEdgeDef{From: ce.From, CondFuncRef: ce.CondFuncRef, PathMap: ce.PathMap}
	}
	stateFields := make([]biz.StateFieldDef, len(cfg.StateFields))
	for i, sf := range cfg.StateFields {
		stateFields[i] = biz.StateFieldDef{
			Name: sf.Name, Type: sf.Type, Reducer: biz.ReducerType(sf.Reducer),
			DefaultValue: sf.DefaultValue, Required: sf.Required, DisableDeepCopy: sf.DisableDeepCopy,
		}
	}
	subgraphs := make([]biz.SubgraphDef, len(cfg.Subgraphs))
	for i, s := range cfg.Subgraphs {
		subgraphs[i] = biz.SubgraphDef{
			ID: s.ID, GraphID: s.GraphID, BuildConfig: trpcCfgToBiz(s.BuildConfig),
			InterruptBefore: s.InterruptBefore, InterruptAfter: s.InterruptAfter,
		}
	}
	return biz.GraphBuildConfig{
		Nodes: nodes, Edges: edges, ConditionalEdges: condEdges, Subgraphs: subgraphs,
		StateFields: stateFields, EntryPoint: cfg.EntryPoint, FinishPoint: cfg.FinishPoint,
		EnableCheckpoint: cfg.EnableCheckpoint, ExecutionEngine: biz.ExecutionEngineType(cfg.ExecutionEngine),
		InterruptBefore: cfg.InterruptBefore, InterruptAfter: cfg.InterruptAfter,
	}
}

func (f *trpcGraphBuilderFactory) buildRuntime(ctx context.Context, cfg biz.GraphBuildConfig, sessionID, graphID, execID, lineageID string) (*trpcGraphRuntime, error) {
	trpcCfg := bizCfgToTrpc(cfg)
	g, subAgents, err := graphtrpc.BuildStateGraphWithRegistryAndLogger(ctx, trpcCfg, f.registry, f.resolvers.ToBuildDepsPtr(), f.lg)
	if err != nil {
		return nil, err
	}
	name := cfg.EntryPoint
	graphAgent, err := f.createAgent(name, g, cfg.EnableCheckpoint, graphtrpc.ExecutionEngineType(cfg.ExecutionEngine), subAgents)
	if err != nil {
		return nil, err
	}
	return &trpcGraphRuntime{
		agent: graphAgent, graph: g, lineageID: lineageID, eventBus: f.eventBus,
		sessionID: sessionID, graphID: graphID, execID: execID, lg: f.lg,
	}, nil
}

func (f *trpcGraphBuilderFactory) BuildRuntime(ctx context.Context, cfg biz.GraphBuildConfig, sessionID, graphID, execID, lineageID string) (biz.GraphRuntime, error) {
	return f.buildRuntime(ctx, cfg, sessionID, graphID, execID, lineageID)
}

func (f *trpcGraphBuilderFactory) BuildAndRun(ctx context.Context, cfg biz.GraphBuildConfig, sessionID, graphID, execID string, initialState map[string]any) (biz.GraphRuntime, <-chan biz.GraphRuntimeEvent, error) {
	runtime, err := f.buildRuntime(ctx, cfg, sessionID, graphID, execID, "")
	if err != nil {
		return nil, nil, err
	}
	eventCh, err := runtime.Run(ctx, initialState)
	if err != nil {
		return nil, nil, err
	}
	return runtime, eventCh, nil
}

func (f *trpcGraphBuilderFactory) BuildAndResume(ctx context.Context, cfg biz.GraphBuildConfig, sessionID, graphID, execID, lineageID string, resumeValue map[string]any) (biz.GraphRuntime, <-chan biz.GraphRuntimeEvent, error) {
	runtime, err := f.buildRuntime(ctx, cfg, sessionID, graphID, execID, lineageID)
	if err != nil {
		return nil, nil, err
	}
	eventCh, err := runtime.Resume(ctx, lineageID, resumeValue)
	if err != nil {
		return nil, nil, err
	}
	return runtime, eventCh, nil
}

func (f *trpcGraphBuilderFactory) Visualize(ctx context.Context, cfg biz.GraphBuildConfig) (any, error) {
	trpcCfg := bizCfgToTrpc(cfg)
	g, _, err := graphtrpc.BuildStateGraphWithRegistryAndLogger(ctx, trpcCfg, f.registry, f.resolvers.ToBuildDepsPtr(), f.lg)
	if err != nil {
		return nil, kerrors.InternalServer("GRAPH", fmt.Sprintf("build state graph for visualization: %v", err))
	}
	dot := g.DOT()
	vg := graphtrpc.ParseDOTToVisualGraph(dot, trpcCfg.Nodes, trpcCfg.ConditionalEdges)
	startEnd := graphtrpc.BuildStartEndNodes()
	allNodes := make([]graphtrpc.VisualGraphNode, 0, len(vg.Nodes)+2)
	allNodes = append(allNodes, startEnd...)
	allNodes = append(allNodes, vg.Nodes...)
	vg.Nodes = allNodes
	return vg, nil
}

func validationResultToBiz(vr *graphtrpc.ValidationResult) *biz.GraphValidationResult {
	if vr == nil {
		return &biz.GraphValidationResult{}
	}
	out := &biz.GraphValidationResult{
		Errors:   make([]biz.GraphValidationIssue, 0, len(vr.Errors)),
		Warnings: make([]biz.GraphValidationIssue, 0, len(vr.Warnings)),
	}
	for _, e := range vr.Errors {
		out.Errors = append(out.Errors, biz.GraphValidationIssue{
			Code: string(e.Code), NodeID: e.NodeID, Field: e.Field, Message: e.Message,
		})
	}
	for _, w := range vr.Warnings {
		out.Warnings = append(out.Warnings, biz.GraphValidationIssue{
			Code: string(w.Code), NodeID: w.NodeID, Field: w.Field, Message: w.Message,
		})
	}
	return out
}

func (f *trpcGraphBuilderFactory) Validate(ctx context.Context, cfg biz.GraphBuildConfig) (*biz.GraphValidationResult, error) {
	trpcCfg := bizCfgToTrpc(cfg)
	checker := graphtrpc.AgentExistenceChecker(f.agentChecker)
	return validationResultToBiz(graphtrpc.ValidateGraph(ctx, &trpcCfg, checker, f.registry)), nil
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
	bizCfg := trpcCfgToBiz(cfg)
	return &biz.GraphDefinition{
		Name: name, Description: description, StateFields: bizCfg.StateFields,
		Nodes: bizCfg.Nodes, Edges: bizCfg.Edges, ConditionalEdges: bizCfg.ConditionalEdges,
		EntryPoint: bizCfg.EntryPoint, FinishPoint: bizCfg.FinishPoint,
		EnableCheckpoint: bizCfg.EnableCheckpoint, ExecutionEngine: bizCfg.ExecutionEngine,
	}
}

func (f *trpcGraphBuilderFactory) AgentExists(ctx context.Context, agentID string) bool {
	if f.agentChecker == nil {
		return false
	}
	return f.agentChecker(ctx, agentID)
}

func (f *trpcGraphBuilderFactory) FindNodeDef(cfg biz.GraphBuildConfig, nodeID string) *biz.NodeDefInfo {
	for i := range cfg.Nodes {
		if cfg.Nodes[i].ID == nodeID {
			return &biz.NodeDefInfo{
				RequiredRole: cfg.Nodes[i].RequiredRole, AssignmentMode: cfg.Nodes[i].AssignmentMode,
				AssignmentStrategy: cfg.Nodes[i].AssignmentStrategy, ReviewerAgent: cfg.Nodes[i].ReviewerAgent,
				ReviewRules: cfg.Nodes[i].ReviewRules, TimeoutSeconds: cfg.Nodes[i].TimeoutSeconds,
				HeartbeatIntervalSeconds: cfg.Nodes[i].HeartbeatIntervalSeconds,
				EnableLeaseExtension:     cfg.Nodes[i].EnableLeaseExtension,
			}
		}
	}
	return nil
}

func (f *trpcGraphBuilderFactory) createAgent(name string, g *trpcgraph.Graph, enableCheckpoint bool, ee graphtrpc.ExecutionEngineType, subAgents []trpcagent.Agent) (*graphtrpc.GraphAgent, error) {
	if f.saver != nil && enableCheckpoint {
		return graphtrpc.NewGraphAgentWithSaver(name, g, f.saver, ee, subAgents...)
	} else if ee != "" && ee != graphtrpc.EngineBSP {
		return graphtrpc.NewGraphAgentWithEngine(name, g, enableCheckpoint, ee, subAgents...)
	}
	return graphtrpc.NewGraphAgent(name, g, enableCheckpoint, subAgents...)
}
