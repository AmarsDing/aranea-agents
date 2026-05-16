package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/pkg/safego"

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
	CurrentState  map[string]any
	Steps         []GraphStepSnapshot
	InterruptNode string
	runtime       GraphRuntime
	StartedAt     time.Time
	FinishedAt    *time.Time
}

type GraphStepSnapshot struct {
	NodeID      string         `json:"node_id"`
	StepIndex   int            `json:"step_index"`
	InputState  map[string]any `json:"input_state,omitempty"`
	OutputState map[string]any `json:"output_state,omitempty"`
	Status      string         `json:"status"`
	Error       string         `json:"error"`
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
	repo       GraphRepo
	runRepo    GraphRunRepo
	factory    GraphBuilderFactory
	mu         sync.RWMutex
	defs       map[string]*GraphDefinition
	executions map[string]*GraphExecution
}

func NewGraphUsecase(repo GraphRepo, runRepo GraphRunRepo, factory GraphBuilderFactory) *GraphUsecase {
	uc := &GraphUsecase{
		repo:       repo,
		runRepo:    runRepo,
		factory:    factory,
		defs:       make(map[string]*GraphDefinition),
		executions: make(map[string]*GraphExecution),
	}
	go uc.gcLoop()
	return uc
}

const gcInterval = 5 * time.Minute
const executionMaxAge = 30 * time.Minute

func (uc *GraphUsecase) gcLoop() {
	ticker := time.NewTicker(gcInterval)
	defer ticker.Stop()
	for range ticker.C {
		uc.gc()
	}
}

func (uc *GraphUsecase) gc() {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	now := time.Now()
	for id, exec := range uc.executions {
		if exec.Status == "running" || exec.Status == "waiting_human" {
			continue
		}
		if exec.FinishedAt != nil && now.Sub(*exec.FinishedAt) > executionMaxAge {
			delete(uc.executions, id)
		} else if exec.FinishedAt == nil && now.Sub(exec.StartedAt) > executionMaxAge {
			exec.Status = "expired"
			nowCopy := now
			exec.FinishedAt = &nowCopy
			delete(uc.executions, id)
		}
	}
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

func (uc *GraphUsecase) ExecuteGraph(ctx context.Context, graphID string, sessionID string, initialState map[string]any) (*GraphExecution, error) {
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil, err
	}

	cfg := defToBuildConfig(def)
	execID := uuid.New().String()

	runtime, eventCh, err := uc.factory.BuildAndRun(ctx, cfg, sessionID, graphID, execID, initialState)
	if err != nil {
		return nil, err
	}

	exec := &GraphExecution{
		ID:        execID,
		GraphID:   graphID,
		SessionID: sessionID,
		Status:    "running",
		runtime:   runtime,
		LineageID: runtime.GetLineageID(),
		StartedAt: time.Now(),
	}

	if err := uc.runRepo.SaveRun(ctx, exec); err != nil {
		return nil, fmt.Errorf("graph execute save run: %w", err)
	}

	safego.Go(context.Background(), "graph.consumeEvents", func() {
		uc.consumeRuntimeEvents(eventCh, exec, execID, graphID, sessionID)
	})

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
	if exec.runtime == nil {
		return nil, ErrNotFound
	}

	lineageID := exec.LineageID
	if lineageID == "" {
		lineageID = uuid.New().String()
		exec.LineageID = lineageID
	}

	cfg := defToBuildConfig(&GraphDefinition{ID: exec.GraphID})
	runtime, eventCh, err := uc.factory.BuildAndResume(ctx, cfg, exec.SessionID, exec.GraphID, executionID, lineageID, resumeValue)
	if err != nil {
		return nil, fmt.Errorf("graph resume: %w", err)
	}

	exec.runtime = runtime
	exec.Status = "running"

	safego.Go(context.Background(), "graph.consumeEvents(resume)", func() {
		uc.consumeRuntimeEvents(eventCh, exec, executionID, exec.GraphID, exec.SessionID)
	})

	return exec, nil
}

func (uc *GraphUsecase) consumeRuntimeEvents(eventCh <-chan GraphRuntimeEvent, exec *GraphExecution, execID, graphID, sessionID string) {
	for e := range eventCh {
		uc.updateExecutionFromRuntimeEvent(exec, e)
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

func (uc *GraphUsecase) updateExecutionFromRuntimeEvent(exec *GraphExecution, e GraphRuntimeEvent) {
	switch e.Type {
	case DomainEventGraphNodeStart:
		uc.mu.Lock()
		exec.CurrentNode = e.NodeID
		uc.mu.Unlock()
	case DomainEventGraphNodeError:
		uc.mu.Lock()
		exec.ErrorMessage = e.Error
		exec.Status = "failed"
		uc.mu.Unlock()
	case DomainEventGraphInterrupt:
		uc.mu.Lock()
		exec.Status = "waiting_human"
		uc.mu.Unlock()
	}
}

func (uc *GraphUsecase) TimeTravelGetState(ctx context.Context, executionID string, checkpointID string, namespace string) (any, error) {
	uc.mu.RLock()
	exec, ok := uc.executions[executionID]
	uc.mu.RUnlock()
	if !ok || exec.runtime == nil {
		return nil, ErrNotFound
	}
	return exec.runtime.TimeTravelGetState(ctx, exec.LineageID, checkpointID, namespace)
}

func (uc *GraphUsecase) TimeTravelHistory(ctx context.Context, executionID string, namespace string, limit int) (any, error) {
	uc.mu.RLock()
	exec, ok := uc.executions[executionID]
	uc.mu.RUnlock()
	if !ok || exec.runtime == nil {
		return nil, ErrNotFound
	}
	return exec.runtime.TimeTravelHistory(ctx, exec.LineageID, namespace, limit)
}

func (uc *GraphUsecase) TimeTravelEditState(ctx context.Context, executionID string, checkpointID string, namespace string, patch map[string]any) (any, error) {
	uc.mu.RLock()
	exec, ok := uc.executions[executionID]
	uc.mu.RUnlock()
	if !ok || exec.runtime == nil {
		return nil, ErrNotFound
	}
	return exec.runtime.TimeTravelEditState(ctx, exec.LineageID, checkpointID, namespace, patch)
}

func (uc *GraphUsecase) ListCheckpoints(ctx context.Context, executionID string, namespace string, limit int) (any, error) {
	uc.mu.RLock()
	exec, ok := uc.executions[executionID]
	uc.mu.RUnlock()
	if !ok || exec.runtime == nil {
		return nil, ErrNotFound
	}
	return exec.runtime.ListCheckpoints(ctx, exec.LineageID, namespace, limit)
}

func (uc *GraphUsecase) GetStateSnapshot(ctx context.Context, executionID string, checkpointID string, namespace string) (any, error) {
	uc.mu.RLock()
	exec, ok := uc.executions[executionID]
	uc.mu.RUnlock()
	if !ok || exec.runtime == nil {
		return nil, ErrNotFound
	}
	return exec.runtime.TimeTravelGetState(ctx, exec.LineageID, checkpointID, namespace)
}

func (uc *GraphUsecase) EditState(ctx context.Context, executionID string, checkpointID string, namespace string, patch map[string]any) (any, error) {
	uc.mu.RLock()
	exec, ok := uc.executions[executionID]
	uc.mu.RUnlock()
	if !ok || exec.runtime == nil {
		return nil, ErrNotFound
	}
	return exec.runtime.TimeTravelEditState(ctx, exec.LineageID, checkpointID, namespace, patch)
}

func (uc *GraphUsecase) VisualizeGraph(ctx context.Context, graphID string, format string) (any, error) {
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil, err
	}
	cfg := defToBuildConfig(def)
	return uc.factory.Visualize(ctx, cfg)
}

func (uc *GraphUsecase) ValidateGraph(ctx context.Context, graphID string) (any, error) {
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil, err
	}
	cfg := defToBuildConfig(def)
	return uc.factory.Validate(ctx, cfg)
}

func (uc *GraphUsecase) ListGraphTemplates(ctx context.Context) any {
	return uc.factory.ListTemplates()
}

func (uc *GraphUsecase) CreateGraphFromTemplate(ctx context.Context, templateID string, name string, description string) (*GraphDefinition, error) {
	tmpl, ok := uc.factory.GetTemplate(templateID)
	if !ok {
		return nil, fmt.Errorf("graph template %q not found", templateID)
	}
	def := uc.factory.TemplateToDef(tmpl, name, description)
	return uc.CreateGraph(ctx, def)
}

func (uc *GraphUsecase) FindNodeDef(ctx context.Context, graphID string, nodeID string) *NodeDefInfo {
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil
	}
	cfg := defToBuildConfig(def)
	return uc.factory.FindNodeDef(cfg, nodeID)
}

func defToBuildConfig(def *GraphDefinition) graphtrpc.GraphBuildConfig {
	return graphtrpc.GraphBuildConfig{
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
