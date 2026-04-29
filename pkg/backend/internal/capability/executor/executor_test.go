package executor

import (
	"testing"

	"arenea/backend/internal/capability/middleware"
	"arenea/backend/internal/capability/toolctx"
	"arenea/backend/internal/capability/tooldef"
)

type echoTool struct{}

func (echoTool) Name() string                  { return "echo" }
func (echoTool) DisplayName() string           { return "Echo" }
func (echoTool) Description() string           { return "Echo arguments" }
func (echoTool) Category() string              { return "test" }
func (echoTool) InputSchema() map[string]any   { return map[string]any{"type": "object"} }
func (echoTool) OutputSchema() map[string]any  { return map[string]any{"type": "object"} }
func (echoTool) Validate(map[string]any) error { return nil }
func (echoTool) Execute(_ *toolctx.ToolContext, params map[string]any) (map[string]any, error) {
	return params, nil
}

func TestExecutorEmitsBudgetEvents(t *testing.T) {
	events := 0
	exec := New(middleware.Budget(middleware.NewBudgetState(), 1, 1, func(_ tooldef.Event) error {
		events++
		return nil
	}))
	out, err := exec.Run(toolctx.New(nil), echoTool{}, map[string]any{"value": "ok"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out["value"] != "ok" {
		t.Fatalf("unexpected output: %#v", out)
	}
	if events != 2 {
		t.Fatalf("expected before/after events, got %d", events)
	}
}
