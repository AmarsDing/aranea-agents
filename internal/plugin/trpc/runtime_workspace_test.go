package plugintrpc

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

func TestRuntime_Apply_workspacePartitionsDoNotOverwrite(t *testing.T) {
	rt := NewRuntime(nil, loggateway.NewNoop())

	shared := biz.Plugin{
		Key:               "audit_log",
		Enabled:           true,
		Scope:             "global",
		WorkspaceID:       "",
		ConfigJSON:        `{}`,
		DefaultConfigJSON: `{}`,
	}
	pluginA := biz.Plugin{
		Key:               "skill_usage_tracker",
		Enabled:           true,
		Scope:             "global",
		WorkspaceID:       "ws-a",
		ConfigJSON:        `{}`,
		DefaultConfigJSON: `{}`,
	}
	pluginB := biz.Plugin{
		Key:               "sensitive_data_mask",
		Enabled:           true,
		Scope:             "global",
		WorkspaceID:       "ws-b",
		ConfigJSON:        `{}`,
		DefaultConfigJSON: `{}`,
	}

	ctxA := workspace.WithContext(context.Background(), "ws-a")
	rt.Apply(ctxA, []biz.Plugin{shared, pluginA})

	ctxB := workspace.WithContext(context.Background(), "ws-b")
	// Tenant B reload mirrors List(shared+own); must not wipe ws-a partition.
	rt.Apply(ctxB, []biz.Plugin{shared, pluginB})

	gotA := rt.PluginsForAgent("agent-1", "ws-a")
	if len(gotA) != 2 {
		t.Fatalf("ws-a plugins=%d want 2 (shared+own)", len(gotA))
	}
	gotB := rt.PluginsForAgent("agent-1", "ws-b")
	if len(gotB) != 2 {
		t.Fatalf("ws-b plugins=%d want 2 (shared+own)", len(gotB))
	}
	gotC := rt.PluginsForAgent("agent-1", "ws-c")
	if len(gotC) != 1 {
		t.Fatalf("ws-c should see only shared, got %d", len(gotC))
	}
}
