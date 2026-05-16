package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"aranea-agents/internal/event"
	graphtrpc "aranea-agents/internal/graph/trpc"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"

	"github.com/google/uuid"
)

type GraphDefinition struct {
	ID               string
	Name             string
	Description      string
	StateFields      []graphtrpc.StateFieldDef
	Nodes            []graphtrpc.NodeDef
	Edges            []graphtrpc.EdgeDef
	ConditionalEdges []graphtrpc.ConditionalEdgeDef
	Subgraphs        []graphtrpc.SubgraphDef
	EntryPoint       string
	FinishPoint      string
	EnableCheckpoint bool
	ExecutionEngine  graphtrpc.ExecutionEngineType
	InterruptBefore  []string
	InterruptAfter   []string
	Metadata         map[string]any
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type GraphExecution struct {
	ID            string
	GraphID       string
	SessionID     string
	Status        string
	CurrentNode   string
	LineageID     string
	ErrorMessage  string
	CurrentState  trpcgraph.State
	Steps         []GraphStepSnapshot
	InterruptNode string
	Agent         trpcagent.Agent
	GraphAgent    *graphtrpc.GraphAgent
	StartedAt     time.Time
	FinishedAt    *time.Time
}

type GraphStepSnapshot struct {
	NodeID      string `json:"node_id"`
	StepIndex   int    `json:"step_index"`
	InputState  trpcgraph.State
	OutputState trpcgraph.State
	Status      string `json:"status"`
	Error       string `json:"error"`
	Timestamp   time.Time
}

type GraphRepo interface {
	SaveDefinition(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error)
	GetDefinition(ctx context.Context, id string) (*GraphDefinition, error)
	ListDefinitions(ctx context.Context, pageSize int, pageToken string) ([]*GraphDefinition, string, error)
	UpdateDefinition(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error)
	DeleteDefinition(ctx context.Context, id string) error
}

type GraphRunRepo interface {
	SaveRun(ctx context.Context, exec *GraphExecution) error
	GetRun(ctx context.Context, id string) (*GraphExecution, error)
	ListRunsByGraph(ctx context.Context, graphID string, pageSize int, pageToken string) ([]*GraphExecution, string, error)
	UpdateRun(ctx context.Context, exec *GraphExecution) error
}

type GraphUsecase struct {
	repo         GraphRepo
	runRepo      GraphRunRepo
	registry     *graphtrpc.Registry
	saver        trpcgraph.CheckpointSaver
	eventBus     event.Bus
	agentChecker graphtrpc.AgentExistenceChecker
	mu           sync.RWMutex
	defs         map[string]*GraphDefinition
	executions   map[string]*GraphExecution
}

func NewGraphUsecase(repo GraphRepo, runRepo GraphRunRepo, registry *graphtrpc.Registry, saver trpcgraph.CheckpointSaver, eventBus event.Bus, agents AgentRepository) *GraphUsecase {
	uc := &GraphUsecase{
		repo:         repo,
		runRepo:      runRepo,
		registry:     registry,
		saver:        saver,
		eventBus:     eventBus,
		agentChecker: ProvideAgentExistenceChecker(agents),
		defs:         make(map[string]*GraphDefinition),
		executions:   make(map[string]*GraphExecution),
	}
	return uc
}

func (uc *GraphUsecase) CreateGraph(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error) {
	if def.ID == "" {
		def.ID = uuid.New().String()
	}
	now := time.Now()
	def.CreatedAt = now
	def.UpdatedAt = now
	saved, err := uc.repo.SaveDefinition(ctx, def)
	if err != nil {
		return nil, err
	}
	uc.mu.Lock()
	uc.defs[saved.ID] = saved
	uc.mu.Unlock()
	return saved, nil
}

func (uc *GraphUsecase) GetGraph(ctx context.Context, id string) (*GraphDefinition, error) {
	uc.mu.RLock()
	if def, ok := uc.defs[id]; ok {
		uc.mu.RUnlock()
		return def, nil
	}
	uc.mu.RUnlock()
	def, err := uc.repo.GetDefinition(ctx, id)
	if err != nil {
		return nil, err
	}
	uc.mu.Lock()
	uc.defs[id] = def
	uc.mu.Unlock()
	return def, nil
}

func (uc *GraphUsecase) ListGraphs(ctx context.Context, pageSize int, pageToken string) ([]*GraphDefinition, string, error) {
	return uc.repo.ListDefinitions(ctx, pageSize, pageToken)
}

func (uc *GraphUsecase) UpdateGraph(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error) {
	now := time.Now()
	def.UpdatedAt = now
	saved, err := uc.repo.UpdateDefinition(ctx, def)
	if err != nil {
		return nil, err
	}
	uc.mu.Lock()
	uc.defs[saved.ID] = saved
	uc.mu.Unlock()
	return saved, nil
}

func (uc *GraphUsecase) DeleteGraph(ctx context.Context, id string) error {
	err := uc.repo.DeleteDefinition(ctx, id)
	if err != nil {
		return err
	}
	uc.mu.Lock()
	delete(uc.defs, id)
	uc.mu.Unlock()
	return nil
}

func (uc *GraphUsecase) ExecuteGraph(ctx context.Context, graphID string, sessionID string, initialState trpcgraph.State) (*GraphExecution, error) {
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil, err
	}
	cfg := graphtrpc.GraphBuildConfig{
		Nodes:            def.Nodes,
		Edges:            def.Edges,
		ConditionalEdges: def.ConditionalEdges,
		Subgraphs:        def.Subgraphs,
		StateFields:      def.StateFields,
		EntryPoint:       def.EntryPoint,
		FinishPoint:      def.FinishPoint,
		EnableCheckpoint: def.EnableCheckpoint,
		ExecutionEngine:  def.ExecutionEngine,
		InterruptBefore:  def.InterruptBefore,
		InterruptAfter:   def.InterruptAfter,
	}
	g, subAgents, err := graphtrpc.BuildStateGraphWithRegistry(cfg, uc.registry)
	if err != nil {
		return nil, err
	}

	var graphAgent *graphtrpc.GraphAgent
	if uc.saver != nil && def.EnableCheckpoint {
		graphAgent, err = graphtrpc.NewGraphAgentWithSaver(def.Name, g, uc.saver, def.ExecutionEngine)
	} else if def.ExecutionEngine != "" && def.ExecutionEngine != graphtrpc.EngineBSP {
		graphAgent, err = graphtrpc.NewGraphAgentWithEngine(def.Name, g, def.EnableCheckpoint, def.ExecutionEngine)
	} else {
		graphAgent, err = graphtrpc.NewGraphAgent(def.Name, g, def.EnableCheckpoint)
	}
	if err != nil {
		return nil, err
	}
	_ = subAgents

	lineageID := uuid.New().String()
	runtimeState := trpcgraph.CheckpointRef{
		LineageID:    lineageID,
		CheckpointID: "",
	}.ToRuntimeState()

	if initialState != nil {
		for k, v := range initialState {
			runtimeState[k] = v
		}
	}

	inv := &trpcagent.Invocation{
		RunOptions: trpcagent.RunOptions{
			RuntimeState: runtimeState,
		},
	}

	eventCh, err := graphAgent.Run(ctx, inv)
	if err != nil {
		return nil, fmt.Errorf("graph execute: %w", err)
	}

	execID := uuid.New().String()
	exec := &GraphExecution{
		ID:         execID,
		GraphID:    graphID,
		SessionID:  sessionID,
		Status:     "running",
		Agent:      graphAgent,
		GraphAgent: graphAgent,
		LineageID:  lineageID,
		StartedAt:  time.Now(),
	}

	if err := uc.runRepo.SaveRun(ctx, exec); err != nil {
		return nil, fmt.Errorf("graph execute save run: %w", err)
	}

	go uc.consumeEvents(eventCh, exec, execID, graphID, sessionID)

	uc.mu.Lock()
	uc.executions[execID] = exec
	uc.mu.Unlock()
	return exec, nil
}

func (uc *GraphUsecase) GetExecution(ctx context.Context, executionID string) (*GraphExecution, error) {
	uc.mu.RLock()
	exec, ok := uc.executions[executionID]
	uc.mu.RUnlock()
	if ok {
		return exec, nil
	}
	persisted, err := uc.runRepo.GetRun(ctx, executionID)
	if err != nil {
		return nil, ErrNotFound
	}
	return persisted, nil
}

func (uc *GraphUsecase) ListExecutions(ctx context.Context, graphID string, pageSize int, pageToken string) ([]*GraphExecution, string, error) {
	return uc.runRepo.ListRunsByGraph(ctx, graphID, pageSize, pageToken)
}

func (uc *GraphUsecase) CancelExecution(ctx context.Context, executionID string) error {
	uc.mu.RLock()
	exec, ok := uc.executions[executionID]
	uc.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	if exec.Status != "running" && exec.Status != "waiting_human" {
		return fmt.Errorf("graph: cannot cancel execution in status %q", exec.Status)
	}
	exec.Status = "cancelled"
	now := time.Now()
	exec.FinishedAt = &now
	return uc.runRepo.UpdateRun(ctx, exec)
}

func (uc *GraphUsecase) ResumeExecution(ctx context.Context, executionID string, resumeValue map[string]any) (*GraphExecution, error) {
	uc.mu.RLock()
	exec, ok := uc.executions[executionID]
	uc.mu.RUnlock()
	if !ok {
		persisted, err := uc.runRepo.GetRun(ctx, executionID)
		if err != nil {
			return nil, ErrNotFound
		}
		exec = persisted
		uc.mu.Lock()
		uc.executions[executionID] = exec
		uc.mu.Unlock()
	}
	if exec.GraphAgent == nil {
		return nil, ErrNotFound
	}

	lineageID := exec.LineageID
	if lineageID == "" {
		lineageID = uuid.New().String()
		exec.LineageID = lineageID
	}

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

	eventCh, err := exec.GraphAgent.Run(ctx, inv)
	if err != nil {
		return nil, fmt.Errorf("graph resume: %w", err)
	}

	exec.Status = "running"

	go uc.consumeEvents(eventCh, exec, executionID, exec.GraphID, exec.SessionID)

	return exec, nil
}

func (uc *GraphUsecase) consumeEvents(eventCh <-chan *trpcevent.Event, exec *GraphExecution, execID, graphID, sessionID string) {
	var bridge *graphtrpc.EventBridge
	if uc.eventBus != nil {
		bridge = graphtrpc.NewEventBridge(uc.eventBus, sessionID, graphID, execID)
	}

	for e := range eventCh {
		if bridge != nil {
			env := bridge.ConvertEvent(e)
			if env != nil {
				uc.eventBus.Publish(context.Background(), *env)
			}
		}
		uc.updateExecutionFromEvent(exec, e)
	}

	uc.mu.Lock()
	if exec.Status == "running" {
		exec.Status = "completed"
		now := time.Now()
		exec.FinishedAt = &now
	}
	uc.executions[execID] = exec
	uc.mu.Unlock()
	_ = uc.runRepo.UpdateRun(context.Background(), exec)
}

func (uc *GraphUsecase) updateExecutionFromEvent(exec *GraphExecution, e *trpcevent.Event) {
	switch e.Object {
	case trpcgraph.ObjectTypeGraphNodeStart:
		meta := graphtrpc.ExtractNodeMeta(e)
		uc.mu.Lock()
		exec.CurrentNode = meta.NodeID
		uc.mu.Unlock()
	case trpcgraph.ObjectTypeGraphNodeError:
		meta := graphtrpc.ExtractNodeMeta(e)
		uc.mu.Lock()
		exec.ErrorMessage = meta.Error
		exec.Status = "failed"
		uc.mu.Unlock()
	case trpcgraph.ObjectTypeGraphCheckpointInterrupt:
		uc.mu.Lock()
		exec.Status = "waiting_human"
		uc.mu.Unlock()
	}
}

func (uc *GraphUsecase) TimeTravelGetState(ctx context.Context, executionID string, checkpointID string, namespace string) (*trpcgraph.StateSnapshot, error) {
	uc.mu.RLock()
	exec, ok := uc.executions[executionID]
	uc.mu.RUnlock()
	if !ok {
		return nil, ErrNotFound
	}
	if exec.GraphAgent == nil {
		return nil, ErrNotFound
	}
	tt, err := exec.GraphAgent.TimeTravel()
	if err != nil {
		return nil, fmt.Errorf("time travel not available: %w", err)
	}
	ref := trpcgraph.CheckpointRef{
		LineageID:    exec.LineageID,
		Namespace:    namespace,
		CheckpointID: checkpointID,
	}
	return tt.GetState(ctx, ref)
}

func (uc *GraphUsecase) TimeTravelHistory(ctx context.Context, executionID string, namespace string, limit int) ([]trpcgraph.CheckpointInfo, error) {
	uc.mu.RLock()
	exec, ok := uc.executions[executionID]
	uc.mu.RUnlock()
	if !ok {
		return nil, ErrNotFound
	}
	if exec.GraphAgent == nil {
		return nil, ErrNotFound
	}
	tt, err := exec.GraphAgent.TimeTravel()
	if err != nil {
		return nil, fmt.Errorf("time travel not available: %w", err)
	}
	return tt.History(ctx, exec.LineageID, namespace, limit)
}

func (uc *GraphUsecase) TimeTravelEditState(ctx context.Context, executionID string, checkpointID string, namespace string, patch trpcgraph.State) (trpcgraph.CheckpointRef, error) {
	uc.mu.RLock()
	exec, ok := uc.executions[executionID]
	uc.mu.RUnlock()
	if !ok {
		return trpcgraph.CheckpointRef{}, ErrNotFound
	}
	if exec.GraphAgent == nil {
		return trpcgraph.CheckpointRef{}, ErrNotFound
	}
	tt, err := exec.GraphAgent.TimeTravel()
	if err != nil {
		return trpcgraph.CheckpointRef{}, fmt.Errorf("time travel not available: %w", err)
	}
	base := trpcgraph.CheckpointRef{
		LineageID:    exec.LineageID,
		Namespace:    namespace,
		CheckpointID: checkpointID,
	}
	return tt.EditState(ctx, base, patch)
}

func (uc *GraphUsecase) GetCheckpointSaver() trpcgraph.CheckpointSaver {
	return uc.saver
}

func (uc *GraphUsecase) ListCheckpoints(ctx context.Context, executionID string, namespace string, limit int) ([]trpcgraph.CheckpointInfo, error) {
	uc.mu.RLock()
	exec, ok := uc.executions[executionID]
	uc.mu.RUnlock()
	if !ok {
		return nil, ErrNotFound
	}
	if exec.GraphAgent == nil {
		return nil, ErrNotFound
	}
	tt, err := exec.GraphAgent.TimeTravel()
	if err != nil {
		return nil, fmt.Errorf("time travel not available: %w", err)
	}
	return tt.History(ctx, exec.LineageID, namespace, limit)
}

func (uc *GraphUsecase) GetStateSnapshot(ctx context.Context, executionID string, checkpointID string, namespace string) (*trpcgraph.StateSnapshot, error) {
	uc.mu.RLock()
	exec, ok := uc.executions[executionID]
	uc.mu.RUnlock()
	if !ok {
		return nil, ErrNotFound
	}
	if exec.GraphAgent == nil {
		return nil, ErrNotFound
	}
	tt, err := exec.GraphAgent.TimeTravel()
	if err != nil {
		return nil, fmt.Errorf("time travel not available: %w", err)
	}
	ref := trpcgraph.CheckpointRef{
		LineageID:    exec.LineageID,
		Namespace:    namespace,
		CheckpointID: checkpointID,
	}
	return tt.GetState(ctx, ref)
}

func (uc *GraphUsecase) EditState(ctx context.Context, executionID string, checkpointID string, namespace string, patch trpcgraph.State) (trpcgraph.CheckpointRef, error) {
	uc.mu.RLock()
	exec, ok := uc.executions[executionID]
	uc.mu.RUnlock()
	if !ok {
		return trpcgraph.CheckpointRef{}, ErrNotFound
	}
	if exec.GraphAgent == nil {
		return trpcgraph.CheckpointRef{}, ErrNotFound
	}
	tt, err := exec.GraphAgent.TimeTravel()
	if err != nil {
		return trpcgraph.CheckpointRef{}, fmt.Errorf("time travel not available: %w", err)
	}
	base := trpcgraph.CheckpointRef{
		LineageID:    exec.LineageID,
		Namespace:    namespace,
		CheckpointID: checkpointID,
	}
	return tt.EditState(ctx, base, patch)
}

func (uc *GraphUsecase) VisualizeGraph(ctx context.Context, graphID string, format string) (*graphtrpc.VisualGraph, error) {
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil, err
	}

	cfg := graphtrpc.GraphBuildConfig{
		Nodes:            def.Nodes,
		Edges:            def.Edges,
		ConditionalEdges: def.ConditionalEdges,
		Subgraphs:        def.Subgraphs,
		StateFields:      def.StateFields,
		EntryPoint:       def.EntryPoint,
		FinishPoint:      def.FinishPoint,
		EnableCheckpoint: def.EnableCheckpoint,
		ExecutionEngine:  def.ExecutionEngine,
		InterruptBefore:  def.InterruptBefore,
		InterruptAfter:   def.InterruptAfter,
	}

	g, _, err := graphtrpc.BuildStateGraphWithRegistry(cfg, uc.registry)
	if err != nil {
		return nil, fmt.Errorf("build state graph for visualization: %w", err)
	}

	dot := g.DOT()
	vg := graphtrpc.ParseDOTToVisualGraph(dot, def.Nodes, def.ConditionalEdges)

	startEnd := graphtrpc.BuildStartEndNodes()
	allNodes := make([]graphtrpc.VisualGraphNode, 0, len(vg.Nodes)+2)
	allNodes = append(allNodes, startEnd...)
	allNodes = append(allNodes, vg.Nodes...)
	vg.Nodes = allNodes

	return vg, nil
}

func graphStepSnapshotToJSON(steps []GraphStepSnapshot) string {
	type jsonStep struct {
		NodeID      string `json:"node_id"`
		StepIndex   int    `json:"step_index"`
		InputState  any    `json:"input_state,omitempty"`
		OutputState any    `json:"output_state,omitempty"`
		Status      string `json:"status"`
		Error       string `json:"error"`
		Timestamp   string `json:"timestamp"`
	}
	js := make([]jsonStep, len(steps))
	for i, s := range steps {
		js[i] = jsonStep{
			NodeID:      s.NodeID,
			StepIndex:   s.StepIndex,
			InputState:  s.InputState,
			OutputState: s.OutputState,
			Status:      s.Status,
			Error:       s.Error,
			Timestamp:   s.Timestamp.Format(time.RFC3339Nano),
		}
	}
	b, _ := json.Marshal(js)
	return string(b)
}

func (uc *GraphUsecase) ValidateGraph(ctx context.Context, graphID string) (*graphtrpc.ValidationResult, error) {
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil, err
	}
	cfg := graphtrpc.GraphBuildConfig{
		Nodes:            def.Nodes,
		Edges:            def.Edges,
		ConditionalEdges: def.ConditionalEdges,
		Subgraphs:        def.Subgraphs,
		StateFields:      def.StateFields,
		EntryPoint:       def.EntryPoint,
		FinishPoint:      def.FinishPoint,
		EnableCheckpoint: def.EnableCheckpoint,
		ExecutionEngine:  def.ExecutionEngine,
		InterruptBefore:  def.InterruptBefore,
		InterruptAfter:   def.InterruptAfter,
	}
	return graphtrpc.ValidateGraph(&cfg, uc.agentChecker, uc.registry), nil
}

func (uc *GraphUsecase) ListGraphTemplates(ctx context.Context) []graphtrpc.GraphTemplate {
	return graphtrpc.ListBuiltinTemplates()
}

func (uc *GraphUsecase) CreateGraphFromTemplate(ctx context.Context, templateID string, name string, description string) (*GraphDefinition, error) {
	tmpl := graphtrpc.GetBuiltinTemplate(templateID)
	if tmpl == nil {
		return nil, fmt.Errorf("graph template %q not found", templateID)
	}
	cfg := graphtrpc.TemplateToBuildConfig(*tmpl)
	def := &GraphDefinition{
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
	return uc.CreateGraph(ctx, def)
}
