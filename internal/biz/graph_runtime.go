package biz

import "context"

type GraphRuntimeEvent struct {
	Type       DomainEventType
	NodeID     string
	Error      string
	StepNumber int
	RawEvent   any
}

type GraphRuntime interface {
	Run(ctx context.Context, initialState map[string]any) (<-chan GraphRuntimeEvent, error)
	Resume(ctx context.Context, lineageID string, resumeValue map[string]any) (<-chan GraphRuntimeEvent, error)
	Cancel() error
	TimeTravelGetState(ctx context.Context, lineageID, checkpointID, namespace string) (any, error)
	TimeTravelHistory(ctx context.Context, lineageID, namespace string, limit int) (any, error)
	TimeTravelEditState(ctx context.Context, lineageID, checkpointID, namespace string, patch map[string]any) (any, error)
	ListCheckpoints(ctx context.Context, lineageID, namespace string, limit int) (any, error)
	GetLineageID() string
}

type NodeDefInfo struct {
	RequiredRole             string
	AssignmentMode           string
	AssignmentStrategy       string
	ReviewerAgent            string
	ReviewRules              string
	TimeoutSeconds           int
	HeartbeatIntervalSeconds int
	EnableLeaseExtension     bool
}

// GraphBuilderFactory converts biz-level GraphBuildConfig into a running GraphRuntime.
// Implementations live in internal/graph/trpc/ and may import trpc-agent-go freely.
type GraphBuilderFactory interface {
	BuildAndRun(ctx context.Context, cfg GraphBuildConfig, sessionID, graphID, execID string, initialState map[string]any) (GraphRuntime, <-chan GraphRuntimeEvent, error)
	BuildAndResume(ctx context.Context, cfg GraphBuildConfig, sessionID, graphID, execID, lineageID string, resumeValue map[string]any) (GraphRuntime, <-chan GraphRuntimeEvent, error)
	BuildRuntime(ctx context.Context, cfg GraphBuildConfig, sessionID, graphID, execID, lineageID string) (GraphRuntime, error)
	Visualize(ctx context.Context, cfg GraphBuildConfig) (any, error)
	Validate(ctx context.Context, cfg GraphBuildConfig) (*GraphValidationResult, error)
	ListTemplates() any
	GetTemplate(templateID string) (any, bool)
	TemplateToDef(template any, name, description string) *GraphDefinition
	AgentExists(agentID string) bool
	FindNodeDef(cfg GraphBuildConfig, nodeID string) *NodeDefInfo
}
