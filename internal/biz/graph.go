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

// DeliverableStateKey is the graph state field name used by set_deliverable/get_deliverable
// tools to share structured deliverables between team members (one agent's output → next agent's input).
// Reducer is Merge (top-level key union): topic-scoped writes from parallel members
// coexist; a no-topic write overwrites only the keys it carries.
const DeliverableStateKey = "deliverable"

// ExecutionEngineType selects the graph execution strategy.
type ExecutionEngineType string

const (
	EngineBSP ExecutionEngineType = ExecEngineBSP
	EngineDAG ExecutionEngineType = ExecEngineDAG
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
	ID                       string   `json:"id"`
	FuncRef                  string   `json:"func_ref"`
	Type                     string   `json:"type"`
	Description              string   `json:"description"`
	Instruction              string   `json:"instruction"`
	ModelName                string   `json:"model_name"`
	ToolNames                []string `json:"tool_names"`
	AgentName                string   `json:"agent_name"`
	InterruptBefore          bool     `json:"interrupt_before"`
	InterruptAfter           bool     `json:"interrupt_after"`
	Destinations             []string `json:"destinations"`
	RetryMaxAttempts         int      `json:"retry_max_attempts"`
	FailureAction            string   `json:"failure_action"`
	FallbackAgent            string   `json:"fallback_agent"`
	InputMapperJSON          string   `json:"input_mapper_json"`
	OutputMapperJSON         string   `json:"output_mapper_json"`
	IsolatedMessages         bool     `json:"isolated_messages"`
	InputFromLastResponse    bool     `json:"input_from_last_response"`
	CacheEnabled             bool     `json:"cache_enabled"`
	CacheTTLSeconds          int      `json:"cache_ttl_seconds"`
	RequiredRole             string   `json:"required_role"`
	AssignmentMode           string   `json:"assignment_mode"`
	AssignmentStrategy       string   `json:"assignment_strategy"`
	ReviewerAgent            string   `json:"reviewer_agent"`
	ReviewRules              string   `json:"review_rules"`
	TimeoutSeconds           int      `json:"timeout_seconds"`
	HeartbeatIntervalSeconds int      `json:"heartbeat_interval_seconds"`
	EnableLeaseExtension     bool     `json:"enable_lease_extension"`
}

// EdgeDef is a directed edge between two graph nodes.
type EdgeDef struct {
	From string `json:"from"`
	To   string `json:"to"`
	// Kind is optional metadata for visualization (e.g. "transfer" dashed edges). Runtime ignores unknown kinds.
	Kind string `json:"kind"`
}

// EndNodeID is the graph terminal sentinel mirrored from trpc-agent-go
// (trpcgraph.End = "__end__"). Conditional edge PathMap targets use it to
// terminate graph execution instead of scheduling another node.
const EndNodeID = "__end__"

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
	// CircuitBreaker is attached by ApplyCircuitBreakerPolicy (FP-02) for graph runtime.
	CircuitBreaker *CircuitBreakerPolicy `json:"circuit_breaker,omitempty"`
	// CircuitBreakerScope namespaces breaker keys (e.g. "team:{teamID}").
	CircuitBreakerScope string `json:"circuit_breaker_scope,omitempty"`
	// SwarmSafety carries Graph-path swarm limits (MaxHandoffs / repetitive handoff).
	SwarmSafety *SwarmSafetySpec `json:"swarm_safety,omitempty"`
	// MaxSteps is an explicit graph-execution step ceiling wired to
	// trpcgraph.WithMaxSteps. 0 means the framework default (100). Compile
	// paths that introduce loops (e.g. team critic_loop) set this explicitly
	// from the loop bound so a runaway graph is truncated close to the
	// expected iteration count instead of the opaque framework default.
	MaxSteps int `json:"max_steps,omitempty"`
}

// SwarmSafetySpec is the Graph-path equivalent of native team.SwarmConfig limits.
type SwarmSafetySpec struct {
	MaxHandoffs                int  `json:"max_handoffs"`
	RepetitiveHandoffWindow    int  `json:"repetitive_handoff_window"`
	RepetitiveHandoffMinUnique int  `json:"repetitive_handoff_min_unique"`
	CrossRequestTransfer       bool `json:"cross_request_transfer"`
}

// Swarm graph state keys (AS-FSM / swarm safety).
const (
	SwarmHandoffCountStateKey   = "_swarm_handoff_count"
	SwarmRecentTargetsStateKey  = "_swarm_recent_targets"
	SwarmActiveAgentSessionMeta = "swarm_active_agent"
)

// GraphExecutor is the biz-level port for executing graphs from other modules.
// Consumers (Channel, Cron) depend on this interface instead of *GraphService,
// following the dependency inversion principle. Wire binds it in service layer.
// Stability:evolving
type GraphExecutor interface {
	ExecuteGraphByID(ctx context.Context, graphID, sessionID string, initialState map[string]any) (executionID string, err error)
	ExecuteGraphBuildConfig(ctx context.Context, graphID, sessionID string, cfg GraphBuildConfig, initialState map[string]any) (executionID string, err error)
}

type GraphDefinition struct {
	ID                string
	Name              string
	Description       string
	StateFields       []StateFieldDef
	Nodes             []NodeDef
	Edges             []EdgeDef
	ConditionalEdges  []ConditionalEdgeDef
	Subgraphs         []SubgraphDef
	EntryPoint        string
	FinishPoint       string
	EnableCheckpoint  bool
	ExecutionEngine   ExecutionEngineType
	InterruptBefore   []string
	InterruptAfter    []string
	Metadata          map[string]any
	Version           int
	SortOrder         int
	VerificationGates string // JSON array of VerificationGate
	TeamID            string // owning team ID (empty for template graphs)
	IsTemplate        bool   // whether this graph is a reusable template
	// WorkspaceID is the owning workspace ID for tenant isolation (P2-B).
	// empty = shared/legacy (visible to all workspaces); non-empty = tenant-private.
	WorkspaceID string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type GraphExecution struct {
	ID              string
	GraphID         string
	SessionID       string
	SpiritSessionID string // cross-session aggregation key (root spirit session); equals SessionID for direct spirit runs
	// DefinitionHash is the SHA256 of the GraphBuildConfig the execution
	// started with (Y4). Resume compares it against the config resolved from
	// the CURRENT definition; a mismatch means the graph was edited after the
	// checkpoint was written and resuming would route stale state through a
	// changed topology. Empty for legacy rows (pre-20261214) — check skipped.
	DefinitionHash  string
	Status          string
	CurrentNode     string
	LineageID       string
	ErrorMessage    string
	CurrentState    map[string]any
	Steps           []GraphStepSnapshot
	InterruptNode   string
	interrupted     bool
	interruptMu     sync.RWMutex
	runtime         GraphRuntime
	StartedAt       time.Time
	FinishedAt      *time.Time
	execMu          sync.RWMutex    // protects Status, CurrentNode, LineageID, ErrorMessage, Steps, runtime, FinishedAt, streamGen
	evicted         bool            // set by GC before removing from map; not persisted
	ctx             context.Context // detached context preserving trace info for background DB writes
	// streamGen identifies the active event-stream generation (Y2). Run/Resume
	// increment it before spawning a consumer; a stale consumer (older gen)
	// must not converge terminal state or apply status-mutating events, or the
	// old stream ending would falsely fail the new, still-running stream.
	streamGen int64
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

// NextStreamGen increments and returns the stream generation (Y2). Call before
// spawning a new consumeRuntimeEvents goroutine and pass the value to it.
func (e *GraphExecution) NextStreamGen() int64 {
	e.execMu.Lock()
	defer e.execMu.Unlock()
	e.streamGen++
	return e.streamGen
}

// IsCurrentStream reports whether gen is still the active stream generation.
func (e *GraphExecution) IsCurrentStream(gen int64) bool {
	e.execMu.RLock()
	defer e.execMu.RUnlock()
	return e.streamGen == gen
}

// SnapshotForPersist returns a deep copy of the execution safe for
// out-of-lock DB writes. Caller must hold execMu (or RLock) while calling.
func (e *GraphExecution) SnapshotForPersist() *GraphExecution {
	snap := &GraphExecution{
		ID:              e.ID,
		GraphID:         e.GraphID,
		SessionID:       e.SessionID,
		SpiritSessionID: e.SpiritSessionID,
		DefinitionHash:  e.DefinitionHash,
		Status:          e.Status,
		CurrentNode:     e.CurrentNode,
		LineageID:       e.LineageID,
		ErrorMessage:    e.ErrorMessage,
		StartedAt:       e.StartedAt,
		FinishedAt:      e.FinishedAt,
	}
	if e.Steps != nil {
		snap.Steps = make([]GraphStepSnapshot, len(e.Steps))
		for i, s := range e.Steps {
			snap.Steps[i] = GraphStepSnapshot{
				NodeID:      s.NodeID,
				StepIndex:   s.StepIndex,
				InputState:  deepCopyMap(s.InputState),
				OutputState: deepCopyMap(s.OutputState),
				Status:      s.Status,
				Error:       s.Error,
				Timestamp:   s.Timestamp,
			}
		}
	}
	snap.InterruptNode = e.InterruptNode
	if e.CurrentState != nil {
		snap.CurrentState = deepCopyMap(e.CurrentState)
	}
	return snap
}

// deepCopyMap creates a deep copy of a map[string]any by round-tripping through
// JSON. This ensures that reference-typed values (slices, nested maps) are not
// shared between the original and the copy, preventing data races when the
// snapshot is used for out-of-lock DB writes.
func deepCopyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	raw, err := json.Marshal(src)
	if err != nil {
		// Fallback to shallow copy if serialization fails — better than crashing.
		out := make(map[string]any, len(src))
		for k, v := range src {
			out[k] = v
		}
		return out
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		out = make(map[string]any, len(src))
		for k, v := range src {
			out[k] = v
		}
		return out
	}
	return out
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

// GraphReader provides read access to graph definitions.
// Stability:stable
type GraphReader interface {
	GetDefinition(ctx context.Context, id string) (*GraphDefinition, error)
	GetDefinitionByName(ctx context.Context, name string) (*GraphDefinition, error)
	ListDefinitions(ctx context.Context, pageSize int, pageToken string) ([]*GraphDefinition, string, error)
	ListUserTemplateDefinitions(ctx context.Context, pageSize int) ([]*GraphDefinition, error)
	// ListDefinitionsByWorkspace returns graph definitions visible to the given workspace (P2-B).
	// empty workspaceID = system caller (see all); non-empty = tenant caller
	// (see shared + own). Stability:stable
	ListDefinitionsByWorkspace(ctx context.Context, pageSize int, pageToken string, workspaceID string) ([]*GraphDefinition, string, error)
	// ListUserTemplateDefinitionsByWorkspace returns user template graph definitions
	// visible to the given workspace (P2-B). Stability:stable
	ListUserTemplateDefinitionsByWorkspace(ctx context.Context, pageSize int, workspaceID string) ([]*GraphDefinition, error)
}

// GraphWriter provides write access to graph definitions.
// Stability:stable
type GraphWriter interface {
	SaveDefinition(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error)
	UpdateDefinition(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error)
	DeleteDefinition(ctx context.Context, id string) error
	ReorderGraphs(ctx context.Context, ids []string) error
}

// GraphRepo is the composite interface combining read and write operations.
// Kept for backward compatibility; new code should depend on GraphReader or GraphWriter.
// Stability:stable
type GraphRepo interface {
	GraphReader
	GraphWriter
}

// Stability:evolving
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

// teamDefinitionJSONAdapter adapts TeamReader to the TeamDefinitionJSONProvider
// interface used by GraphCacheManager for definition hash verification.
type teamDefinitionJSONAdapter struct {
	reader TeamReader
}

func (a *teamDefinitionJSONAdapter) GetTeamDefinitionJSON(ctx context.Context, teamID string) (string, error) {
	team, err := a.reader.GetTeamByID(ctx, teamID)
	if err != nil {
		return "", err
	}
	return team.DefinitionJSON, nil
}

// GraphUsecaseDeps groups the dependencies for GraphUsecase construction.
// Using a deps struct avoids long parameter lists (CS-B7).
type GraphUsecaseDeps struct {
	Repo         GraphRepo
	RunRepo      GraphRunRepo
	Factory      GraphBuilderFactory
	Observer     GraphExecutionObserver
	CompiledTeam CompiledTeamRepo
	TeamReader   TeamReader
	Lg           loggateway.Logger
	GCConfig     GraphGCConfig
}

// GraphUsecase is the facade that composes definition, execution, and cache
// sub-usecases. It preserves the original public API for backward compatibility
// while delegating internally to specialized components.
type GraphUsecase struct {
	defUC    *GraphDefinitionUsecase
	execUC   *GraphExecutionUsecase
	cacheMgr *GraphCacheManager
}

func NewGraphUsecase(d GraphUsecaseDeps) *GraphUsecase {
	cfg := d.GCConfig
	if cfg.Interval <= 0 || cfg.ExecutionMaxAge <= 0 || cfg.MaxExecutions <= 0 {
		cfg = DefaultGraphGCConfig()
	}
	defUC := NewGraphDefinitionUsecase(d.Repo, d.Factory, d.Factory, d.Lg)
	var teamDefProvider TeamDefinitionJSONProvider
	if d.TeamReader != nil {
		teamDefProvider = &teamDefinitionJSONAdapter{reader: d.TeamReader}
	}
	cacheMgr := NewGraphCacheManager(d.CompiledTeam, defUC, teamDefProvider, d.Lg)
	execUC := NewGraphExecutionUsecase(d.RunRepo, d.Factory, d.Observer, cacheMgr, defUC, d.Lg, cfg)
	return &GraphUsecase{
		defUC:    defUC,
		execUC:   execUC,
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
	case NodeTypeAgent, NodeTypeLLM, NodeTypeTool, NodeTypeTools, NodeTypeTask, NodeTypeReview:
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
	case NodeTypeTask, NodeTypeReview:
		return true
	default:
		return false
	}
}

// GraphGCConfig controls the graph execution garbage collector.
type GraphGCConfig struct {
	Interval        time.Duration
	ExecutionMaxAge time.Duration
	MaxExecutions   int
}

// DefaultGraphGCConfig returns the default GC configuration.
func DefaultGraphGCConfig() GraphGCConfig {
	return GraphGCConfig{
		Interval:        5 * time.Minute,
		ExecutionMaxAge: 30 * time.Minute,
		MaxExecutions:   500,
	}
}

func (uc *GraphUsecase) CreateGraph(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error) {
	return uc.defUC.CreateGraph(ctx, def)
}

func (uc *GraphUsecase) GetGraph(ctx context.Context, id string) (*GraphDefinition, error) {
	return uc.defUC.GetGraph(ctx, id)
}

func (uc *GraphUsecase) ListGraphs(ctx context.Context, pageSize int, pageToken string) ([]*GraphDefinition, string, error) {
	return uc.defUC.ListGraphs(ctx, pageSize, pageToken)
}

// ListGraphsByWorkspace returns graph definitions visible to the given workspace (P2-B).
func (uc *GraphUsecase) ListGraphsByWorkspace(ctx context.Context, pageSize int, pageToken string, workspaceID string) ([]*GraphDefinition, string, error) {
	return uc.defUC.ListGraphsByWorkspace(ctx, pageSize, pageToken, workspaceID)
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

func (uc *GraphUsecase) CreateGraphFromTemplate(ctx context.Context, templateID string, name string, description string, workspaceID string) (*GraphDefinition, error) {
	return uc.defUC.CreateGraphFromTemplate(ctx, templateID, name, description, workspaceID)
}

func (uc *GraphUsecase) ExportGraph(ctx context.Context, graphID string) ([]byte, *GraphDefinition, error) {
	return uc.defUC.ExportGraph(ctx, graphID)
}

func (uc *GraphUsecase) ImportGraph(ctx context.Context, raw []byte, name, description, workspaceID string) (*GraphDefinition, error) {
	return uc.defUC.ImportGraph(ctx, raw, name, description, workspaceID)
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

// ListUserTemplateGraphsByWorkspace returns user template graphs visible to the given workspace (P2-B).
func (uc *GraphUsecase) ListUserTemplateGraphsByWorkspace(ctx context.Context, workspaceID string) ([]*GraphDefinition, error) {
	return uc.defUC.ListUserTemplateGraphsByWorkspace(ctx, workspaceID)
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

func (uc *GraphUsecase) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, spiritSessionID, teamID, teamRunID, linkedGraphID string, ct *CompiledTeam) error {
	return uc.execUC.RegisterTeamGraphExecution(ctx, execID, sessionID, spiritSessionID, teamID, teamRunID, linkedGraphID, ct)
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
