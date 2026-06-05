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

type GraphRunnerFactory interface {
	BuildAndRun(ctx context.Context, cfg GraphBuildConfig, sessionID, graphID, execID string, initialState map[string]any) (GraphRuntime, <-chan GraphRuntimeEvent, error)
	BuildAndResume(ctx context.Context, cfg GraphBuildConfig, sessionID, graphID, execID, lineageID string, resumeValue map[string]any) (GraphRuntime, <-chan GraphRuntimeEvent, error)
	BuildRuntime(ctx context.Context, cfg GraphBuildConfig, sessionID, graphID, execID, lineageID string) (GraphRuntime, error)
}

type GraphVisualizer interface {
	Visualize(ctx context.Context, cfg GraphBuildConfig) (any, error)
}

type GraphValidator interface {
	Validate(ctx context.Context, cfg GraphBuildConfig) (*GraphValidationResult, error)
}

type GraphTemplateProvider interface {
	ListTemplates() any
	GetTemplate(templateID string) (any, bool)
	TemplateToDef(template any, name, description string) *GraphDefinition
}

type GraphNodeInfoProvider interface {
	AgentExists(ctx context.Context, agentID string) bool
	FindNodeDef(cfg GraphBuildConfig, taskMeta map[string]NodeTaskMeta, nodeID string) *NodeTaskMeta
}

type GraphBuilderFactory interface {
	GraphRunnerFactory
	GraphVisualizer
	GraphValidator
	GraphTemplateProvider
	GraphNodeInfoProvider
}
