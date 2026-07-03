package adapter

import (
	"context"
	"fmt"
	"sync"

	"aranea-agents/pkg/apierror"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/graph"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"

	"github.com/google/uuid"
)

type trpcGraphRuntime struct {
	agent           *graphtrpc.GraphAgent
	graph           *trpcgraph.Graph
	lineageID       string
	activityBus     biz.ActivityEventBus
	sessionID       string
	spiritSessionID string
	graphID         string
	execID          string
	lg              loggateway.Logger
	bridge          *graphtrpc.EventBridge

	// callbacks holds the NodeCallbacks (replanner OnNodeError + evolver
	// AfterNode) injected via StateKeyNodeCallbacks in the runtime state.
	// Nil when no replanner/evolver is configured.
	callbacks *trpcgraph.NodeCallbacks

	cancelMu  sync.Mutex
	runCancel context.CancelFunc
}

var (
	_ biz.GraphExecutionControl = (*trpcGraphRuntime)(nil)
	_ biz.GraphCheckpoint       = (*trpcGraphRuntime)(nil)
	_ biz.GraphRuntime          = (*trpcGraphRuntime)(nil)
)

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

	// Inject NodeCallbacks (replanner + evolver) into the runtime state so
	// the executor's getMergedCallbacks can retrieve them. StateKeyNodeCallbacks
	// is an "unsafe" key that is retained in state copies (see graph/keys.go).
	if r.callbacks != nil {
		runtimeState[trpcgraph.StateKeyNodeCallbacks] = r.callbacks
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
			loggateway.StepID("graph.runtime_run_fail"),
			loggateway.Str("session_id", r.sessionID),
			loggateway.Str("graph_id", r.graphID),
			loggateway.Str("execution_id", r.execID),
			loggateway.Err(err),
		)
		return nil, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("graph runtime run: %v", err))
	}

	out := make(chan biz.GraphRuntimeEvent, 64)
	safego.Go(ctx, "graph-event-bridge", func() {
		defer func() {
			r.clearRunCancel()
			close(out)
		}()
		for e := range eventCh {
			runtimeEvt := convertTrpcEvent(e, r.bridge, r.lg)
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

	// Inject NodeCallbacks (replanner + evolver) into the runtime state so
	// the executor's getMergedCallbacks can retrieve them on resume.
	if r.callbacks != nil {
		runtimeState[trpcgraph.StateKeyNodeCallbacks] = r.callbacks
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
			loggateway.StepID("graph.runtime_resume_fail"),
			loggateway.Str("session_id", r.sessionID),
			loggateway.Str("graph_id", r.graphID),
			loggateway.Str("execution_id", r.execID),
			loggateway.Err(err),
		)
		return nil, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("graph runtime resume: %v", err))
	}

	out := make(chan biz.GraphRuntimeEvent, 64)
	safego.Go(ctx, "graph-resume-bridge", func() {
		defer func() {
			r.clearRunCancel()
			close(out)
		}()
		for e := range eventCh {
			runtimeEvt := convertTrpcEvent(e, r.bridge, r.lg)
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

func (r *trpcGraphRuntime) TimeTravelGetState(ctx context.Context, lineageID, checkpointID, namespace string) (*biz.GraphCheckpointState, error) {
	tt, err := r.agent.TimeTravel()
	if err != nil {
		return nil, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("time travel not available: %v", err))
	}
	ref := trpcgraph.CheckpointRef{
		LineageID:    lineageID,
		Namespace:    namespace,
		CheckpointID: checkpointID,
	}
	snapshot, err := tt.GetState(ctx, ref)
	if err != nil {
		return nil, err
	}
	return convertStateSnapshot(snapshot), nil
}

func (r *trpcGraphRuntime) TimeTravelHistory(ctx context.Context, lineageID, namespace string, limit int) (biz.GraphCheckpointList, error) {
	tt, err := r.agent.TimeTravel()
	if err != nil {
		return nil, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("time travel not available: %v", err))
	}
	infos, err := tt.History(ctx, lineageID, namespace, limit)
	if err != nil {
		return nil, err
	}
	return convertCheckpointInfoList(infos), nil
}

func (r *trpcGraphRuntime) TimeTravelEditState(ctx context.Context, lineageID, checkpointID, namespace string, patch map[string]any) (*biz.GraphEditedState, error) {
	tt, err := r.agent.TimeTravel()
	if err != nil {
		return nil, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("time travel not available: %v", err))
	}
	base := trpcgraph.CheckpointRef{
		LineageID:    lineageID,
		Namespace:    namespace,
		CheckpointID: checkpointID,
	}
	ref, err := tt.EditState(ctx, base, patch)
	if err != nil {
		return nil, err
	}
	return &biz.GraphEditedState{Ref: convertCheckpointRef(ref)}, nil
}

func (r *trpcGraphRuntime) ListCheckpoints(ctx context.Context, lineageID, namespace string, limit int) (biz.GraphCheckpointList, error) {
	tt, err := r.agent.TimeTravel()
	if err != nil {
		return nil, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("time travel not available: %v", err))
	}
	infos, err := tt.History(ctx, lineageID, namespace, limit)
	if err != nil {
		return nil, err
	}
	return convertCheckpointInfoList(infos), nil
}

func (r *trpcGraphRuntime) GetLineageID() string {
	return r.lineageID
}

func convertTrpcEvent(e *trpcevent.Event, bridge *graphtrpc.EventBridge, lg loggateway.Logger) biz.GraphRuntimeEvent {
	if bridge != nil {
		ev := bridge.ConvertEvent(e)
		if ev != nil && bridge.ActivityBus() != nil {
			// TODO(phase3b-d): migrate to v2 EventBus. Blocked by team_graph_run_coordinator.go's
			// v1 Subscribe (consumes graph_stage events from EventBridge via the same bus).
			// Migrating this publish alone would break the graph watch state machine.
			bridge.ActivityBus().Publish(context.Background(), *ev)
		}
	}

	runtimeEvt := biz.GraphRuntimeEvent{
		RawEvent: convertRawEvent(e),
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
	activityBus  biz.ActivityEventBus
	agentChecker biz.AgentExistenceCheckerFunc
	resolvers    graphtrpc.GraphNodeResolverSet
	lg           loggateway.Logger

	// replanner handles node failure analysis and replan decisions (B3).
	// May be nil when not configured; the OnNodeError callback is skipped.
	replanner graph.RuntimeReplanner
	// evolver handles dynamic topology evolution based on execution insights (B4).
	// May be nil when not configured; the AfterNode callback is skipped.
	evolver graph.TopologyEvolver
}

var _ biz.GraphBuilderFactory = (*trpcGraphBuilderFactory)(nil)

func NewGraphBuilderFactory(
	registry *graphtrpc.Registry,
	saver trpcgraph.CheckpointSaver,
	activityBus biz.ActivityEventBus,
	agentChecker biz.AgentExistenceCheckerFunc,
	resolvers graphtrpc.GraphNodeResolverSet,
	replanner graph.RuntimeReplanner,
	evolver graph.TopologyEvolver,
	lg loggateway.Logger,
) biz.GraphBuilderFactory {
	RegisterCriticLoopCondFunc(registry, DefaultCriticLoopThreshold, lg)
	return &trpcGraphBuilderFactory{
		registry:     registry,
		saver:        saver,
		activityBus:  activityBus,
		agentChecker: agentChecker,
		resolvers:    resolvers,
		lg:           lg,
		replanner:    replanner,
		evolver:      evolver,
	}
}

func (f *trpcGraphBuilderFactory) buildRuntime(ctx context.Context, cfg biz.GraphBuildConfig, sessionID, spiritSessionID, graphID, execID, lineageID string) (*trpcGraphRuntime, error) {
	// Use the NodeAgents variant so the GraphAgent can resolve FindSubAgent
	// calls by node ID (e.g. "member-1") rather than by agent Info().Name
	// (e.g. "key-a1"). Without this, agent node execution fails with
	// "parent agent not found in state for agent node X" once the parent
	// agent is injected, because FindSubAgent(nodeID) returns nil.
	g, subAgents, nodeAgents, err := graphtrpc.BuildStateGraphWithRegistryAndNodeAgents(ctx, cfg, f.registry, &f.resolvers, f.lg)
	if err != nil {
		return nil, err
	}
	name := cfg.EntryPoint
	graphAgent, err := f.createAgent(name, g, cfg.EnableCheckpoint, cfg.ExecutionEngine, subAgents, nodeAgents)
	if err != nil {
		return nil, err
	}
	return &trpcGraphRuntime{
		agent: graphAgent, graph: g, lineageID: lineageID, activityBus: f.activityBus,
		sessionID: sessionID, spiritSessionID: spiritSessionID, graphID: graphID, execID: execID, lg: f.lg,
		bridge:    graphtrpc.NewEventBridge(f.activityBus, sessionID, spiritSessionID, graphID, execID, f.lg),
		callbacks: f.buildNodeCallbacks(sessionID, spiritSessionID, graphID, execID),
	}, nil
}

// buildNodeCallbacks assembles the NodeCallbacks for the graph execution.
// B3: registers OnNodeError callback that invokes the RuntimeReplanner to
// analyze node failures and decide replan actions (retry/reroute/insert_fallback/
// rebuild_subgraph). The replan action is logged; actual topology mutation is
// deferred to a future iteration (the framework does not yet support runtime
// graph mutation).
// B4: registers AfterNode callback that invokes the TopologyEvolver to decide
// whether a dynamic transfer edge should be added based on execution insights.
// The evolver is only called when a meaningful insight (non-empty source and
// target) is available; on evolver failure, only a Warn log is emitted and
// execution continues (degrade gracefully).
// Returns nil when neither replanner nor evolver is configured, so the runtime
// skips injecting StateKeyNodeCallbacks into the state.
func (f *trpcGraphBuilderFactory) buildNodeCallbacks(sessionID, spiritSessionID, graphID, execID string) *trpcgraph.NodeCallbacks {
	if f.replanner == nil && f.evolver == nil {
		return nil
	}
	cb := trpcgraph.NewNodeCallbacks()

	if f.replanner != nil {
		replanner := f.replanner
		lg := f.lg
		cb.RegisterOnNodeError(func(ctx context.Context, cbCtx *trpcgraph.NodeCallbackContext, state trpcgraph.State, err error) {
			if cbCtx == nil || err == nil {
				return
			}
			exec := biz.NewGraphExecution(ctx, execID, graphID, sessionID, "")
			exec.SpiritSessionID = spiritSessionID
			action, replanErr := replanner.OnNodeFailure(ctx, exec, cbCtx.NodeID, err)
			if replanErr != nil {
				lg.Warn("runtime replanner: OnNodeFailure failed, execution continues without replan",
					loggateway.StepID("replanner.callback_fail"),
					loggateway.Str("execution_id", execID),
					loggateway.Str("session_id", sessionID),
					loggateway.Str("graph_id", graphID),
					loggateway.Str("failed_node", cbCtx.NodeID),
					loggateway.Err(replanErr),
				)
				return
			}
			if action != nil {
				lg.Info("runtime replanner: replan action decided",
					loggateway.StepID("replanner.action_decided"),
					loggateway.Str("execution_id", execID),
					loggateway.Str("session_id", sessionID),
					loggateway.Str("graph_id", graphID),
					loggateway.Str("failed_node", cbCtx.NodeID),
					loggateway.Str("replan_type", string(action.Type)),
					loggateway.Int("new_nodes", len(action.NewNodes)),
					loggateway.Int("new_edges", len(action.NewEdges)),
					loggateway.Int("skip_nodes", len(action.SkipNodes)),
				)
			}
		})
	}

	if f.evolver != nil {
		evolver := f.evolver
		lg := f.lg
		cb.RegisterAfterNode(func(ctx context.Context, cbCtx *trpcgraph.NodeCallbackContext, state trpcgraph.State, result any, nodeErr error) (any, error) {
			if cbCtx == nil {
				return nil, nil
			}
			// Only evaluate topology evolution after successful node completion.
			// On node error (nodeErr != nil), the replanner's OnNodeError callback
			// handles the failure path; invoking the evolver on error would be
			// noisy and semantically incorrect.
			if nodeErr != nil {
				return nil, nil
			}
			// Construct a best-effort ExecutionInsight. The evolver requires
			// non-empty SourceNode and TargetNode; without a TargetNode (which
			// the framework does not provide via the callback context), the
			// insight is incomplete and the evolver call is skipped. Future
			// work: derive TargetNode from the node result or graph topology.
			sourceNode := cbCtx.NodeID
			if sourceNode == "" {
				return nil, nil
			}
			// TargetNode is unknown at this layer; skip the evolver call when
			// we cannot construct a meaningful insight. The evolver remains
			// wired and ready for future insight producers.
			insight := graph.ExecutionInsight{
				SourceNode: sourceNode,
				Reason:     "post-completion topology check",
			}
			if insight.TargetNode == "" {
				return nil, nil
			}
			exec := biz.NewGraphExecution(ctx, execID, graphID, sessionID, "")
			exec.SpiritSessionID = spiritSessionID
			edge, evolveErr := evolver.OnExecutionInsight(ctx, exec, insight)
			if evolveErr != nil {
				// B4 requirement: on failure, only Warn log, don't block execution.
				lg.Warn("topology evolver: OnExecutionInsight failed, execution continues",
					loggateway.StepID("topology.callback_fail"),
					loggateway.Str("execution_id", execID),
					loggateway.Str("session_id", sessionID),
					loggateway.Str("graph_id", graphID),
					loggateway.Str("source_node", sourceNode),
					loggateway.Str("target_node", insight.TargetNode),
					loggateway.Err(evolveErr),
				)
				return nil, nil
			}
			if edge != nil {
				lg.Info("topology evolver: edge decided",
					loggateway.StepID("topology.edge_decided"),
					loggateway.Str("execution_id", execID),
					loggateway.Str("session_id", sessionID),
					loggateway.Str("graph_id", graphID),
					loggateway.Str("from", edge.From),
					loggateway.Str("to", edge.To),
					loggateway.Str("edge_kind", edge.Kind),
				)
			}
			return nil, nil
		})
	}

	return cb
}

func (f *trpcGraphBuilderFactory) BuildRuntime(ctx context.Context, cfg biz.GraphBuildConfig, sessionID, spiritSessionID, graphID, execID, lineageID string) (biz.GraphRuntime, error) {
	return f.buildRuntime(ctx, cfg, sessionID, spiritSessionID, graphID, execID, lineageID)
}

func (f *trpcGraphBuilderFactory) BuildAndRun(ctx context.Context, cfg biz.GraphBuildConfig, sessionID, spiritSessionID, graphID, execID string, initialState map[string]any) (biz.GraphRuntime, <-chan biz.GraphRuntimeEvent, error) {
	runtime, err := f.buildRuntime(ctx, cfg, sessionID, spiritSessionID, graphID, execID, "")
	if err != nil {
		return nil, nil, err
	}
	eventCh, err := runtime.Run(ctx, initialState)
	if err != nil {
		return nil, nil, err
	}
	return runtime, eventCh, nil
}

func (f *trpcGraphBuilderFactory) BuildAndResume(ctx context.Context, cfg biz.GraphBuildConfig, sessionID, spiritSessionID, graphID, execID, lineageID string, resumeValue map[string]any) (biz.GraphRuntime, <-chan biz.GraphRuntimeEvent, error) {
	runtime, err := f.buildRuntime(ctx, cfg, sessionID, spiritSessionID, graphID, execID, lineageID)
	if err != nil {
		return nil, nil, err
	}
	eventCh, err := runtime.Resume(ctx, lineageID, resumeValue)
	if err != nil {
		return nil, nil, err
	}
	return runtime, eventCh, nil
}

func (f *trpcGraphBuilderFactory) Visualize(ctx context.Context, cfg biz.GraphBuildConfig) (*biz.GraphVisualization, error) {
	g, _, err := graphtrpc.BuildStateGraphWithRegistryAndLogger(ctx, cfg, f.registry, &f.resolvers, f.lg)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("build state graph for visualization: %v", err))
	}
	dot := g.DOT()
	vg := graphtrpc.ParseDOTToVisualGraph(dot, cfg.Nodes, cfg.ConditionalEdges)
	startEnd := graphtrpc.BuildStartEndNodes()
	allNodes := make([]graphtrpc.VisualGraphNode, 0, len(vg.Nodes)+2)
	allNodes = append(allNodes, startEnd...)
	allNodes = append(allNodes, vg.Nodes...)
	vg.Nodes = allNodes
	return convertVisualGraph(vg), nil
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
	checker := graphtrpc.AgentExistenceChecker(f.agentChecker)
	return validationResultToBiz(graphtrpc.ValidateGraph(ctx, &cfg, checker, f.registry)), nil
}

func (f *trpcGraphBuilderFactory) ListTemplates() []biz.GraphTemplateRef {
	templates := graphtrpc.ListBuiltinTemplates()
	out := make([]biz.GraphTemplateRef, len(templates))
	for i := range templates {
		out[i] = convertGraphTemplate(templates[i])
	}
	return out
}

func (f *trpcGraphBuilderFactory) GetTemplate(templateID string) (biz.GraphTemplateRef, bool) {
	tmpl := graphtrpc.GetBuiltinTemplate(templateID)
	if tmpl == nil {
		return biz.GraphTemplateRef{}, false
	}
	return convertGraphTemplate(*tmpl), true
}

func (f *trpcGraphBuilderFactory) TemplateToDef(template biz.GraphTemplateRef, name, description string) *biz.GraphDefinition {
	tmpl := revertGraphTemplate(template)
	cfg := graphtrpc.TemplateToBuildConfig(tmpl)
	return &biz.GraphDefinition{
		Name: name, Description: description, StateFields: cfg.StateFields,
		Nodes: cfg.Nodes, Edges: cfg.Edges, ConditionalEdges: cfg.ConditionalEdges,
		EntryPoint: cfg.EntryPoint, FinishPoint: cfg.FinishPoint,
		EnableCheckpoint: cfg.EnableCheckpoint, ExecutionEngine: cfg.ExecutionEngine,
	}
}

func (f *trpcGraphBuilderFactory) AgentExists(ctx context.Context, agentID string) bool {
	if f.agentChecker == nil {
		return false
	}
	return f.agentChecker(ctx, agentID)
}

func (f *trpcGraphBuilderFactory) FindNodeDef(cfg biz.GraphBuildConfig, taskMeta map[string]biz.NodeTaskMeta, nodeID string) *biz.NodeTaskMeta {
	for i := range cfg.Nodes {
		if cfg.Nodes[i].ID == nodeID {
			if m, ok := taskMeta[nodeID]; ok {
				return &m
			}
			return &biz.NodeTaskMeta{}
		}
	}
	return nil
}

func (f *trpcGraphBuilderFactory) createAgent(name string, g *trpcgraph.Graph, enableCheckpoint bool, ee biz.ExecutionEngineType, subAgents []trpcagent.Agent, nodeAgents map[string]trpcagent.Agent) (*graphtrpc.GraphAgent, error) {
	// P1-8: Force-enable CheckpointSaver for all graph runs when a saver is
	// available, regardless of the per-graph EnableCheckpoint flag. This
	// guarantees that every Run can be recovered by RecoveryWorker after a
	// process restart. The EnableCheckpoint flag is now only a hint for
	// graph definitions that explicitly opt out (saver == nil).
	var (
		agent *graphtrpc.GraphAgent
		err   error
	)
	if f.saver != nil {
		agent, err = graphtrpc.NewGraphAgentWithSaver(name, g, f.saver, ee, subAgents...)
	} else if ee != "" && ee != biz.EngineBSP {
		agent, err = graphtrpc.NewGraphAgentWithEngine(name, g, enableCheckpoint, ee, subAgents...)
	} else {
		agent, err = graphtrpc.NewGraphAgent(name, g, enableCheckpoint, subAgents...)
	}
	if err != nil {
		return nil, err
	}
	// SetNodeAgents is required for all three construction paths because
	// NewGraphAgentWithSaver / NewGraphAgentWithEngine / NewGraphAgent all
	// pre-date the nodeAgents field and don't accept it as a parameter.
	agent.SetNodeAgents(nodeAgents)
	return agent, nil
}

// ---------------------------------------------------------------------------
// Type conversion helpers: trpc-agent-go → biz layer
// ---------------------------------------------------------------------------

func convertCheckpointRef(ref trpcgraph.CheckpointRef) biz.GraphCheckpointRef {
	return biz.GraphCheckpointRef{
		LineageID:    ref.LineageID,
		Namespace:    ref.Namespace,
		CheckpointID: ref.CheckpointID,
	}
}

func convertCheckpointInfo(info trpcgraph.CheckpointInfo) biz.GraphCheckpointInfo {
	return biz.GraphCheckpointInfo{
		Ref:              convertCheckpointRef(info.Ref),
		ParentCheckpoint: info.ParentCheckpoint,
		Source:           info.Source,
		Step:             info.Step,
		Timestamp:        info.Timestamp,
	}
}

func convertCheckpointInfoList(infos []trpcgraph.CheckpointInfo) biz.GraphCheckpointList {
	out := make(biz.GraphCheckpointList, len(infos))
	for i := range infos {
		out[i] = convertCheckpointInfo(infos[i])
	}
	return out
}

func convertStateSnapshot(snapshot *trpcgraph.StateSnapshot) *biz.GraphCheckpointState {
	if snapshot == nil {
		return nil
	}
	return &biz.GraphCheckpointState{
		Ref:              convertCheckpointRef(snapshot.Ref),
		ParentCheckpoint: snapshot.ParentCheckpoint,
		Source:           snapshot.Source,
		Step:             snapshot.Step,
		Timestamp:        snapshot.Timestamp,
		State:            snapshot.State,
		NextNodes:        snapshot.NextNodes,
		NextChannels:     snapshot.NextChannels,
	}
}

func convertVisualGraph(vg *graphtrpc.VisualGraph) *biz.GraphVisualization {
	if vg == nil {
		return nil
	}
	nodes := make([]biz.GraphVisualizationNode, len(vg.Nodes))
	for i, n := range vg.Nodes {
		nodes[i] = biz.GraphVisualizationNode{
			ID: n.ID, Label: n.Label, Type: n.Type,
			Shape: n.Shape, FillColor: n.FillColor, BorderColor: n.BorderColor,
		}
	}
	edges := make([]biz.GraphVisualizationEdge, len(vg.Edges))
	for i, e := range vg.Edges {
		edges[i] = biz.GraphVisualizationEdge{
			From: e.From, To: e.To, Type: e.Type, Label: e.Label,
		}
	}
	return &biz.GraphVisualization{Nodes: nodes, Edges: edges, DOT: vg.DOT}
}

func convertGraphTemplate(t graphtrpc.GraphTemplate) biz.GraphTemplateRef {
	nodes := make([]biz.GraphTemplateNodeRef, len(t.Nodes))
	for i, n := range t.Nodes {
		nodes[i] = biz.GraphTemplateNodeRef{
			NodeID: n.NodeID, Type: n.Type, Label: n.Label, Description: n.Description,
		}
	}
	edges := make([]biz.GraphTemplateEdgeRef, len(t.Edges))
	for i, e := range t.Edges {
		edges[i] = biz.GraphTemplateEdgeRef{
			FromNode: e.FromNode, ToNode: e.ToNode, Type: e.Type, Label: e.Label,
		}
	}
	stateFields := make([]biz.StateFieldDef, len(t.StateFields))
	copy(stateFields, t.StateFields)
	return biz.GraphTemplateRef{
		ID: t.ID, Name: t.Name, Description: t.Description, Category: t.Category,
		Nodes: nodes, Edges: edges, StateFields: stateFields,
		EntryPoint: t.EntryPoint, FinishPoint: t.FinishPoint,
	}
}

// revertGraphTemplate converts a biz GraphTemplateRef back to the trpc layer
// GraphTemplate so it can be fed into graphtrpc.TemplateToBuildConfig.
func revertGraphTemplate(t biz.GraphTemplateRef) graphtrpc.GraphTemplate {
	nodes := make([]graphtrpc.TemplateNode, len(t.Nodes))
	for i, n := range t.Nodes {
		nodes[i] = graphtrpc.TemplateNode{
			NodeID: n.NodeID, Type: n.Type, Label: n.Label, Description: n.Description,
		}
	}
	edges := make([]graphtrpc.TemplateEdge, len(t.Edges))
	for i, e := range t.Edges {
		edges[i] = graphtrpc.TemplateEdge{
			FromNode: e.FromNode, ToNode: e.ToNode, Type: e.Type, Label: e.Label,
		}
	}
	stateFields := make([]graphtrpc.StateFieldDef, len(t.StateFields))
	copy(stateFields, t.StateFields)
	return graphtrpc.GraphTemplate{
		ID: t.ID, Name: t.Name, Description: t.Description, Category: t.Category,
		Nodes: nodes, Edges: edges, StateFields: stateFields,
		EntryPoint: t.EntryPoint, FinishPoint: t.FinishPoint,
	}
}

func convertRawEvent(e *trpcevent.Event) biz.GraphRawEvent {
	if e == nil {
		return biz.GraphRawEvent{}
	}
	return biz.GraphRawEvent{
		Object: e.Object,
	}
}
