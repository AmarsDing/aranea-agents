package biz

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// ReducerType controls how state field values are merged.
type ReducerType string

const (
	ReducerDefault ReducerType = "default"
	ReducerAppend  ReducerType = "append"
	ReducerCover   ReducerType = "cover"
	ReducerMerge   ReducerType = "merge"
)

// ExecutionEngineType selects the graph execution strategy.
type ExecutionEngineType string

const (
	EngineBSP ExecutionEngineType = "bsp"
	EngineDAG ExecutionEngineType = "dag"
)

// StateFieldDef describes a single typed state field in a graph.
type StateFieldDef struct {
	Name            string
	Type            string
	Reducer         ReducerType
	DefaultValue    any
	Required        bool
	DisableDeepCopy bool
}

// NodeDef is the schema-level (biz) description of a graph node.
// Func/function pointers are resolved in the graph/trpc adapter layer.
type NodeDef struct {
	ID                       string
	FuncRef                  string
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

// EdgeDef is a directed edge between two graph nodes.
type EdgeDef struct {
	From string
	To   string
	// Kind is optional metadata for visualization (e.g. "transfer" dashed edges). Runtime ignores unknown kinds.
	Kind string
}

// ConditionalEdgeDef is a conditional routing edge.
// CondFunc is resolved in the graph/trpc adapter layer.
type ConditionalEdgeDef struct {
	From        string
	CondFuncRef string
	PathMap     map[string]string
}

// SubgraphDef embeds a nested graph inside a parent graph.
// InputMapper/OutputMapper are trpc-specific and resolved in the adapter layer.
type SubgraphDef struct {
	ID              string
	GraphID         string
	BuildConfig     GraphBuildConfig
	InterruptBefore bool
	InterruptAfter  bool
}

// GraphBuildConfig is the schema-level (biz) graph build configuration.
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
	FailurePolicy    *TeamFailurePolicy
	// ParallelBranchIDs lists explicit parallel branch agent node ids (from embedded join compile).
	ParallelBranchIDs []string
}

// GraphExecutor is the biz-level port for executing graphs from other modules.
// Consumers (Channel, Cron) depend on this interface instead of *GraphService,
// following the dependency inversion principle. Wire binds it in service layer.
type GraphExecutor interface {
	ExecuteGraphByID(ctx context.Context, graphID, sessionID string, initialState map[string]any) (executionID string, err error)
	ExecuteGraphBuildConfig(ctx context.Context, graphID, sessionID string, cfg GraphBuildConfig, initialState map[string]any) (executionID string, err error)
}

type GraphDefinition struct {
	ID               string
	Name             string
	Description      string
	StateFields      []StateFieldDef
	Nodes            []NodeDef
	Edges            []EdgeDef
	ConditionalEdges []ConditionalEdgeDef
	Subgraphs        []SubgraphDef
	EntryPoint       string
	FinishPoint      string
	EnableCheckpoint bool
	ExecutionEngine  ExecutionEngineType
	InterruptBefore  []string
	InterruptAfter   []string
	Metadata         map[string]any
	Version          int
	SortOrder        int
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
	ListUserTemplateDefinitions(ctx context.Context, pageSize int) ([]*GraphDefinition, error)
	UpdateDefinition(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error)
	DeleteDefinition(ctx context.Context, id string) error
	ReorderGraphs(ctx context.Context, ids []string) error
}

type GraphRunRepo interface {
	SaveRun(ctx context.Context, exec *GraphExecution) error
	GetRun(ctx context.Context, id string) (*GraphExecution, error)
	ListRunsByGraph(ctx context.Context, graphID string, pageSize int, pageToken string, opts ...GraphRunListOption) ([]*GraphExecution, string, error)
	UpdateRun(ctx context.Context, exec *GraphExecution) error
}

type GraphRunListOption struct {
	Status       string
	StartedAfter *time.Time
}

type GraphUsecase struct {
	repo             GraphRepo
	runRepo          GraphRunRepo
	factory          GraphBuilderFactory
	execObserver     GraphExecutionObserver
	taskCoord        GraphTaskCoordinator
	mu               sync.RWMutex
	defs             map[string]*GraphDefinition
	executions       map[string]*GraphExecution
	teamBuildConfigs map[string]GraphBuildConfig
	lg               loggateway.Logger
}

func NewGraphUsecase(repo GraphRepo, runRepo GraphRunRepo, factory GraphBuilderFactory, observer GraphExecutionObserver, lg loggateway.Logger) *GraphUsecase {
	uc := &GraphUsecase{
		repo:         repo,
		runRepo:      runRepo,
		factory:      factory,
		execObserver: observer,
		defs:         make(map[string]*GraphDefinition),
		executions:   make(map[string]*GraphExecution),
		lg:           lg,
	}
	safego.Go(context.Background(), "graph-gc-loop", func() { uc.gcLoop() })
	return uc
}

func (uc *GraphUsecase) SetTaskCoordinator(c GraphTaskCoordinator) {
	uc.taskCoord = c
}

func nodeDefFromConfig(cfg GraphBuildConfig, nodeID string) *NodeDef {
	for i := range cfg.Nodes {
		if cfg.Nodes[i].ID == nodeID {
			n := cfg.Nodes[i]
			return &n
		}
	}
	return nil
}

// ShouldCreateTaskForNode reports whether a standalone Graph run should spawn a Kanban task row (M54).
func ShouldCreateTaskForNode(node *NodeDef) bool {
	if node == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(node.Type)) {
	case "agent", "llm", "tool", "tools", "task", "review":
		return true
	default:
		return node.RequiredRole != "" || node.AssignmentMode != "" || node.ReviewerAgent != ""
	}
}

// ShouldCreateTeamGraphTaskNode reports whether a Team Graph run should spawn a human task row (M53 TG-RT-TASK).
// Agent/LLM/tool nodes are executed inline and must not create Kanban tasks.
func ShouldCreateTeamGraphTaskNode(node *NodeDef) bool {
	if node == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(node.Type)) {
	case "task", "review":
		return true
	default:
		return false
	}
}

const gcInterval = 5 * time.Minute
const executionMaxAge = 30 * time.Minute
const maxExecutions = 500

func (uc *GraphUsecase) gcLoop() {
	ticker := time.NewTicker(gcInterval)
	defer ticker.Stop()
	for range ticker.C {
		uc.gc()
	}
}

func (uc *GraphUsecase) gc() {
	uc.mu.Lock()
	var expired []*GraphExecution
	now := time.Now()
	for id, exec := range uc.executions {
		if exec.Status == "running" || exec.Status == "waiting_human" {
			continue
		}
		if exec.FinishedAt != nil && now.Sub(*exec.FinishedAt) > executionMaxAge {
			delete(uc.executions, id)
			delete(uc.teamBuildConfigs, id)
		} else if exec.FinishedAt == nil && now.Sub(exec.StartedAt) > executionMaxAge {
			exec.Status = "failed"
			exec.ErrorMessage = "execution expired: no activity within timeout"
			nowCopy := now
			exec.FinishedAt = &nowCopy
			expired = append(expired, exec)
			delete(uc.executions, id)
			delete(uc.teamBuildConfigs, id)
		}
	}
	uc.mu.Unlock()

	// Persist expired executions to repo before discarding from memory.
	for _, exec := range expired {
		_ = uc.runRepo.UpdateRun(context.Background(), exec)
	}
}

func (uc *GraphUsecase) CreateGraph(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error) {
	if def.ID == "" {
		def.ID = uuid.New().String()
	}
	now := time.Now()
	def.CreatedAt = now
	def.UpdatedAt = now
	if def.Version <= 0 {
		def.Version = 1
	}
	syncVersionMetadata(def)
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
	previous, err := uc.repo.GetDefinition(ctx, def.ID)
	if err != nil {
		return nil, err
	}
	appendVersionHistory(def, previous)
	now := time.Now()
	def.UpdatedAt = now
	syncVersionMetadata(def)
	saved, err := uc.repo.UpdateDefinition(ctx, def)
	if err != nil {
		return nil, err
	}
	syncVersionMetadata(saved)
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

func (uc *GraphUsecase) ReorderGraphs(ctx context.Context, ids []string) error {
	return uc.repo.ReorderGraphs(ctx, ids)
}

func (uc *GraphUsecase) VisualizeGraph(ctx context.Context, graphID string, format string) (any, error) {
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil, err
	}
	cfg := defToBuildConfig(def)
	return uc.factory.Visualize(ctx, cfg)
}

func (uc *GraphUsecase) ValidateGraph(ctx context.Context, graphID string) (*GraphValidationResult, error) {
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
	if strings.HasPrefix(templateID, UserTemplateIDPrefix) {
		graphID := strings.TrimPrefix(templateID, UserTemplateIDPrefix)
		src, err := uc.GetGraph(ctx, graphID)
		if err != nil {
			return nil, err
		}
		if ReadUserTemplateMeta(src) == nil {
			return nil, ErrGraphTemplateNotFound
		}
		def := cloneGraphDefinition(src)
		def.ID = ""
		def.Name = name
		def.Description = description
		def.Version = 0
		if def.Metadata != nil {
			delete(def.Metadata, GraphMetadataUserTemplateKey)
			delete(def.Metadata, GraphMetadataVersionHistoryKey)
		}
		return uc.CreateGraph(ctx, def)
	}
	tmpl, ok := uc.factory.GetTemplate(templateID)
	if !ok {
		return nil, ErrGraphTemplateNotFound
	}
	def := uc.factory.TemplateToDef(tmpl, name, description)
	return uc.CreateGraph(ctx, def)
}

func (uc *GraphUsecase) ExportGraph(ctx context.Context, graphID string) ([]byte, *GraphDefinition, error) {
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil, nil, err
	}
	export := cloneGraphDefinition(def)
	syncVersionMetadata(export)
	raw, err := json.Marshal(export)
	if err != nil {
		return nil, nil, err
	}
	return raw, export, nil
}

func (uc *GraphUsecase) ImportGraph(ctx context.Context, raw []byte, name, description string) (*GraphDefinition, error) {
	var def GraphDefinition
	if err := json.Unmarshal(raw, &def); err != nil {
		return nil, errors.BadRequest("GRAPH", "invalid graph json")
	}
	def.ID = ""
	if strings.TrimSpace(name) != "" {
		def.Name = name
	}
	if strings.TrimSpace(description) != "" {
		def.Description = description
	}
	def.Version = 0
	if def.Metadata != nil {
		delete(def.Metadata, GraphMetadataVersionHistoryKey)
	}
	cfg := BuildConfigFromGraphDefinition(&def)
	if err := uc.ensureValidBuildConfig(ctx, cfg); err != nil {
		return nil, err
	}
	return uc.CreateGraph(ctx, &def)
}

func (uc *GraphUsecase) ensureValidBuildConfig(ctx context.Context, cfg GraphBuildConfig) error {
	result, err := uc.factory.Validate(ctx, cfg)
	if err != nil {
		return err
	}
	if result != nil && result.HasErrors() {
		return errors.BadRequest("GRAPH", "graph failed validation")
	}
	return nil
}

func (uc *GraphUsecase) ListGraphVersions(ctx context.Context, graphID string) ([]GraphVersionEntry, error) {
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil, err
	}
	return ListGraphVersionEntries(def), nil
}

func (uc *GraphUsecase) RollbackGraphVersion(ctx context.Context, graphID string, version int) (*GraphDefinition, error) {
	current, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil, err
	}
	snapshot := FindGraphVersionSnapshot(current, version)
	if snapshot == nil {
		return nil, errors.NotFound("GRAPH", "graph version not found")
	}
	restored := cloneGraphDefinition(snapshot)
	restored.ID = graphID
	restored.CreatedAt = current.CreatedAt
	return uc.UpdateGraph(ctx, restored)
}

func (uc *GraphUsecase) SaveGraphAsTemplate(ctx context.Context, graphID, templateName, category, description string) (*UserTemplateMeta, error) {
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(category) == "" {
		category = "custom"
	}
	meta := UserTemplateMeta{
		TemplateID:  UserTemplateIDPrefix + graphID,
		Name:        templateName,
		Category:    category,
		Description: description,
	}
	WriteUserTemplateMeta(def, meta)
	def.UpdatedAt = time.Now()
	saved, err := uc.repo.UpdateDefinition(ctx, def)
	if err != nil {
		return nil, err
	}
	uc.mu.Lock()
	uc.defs[saved.ID] = saved
	uc.mu.Unlock()
	return ReadUserTemplateMeta(saved), nil
}

func (uc *GraphUsecase) ListUserTemplateGraphs(ctx context.Context) ([]*GraphDefinition, error) {
	return uc.repo.ListUserTemplateDefinitions(ctx, 200)
}

func (uc *GraphUsecase) FindNodeDef(ctx context.Context, graphID string, nodeID string) *NodeDefInfo {
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil
	}
	cfg := defToBuildConfig(def)
	return uc.factory.FindNodeDef(cfg, nodeID)
}

func (uc *GraphUsecase) FindGraphNode(ctx context.Context, graphID string, nodeID string) *NodeDef {
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil
	}
	return nodeDefFromConfig(defToBuildConfig(def), nodeID)
}

func defToBuildConfig(def *GraphDefinition) GraphBuildConfig {
	return BuildConfigFromGraphDefinition(def)
}

// BuildConfigFromGraphDefinition maps a persisted graph definition to runtime build config.
func BuildConfigFromGraphDefinition(def *GraphDefinition) GraphBuildConfig {
	if def == nil {
		return GraphBuildConfig{}
	}
	return GraphBuildConfig{
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
