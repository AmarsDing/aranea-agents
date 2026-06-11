package biz

import (
	"context"
	"time"
)

// ---------------------------------------------------------------------------
// Biz-layer value types — eliminate `any` from interface signatures
// ---------------------------------------------------------------------------

// GraphCheckpointRef is a stable pointer to a checkpoint.
type GraphCheckpointRef struct {
	LineageID    string `json:"lineage_id"`
	Namespace    string `json:"namespace,omitempty"`
	CheckpointID string `json:"checkpoint_id,omitempty"`
}

// GraphCheckpointInfo is a lightweight checkpoint header for history views.
type GraphCheckpointInfo struct {
	Ref              GraphCheckpointRef `json:"ref"`
	ParentCheckpoint string             `json:"parent_checkpoint,omitempty"`
	Source           string             `json:"source,omitempty"`
	Step             int                `json:"step"`
	Timestamp        time.Time          `json:"timestamp"`
}

// GraphCheckpointState is a checkpoint state snapshot suitable for debugging and HITL.
type GraphCheckpointState struct {
	Ref          GraphCheckpointRef `json:"ref"`
	ParentCheckpoint string         `json:"parent_checkpoint,omitempty"`
	Source       string             `json:"source,omitempty"`
	Step         int                `json:"step"`
	Timestamp    time.Time          `json:"timestamp"`
	State        map[string]any     `json:"state"`
	NextNodes    []string           `json:"next_nodes,omitempty"`
	NextChannels []string           `json:"next_channels,omitempty"`
}

// GraphEditedState is the result of editing a checkpoint state.
type GraphEditedState struct {
	Ref GraphCheckpointRef `json:"ref"`
}

// GraphCheckpointList is a list of checkpoint info entries.
type GraphCheckpointList []GraphCheckpointInfo

// GraphVisualizationNode represents a single node in a graph visualization.
type GraphVisualizationNode struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Shape       string `json:"shape"`
	FillColor   string `json:"fill_color"`
	BorderColor string `json:"border_color"`
}

// GraphVisualizationEdge represents an edge in a graph visualization.
type GraphVisualizationEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Type  string `json:"type"`
	Label string `json:"label,omitempty"`
}

// GraphVisualization is the result of visualizing a graph.
type GraphVisualization struct {
	Nodes []GraphVisualizationNode `json:"nodes"`
	Edges []GraphVisualizationEdge `json:"edges"`
	DOT   string                   `json:"dot"`
}

// GraphTemplateRef is a reference to a graph template with its metadata.
type GraphTemplateRef struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Category    string                  `json:"category"`
	Nodes       []GraphTemplateNodeRef  `json:"nodes"`
	Edges       []GraphTemplateEdgeRef  `json:"edges"`
	StateFields []StateFieldDef         `json:"state_fields"`
	EntryPoint  string                  `json:"entry_point"`
	FinishPoint string                  `json:"finish_point"`
}

// GraphTemplateNodeRef describes a node within a graph template.
type GraphTemplateNodeRef struct {
	NodeID      string `json:"node_id"`
	Type        string `json:"type"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// GraphTemplateEdgeRef describes an edge within a graph template.
type GraphTemplateEdgeRef struct {
	FromNode string `json:"from_node"`
	ToNode   string `json:"to_node"`
	Type     string `json:"type"`
	Label    string `json:"label,omitempty"`
}

// GraphRawEvent carries the raw event payload from the runtime.
type GraphRawEvent struct {
	Object string `json:"object"`
}

// ---------------------------------------------------------------------------
// Interfaces
// ---------------------------------------------------------------------------

type GraphRuntimeEvent struct {
	Type       DomainEventType
	NodeID     string
	Error      string
	StepNumber int
	RawEvent   GraphRawEvent
}

// GraphExecutionControl provides execution lifecycle methods for a graph runtime.
// Stability:stable
type GraphExecutionControl interface {
	Run(ctx context.Context, initialState map[string]any) (<-chan GraphRuntimeEvent, error)
	Resume(ctx context.Context, lineageID string, resumeValue map[string]any) (<-chan GraphRuntimeEvent, error)
	Cancel() error
	GetLineageID() string
}

// GraphCheckpoint provides checkpoint and time-travel inspection methods for a graph runtime.
// Stability:stable
type GraphCheckpoint interface {
	TimeTravelGetState(ctx context.Context, lineageID, checkpointID, namespace string) (*GraphCheckpointState, error)
	TimeTravelHistory(ctx context.Context, lineageID, namespace string, limit int) (GraphCheckpointList, error)
	TimeTravelEditState(ctx context.Context, lineageID, checkpointID, namespace string, patch map[string]any) (*GraphEditedState, error)
	ListCheckpoints(ctx context.Context, lineageID, namespace string, limit int) (GraphCheckpointList, error)
}

// GraphRuntime is the composite interface combining execution control and checkpoint access.
// It is kept for backward compatibility; consumers that only need one aspect should depend
// on the narrower GraphExecutionControl or GraphCheckpoint instead.
// Stability:stable
type GraphRuntime interface {
	GraphExecutionControl
	GraphCheckpoint
}

// GraphRunnerFactory builds graph runtimes for execution.
// Stability:evolving
type GraphRunnerFactory interface {
	BuildAndRun(ctx context.Context, cfg GraphBuildConfig, sessionID, graphID, execID string, initialState map[string]any) (GraphRuntime, <-chan GraphRuntimeEvent, error)
	BuildAndResume(ctx context.Context, cfg GraphBuildConfig, sessionID, graphID, execID, lineageID string, resumeValue map[string]any) (GraphRuntime, <-chan GraphRuntimeEvent, error)
	BuildRuntime(ctx context.Context, cfg GraphBuildConfig, sessionID, graphID, execID, lineageID string) (GraphRuntime, error)
}

// GraphVisualizer renders graph topology for display.
// Stability:stable
type GraphVisualizer interface {
	Visualize(ctx context.Context, cfg GraphBuildConfig) (*GraphVisualization, error)
}

// GraphValidator validates graph build configurations.
// Stability:stable
type GraphValidator interface {
	Validate(ctx context.Context, cfg GraphBuildConfig) (*GraphValidationResult, error)
}

// GraphTemplateProvider provides built-in graph templates.
// Stability:stable
type GraphTemplateProvider interface {
	ListTemplates() []GraphTemplateRef
	GetTemplate(templateID string) (GraphTemplateRef, bool)
	TemplateToDef(template GraphTemplateRef, name, description string) *GraphDefinition
}

// GraphNodeInfoProvider resolves node metadata for graph definitions.
// Stability:evolving
type GraphNodeInfoProvider interface {
	AgentExists(ctx context.Context, agentID string) bool
	FindNodeDef(cfg GraphBuildConfig, taskMeta map[string]NodeTaskMeta, nodeID string) *NodeTaskMeta
}

// GraphDefinitionFactory groups definition-related runtime capabilities:
// visualization, validation, and template management.
// Stability:evolving
type GraphDefinitionFactory interface {
	GraphVisualizer
	GraphValidator
	GraphTemplateProvider
}

// GraphBuilderFactory is a convenience composition for Wire providers that need
// all graph runtime capabilities. Consumers should prefer depending on the
// narrower sub-interfaces (GraphRunnerFactory, GraphDefinitionFactory,
// GraphNodeInfoProvider) when possible.
// Stability:evolving
type GraphBuilderFactory interface {
	GraphRunnerFactory
	GraphDefinitionFactory
	GraphNodeInfoProvider
}
