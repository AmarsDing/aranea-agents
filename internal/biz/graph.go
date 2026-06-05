package biz

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
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
	Name            string      `json:"name"`
	Type            string      `json:"type"`
	Reducer         ReducerType `json:"reducer"`
	DefaultValue    any         `json:"default_value"`
	Required        bool        `json:"required"`
	DisableDeepCopy bool        `json:"disable_deep_copy"`
}

// NodeDef is the schema-level (biz) description of a graph node.
// Func/function pointers are resolved in the graph/trpc adapter layer.
type NodeDef struct {
	ID                    string   `json:"id"`
	FuncRef               string   `json:"func_ref"`
	Type                  string   `json:"type"`
	Description           string   `json:"description"`
	Instruction           string   `json:"instruction"`
	ModelName             string   `json:"model_name"`
	ToolNames             []string `json:"tool_names"`
	AgentName             string   `json:"agent_name"`
	InterruptBefore       bool     `json:"interrupt_before"`
	InterruptAfter        bool     `json:"interrupt_after"`
	Destinations          []string `json:"destinations"`
	RetryMaxAttempts      int      `json:"retry_max_attempts"`
	FailureAction         string   `json:"failure_action"`
	FallbackAgent         string   `json:"fallback_agent"`
	InputMapperJSON       string   `json:"input_mapper_json"`
	OutputMapperJSON      string   `json:"output_mapper_json"`
	IsolatedMessages      bool     `json:"isolated_messages"`
	InputFromLastResponse bool     `json:"input_from_last_response"`
	CacheEnabled          bool     `json:"cache_enabled"`
	CacheTTLSeconds       int      `json:"cache_ttl_seconds"`
}

// EdgeDef is a directed edge between two graph nodes.
type EdgeDef struct {
	From string `json:"from"`
	To   string `json:"to"`
	// Kind is optional metadata for visualization (e.g. "transfer" dashed edges). Runtime ignores unknown kinds.
	Kind string `json:"kind"`
}

// ConditionalEdgeDef is a conditional routing edge.
// CondFunc is resolved in the graph/trpc adapter layer.
type ConditionalEdgeDef struct {
	From        string            `json:"from"`
	CondFuncRef string            `json:"cond_func_ref"`
	PathMap     map[string]string `json:"path_map"`
}

// SubgraphDef embeds a nested graph inside a parent graph.
// InputMapper/OutputMapper are trpc-specific and resolved in the adapter layer.
type SubgraphDef struct {
	ID              string           `json:"id"`
	GraphID         string           `json:"graph_id"`
	BuildConfig     GraphBuildConfig `json:"build_config"`
	InterruptBefore bool             `json:"interrupt_before"`
	InterruptAfter  bool             `json:"interrupt_after"`
}

// GraphBuildConfig is the schema-level (biz) graph build configuration.
type GraphBuildConfig struct {
	Nodes            []NodeDef            `json:"nodes"`
	Edges            []EdgeDef            `json:"edges"`
	ConditionalEdges []ConditionalEdgeDef `json:"conditional_edges"`
	Subgraphs        []SubgraphDef        `json:"subgraphs"`
	StateFields      []StateFieldDef      `json:"state_fields"`
	EntryPoint       string               `json:"entry_point"`
	FinishPoint      string               `json:"finish_point"`
	EnableCheckpoint bool                 `json:"enable_checkpoint"`
	ExecutionEngine  ExecutionEngineType  `json:"execution_engine"`
	InterruptBefore  []string             `json:"interrupt_before"`
	InterruptAfter   []string             `json:"interrupt_after"`
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
	interrupted   bool
	interruptMu   sync.RWMutex
	runtime       GraphRuntime
	StartedAt     time.Time
	FinishedAt    *time.Time
	execMu        sync.RWMutex // protects Status, CurrentNode, LineageID, ErrorMessage, Steps, runtime, FinishedAt
	evicted       bool         // set by GC before removing from map; not persisted
	ctx           context.Context // detached context preserving trace info for background DB writes
}

// NewGraphExecution creates a GraphExecution with mandatory ctx initialization.
func NewGraphExecution(ctx context.Context, id, graphID, sessionID, status string) *GraphExecution {
	return &GraphExecution{
		ID:        id,
		GraphID:   graphID,
		SessionID: sessionID,
		Status:    status,
		StartedAt: time.Now(),
		ctx:       ctx,
	}
}

func (e *GraphExecution) GetStatus() string {
	e.execMu.RLock()
	defer e.execMu.RUnlock()
	return e.Status
}

func (e *GraphExecution) IsEvicted() bool {
	e.execMu.RLock()
	defer e.execMu.RUnlock()
	return e.evicted
}

// SetEvicted marks the execution as evicted from the in-memory cache.
// Must be called while holding uc.mu (the usecase-level lock).
func (e *GraphExecution) SetEvicted() {
	e.execMu.Lock()
	e.evicted = true
	e.execMu.Unlock()
}

// SnapshotForPersist returns a shallow copy of the execution safe for
// out-of-lock DB writes. Caller must hold execMu (or RLock) while calling.
func (e *GraphExecution) SnapshotForPersist() *GraphExecution {
	snap := &GraphExecution{
		ID:           e.ID,
		GraphID:      e.GraphID,
		SessionID:    e.SessionID,
		Status:       e.Status,
		CurrentNode:  e.CurrentNode,
		LineageID:    e.LineageID,
		ErrorMessage: e.ErrorMessage,
		StartedAt:    e.StartedAt,
		FinishedAt:   e.FinishedAt,
	}
	if e.Steps != nil {
		snap.Steps = make([]GraphStepSnapshot, len(e.Steps))
		copy(snap.Steps, e.Steps)
	}
	snap.InterruptNode = e.InterruptNode
	return snap
}

func (e *GraphExecution) IsInterrupted() bool {
	e.interruptMu.RLock()
	defer e.interruptMu.RUnlock()
	return e.interrupted
}

func (e *GraphExecution) GetInterruptNode() string {
	e.interruptMu.RLock()
	defer e.interruptMu.RUnlock()
	return e.InterruptNode
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

// GraphReader provides read-only access to graph definitions.
type GraphReader interface {
	GetDefinition(ctx context.Context, id string) (*GraphDefinition, error)
	ListDefinitions(ctx context.Context, pageSize int, pageToken string) ([]*GraphDefinition, string, error)
	ListUserTemplateDefinitions(ctx context.Context, pageSize int) ([]*GraphDefinition, error)
}

// GraphWriter provides write access to graph definitions.
type GraphWriter interface {
	SaveDefinition(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error)
	UpdateDefinition(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error)
	DeleteDefinition(ctx context.Context, id string) error
	ReorderGraphs(ctx context.Context, ids []string) error
}

// GraphRepo is the composite interface combining read and write operations.
// Kept for backward compatibility; new code should depend on GraphReader or GraphWriter.
type GraphRepo interface {
	GraphReader
	GraphWriter
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

// GraphUsecase is the facade that composes definition, execution, and cache
// sub-usecases. It preserves the original public API for backward compatibility
// while delegating internally to specialized components.
type GraphUsecase struct {
	defUC   *GraphDefinitionUsecase
	execUC  *GraphExecutionUsecase
	cacheMgr *GraphCacheManager
}

func NewGraphUsecase(repo GraphRepo, runRepo GraphRunRepo, factory GraphBuilderFactory, observer GraphExecutionObserver, compiledTeamRepo CompiledTeamRepo, lg loggateway.Logger) *GraphUsecase {
	defUC := NewGraphDefinitionUsecase(repo, factory, lg)
	cacheMgr := NewGraphCacheManager(compiledTeamRepo, defUC, lg)
	execUC := NewGraphExecutionUsecase(runRepo, factory, observer, cacheMgr, defUC, lg)
	return &GraphUsecase{
		defUC:   defUC,
		execUC:  execUC,
		cacheMgr: cacheMgr,
	}
}

// DefUC returns the embedded GraphDefinitionUsecase for callers that only need definition operations.
func (uc *GraphUsecase) DefUC() *GraphDefinitionUsecase { return uc.defUC }

// ExecUC returns the embedded GraphExecutionUsecase for callers that need execution operations.
func (uc *GraphUsecase) ExecUC() *GraphExecutionUsecase { return uc.execUC }

// CacheMgr returns the embedded GraphCacheManager for callers that need cache operations.
func (uc *GraphUsecase) CacheMgr() *GraphCacheManager { return uc.cacheMgr }

func (uc *GraphUsecase) SetTaskCoordinator(c GraphTaskCoordinator) {
	uc.execUC.SetTaskCoordinator(c)
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
func ShouldCreateTaskForNode(node *NodeDef, meta NodeTaskMeta) bool {
	if node == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(node.Type)) {
	case "agent", "llm", "tool", "tools", "task", "review":
		return true
	default:
		return meta.RequiredRole != "" || meta.AssignmentMode != "" || meta.ReviewerAgent != ""
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
const maxExecutions = 500 // per-node execution cache limit; estimated at ~1KB per execution, ~500KB per node

func (uc *GraphUsecase) CreateGraph(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error) {
	return uc.defUC.CreateGraph(ctx, def)
}

func (uc *GraphUsecase) GetGraph(ctx context.Context, id string) (*GraphDefinition, error) {
	return uc.defUC.GetGraph(ctx, id)
}

func (uc *GraphUsecase) ListGraphs(ctx context.Context, pageSize int, pageToken string) ([]*GraphDefinition, string, error) {
	return uc.defUC.ListGraphs(ctx, pageSize, pageToken)
}

func (uc *GraphUsecase) UpdateGraph(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error) {
	return uc.defUC.UpdateGraph(ctx, def)
}

func (uc *GraphUsecase) DeleteGraph(ctx context.Context, id string) error {
	return uc.defUC.DeleteGraph(ctx, id)
}

func (uc *GraphUsecase) ReorderGraphs(ctx context.Context, ids []string) error {
	return uc.defUC.ReorderGraphs(ctx, ids)
}

func (uc *GraphUsecase) VisualizeGraph(ctx context.Context, graphID string, format string) (*GraphVisualization, error) {
	return uc.defUC.VisualizeGraph(ctx, graphID, format)
}

func (uc *GraphUsecase) ValidateGraph(ctx context.Context, graphID string) (*GraphValidationResult, error) {
	return uc.defUC.ValidateGraph(ctx, graphID)
}

func (uc *GraphUsecase) ListGraphTemplates(ctx context.Context) []GraphTemplateRef {
	return uc.defUC.ListGraphTemplates(ctx)
}

func (uc *GraphUsecase) CreateGraphFromTemplate(ctx context.Context, templateID string, name string, description string) (*GraphDefinition, error) {
	return uc.defUC.CreateGraphFromTemplate(ctx, templateID, name, description)
}

func (uc *GraphUsecase) ExportGraph(ctx context.Context, graphID string) ([]byte, *GraphDefinition, error) {
	return uc.defUC.ExportGraph(ctx, graphID)
}

func (uc *GraphUsecase) ImportGraph(ctx context.Context, raw []byte, name, description string) (*GraphDefinition, error) {
	return uc.defUC.ImportGraph(ctx, raw, name, description)
}

func (uc *GraphUsecase) ListGraphVersions(ctx context.Context, graphID string) ([]GraphVersionEntry, error) {
	return uc.defUC.ListGraphVersions(ctx, graphID)
}

func (uc *GraphUsecase) RollbackGraphVersion(ctx context.Context, graphID string, version int) (*GraphDefinition, error) {
	return uc.defUC.RollbackGraphVersion(ctx, graphID, version)
}

func (uc *GraphUsecase) SaveGraphAsTemplate(ctx context.Context, graphID, templateName, category, description string) (*UserTemplateMeta, error) {
	return uc.defUC.SaveGraphAsTemplate(ctx, graphID, templateName, category, description)
}

func (uc *GraphUsecase) ListUserTemplateGraphs(ctx context.Context) ([]*GraphDefinition, error) {
	return uc.defUC.ListUserTemplateGraphs(ctx)
}

func (uc *GraphUsecase) FindNodeDef(ctx context.Context, graphID string, nodeID string) *NodeTaskMeta {
	return uc.defUC.FindNodeDef(ctx, graphID, nodeID)
}

func (uc *GraphUsecase) FindGraphNode(ctx context.Context, graphID string, nodeID string) *NodeDef {
	return uc.defUC.FindGraphNode(ctx, graphID, nodeID)
}

// ---------------------------------------------------------------------------
// Execution delegation
// ---------------------------------------------------------------------------

func (uc *GraphUsecase) ExecuteGraph(ctx context.Context, graphID, sessionID, execID string, initialState map[string]any) (*GraphExecution, error) {
	return uc.execUC.ExecuteGraph(ctx, graphID, sessionID, execID, initialState)
}

func (uc *GraphUsecase) ExecuteGraphBuildConfig(ctx context.Context, graphID, sessionID, execID string, cfg GraphBuildConfig, initialState map[string]any) (*GraphExecution, error) {
	return uc.execUC.ExecuteGraphBuildConfig(ctx, graphID, sessionID, execID, cfg, initialState)
}

func (uc *GraphUsecase) GetExecution(ctx context.Context, executionID string) (*GraphExecution, error) {
	return uc.execUC.GetExecution(ctx, executionID)
}

func (uc *GraphUsecase) ListExecutions(ctx context.Context, graphID string, pageSize int, pageToken string, opts ...GraphRunListOption) ([]*GraphExecution, string, error) {
	return uc.execUC.ListExecutions(ctx, graphID, pageSize, pageToken, opts...)
}

func (uc *GraphUsecase) CancelExecution(ctx context.Context, executionID string) error {
	return uc.execUC.CancelExecution(ctx, executionID)
}

func (uc *GraphUsecase) ResumeExecution(ctx context.Context, executionID string, resumeValue map[string]any) (*GraphExecution, error) {
	return uc.execUC.ResumeExecution(ctx, executionID, resumeValue)
}

func (uc *GraphUsecase) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, teamID, teamRunID string, ct *CompiledTeam) error {
	return uc.execUC.RegisterTeamGraphExecution(ctx, execID, sessionID, teamID, teamRunID, ct)
}

func (uc *GraphUsecase) MarkTeamGraphInterrupt(ctx context.Context, execID, nodeID, lineageID string) error {
	return uc.execUC.MarkTeamGraphInterrupt(ctx, execID, nodeID, lineageID)
}

func (uc *GraphUsecase) TimeTravelGetState(ctx context.Context, executionID string, checkpointID string, namespace string) (*GraphCheckpointState, error) {
	return uc.execUC.TimeTravelGetState(ctx, executionID, checkpointID, namespace)
}

func (uc *GraphUsecase) TimeTravelHistory(ctx context.Context, executionID string, namespace string, limit int) (GraphCheckpointList, error) {
	return uc.execUC.TimeTravelHistory(ctx, executionID, namespace, limit)
}

func (uc *GraphUsecase) TimeTravelEditState(ctx context.Context, executionID string, checkpointID string, namespace string, patch map[string]any) (*GraphEditedState, error) {
	return uc.execUC.TimeTravelEditState(ctx, executionID, checkpointID, namespace, patch)
}

func (uc *GraphUsecase) ListCheckpoints(ctx context.Context, executionID string, namespace string, limit int) (GraphCheckpointList, error) {
	return uc.execUC.ListCheckpoints(ctx, executionID, namespace, limit)
}

func (uc *GraphUsecase) GetStateSnapshot(ctx context.Context, executionID string, checkpointID string, namespace string) (*GraphCheckpointState, error) {
	return uc.execUC.GetStateSnapshot(ctx, executionID, checkpointID, namespace)
}

func (uc *GraphUsecase) EditState(ctx context.Context, executionID string, checkpointID string, namespace string, patch map[string]any) (*GraphEditedState, error) {
	return uc.execUC.EditState(ctx, executionID, checkpointID, namespace, patch)
}

// buildConfigForExecution delegates to the cache manager for backward-compatible test access.
func (uc *GraphUsecase) buildConfigForExecution(ctx context.Context, exec *GraphExecution) (*CompiledTeam, error) {
	return uc.cacheMgr.BuildConfigForExecution(ctx, exec)
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
