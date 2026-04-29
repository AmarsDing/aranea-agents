package tooldef

import (
	"time"

	"arenea/backend/internal/capability/toolctx"
)

// Tool is Aranea's runtime-facing tool contract. Catalog rows describe how a
// tool is managed; this interface describes how an enabled tool is executed.
type Tool interface {
	Name() string
	DisplayName() string
	Description() string
	Category() string
	InputSchema() map[string]any
	OutputSchema() map[string]any
	Validate(map[string]any) error
	Execute(*toolctx.ToolContext, map[string]any) (map[string]any, error)
}

type StreamingTool interface {
	Tool
	Stream(*toolctx.ToolContext, map[string]any) (<-chan StreamChunk, error)
}

type ApprovableTool interface {
	Tool
	RequiresApproval(*toolctx.ToolContext, map[string]any) bool
}

type StreamChunk struct {
	Content map[string]any
	Err     error
	Done    bool
}

type Event struct {
	ID          string
	Phase       string
	Status      string
	ToolName    string
	ToolLabel   string
	Arguments   map[string]any
	Result      map[string]any
	Error       string
	OccurredAt  time.Time
	DurationMS  int
	MessageHint string
}

type EventSink func(Event) error
