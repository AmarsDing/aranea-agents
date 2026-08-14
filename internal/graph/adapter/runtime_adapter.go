package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"aranea-agents/pkg/apierror"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/graph"
	graphtrpc "aranea-agents/internal/graph/trpc"
	deliverabletools "aranea-agents/internal/tools/deliverable"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"

	"github.com/google/uuid"
)

// TECH-DEBT(COG): file_lines=963, limit=500 (AS-COG-01); converter helpers
// (checkpoint/template/visual) should move to a separate convert file in a
// future iteration.
type trpcGraphRuntime struct {
	agent           *graphtrpc.GraphAgent
	graph           *trpcgraph.Graph
	lineageID       string
	eventBus        biz.EventBus
	monitorBus      contract.MonitorBus
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

	// replanner is the shared RuntimeReplanner whose per-execution counters
	// must be released when this runtime's event stream ends (S3).
	replanner graph.RuntimeReplanner

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

// flowEmitter builds a run-scoped flow-log emitter for the graph domain.
// Returns nil when no monitor bus is configured (nil-safe: emission skipped).
func (r *trpcGraphRuntime) flowEmitter(ctx context.Context) *event.TraceEmitter {
	if r == nil || r.monitorBus == nil {
		return nil
	}
	return event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:       ctx,
		SessionID: r.sessionID,
		RunID:     r.execID,
		Domain:    event.TraceDomainGraph,
		LG:        r.lg,
		Infra:     event.NewInfraFromBus(r.monitorBus),
	})
}

func (r *trpcGraphRuntime) Run(ctx context.Context, initialState map[string]any) (<-chan biz.GraphRuntimeEvent, error) {
	lineageID := r.lineageID
	if lineageID == "" {
		lineageID = uuid.New().String()
		r.lineageID = lineageID
	}

	flow := r.flowEmitter(ctx)
	if flow != nil {
		flow.LogStart("graph.run.start", "图运行开始",
			event.P("graph_id", r.graphID),
			event.P("execution_id", r.execID),
		)
		// Y7：run 域 emitter 注入 ctx，节点回调链（replanner 等）经
		// TraceEmitterFromContext 复用同一 emitter 发流程日志。
		ctx = event.WithTraceEmitter(ctx, flow)
	}

	// 全新执行只传 lineage_id；绝不能带 CfgKeyCheckpointID 键——
	// executor 以"键存在"作为 resume 信号（Resume 路径才需要），
	// 新 lineage 下 resume-from-latest 必然 ErrCheckpointNotFound。
	runtimeState := map[string]any{
		trpcgraph.CfgKeyLineageID: lineageID,
	}

	for k, v := range initialState {
		runtimeState[k] = v
	}

	// Inject NodeCallbacks (replanner + evolver) into the runtime state so
	// the executor's getMergedCallbacks can retrieve them. StateKeyNodeCallbacks
	// is an "unsafe" key that is retained in state copies (see graph/keys.go).
	if r.callbacks != nil {
		runtimeState[trpcgraph.StateKeyNodeCallbacks] = r.callbacks
	}

	// Invocation 必须经 NewInvocation 构造（初始化 noticeMu/noticeChannels，
	// 裸 &Invocation{} 会触发 "noticeMu is uninitialized" 且事件通知链断裂）。
	// 用户输入必须走 invocation.Message——GraphAgent.createInitialState 只从
	// invocation.Message 派生 StateKeyUserInput（agent 节点用户消息来源），
	// 放 initialState["input"] 里不会被消费，会导致 LLM 请求空 messages 秒回。
	invOpts := []trpcagent.InvocationOptions{
		trpcagent.WithInvocationRunOptions(trpcagent.RunOptions{
			RuntimeState: runtimeState,
		}),
	}
	if input, ok := initialState["input"].(string); ok && input != "" {
		invOpts = append(invOpts, trpcagent.WithInvocationMessage(trpcmodel.Message{
			Role:    trpcmodel.RoleUser,
			Content: input,
		}))
	}
	inv := trpcagent.NewInvocation(invOpts...)

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
		if flow != nil {
			flow.LogError("system.graph.runtime_run_fail", "图运行时启动失败",
				event.P("graph_id", r.graphID),
				event.P("execution_id", r.execID),
				event.P("error", err.Error()),
			)
		}
		return nil, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("graph runtime run: %v", err))
	}

	out := make(chan biz.GraphRuntimeEvent, 64)
	safego.Go(ctx, "graph-event-bridge", func() {
		r.forwardEvents(eventCh, out, flow)
	})

	return out, nil
}

// forwardEvents pumps framework events into the biz runtime-event channel and
// emits the terminal flow log (graph.run.finish) when the stream ends:
// done on the GraphExecution completion event, error when a non-retrying node
// error was observed without a completion (cancel/interrupt are not terminal).
func (r *trpcGraphRuntime) forwardEvents(eventCh <-chan *trpcevent.Event, out chan<- biz.GraphRuntimeEvent, flow *event.TraceEmitter) {
	defer func() {
		r.clearRunCancel()
		// S3：流结束（done/failed/interrupt/cancel 的统一出口）释放 replan
		// 计数 entry，防止 ManagedMap 无限增长。Resume 重新 Run 时会重新计数。
		if r.replanner != nil {
			r.replanner.ReleaseExecution(r.execID)
		}
		close(out)
	}()
	var (
		sawDone      bool
		sawInterrupt bool
		lastNodeErr  string
	)
	for e := range eventCh {
		if e != nil {
			switch e.Object {
			case trpcgraph.ObjectTypeGraphExecution:
				if e.Done {
					sawDone = true
				}
			case trpcgraph.ObjectTypeGraphCheckpointInterrupt:
				sawInterrupt = true
			case trpcgraph.ObjectTypeGraphNodeError:
				if meta := graphtrpc.ExtractNodeMeta(e, r.lg); !meta.Retrying && meta.Error != "" {
					lastNodeErr = meta.Error
				}
			}
		}
		out <- convertTrpcEvent(e, r.bridge, r.lg)
	}
	if flow == nil {
		return
	}
	switch {
	case sawDone:
		flow.LogDone("graph.run.finish", "图运行结束",
			event.P("graph_id", r.graphID),
			event.P("execution_id", r.execID),
			event.P("status", "completed"),
		)
	case sawInterrupt:
		// HITL pause is not terminal; graph.hitl.wait is emitted by the event bridge.
	case lastNodeErr != "":
		flow.LogError("graph.run.finish", "图运行失败",
			event.P("graph_id", r.graphID),
			event.P("execution_id", r.execID),
			event.P("status", "failed"),
			event.P("error", lastNodeErr),
		)
	}
}

func (r *trpcGraphRuntime) Resume(ctx context.Context, lineageID string, resumeValue map[string]any) (<-chan biz.GraphRuntimeEvent, error) {
	r.lineageID = lineageID
	flow := r.flowEmitter(ctx)
	if flow != nil {
		flow.LogStart("graph.run.resume", "图运行恢复",
			event.P("graph_id", r.graphID),
			event.P("execution_id", r.execID),
			event.P("lineage_id", lineageID),
		)
		// Y7：同 Run——resume 路径的节点回调链也复用 run 域 emitter。
		ctx = event.WithTraceEmitter(ctx, flow)
	}
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

	// 同 Run：NewInvocation 构造以初始化 noticeMu/通知链（见 Run 注释）。
	inv := trpcagent.NewInvocation(trpcagent.WithInvocationRunOptions(trpcagent.RunOptions{
		RuntimeState: runtimeState,
	}))

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
		if flow != nil {
			flow.LogError("graph.run.resume", "图运行恢复失败",
				event.P("graph_id", r.graphID),
				event.P("execution_id", r.execID),
				event.P("error", err.Error()),
			)
			flow.LogError("system.graph.runtime_resume_fail", "图运行时恢复失败",
				event.P("graph_id", r.graphID),
				event.P("execution_id", r.execID),
				event.P("error", err.Error()),
			)
		}
		return nil, apierror.Internal(apierror.DomainGraph, fmt.Sprintf("graph runtime resume: %v", err))
	}

	if flow != nil {
		flow.LogDone("graph.run.resume", "图运行已恢复",
			event.P("graph_id", r.graphID),
			event.P("execution_id", r.execID),
		)
	}

	out := make(chan biz.GraphRuntimeEvent, 64)
	safego.Go(ctx, "graph-resume-bridge", func() {
		r.forwardEvents(eventCh, out, flow)
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
		if ev != nil {
			// C-23: never surface ControlCommand / replan control as agent text.
			sanitizeActivityControlCommand(ev, e)
		}
		if ev != nil && bridge.EventBus() != nil {
			bridge.EventBus().Publish(context.Background(), graphtrpc.ActivityEventToSystemNotice(*ev))
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
		runtimeEvt.Retrying = meta.Retrying
	case trpcgraph.ObjectTypeGraphCheckpointInterrupt:
		meta := graphtrpc.ExtractNodeMeta(e, lg)
		runtimeEvt.Type = biz.DomainEventGraphInterrupt
		runtimeEvt.NodeID = meta.NodeID
		runtimeEvt.StepNumber = meta.StepNumber
	case trpcgraph.ObjectTypeGraphPregelStep:
		meta := graphtrpc.ExtractPregelMeta(e, lg)
		switch {
		case meta.Error != "":
			// N1: framework fatal (panic / max steps / executeGraph failure)
			// must surface as an execution-level failure, not be dropped.
			runtimeEvt.Type = biz.DomainEventGraphExecutionError
			runtimeEvt.Error = meta.Error
			runtimeEvt.StepNumber = meta.StepNumber
		case meta.InterruptKey != "" && meta.NodeID != "":
			// N2: HITL interrupt — the only reachable carrier when
			// StreamModeCheckpoints is not enabled.
			runtimeEvt.Type = biz.DomainEventGraphInterrupt
			runtimeEvt.NodeID = meta.NodeID
			runtimeEvt.StepNumber = meta.StepNumber
		}
		// Plain step progress events carry no domain meaning.
	case trpcgraph.ObjectTypeGraphExecution:
		// N1: explicit framework completion — the only signal allowed to
		// converge an execution to Completed (done-driven terminal check).
		if e.Response != nil && e.Response.Done {
			runtimeEvt.Type = biz.DomainEventGraphDone
			runtimeEvt.FinalState = decodeFinalStateDelta(e.StateDelta)
		}
	}

	return runtimeEvt
}

// decodeFinalStateDelta 反序列化完成事件 StateDelta 中的最终图状态
// （serializeFinalState 按 key 写入 JSON bytes；跳过 "_" 前缀的元数据键）。
func decodeFinalStateDelta(delta map[string][]byte) map[string]any {
	if len(delta) == 0 {
		return nil
	}
	state := make(map[string]any, len(delta))
	for k, raw := range delta {
		if k == "" || k[0] == '_' || len(raw) == 0 {
			continue
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			continue
		}
		state[k] = v
	}
	if len(state) == 0 {
		return nil
	}
	return state
}

// sanitizeActivityControlCommand marks ControlCommand signals in activity meta
// and clears Content so the UI does not render control as agent text (C-23).
func sanitizeActivityControlCommand(ev *biz.ActivityEvent, e *trpcevent.Event) {
	if ev == nil {
		return
	}
	if ev.Activity.Meta == nil {
		ev.Activity.Meta = map[string]any{}
	}
	// Content is a string in Activity; also reject typed ControlCommand if ever set.
	if graph.IsControlCommand(ev.Activity.Content) || strings.HasPrefix(ev.Activity.Content, "ControlCommand{") {
		ev.Activity.Meta["control_command"] = true
		ev.Activity.Content = ""
	}
	if e != nil && e.StateDelta != nil {
		if raw, ok := e.StateDelta[graph.StateKeyControlCommand]; ok && len(raw) > 0 {
			ev.Activity.Meta["control_command"] = true
			ev.Activity.Meta[graph.StateKeyControlCommand] = true
		}
	}
	if iv, ok := ev.Activity.Meta["interrupt_value"]; ok {
		if m, ok := iv.(map[string]any); ok {
			if ctrl, _ := m["control"].(string); ctrl != "" {
				ev.Activity.Meta["control_command"] = true
				ev.Activity.Meta["replan_type"] = ctrl
				if fa, _ := m["fallback_agent"].(string); fa != "" {
					ev.Activity.Meta["fallback_agent"] = fa
				}
				ev.Activity.Content = ""
			}
		}
	}
}

type trpcGraphBuilderFactory struct {
	registry     *graphtrpc.Registry
	saver        trpcgraph.CheckpointSaver
	eventBus     biz.EventBus
	monitorBus   contract.MonitorBus
	agentChecker biz.AgentExistenceCheckerFunc
	resolvers    graphtrpc.GraphNodeResolverSet
	lg           loggateway.Logger

	// replanner handles node failure analysis and replan decisions (B3).
	// May be nil when not configured; the OnNodeError callback is skipped.
	replanner graph.RuntimeReplanner
}

var _ biz.GraphBuilderFactory = (*trpcGraphBuilderFactory)(nil)

func NewGraphBuilderFactory(
	registry *graphtrpc.Registry,
	saver trpcgraph.CheckpointSaver,
	eventBus biz.EventBus,
	monitorBus contract.MonitorBus,
	agentChecker biz.AgentExistenceCheckerFunc,
	resolvers graphtrpc.GraphNodeResolverSet,
	replanner graph.RuntimeReplanner,
	lg loggateway.Logger,
) biz.GraphBuilderFactory {
	RegisterCriticLoopCondFunc(registry, DefaultCriticLoopThreshold, lg)
	return &trpcGraphBuilderFactory{
		registry:     registry,
		saver:        saver,
		eventBus:     eventBus,
		monitorBus:   monitorBus,
		agentChecker: agentChecker,
		resolvers:    resolvers,
		lg:           lg,
		replanner:    replanner,
	}
}

// hasDeliverableStateField reports whether the graph schema carries the
// deliverable StateField injected by EnableStateDeliverable
// (team.finalizeRuntimeGraphConfig → ensureDeliverableStateField).
func hasDeliverableStateField(cfg biz.GraphBuildConfig) bool {
	for _, sf := range cfg.StateFields {
		if sf.Name == biz.DeliverableStateKey {
			return true
		}
	}
	return false
}

// resolversFor returns the resolver set to use when building the given graph.
// C1/C3: when the graph carries the deliverable StateField (injected by
// EnableStateDeliverable), every agent node must be built with the
// set/get/ack deliverable tools so members can actually read, write, and
// acknowledge that state field. The resolver is cloned, not mutated, so
// plain graphs keep building tool-free agents. This generic graph path is
// contract-free; the MDC contract is installed only on the team compile
// path (internal/team BuildTeamMemberAgents).
func (f *trpcGraphBuilderFactory) resolversFor(cfg biz.GraphBuildConfig) graphtrpc.GraphNodeResolverSet {
	resolvers := f.resolvers
	if hasDeliverableStateField(cfg) {
		if cat, ok := resolvers.Agents.(*CatalogAgentResolver); ok {
			resolvers.Agents = cat.WithExtraCustomTools(deliverabletools.Tools()...)
		}
	}
	return resolvers
}

func (f *trpcGraphBuilderFactory) buildRuntime(ctx context.Context, cfg biz.GraphBuildConfig, sessionID, spiritSessionID, graphID, execID, lineageID string) (*trpcGraphRuntime, error) {
	resolvers := f.resolversFor(cfg)
	// Use the NodeAgents variant so the GraphAgent can resolve FindSubAgent
	// calls by node ID (e.g. "member-1") rather than by agent Info().Name
	// (e.g. "key-a1"). Without this, agent node execution fails with
	// "parent agent not found in state for agent node X" once the parent
	// agent is injected, because FindSubAgent(nodeID) returns nil.
	g, subAgents, nodeAgents, err := graphtrpc.BuildStateGraphWithRegistryAndNodeAgents(ctx, cfg, f.registry, &resolvers, f.lg)
	if err != nil {
		return nil, err
	}
	name := cfg.EntryPoint
	graphAgent, err := f.createAgent(name, g, cfg, cfg.EnableCheckpoint, cfg.ExecutionEngine, subAgents, nodeAgents)
	if err != nil {
		return nil, err
	}
	return &trpcGraphRuntime{
		agent: graphAgent, graph: g, lineageID: lineageID, eventBus: f.eventBus,
		monitorBus: f.monitorBus,
		sessionID:  sessionID, spiritSessionID: spiritSessionID, graphID: graphID, execID: execID, lg: f.lg,
		bridge:    graphtrpc.NewEventBridge(f.eventBus, f.monitorBus, sessionID, spiritSessionID, graphID, execID, f.lg),
		callbacks: f.buildNodeCallbacks(sessionID, spiritSessionID, graphID, execID),
		replanner: f.replanner,
	}, nil
}

// buildNodeCallbacks assembles the NodeCallbacks for the graph execution (B3).
// ADR-F D2：实现已提取为包级 NewReplanNodeCallbacks（graph run 域与 team 域
// 共用）；本方法仅为兼容既有调用点的薄委托。落地语义见 replan_callbacks.go。
// Returns nil when no replanner is configured, so the runtime skips injecting
// StateKeyNodeCallbacks into the state.
func (f *trpcGraphBuilderFactory) buildNodeCallbacks(sessionID, spiritSessionID, graphID, execID string) *trpcgraph.NodeCallbacks {
	return NewReplanNodeCallbacks(f.replanner, f.lg, sessionID, spiritSessionID, graphID, execID)
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

func (f *trpcGraphBuilderFactory) createAgent(name string, g *trpcgraph.Graph, cfg biz.GraphBuildConfig, enableCheckpoint bool, ee biz.ExecutionEngineType, subAgents []trpcagent.Agent, nodeAgents map[string]trpcagent.Agent) (*graphtrpc.GraphAgent, error) {
	// P1-8: Force-enable CheckpointSaver for all graph runs when a saver is
	// available, regardless of the per-graph EnableCheckpoint flag. This
	// guarantees that every Run can be recovered by RecoveryWorker after a
	// process restart. The EnableCheckpoint flag is now only a hint for
	// graph definitions that explicitly opt out (saver == nil).
	var extraOpts []trpcgraph.ExecutorOption
	if d := graphtrpc.MaxNodeTimeout(cfg.Nodes); d > 0 {
		extraOpts = append(extraOpts, trpcgraph.WithNodeTimeout(d))
	}
	// 编译期显式步数天花板（critic_loop 等含环图）：失控图在贴近预期
	// 迭代数处截断，而非框架默认 100。0 = 框架默认。
	if cfg.MaxSteps > 0 {
		extraOpts = append(extraOpts, trpcgraph.WithMaxSteps(cfg.MaxSteps))
	}
	var (
		agent *graphtrpc.GraphAgent
		err   error
	)
	if f.saver != nil {
		agent, err = graphtrpc.NewGraphAgentWithSaver(name, g, f.saver, ee, subAgents, extraOpts...)
	} else if ee != "" && ee != biz.EngineBSP {
		agent, err = graphtrpc.NewGraphAgentWithEngine(name, g, enableCheckpoint, ee, subAgents, extraOpts...)
	} else {
		agent, err = graphtrpc.NewGraphAgentWithSubAgents(name, g, enableCheckpoint, nil, subAgents, extraOpts...)
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
